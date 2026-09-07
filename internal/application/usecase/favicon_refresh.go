package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
	"sync"
	"time"

	appport "github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/favicon"
)

const (
	// maxRefreshCandidates bounds how many icon URLs one refresh operation
	// accepts; extras are skipped, never queued.
	maxRefreshCandidates = 16
	// maxRefreshCandidateLen bounds a single candidate URL length; longer
	// inputs are skipped as invalid rather than fetched.
	maxRefreshCandidateLen = 8 * 1024
	// maxActiveRefreshes bounds distinct concurrent refresh operations
	// across direct and scheduler-backed paths. Overload skips optional
	// background work or reports ErrFaviconBusy to direct callers.
	maxActiveRefreshes = 8
	// refreshOpTimeout bounds a whole refresh operation, including
	// context-aware DNS/validation, not per candidate.
	refreshOpTimeout = 5 * time.Second
	// faviconMissTTL bounds negative-cache entries for genuine absence.
	faviconMissTTL = 30 * time.Second
	// maxMissEntries bounds the negative cache; oldest entries are evicted.
	maxMissEntries = 256
)

// refreshRequestKey builds a fixed-size digest identifying request-equivalent
// refresh work: the validated page origin plus the ordered, normalized
// candidate list. Scheme, host, port, path, and query are significant;
// fragments are dropped and hosts lowercased. Relative candidates resolve
// against the page URL. At most maxRefreshCandidates URLs of at most
// maxRefreshCandidateLen bytes are accepted; the rest are skipped. It
// returns false when the page URL has no valid origin hierarchy.
func refreshRequestKey(pageURL string, iconURLs []string) (string, []string, bool) {
	pageKey, ok := favicon.CanonicalKey(pageURL)
	if !ok {
		return "", nil, false
	}
	page, err := url.Parse(strings.TrimSpace(pageURL))
	if err != nil || page.Hostname() == "" {
		return "", nil, false
	}
	accepted := make([]string, 0, len(iconURLs))
	for _, raw := range iconURLs {
		raw = strings.TrimSpace(raw)
		if raw == "" || len(raw) > maxRefreshCandidateLen {
			continue
		}
		candidate, err := page.Parse(raw)
		if err != nil {
			continue
		}
		if candidate.Scheme != "http" && candidate.Scheme != "https" {
			continue
		}
		if candidate.Hostname() == "" {
			continue
		}
		candidate.Fragment = ""
		candidate.Host = strings.ToLower(candidate.Host)
		if host, port, ok := splitHostPort(candidate.Host); ok {
			if isDefaultPort(candidate.Scheme, port) {
				candidate.Host = host
			}
		}
		accepted = append(accepted, candidate.String())
		if len(accepted) >= maxRefreshCandidates {
			break
		}
	}
	sum := sha256.Sum256([]byte(string(pageKey) + "\x00" + strings.Join(accepted, "\x00")))
	return hex.EncodeToString(sum[:]), accepted, true
}

func splitHostPort(hostport string) (string, string, bool) {
	host, port, found := strings.Cut(hostport, ":")
	if !found {
		return "", "", false
	}
	return host, port, true
}

func isDefaultPort(scheme, port string) bool {
	return (scheme == "http" && port == "80") || (scheme == "https" && port == "443")
}

// refreshCall is one shared in-flight refresh operation. Waiters join by
// request key; the first waiter executes while the rest observe. One
// canceled waiter never cancels the others; the operation is canceled only
// when the last waiter leaves.
type refreshCall struct {
	done    chan struct{}
	err     error
	waiters int
	cancel  context.CancelFunc
}

// refreshMiss records a negatively cached genuine absence.
type refreshMiss struct {
	expiresAt time.Time
}

// refreshCoordinator state lives on FaviconUseCase; see favicon.go.
type refreshCoordinator struct {
	mu     sync.Mutex
	active map[string]*refreshCall
	slots  chan struct{}
	epoch  uint64
	misses map[string]refreshMiss
	order  []string
}

func newRefreshCoordinator() *refreshCoordinator {
	return &refreshCoordinator{
		active: make(map[string]*refreshCall),
		slots:  make(chan struct{}, maxActiveRefreshes),
		misses: make(map[string]refreshMiss),
	}
}

// epochValue returns the current invalidation epoch.
func (uc *FaviconUseCase) epochValue() uint64 {
	uc.shared.mu.Lock()
	defer uc.shared.mu.Unlock()
	return uc.shared.epoch
}

// bumpEpoch advances the invalidation epoch so older in-flight completions
// cannot repopulate invalidated entries.
func (uc *FaviconUseCase) bumpEpoch() {
	uc.shared.mu.Lock()
	defer uc.shared.mu.Unlock()
	uc.shared.epoch++
}

