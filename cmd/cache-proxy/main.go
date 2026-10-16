package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/atakanzen/cache-proxy/internal/cache"
	"github.com/atakanzen/cache-proxy/internal/cli"
	"github.com/atakanzen/cache-proxy/internal/proxy"
)

const (
	requestTimeout    = 30 * time.Second
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 15 * time.Second
	idleTimeout       = 60 * time.Second
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := cli.Parse(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		log.Fatalf("failed to parse arguments: %v", err)
	}

	store := cache.NewMemory()
	if cfg.ClearCache {
		store.Clear()
		log.Println("cache store cleared successfully")
		os.Exit(0)
	}

	addr := fmt.Sprintf(":%d", cfg.Port)

	originURL, err := url.Parse(cfg.Origin)
	if err != nil {
		log.Fatalf("failed to parse the origin URL: %v", err)
	}

	httpClient := &http.Client{
		Timeout: requestTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	proxyHandler := proxy.New(originURL, store, httpClient)

	server := &http.Server{
		Addr:              addr,
		Handler:           proxyHandler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	go func() {
		log.Println("cache-proxy server listening on:", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("failed to run cache-proxy server: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	server.Shutdown(shutdownCtx)
}
