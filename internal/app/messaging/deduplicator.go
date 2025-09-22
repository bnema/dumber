package messaging

import (
	"crypto/sha256"
	"fmt"
	"log"
	"sync"
	"time"
)

type RequestFingerprint struct {
	URL       string
	WebViewID string
	Timestamp int64
	Hash      string
}

type PaneRequestDeduplicator struct {
	mu              sync.RWMutex
	recentRequests  map[string]*RequestFingerprint
	requestIDs      map[string]bool
	debounceWindow  time.Duration
	cleanupInterval time.Duration
	lastCleanup     time.Time
}

func NewPaneRequestDeduplicator() *PaneRequestDeduplicator {
	return &PaneRequestDeduplicator{
		recentRequests:  make(map[string]*RequestFingerprint),
		requestIDs:      make(map[string]bool),
		debounceWindow:  200 * time.Millisecond,
		cleanupInterval: 5 * time.Second,
		lastCleanup:     time.Now(),
	}
}

func (d *PaneRequestDeduplicator) generateFingerprint(intent *WindowIntent, webViewID string) string {
	normalizedTime := intent.Timestamp / 100
	data := fmt.Sprintf("%s:%s:%d", intent.URL, webViewID, normalizedTime)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash[:8])
}

func (d *PaneRequestDeduplicator) IsDuplicate(intent *WindowIntent, webViewID string) (bool, string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if time.Since(d.lastCleanup) > d.cleanupInterval {
		d.cleanup()
	}

	if intent.RequestID != "" {
		if d.requestIDs[intent.RequestID] {
			return true, fmt.Sprintf("duplicate request ID: %s", intent.RequestID)
		}
	}

	fingerprint := d.generateFingerprint(intent, webViewID)

	if existing, exists := d.recentRequests[fingerprint]; exists {
		timeDiff := time.Duration(intent.Timestamp-existing.Timestamp) * time.Millisecond
		if timeDiff < d.debounceWindow {
			return true, fmt.Sprintf("duplicate content fingerprint: %s (within %v)", fingerprint, timeDiff)
		}
	}

	d.recentRequests[fingerprint] = &RequestFingerprint{
		URL:       intent.URL,
		WebViewID: webViewID,
		Timestamp: intent.Timestamp,
		Hash:      fingerprint,
	}

	if intent.RequestID != "" {
		d.requestIDs[intent.RequestID] = true
	}

	return false, ""
}

func (d *PaneRequestDeduplicator) cleanup() {
	now := time.Now().UnixMilli()
	cutoff := now - int64(d.debounceWindow*3/time.Millisecond)

	for key, req := range d.recentRequests {
		if now-req.Timestamp > cutoff {
			delete(d.recentRequests, key)
		}
	}

	if len(d.requestIDs) > 100 {
		d.requestIDs = make(map[string]bool)
	}

	d.lastCleanup = time.Now()
	log.Printf("[deduplicator] Cleanup completed: %d fingerprints, %d request IDs", len(d.recentRequests), len(d.requestIDs))
}

// ClearRequestID removes a specific request ID from the deduplication cache
// This allows the same RequestID to be used again (e.g., for OAuth verification -> real popup flow)
func (d *PaneRequestDeduplicator) ClearRequestID(requestID string) {
	if requestID == "" {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.requestIDs[requestID] {
		delete(d.requestIDs, requestID)
		log.Printf("[deduplicator] Cleared request ID: %s", requestID)
	}
}
