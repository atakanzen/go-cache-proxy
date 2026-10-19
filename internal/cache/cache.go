package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
)

type Entry struct {
	Body       []byte
	StatusCode int
	Headers    http.Header
}

type Cache interface {
	Get(ctx context.Context, key string) (Entry, bool)
	Set(ctx context.Context, key string, entry Entry) error
	Clear(ctx context.Context) error
}

func HashURLPath(urlPath string) string {
	sum := sha256.Sum256([]byte(urlPath))
	return hex.EncodeToString(sum[:])
}
