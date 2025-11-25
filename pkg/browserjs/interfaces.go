package browserjs

import (
	"io"
	"net/http"
	"time"
)

// HTTPDoer abstracts HTTP client for fetch/XHR operations.
// *http.Client implements this interface.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Clock abstracts time operations for testability.
type Clock interface {
	Now() time.Time
	AfterFunc(d time.Duration, f func()) Timer
	NewTicker(d time.Duration) Ticker
}

// Timer represents a one-shot timer that can be stopped.
type Timer interface {
	Stop() bool
}

// Ticker represents a repeating timer.
type Ticker interface {
	Stop()
	C() <-chan time.Time
}

// Logger receives log output from console.* methods.
type Logger interface {
	Log(level string, args ...any)
}

// RandomSource provides cryptographic randomness.
// crypto/rand.Reader implements this interface.
type RandomSource interface {
	io.Reader
}

// realClock implements Clock using the standard time package.
type realClock struct{}

func (c *realClock) Now() time.Time {
	return time.Now()
}

func (c *realClock) AfterFunc(d time.Duration, f func()) Timer {
	return &realTimer{timer: time.AfterFunc(d, f)}
}

func (c *realClock) NewTicker(d time.Duration) Ticker {
	return &realTicker{ticker: time.NewTicker(d)}
}

// realTimer wraps *time.Timer to implement Timer interface.
type realTimer struct {
	timer *time.Timer
}

func (t *realTimer) Stop() bool {
	return t.timer.Stop()
}

// realTicker wraps *time.Ticker to implement Ticker interface.
type realTicker struct {
	ticker *time.Ticker
}

func (t *realTicker) Stop() {
	t.ticker.Stop()
}

func (t *realTicker) C() <-chan time.Time {
	return t.ticker.C
}
