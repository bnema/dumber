package transcoder

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bnema/dumber/internal/application/port"
)

// Compile-time check: session implements port.TranscodeSession.
var _ port.TranscodeSession = (*session)(nil)

// session wraps an io.PipeReader fed by a pipeline goroutine.
type session struct {
	id        string
	pr        *io.PipeReader
	cancel    context.CancelFunc
	createdAt time.Time
	lastRead  atomic.Int64 // unix timestamp of last Read() call
}

// Read delegates to the underlying pipe reader and records the timestamp.
func (s *session) Read(buf []byte) (int, error) {
	n, err := s.pr.Read(buf)
	if n > 0 {
		s.lastRead.Store(time.Now().Unix())
	}
	return n, err
}

// Close cancels the pipeline context and closes the pipe reader.
func (s *session) Close() error {
	s.cancel()
	return s.pr.Close()
}

// ContentType returns the MIME type of the transcoded output.
func (*session) ContentType() string {
	return "video/webm"
}

// errPoolAtCapacity is returned when the session pool has reached its
// maximum number of concurrent sessions.
var errPoolAtCapacity = errors.New("transcoder: session pool at capacity")

// sessionPool tracks active transcode sessions with a concurrency limit.
type sessionPool struct {
	mu       sync.RWMutex
	sessions map[string]*session
	maxConc  int
}

// newSessionPool creates a pool with the given concurrency limit.
func newSessionPool(maxConcurrent int) *sessionPool {
	return &sessionPool{
		sessions: make(map[string]*session),
		maxConc:  maxConcurrent,
	}
}

// add registers a session in the pool. Returns errPoolAtCapacity if the
// pool is already at its maximum concurrency.
func (p *sessionPool) add(s *session) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.sessions) >= p.maxConc {
		return errPoolAtCapacity
	}
	p.sessions[s.id] = s
	return nil
}

// remove unregisters a session from the pool.
func (p *sessionPool) remove(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.sessions, id)
}

// count returns the number of active sessions.
func (p *sessionPool) count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.sessions)
}

// closeAll cancels and closes every active session. It collects sessions
// under the lock, clears the map, then releases the lock before calling
// Close on each session to avoid holding the lock during potentially
// blocking I/O.
func (p *sessionPool) closeAll() {
	p.mu.Lock()
	snapshot := make([]*session, 0, len(p.sessions))
	for _, s := range p.sessions {
		snapshot = append(snapshot, s)
	}
	// Clear the map while holding the lock.
	for id := range p.sessions {
		delete(p.sessions, id)
	}
	p.mu.Unlock()

	// Close sessions outside the lock.
	for _, s := range snapshot {
		_ = s.Close()
	}
}
