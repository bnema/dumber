package url

import (
	"strings"
	"time"
)

// RelayReuseWindow is how long a pane opened for an externally relayed URL
// remains eligible to be reused when the same URL is relayed again.
const RelayReuseWindow = 2 * time.Minute

// RelayURLKey returns the canonical key identifying a relayed URL for reuse
// decisions.
func RelayURLKey(raw string) string {
	return Normalize(strings.TrimSpace(raw))
}

// ShouldReuseRelayPane reports whether a pane that was opened for recordedURL
// at openedAt should be reused for requestedURL at now, instead of creating a
// new pane. It is true only when both URLs are non-empty and identical after
// normalization and the pane is no older than window. A pane recorded in the
// future (clock anomaly) or with a non-positive window is never reused.
func ShouldReuseRelayPane(recordedURL string, openedAt time.Time, requestedURL string, now time.Time, window time.Duration) bool {
	if window <= 0 {
		return false
	}
	requested := RelayURLKey(requestedURL)
	if requested == "" || requested != RelayURLKey(recordedURL) {
		return false
	}
	age := now.Sub(openedAt)
	return age >= 0 && age <= window
}
