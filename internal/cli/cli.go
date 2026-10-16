package cli

import (
	"flag"
	"fmt"

	"github.com/atakanzen/cache-proxy/internal/config"
)

const (
	defaultPort = 3000
)

func Parse(args []string) (config.Config, error) {
	fs := flag.NewFlagSet("cache-proxy", flag.ContinueOnError)

	var cfg config.Config
	fs.UintVar(&cfg.Port, "port", defaultPort, "The port for the cache proxy to run")
	fs.StringVar(&cfg.Origin, "origin", "", "The origin url to put the cache in front of (required)")
	fs.BoolVar(&cfg.ClearCache, "clear", false, "If passed, this flag will clear the cache")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "cache-proxy, powered by go. Available flags:\n\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return config.Config{}, err
	}

	if cfg.Origin == "" {
		return config.Config{}, fmt.Errorf("-origin is required")
	}

	return cfg, nil
}
