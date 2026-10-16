package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/atakanzen/cache-proxy/internal/cache"
	"github.com/atakanzen/cache-proxy/internal/cli"
	"github.com/atakanzen/cache-proxy/internal/middleware"
	"github.com/atakanzen/cache-proxy/internal/proxy"
	"github.com/atakanzen/cache-proxy/internal/server"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := cli.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}

	store := cache.NewMemory()
	if cfg.ClearCache {
		store.Clear()
		log.Println("cache store cleared successfully")
		return nil
	}

	originURL, err := url.Parse(cfg.Origin)
	if err != nil {
		return err
	}

	client := server.NewOriginClient()
	proxyHandler := proxy.New(originURL, store, client)
	addr := fmt.Sprintf(":%d", cfg.Port)

	chain := middleware.Chain{
		middleware.LogRequest,
		middleware.AddProxyHeaders,
	}
	handler := chain.Then(proxyHandler)

	srv := server.NewHTTPServer(addr, handler)

	log.Printf("cache-proxy server started listening on %s\n", addr)
	go srv.ListenAndServe()
	<-ctx.Done()
	shutDownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutDownCtx)
}
