package middleware

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
)

type Chain []func(http.Handler) http.Handler

func (c Chain) Then(h http.Handler) http.Handler {
	for _, mw := range slices.Backward(c) {
		h = mw(h)
	}
	return h
}

func LogRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var (
			ip     = r.RemoteAddr
			method = r.Method
			url    = r.URL.String()
			proto  = r.Proto
		)

		userAttrs := slog.Group("user", "ip", ip)
		requestAttrs := slog.Group("request", "method", method, "url", url, "proto", proto)

		slog.Info("request received", userAttrs, requestAttrs)
		next.ServeHTTP(w, r)
	})
}

func AddProxyHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			clientIP = r.RemoteAddr // no port - use as-is
		}

		proto := "http"
		if r.TLS != nil {
			proto = "https"
		}

		// RFC 7239 Forwarded (the new standard)
		forwarded := fmt.Sprintf("for=%s;host=%s;proto=%s", potentiallyQuotedIP(clientIP), r.Host, proto)
		if prior := r.Header.Get("Forwarded"); prior != "" {
			r.Header.Set("Forwarded", prior+", "+forwarded)
		} else {
			r.Header.Set("Forwarded", forwarded)
		}

		version := fmt.Sprintf("%d.%d", r.ProtoMajor, r.ProtoMinor)
		if prior := r.Header.Get("Via"); prior != "" {
			r.Header.Set("Via", prior+", "+version+" cache-proxy")
		} else {
			r.Header.Set("Via", version+" cache-proxy")
		}

		next.ServeHTTP(w, r)
	})
}

// IPV6 values must be quoted
func potentiallyQuotedIP(ip string) string {
	if strings.Contains(ip, ":") {
		return fmt.Sprintf("%q", "["+ip+"]")
	}
	return ip
}
