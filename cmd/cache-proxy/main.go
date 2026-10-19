package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
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
	"github.com/redis/go-redis/v9"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
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

	originURL, err := url.Parse(cfg.Origin)
	if err != nil {
		return err
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
		Protocol: 2,
	})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis unreachable: %w", err)
	}

	store := cache.NewRedis(rdb, originURL, cfg.TTL)
	if cfg.ClearCache {
		return store.Clear(ctx)
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

	slog.Info("cache-proxy server started listening on", "addr", addr)
	go srv.ListenAndServe()
	<-ctx.Done()
	shutDownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	return srv.Shutdown(shutDownCtx)
}
