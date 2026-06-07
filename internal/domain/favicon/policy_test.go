package favicon

import (
	"testing"
	"time"
)

func TestShouldRefresh(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	ttl := 30 * 24 * time.Hour

	if !ShouldRefresh(nil, now, ttl) {
		t.Fatal("missing metadata should refresh")
	}

	fresh := &Metadata{LastCheckedAt: now.Add(-ttl + time.Minute)}
	if ShouldRefresh(fresh, now, ttl) {
		t.Fatal("fresh metadata should not refresh")
	}

	stale := &Metadata{LastCheckedAt: now.Add(-ttl - time.Minute)}
	if !ShouldRefresh(stale, now, ttl) {
		t.Fatal("stale metadata should refresh")
	}
}

func TestContentHashChanged(t *testing.T) {
	data := []byte("favicon bytes")
	meta := &Metadata{ContentHash: Hash(data)}

	if HasContentChanged(meta, data) {
		t.Fatal("same bytes should not be changed")
	}

	if !HasContentChanged(meta, []byte("different bytes")) {
		t.Fatal("different bytes should be changed")
	}

	if !HasContentChanged(nil, data) {
		t.Fatal("missing metadata should be treated as changed")
	}
}
