package favicon

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

const DefaultTTL = 30 * 24 * time.Hour

func Hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func ShouldRefresh(meta *Metadata, now time.Time, ttl time.Duration) bool {
	if meta == nil {
		return true
	}
	if ttl <= 0 {
		return true
	}
	return !meta.LastCheckedAt.Add(ttl).After(now)
}

func HasContentChanged(meta *Metadata, data []byte) bool {
	if meta == nil {
		return true
	}
	return meta.ContentHash != Hash(data)
}
