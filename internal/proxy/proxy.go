package proxy

import (
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/atakanzen/cache-proxy/internal/cache"
)

const (
	// ----- CACHE CONTROL -----
	cacheHeaderKey        = "X-Cache"
	hit            string = "HIT"
	miss           string = "MISS"

	// ----- RESPONSE SIZE -----
	maxBodySize = 10 * 1024 * 1024 // 10 MB

)

type ProxyHandler struct {
	origin     *url.URL
	cache      cache.Cache
	httpClient *http.Client
}

func New(origin *url.URL, c cache.Cache, httpClient *http.Client) *ProxyHandler {
	return &ProxyHandler{
		origin:     origin,
		cache:      c,
		httpClient: httpClient,
	}
}

func (p *ProxyHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	target := p.createTargetURL(*req.URL)

	// We pass through any request that's not GET
	if req.Method != http.MethodGet {
		nonProxiedReq, err := http.NewRequest(req.Method, target.String(), req.Body)
		if err != nil {
			log.Printf("failed to create a non-proxied request: %s", err.Error())
			http.Error(w, "something went wrong", http.StatusInternalServerError)
			return
		}
		p.copyHeaders(nonProxiedReq.Header, req.Header)

		resp, err := p.httpClient.Do(nonProxiedReq)
		if err != nil {
			log.Printf("failed to reach origin server %s: %v", target.String(), err)
			http.Error(w, "failed to reach origin server", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		resp.Body = http.MaxBytesReader(w, resp.Body, maxBodySize)

		p.copyHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)

		if _, err := io.Copy(w, resp.Body); err != nil {
			log.Printf("failed to copy response body: %v", err)
			return
		}

		return
	}

	hashedRequestURI := cache.HashURLPath(req.URL.RequestURI())

	// Cached, return the entry and add the X-Cache: HIT
	if entry, ok := p.cache.Get(hashedRequestURI); ok {
		p.copyHeaders(w.Header(), entry.Headers)
		w.Header().Set("Content-Length", strconv.Itoa(len(entry.Body)))
		w.Header().Add(cacheHeaderKey, hit)
		w.WriteHeader(entry.StatusCode)
		w.Write(entry.Body)
		return
	}

	// Not cached, make the original request
	originRequest, err := http.NewRequest(http.MethodGet, target.String(), req.Body)
	if err != nil {
		log.Printf("failed to create a new origin request: %v", err)
		http.Error(w, "failed to create a new origin request", http.StatusInternalServerError)
		return
	}
	p.copyHeaders(originRequest.Header, req.Header)

	res, err := p.httpClient.Do(originRequest)
	if err != nil {
		log.Printf("failed to GET origin request: %v", err)
		// This err happens only for the HTTP protocol level, non 2XX responses do not return an error from upstream
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer res.Body.Close()

	res.Body = http.MaxBytesReader(w, res.Body, maxBodySize)

	body, err := io.ReadAll(res.Body)
	if err != nil {
		log.Printf("failed to read origin response body: %v", err)
		http.Error(w, "failed to read origin response body", http.StatusInternalServerError)
		return
	}

	// Set the response to cache if status OK
	if res.StatusCode >= 200 && res.StatusCode <= 299 {
		entry := cache.Entry{
			Body:       body,
			StatusCode: res.StatusCode,
			Headers:    res.Header.Clone(),
		}

		p.cache.Set(hashedRequestURI, entry)
	}

	p.copyHeaders(w.Header(), res.Header)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Add(cacheHeaderKey, miss)
	w.WriteHeader(res.StatusCode)

	if _, err := w.Write(body); err != nil {
		log.Printf("failed to write response body: %v", err)
		http.Error(w, "failed to write response body", http.StatusInternalServerError)
	}
}

func (p *ProxyHandler) createTargetURL(requestURL url.URL) *url.URL {
	target := *p.origin
	target.Path = requestURL.Path
	target.RawQuery = requestURL.RawQuery

	return &target
}

func (p *ProxyHandler) copyHeaders(dest, src http.Header) {
	hopHeaders := map[string]struct{}{
		"Connection":          {},
		"Keep-Alive":          {},
		"Proxy-Connection":    {},
		"Proxy-Authenticate":  {},
		"Proxy-Authorization": {},
		"Te":                  {},
		"Trailer":             {},
		"Transfer-Encoding":   {},
		"Upgrade":             {},
		// Also Content-Length so we will calculate this instead.
		"Content-Length": {},
	}

	// Additional to the static list, also respect Connection header values deletion
	for _, tok := range src["Connection"] {
		for _, name := range strings.Split(tok, ",") {
			if h := strings.TrimSpace(name); h != "" {
				src.Del(h)
			}
		}
	}

	for name, values := range src {
		if _, ok := hopHeaders[name]; ok {
			continue
		}

		for _, v := range values {
			dest.Add(name, v)
		}
	}
}
