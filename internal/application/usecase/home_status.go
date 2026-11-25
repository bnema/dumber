package usecase

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/logging"
)

const (
	// homeStatusTimeout bounds one service probe.
	homeStatusTimeout = 5 * time.Second
	// homeStatusTTL is how long a probe result is reused.
	homeStatusTTL = 30 * time.Second
)

// homeStatusState caches probe results and shares in-flight probes per
// service. The zero value is ready to use.
type homeStatusState struct {
	mu       sync.Mutex
	cache    map[string]homeStatusCacheEntry
	inflight map[string]*homeStatusFlight
	lastHash [sha256.Size]byte
	seen     bool
}

type homeStatusCacheEntry struct {
	url  string
	at   time.Time
	resp dto.HomeStatusResponse
}

type homeStatusFlight struct {
	url  string
	done chan struct{}
	resp dto.HomeStatusResponse
}

// CheckHomeStatus probes the url of serviceID and reports whether it is up.
// Probe failures are reported as state down; the error return is for
// unknown or disabled services and for the caller's own cancellation.
func (uc *HomeUseCase) CheckHomeStatus(ctx context.Context, serviceID string) (*dto.HomeStatusResponse, error) {
	if uc.source == nil || uc.status == nil {
		return nil, errors.New("home status not configured")
	}
	snap, err := uc.source.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("load home config: %w", err)
	}
	svc, ok := snap.Layout.FindService(serviceID)
	if !ok {
		return nil, fmt.Errorf("unknown home service %q", serviceID)
	}
	if !svc.StatusEnabled(snap.Layout.Settings) {
		return nil, fmt.Errorf("home service %q has no status check", serviceID)
	}

	st := &uc.statusState
	st.mu.Lock()
	st.pruneLocked(snap)
	if entry, ok := st.cache[serviceID]; ok && entry.url == svc.URL && uc.now().Sub(entry.at) < homeStatusTTL {
		st.mu.Unlock()
		resp := entry.resp
		return &resp, nil
	}
	f, running := st.inflight[serviceID]
	if !running || f.url != svc.URL {
		// A flight for an older url keeps running for its own waiters.
		f = &homeStatusFlight{url: svc.URL, done: make(chan struct{})}
		st.inflight[serviceID] = f
		// The shared probe must not die with the first caller's request.
		workCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), homeStatusTimeout)
		go uc.runStatusFlight(workCtx, cancel, f, serviceID, svc.URL)
	}
	st.mu.Unlock()

	select {
	case <-f.done:
		resp := f.resp
		return &resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// runStatusFlight performs one shared probe and publishes its result.
// Cleanup is deferred so a panic still releases the waiters.
func (uc *HomeUseCase) runStatusFlight(
	ctx context.Context, cancel context.CancelFunc, f *homeStatusFlight, serviceID, url string,
) {
	defer cancel()
	resp := dto.HomeStatusResponse{ServiceID: serviceID, State: dto.HomeServiceStateDown}
	ok := false
	defer func() {
		if r := recover(); r != nil {
			logging.FromContext(ctx).Error().Interface("panic", r).Str("home_service", serviceID).
				Msg("home status probe panicked")
		}
		st := &uc.statusState
		st.mu.Lock()
		defer st.mu.Unlock()
		if ok {
			st.cache[serviceID] = homeStatusCacheEntry{url: url, at: uc.now(), resp: resp}
		}
		if st.inflight[serviceID] == f {
			delete(st.inflight, serviceID)
		}
		f.resp = resp
		close(f.done)
	}()
	res, err := uc.status.Probe(ctx, url, homeStatusTimeout)
	if err != nil {
		logging.FromContext(ctx).Debug().Str("home_service", serviceID).
			Str("reason", safeHomeHTTPError(ctx, err)).Msg("home status probe failed")
	}
	resp = homeStatusResponse(serviceID, res, err)
	ok = true
}

// homeStatusResponse maps a probe result to the page state.
func homeStatusResponse(serviceID string, res port.HomeProbeResult, err error) dto.HomeStatusResponse {
	resp := dto.HomeStatusResponse{ServiceID: serviceID, State: dto.HomeServiceStateDown}
	if err != nil {
		return resp
	}
	resp.LatencyMS = int(res.Latency.Milliseconds())
	resp.Code = res.StatusCode
	resp.TLSUnverified = res.TLSUnverified
	switch {
	case res.TLSUnverified || (res.StatusCode > 0 && res.StatusCode < http.StatusInternalServerError):
		resp.State = dto.HomeServiceStateUp
	case res.StatusCode >= http.StatusInternalServerError:
		resp.State = dto.HomeServiceStateError
	}
	return resp
}

// pruneLocked drops cached results of services that left the layout. It only
// scans when the home.toml content hash changed. Caller holds s.mu.
func (s *homeStatusState) pruneLocked(snap port.HomeConfigSnapshot) {
	if s.cache == nil {
		s.cache = map[string]homeStatusCacheEntry{}
		s.inflight = map[string]*homeStatusFlight{}
	}
	if s.seen && snap.ContentHash == s.lastHash {
		return
	}
	s.seen = true
	s.lastHash = snap.ContentHash
	for id := range s.cache {
		if _, ok := snap.Layout.FindService(id); !ok {
			delete(s.cache, id)
		}
	}
}
