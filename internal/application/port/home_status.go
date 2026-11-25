package port

import (
	"context"
	"time"
)

// HomeProbeResult is the outcome of a reachability probe that got an answer.
type HomeProbeResult struct {
	StatusCode int
	Latency    time.Duration
	// TLSUnverified is true when the server answered the TLS handshake but
	// its certificate could not be verified. StatusCode is then 0.
	TLSUnverified bool
}

// HomeStatusProber checks whether a home service answers. A non-2xx status
// is not an error; errors mean the server could not be reached. Errors name
// the host only.
type HomeStatusProber interface {
	Probe(ctx context.Context, url string, timeout time.Duration) (HomeProbeResult, error)
}
