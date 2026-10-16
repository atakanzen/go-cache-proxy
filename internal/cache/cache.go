package cache

import (
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
	Get(key string) (Entry, bool)
	Set(key string, entry Entry)
	Clear()
}

func HashURLPath(urlPath string) string {
	sum := sha256.Sum256([]byte(urlPath))
	return hex.EncodeToString(sum[:])
}