// cachedMiss reports whether key holds an unexpired genuine-absence entry.
func (uc *FaviconUseCase) cachedMiss(key string) bool {
	now := uc.now()
	uc.shared.mu.Lock()
	defer uc.shared.mu.Unlock()
	miss, ok := uc.shared.misses[key]
	if !ok {
		return false
	}
	if !now.Before(miss.expiresAt) {
		delete(uc.shared.misses, key)
		return false
	}
	return true
}

// noteMiss records a genuine absence (verified 404/410) for request key.
// Only 404/410 are cacheable: 401/403, 5xx, cancellation, DNS/transport,
// and validation failures must never suppress a later retry.
func (uc *FaviconUseCase) noteMiss(key string, fetchErr error) {
	status, ok := appport.FetchStatusCode(fetchErr)
	if !ok || (status != 404 && status != 410) {
		return
	}
	uc.shared.mu.Lock()
	defer uc.shared.mu.Unlock()
	if _, exists := uc.shared.misses[key]; !exists {
		for len(uc.shared.misses) >= maxMissEntries {
			uc.evictMissLocked()
		}
		uc.shared.order = append(uc.shared.order, key)
	}
	uc.shared.misses[key] = refreshMiss{expiresAt: uc.now().Add(faviconMissTTL)}
}

// evictMissLocked removes one entry: expired first, oldest otherwise.
// Caller must hold refresh.mu.
func (uc *FaviconUseCase) evictMissLocked() {
	now := uc.now()
	for key, miss := range uc.shared.misses {
		if !now.Before(miss.expiresAt) {
			delete(uc.shared.misses, key)
			return
		}
	}
	for len(uc.shared.order) > 0 {
		oldest := uc.shared.order[0]
		uc.shared.order = uc.shared.order[1:]
		if _, ok := uc.shared.misses[oldest]; ok {
			delete(uc.shared.misses, oldest)
			return
		}
	}
	for key := range uc.shared.misses {
		delete(uc.shared.misses, key)
		return
	}
}

// joinRefreshOp shares equivalent in-flight work identified by key. The
// first waiter executes exec under a whole-operation deadline derived from
// the owned background context; joiners observe the result. A canceled
// waiter leaves without affecting others; the operation is canceled when
// the last waiter is gone. When the admission budget is exhausted, direct
// callers receive ErrFaviconBusy (never false success).
func (uc *FaviconUseCase) joinRefreshOp(ctx context.Context, key string, exec func(opCtx context.Context) error) error {
	uc.shared.mu.Lock()
	if call, ok := uc.shared.active[key]; ok {
		call.waiters++
		uc.shared.mu.Unlock()
		return uc.awaitRefreshCall(ctx, call)
	}
	select {
	case uc.shared.slots <- struct{}{}:
	default:
		uc.shared.mu.Unlock()
		return appport.ErrFaviconBusy
	}
	opCtx, cancel := context.WithTimeout(uc.background, refreshOpTimeout)
	call := &refreshCall{done: make(chan struct{}), waiters: 1, cancel: cancel}
	uc.shared.active[key] = call
	uc.shared.mu.Unlock()

	go func() {
		err := exec(opCtx)
		<-uc.shared.slots
		cancel()
		uc.shared.mu.Lock()
		call.err = err
		delete(uc.shared.active, key)
		uc.shared.mu.Unlock()
		close(call.done)
	}()

	return uc.awaitRefreshCall(ctx, call)
}

// awaitRefreshCall waits for a shared operation or the waiter's own
// cancellation, releasing the waiter slot exactly once.
func (uc *FaviconUseCase) awaitRefreshCall(ctx context.Context, call *refreshCall) error {
	select {
	case <-call.done:
		uc.shared.mu.Lock()
		err := call.err
		uc.shared.mu.Unlock()
		return err
	case <-ctx.Done():
		uc.shared.mu.Lock()
		call.waiters--
		last := call.waiters <= 0
		uc.shared.mu.Unlock()
		if last {
			call.cancel()
		}
		return ctx.Err()
	}
}

// drainRefreshOps waits until no shared refresh operation remains active,
// up to a bounded multiple of the operation deadline. Callers cancel first
// (see Close); cooperative operations observe the background cancellation
// and finish promptly, so hitting the bound means stuck work that shutdown
// must not wait out.
func (uc *FaviconUseCase) drainRefreshOps() {
	deadline := time.Now().Add(3 * refreshOpTimeout)
	for {
		uc.shared.mu.Lock()
		remaining := len(uc.shared.active)
		uc.shared.mu.Unlock()
		if remaining == 0 {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}
