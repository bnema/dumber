package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/url"
	"slices"
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
	accepted := make([]string, 0, min(len(iconURLs), maxRefreshCandidates))
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
				// Re-bracket IPv6 literals: SplitHostPort strips
				// brackets, and url.URL requires them in Host.
				if strings.Contains(host, ":") {
					candidate.Host = "[" + host + "]"
				} else {
					candidate.Host = host
				}
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

// splitHostPort splits an explicit host:port pair. Bracketed IPv6 literals
// (as produced by url.URL.Host) require net.SplitHostPort; a bare
// strings.Cut on ":" misparses them. Hosts without a port report false.
func splitHostPort(hostport string) (string, string, bool) {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
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
// when the last waiter leaves, decided atomically under the coordinator
// lock so no joiner can observe a half-canceled call.
type refreshCall struct {
	done    chan struct{}
	err     error
	waiters int
	cancel  context.CancelFunc
	// closing marks a call whose last waiter left: it still occupies its
	// slot and map entry until the executor finishes, but no new waiter
	// may join it; joiners create a fresh operation instead.
	closing bool
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
	// wg tracks every spawned executor independently of the active map:
	// a fresh same-key operation may replace a canceled-but-still-running
	// call in active, so the map alone cannot prove shutdown quiescence.
	// Adds happen under mu before the goroutine starts; Done runs when the
	// executor finishes. Sealed admission (closed) guarantees no Add can
	// race a completed Wait.
	wg     sync.WaitGroup
	epoch  uint64
	// closed seals admission: set by Close before cancel/drain, so no
	// refresh can start after the drain observes zero active operations.
	closed bool
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
	_, miss := uc.sealedOrMiss(key)
	return miss
}

// sealedOrMiss atomically reports coordinator shutdown and negative-cache
// state under one lock acquisition, giving late refreshes a single
// linearization point: sealed admission always wins over a cached absence,
// so a refresh arriving after Close reports ErrFaviconShutdown even when
// the key holds a cached 404/410.
func (uc *FaviconUseCase) sealedOrMiss(key string) (closed, miss bool) {
	now := uc.now()
	uc.shared.mu.Lock()
	defer uc.shared.mu.Unlock()
	if uc.shared.closed {
		return true, false
	}
	m, ok := uc.shared.misses[key]
	if !ok {
		return false, false
	}
	if !now.Before(m.expiresAt) {
		delete(uc.shared.misses, key)
		return false, false
	}
	return false, true
}

// clearMiss drops any absence record for key: a later success proves the
// resource exists and must not stay suppressed.
func (uc *FaviconUseCase) clearMiss(key string) {
	uc.shared.mu.Lock()
	defer uc.shared.mu.Unlock()
	delete(uc.shared.misses, key)
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
	// Expired names linger in order until evicted; compact before the
	// backlog can grow without bound.
	if len(uc.shared.order) > 4*maxMissEntries {
		uc.compactMissOrderLocked()
	}
}

// compactMissOrderLocked drops order names with no live entry and collapses
// duplicates to one entry per live key, keeping the newest occurrence.
// Eviction removes entries from the map but not from order, so an evicted
// key that is reinserted would otherwise leave a stale duplicate behind:
// repeated cycles grow order without bound and let an old duplicate evict
// the live entry early. Caller must hold refresh.mu.
func (uc *FaviconUseCase) compactMissOrderLocked() {
	last := make(map[string]int, len(uc.shared.misses))
	for i, key := range uc.shared.order {
		if _, ok := uc.shared.misses[key]; ok {
			last[key] = i
		}
	}
	ordered := make([]string, 0, len(last))
	for key := range last {
		ordered = append(ordered, key)
	}
	slices.SortFunc(ordered, func(a, b string) int { return last[a] - last[b] })
	for i := len(ordered); i < len(uc.shared.order); i++ {
		uc.shared.order[i] = ""
	}
	uc.shared.order = append(uc.shared.order[:0], ordered...)
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
// the owned background context; joiners observe the result. The creating
// waiter also advances the invalidation epoch, so concurrent joiners never
// invalidate the operation they joined. A canceled waiter leaves without
// affecting others; the operation is canceled atomically when the last
// waiter leaves, and late joiners start a fresh operation instead of
// observing a dying call. When the admission budget is exhausted, or the
// coordinator is closed, direct callers receive ErrFaviconBusy or
// ErrFaviconShutdown respectively (never false success).
func (uc *FaviconUseCase) joinRefreshOp(ctx context.Context, key string, exec func(opCtx context.Context, startEpoch uint64) error) error {
	uc.shared.mu.Lock()
	if uc.shared.closed {
		uc.shared.mu.Unlock()
		return appport.ErrFaviconShutdown
	}
	if call, ok := uc.shared.active[key]; ok && !call.closing {
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
	// The creator advances the epoch with creation itself: older
	// completions cannot repopulate entries this operation supersedes,
	// and joiners share the creator's epoch instead of invalidating it.
	uc.shared.epoch++
	startEpoch := uc.shared.epoch
	call := &refreshCall{done: make(chan struct{}), waiters: 1, cancel: cancel}
	uc.shared.active[key] = call
	uc.shared.wg.Add(1)
	uc.shared.mu.Unlock()

	go func() {
		defer uc.shared.wg.Done()
		err := exec(opCtx, startEpoch)
		<-uc.shared.slots
		cancel()
		uc.shared.mu.Lock()
		call.err = err
		if uc.shared.active[key] == call {
			delete(uc.shared.active, key)
		}
		uc.shared.mu.Unlock()
		close(call.done)
	}()

	return uc.awaitRefreshCall(ctx, call)
}

// awaitRefreshCall waits for a shared operation or the waiter's own
// cancellation, releasing the waiter slot exactly once. The last waiter to
// leave marks the call closing and cancels it atomically under the lock:
// context cancellation only signals, so invoking it while holding the
// coordinator mutex cannot block.
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
		if call.waiters <= 0 && !call.closing {
			call.closing = true
			call.cancel()
		}
		uc.shared.mu.Unlock()
		return ctx.Err()
	}
}

// drainRefreshOps waits until every spawned executor finished, up to a
// bounded multiple of the operation deadline. It waits on the executor
// wait group rather than the active map: a replaced
// canceled-but-still-running call no longer occupies its map entry, but
// its executor may still touch repositories. Callers cancel first (see
// Close); cooperative operations observe the background cancellation and
// finish promptly, so hitting the bound means stuck work that shutdown
// must not wait out.
func (uc *FaviconUseCase) drainRefreshOps() {
	done := make(chan struct{})
	go func() {
		uc.shared.wg.Wait()
		close(done)
	}()
	deadline := time.NewTimer(3 * refreshOpTimeout)
	defer deadline.Stop()
	select {
	case <-done:
	case <-deadline.C:
	}
}
