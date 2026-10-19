package proxy

import (
	"context"
	"io"
	"log"
	"log/slog"
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
	ctx := req.Context()

	if req.Method != http.MethodGet {
		resp, err := p.doRequest(ctx, req)
		if err != nil {
			slog.Error("failed request to origin server", "err", err)
			http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
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
	if entry, ok := p.cache.Get(ctx, hashedRequestURI); ok {
		p.copyHeaders(w.Header(), entry.Headers)
		w.Header().Set("Content-Length", strconv.Itoa(len(entry.Body)))
		w.Header().Add(cacheHeaderKey, hit)
		w.WriteHeader(entry.StatusCode)
		w.Write(entry.Body)
		return
	}

	// Not cached, make the original request
	res, err := p.doRequest(ctx, req)
	if err != nil {
		slog.Error("failed request to origin server", "err", err)
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	defer res.Body.Close()

	res.Body = http.MaxBytesReader(w, res.Body, maxBodySize)
	body, err := io.ReadAll(res.Body)
	if err != nil {
		slog.Error("failed to read origin response body", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	// Set the response to cache if status OK
	if res.StatusCode >= 200 && res.StatusCode <= 299 {
		entry := cache.Entry{
			Body:       body,
			StatusCode: res.StatusCode,
			Headers:    res.Header.Clone(),
		}

		p.cache.Set(ctx, hashedRequestURI, entry)
	}

	p.copyHeaders(w.Header(), res.Header)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Add(cacheHeaderKey, miss)
	w.WriteHeader(res.StatusCode)

	if _, err := w.Write(body); err != nil {
		slog.Error("failed to write response body", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (p *ProxyHandler) doRequest(ctx context.Context, incomingReq *http.Request) (*http.Response, error) {
	target := p.createTargetURL(*incomingReq.URL)
	request, err := http.NewRequestWithContext(ctx, incomingReq.Method, target.String(), incomingReq.Body)
	if err != nil {
		slog.Error("failed to create a request", "err", err)
		return nil, err
	}
	p.copyHeaders(request.Header, incomingReq.Header)

	return p.httpClient.Do(request)
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
