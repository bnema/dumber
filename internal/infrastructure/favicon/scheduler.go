package favicon

import (
	"context"
	"sync"

	"github.com/bnema/dumber/internal/domain/favicon"
)

type RefreshScheduler struct {
	mu      sync.Mutex
	running map[favicon.Key]struct{}
}

func NewRefreshScheduler() *RefreshScheduler {
	return &RefreshScheduler{running: make(map[favicon.Key]struct{})}
}

func (s *RefreshScheduler) Schedule(ctx context.Context, key favicon.Key, work func(context.Context)) bool {
	if s == nil || key == "" || work == nil {
		return false
	}
	s.mu.Lock()
	if _, ok := s.running[key]; ok {
		s.mu.Unlock()
		return false
	}
	s.running[key] = struct{}{}
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.running, key)
			s.mu.Unlock()
		}()
		work(ctx)
	}()
	return true
}
