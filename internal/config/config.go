package config

import "time"

type Config struct {
	Port       uint
	Origin     string
	ClearCache bool
	TTL        time.Duration
}
