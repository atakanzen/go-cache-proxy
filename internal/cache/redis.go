package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	cacheKey = "cache"

	fieldBody    = "body"
	fieldStatus  = "status"
	fieldHeaders = "headers"
)

/*
	REDIS - HASH BASED CACHE

	- The keys are formed as cache:hostname:hashed-request-uri
	- Fields are following the Entry struct
		- Body []byte: Response body in byte array
		- StatusCode int: Response tatus code
		- Headers map[string][]string: Response headers
*/

type Redis struct {
	client     *redis.Client
	origin     *url.URL
	defaultTTL time.Duration
}

func NewRedis(client *redis.Client, origin *url.URL, defaultTTL time.Duration) *Redis {
	return &Redis{
		client:     client,
		origin:     origin,
		defaultTTL: defaultTTL,
	}
}

func (r *Redis) Get(ctx context.Context, key string) (Entry, bool) {

	fields, err := r.client.HGetAll(ctx, r.key(key)).Result()
	if err != nil {
		slog.Error("redis hgetall failed", "err", err)
		return Entry{}, false
	}

	if len(fields) == 0 {
		return Entry{}, false
	}

	status, err := strconv.Atoi(fields[fieldStatus])
	if err != nil {
		slog.Error("failed to convert string status to integer", "err", err)
		return Entry{}, false
	}

	var headers http.Header
	if err := json.Unmarshal([]byte(fields[fieldHeaders]), &headers); err != nil {
		slog.Error("failed to unmarshal http-headers from redis", "err", err)
		return Entry{}, false
	}

	return Entry{
		Body:       []byte(fields[fieldBody]),
		StatusCode: status,
		Headers:    headers,
	}, true
}

func (r *Redis) Set(ctx context.Context, key string, entry Entry) error {
	// Set hash url:hash / entry-fields
	headers, err := json.Marshal(entry.Headers)
	if err != nil {
		slog.Error("failed to encode headers", "err", err)
		return err
	}

	k := r.key(key)
	_, err = r.client.TxPipelined(ctx, func(p redis.Pipeliner) error {
		// This is just queueing the commands, so we don't need to check for .Result()
		p.HSet(ctx, k, fieldBody, entry.Body, fieldStatus, entry.StatusCode, fieldHeaders, headers)
		p.Expire(ctx, k, r.defaultTTL)
		return nil
	})
	if err != nil {
		slog.Error("failed to write cache entry to redis", "err", err)
		return err
	}

	return nil
}

func (r *Redis) Clear(ctx context.Context) error {
	// Delete
	pattern := r.keyPattern()
	iter := r.client.Scan(ctx, 0, pattern, 500).Iterator()

	batch := make([]string, 500)
	for iter.Next(ctx) {
		batch = append(batch, iter.Val())
		if len(batch) == cap(batch) {
			if err := r.client.Unlink(ctx, batch...).Err(); err != nil {
				return err
			}
			batch = batch[:0]
		}
	}
	if err := iter.Err(); err != nil {
		return err
	}

	if len(batch) > 0 {
		r.client.Unlink(ctx, batch...).Err()
	}

	return nil
}

func (r *Redis) key(k string) string {
	// cache:hostname:hashed-request-uri
	return fmt.Sprintf("%s:%s:%s", cacheKey, r.origin.Hostname(), k)
}

func (r *Redis) keyPattern() string {
	return fmt.Sprintf("%s:*", cacheKey)
}
