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
func (s *session) ContentType() string {
	return "video/webm"
}

// errPoolAtCapacity is returned when the session pool has reached its
// maximum number of concurrent sessions.
var errPoolAtCapacity = errors.New("transcoder: session pool at capacity")

// sessionPool tracks active transcode sessions with a concurrency limit.
type sessionPool struct {
	mu       sync.Mutex
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
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.sessions)
}

// closeAll cancels and closes every active session.
func (p *sessionPool) closeAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, s := range p.sessions {
		s.cancel()
		s.pr.Close()
		delete(p.sessions, id)
	}
}
