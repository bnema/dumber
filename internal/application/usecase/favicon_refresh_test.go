package usecase

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appport "github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/favicon"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// refreshTestUC builds a FaviconUseCase with a custom fetcher and
// controllable clock for refresh-coordinator tests.
func refreshTestUC(t *testing.T, fetcher appport.FaviconFetcher, now *time.Time, background context.Context) *FaviconUseCase {
	t.Helper()
	if background == nil {
		background = context.Background()
	}
	nowFn := time.Now
	if now != nil {
		nowFn = func() time.Time { return *now }
	}
	return NewFaviconUseCase(FaviconDeps{
		Repository:   mockFaviconRepository(t, newFaviconRepoState()),
		BlobStore:    mockFaviconBlobStore(t, newFaviconBlobStoreState()),
		Converter:    mockFaviconConverter(t, &faviconConverterState{}),
		Scheduler:    mockFaviconScheduler(t, &faviconSchedulerState{seen: map[favicon.Key]bool{}}),
		Invalidators: mockFaviconInvalidators(t, &faviconInvalidatorsState{}),
		Fetcher:      fetcher,
		Now:          nowFn,
		Background:   background,
	})
}

func refreshFetcherMock(t *testing.T, fetch func(context.Context, appport.FaviconFetchRequest) (*appport.FaviconFetchedIcon, error)) *portmocks.MockFaviconFetcher {
	t.Helper()
	fetcher := portmocks.NewMockFaviconFetcher(t)
	fetcher.EXPECT().Fetch(mock.Anything, mock.Anything).RunAndReturn(fetch).Maybe()
	return fetcher
}

func fetchedIcon(iconURL string) *appport.FaviconFetchedIcon {
	return &appport.FaviconFetchedIcon{
		IconURL:     iconURL,
		Bytes:       []byte("icon-bytes"),
		ContentType: "image/png",
	}
}

// TestRefreshFromIconURLs_ConcurrentIdenticalSharesOneFetch reproduces the
// P1.1 bypass: concurrent identical candidate requests must share a single
// fetch instead of issuing one fetch per caller.
func TestRefreshFromIconURLs_ConcurrentIdenticalSharesOneFetch(t *testing.T) {
	var calls atomic.Int64
	entered := make(chan struct{}, 8)
	release := make(chan struct{})
	fetcher := refreshFetcherMock(t, func(_ context.Context, req appport.FaviconFetchRequest) (*appport.FaviconFetchedIcon, error) {
		calls.Add(1)
		entered <- struct{}{}
		<-release
		return fetchedIcon(req.IconURL), nil
	})
	uc := refreshTestUC(t, fetcher, nil, nil)

	const waiters = 5
	page := "https://example.com/a"
	candidates := []string{"https://example.com/favicon.ico"}
	key, _, ok := refreshRequestKey(page, candidates)
	require.True(t, ok)
	results := make(chan error, waiters)
	var wg sync.WaitGroup
	for range waiters {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- uc.RefreshFromIconURLs(context.Background(), page, candidates)
		}()
	}
	// All five waiters must join the single shared operation before the
	// fetch completes; the waiter count is the deterministic join signal.
	require.Eventually(t, func() bool {
		uc.shared.mu.Lock()
		defer uc.shared.mu.Unlock()
		call, ok := uc.shared.active[key]
		return ok && call.waiters == waiters
	}, 10*time.Second, 5*time.Millisecond, "waiters did not join the shared operation")
	close(release)
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, calls.Load(), "identical concurrent refreshes must share one fetch")
}

// TestRefreshFromIconURLs_ChangingCandidatesDoNotShare verifies that
// different candidate lists are distinct operations.
func TestRefreshFromIconURLs_ChangingCandidatesDoNotShare(t *testing.T) {
	var calls atomic.Int64
	fetcher := refreshFetcherMock(t, func(_ context.Context, req appport.FaviconFetchRequest) (*appport.FaviconFetchedIcon, error) {
		calls.Add(1)
		return fetchedIcon(req.IconURL), nil
	})
	uc := refreshTestUC(t, fetcher, nil, nil)

	require.NoError(t, uc.RefreshFromIconURLs(context.Background(), "https://example.com/a", []string{"https://example.com/one.ico"}))
	require.NoError(t, uc.RefreshFromIconURLs(context.Background(), "https://example.com/a", []string{"https://example.com/two.ico"}))
	require.EqualValues(t, 2, calls.Load())
}

// TestRefreshFromIconURLs_CancelOneWaiterKeepsOthers proves waiter
// independence: a canceled waiter leaves while the shared operation serves
// the rest.
func TestRefreshFromIconURLs_CancelOneWaiterKeepsOthers(t *testing.T) {
	release := make(chan struct{})
	fetcher := refreshFetcherMock(t, func(_ context.Context, req appport.FaviconFetchRequest) (*appport.FaviconFetchedIcon, error) {
		<-release
		return fetchedIcon(req.IconURL), nil
	})
	uc := refreshTestUC(t, fetcher, nil, nil)

	page := "https://example.com/a"
	candidates := []string{"https://example.com/favicon.ico"}
	key, _, ok := refreshRequestKey(page, candidates)
	require.True(t, ok, "test page and candidates must form a refresh key")
	leaving, cancelLeaving := context.WithCancel(context.Background())
	left := make(chan error, 1)
	go func() {
		left <- uc.RefreshFromIconURLs(leaving, page, candidates)
	}()
	staying := make(chan error, 1)
	go func() {
		staying <- uc.RefreshFromIconURLs(context.Background(), page, candidates)
	}()
	// Both waiters must share one operation before either leaves: two
	// waiters on the same call proves sharing, not two separate fetches.
	require.Eventually(t, func() bool {
		uc.shared.mu.Lock()
		defer uc.shared.mu.Unlock()
		call, ok := uc.shared.active[key]
		return ok && !call.closing && call.waiters == 2
	}, 10*time.Second, 5*time.Millisecond, "both waiters must join one shared operation")
	cancelLeaving()
	select {
	case err := <-left:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(10 * time.Second):
		t.Fatal("canceled waiter did not return")
	}
	close(release)
	select {
	case err := <-staying:
		require.NoError(t, err, "remaining waiter must still be served")
	case <-time.After(10 * time.Second):
		t.Fatal("remaining waiter was dropped with the canceled one")
	}
}

// TestRefreshFromIconURLs_InvalidPageHierarchyRejected verifies invalid
// origins never reach the fetcher.
func TestRefreshFromIconURLs_InvalidPageHierarchyRejected(t *testing.T) {
	var calls atomic.Int64
	fetcher := refreshFetcherMock(t, func(_ context.Context, req appport.FaviconFetchRequest) (*appport.FaviconFetchedIcon, error) {
		calls.Add(1)
		return fetchedIcon(req.IconURL), nil
	})
	uc := refreshTestUC(t, fetcher, nil, nil)

	for _, page := range []string{"", "not a url", "ftp://example.com/a", "dumb://internal"} {
		require.ErrorIs(t, uc.RefreshFromIconURLs(context.Background(), page, []string{"https://example.com/favicon.ico"}), ErrFaviconMiss)
	}
	require.Zero(t, calls.Load())
}

// TestRefreshRequestKey_Distinctions covers origin and candidate
// significance: scheme/port/query matter, fragments/case/default ports do
// not, relative candidates resolve against the page, over-long and excess
// candidates are skipped.
func TestRefreshRequestKey_Distinctions(t *testing.T) {
	page := "https://example.com:8443/a?x=1"
	keyOf := func(candidates ...string) (string, bool) {
		key, _, ok := refreshRequestKey(page, candidates)
		return key, ok
	}
	base, ok := keyOf("https://example.com:8443/icon.png")
	require.True(t, ok)

	other, _ := keyOf("https://example.com:8443/other.png")
	require.NotEqual(t, base, other, "different candidates must not share")

	portKey, _ := keyOf("https://example.com:9443/icon.png")
	require.NotEqual(t, base, portKey, "ports are significant")

	queryKey, _ := keyOf("https://example.com:8443/icon.png?v=2")
	require.NotEqual(t, base, queryKey, "queries are significant")

	fragKey, _ := keyOf("https://example.com:8443/icon.png#frag")
	require.Equal(t, base, fragKey, "fragments must not split sharing")

	caseKey, _ := keyOf("HTTPS://EXAMPLE.COM:8443/icon.png")
	require.Equal(t, base, caseKey, "scheme/host case must not split sharing")

	defaultPort, _ := keyOf("https://example.com:443/icon.png")
	otherPage, _, ok := refreshRequestKey("https://example.com/icon", []string{"https://example.com:443/icon.png"})
	require.True(t, ok)
	require.NotEqual(t, defaultPort, otherPage, "page origin is part of the identity")

	relKey, relCandidates, ok := refreshRequestKey("https://example.com/a", []string{"/icon.png"})
	require.True(t, ok)
	require.Equal(t, []string{"https://example.com/icon.png"}, relCandidates, "relative candidates resolve against the page")
	require.NotEqual(t, relKey, base, "different pages must not share")

	long := "https://example.com/" + string(make([]byte, maxRefreshCandidateLen))
	_, kept, ok := refreshRequestKey(page, []string{long, "https://example.com:8443/icon.png"})
	require.True(t, ok)
	require.Len(t, kept, 1, "over-long candidates are skipped")

	many := make([]string, maxRefreshCandidates+4)
	for i := range many {
		many[i] = "https://example.com:8443/icon.png"
	}
	_, kept, ok = refreshRequestKey(page, many)
	require.True(t, ok)
	require.Len(t, kept, maxRefreshCandidates, "candidates are capped")

	_, _, ok = refreshRequestKey("not a url", []string{"https://example.com/icon.png"})
	require.False(t, ok, "invalid page hierarchy is rejected")
}

// TestRefreshAdmission_BusyWhenExhausted fills the shared budget with
// blocked operations and proves the next direct caller gets a distinct
// busy outcome instead of false success.
func TestRefreshAdmission_BusyWhenExhausted(t *testing.T) {
	release := make(chan struct{})
	fetcher := refreshFetcherMock(t, func(_ context.Context, req appport.FaviconFetchRequest) (*appport.FaviconFetchedIcon, error) {
		<-release
		return fetchedIcon(req.IconURL), nil
	})
	uc := refreshTestUC(t, fetcher, nil, nil)

	var wg sync.WaitGroup
	for i := range maxActiveRefreshes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Distinct candidates per holder fill distinct budget slots;
			// identical requests would share one.
			icon := fmt.Sprintf("https://example.com/icon-%d.png", i)
			_ = uc.RefreshFromIconURLs(context.Background(), "https://example.com/page", []string{icon})
		}(i)
	}
	require.Eventually(t, func() bool {
		uc.shared.mu.Lock()
		defer uc.shared.mu.Unlock()
		return len(uc.shared.active) == maxActiveRefreshes
	}, 10*time.Second, 5*time.Millisecond, "budget must fill")

	err := uc.RefreshFromIconURLs(context.Background(), "https://other.example/", []string{"https://other.example/icon.png"})
	require.ErrorIs(t, err, appport.ErrFaviconBusy)
	require.NotErrorIs(t, err, ErrFaviconMiss, "overload must stay distinct from a miss")

	close(release)
	wg.Wait()
	require.NoError(t, uc.RefreshFromIconURLs(context.Background(), "https://other.example/", []string{"https://other.example/icon.png"}))
}

// TestRefreshOp_WholeDeadline verifies the shared operation carries the
// bounded whole-operation deadline without waiting it out.
func TestRefreshOp_WholeDeadline(t *testing.T) {
	deadlines := make(chan time.Time, 1)
	fetcher := refreshFetcherMock(t, func(ctx context.Context, req appport.FaviconFetchRequest) (*appport.FaviconFetchedIcon, error) {
		if deadline, ok := ctx.Deadline(); ok {
			deadlines <- deadline
		}
		return fetchedIcon(req.IconURL), nil
	})
	uc := refreshTestUC(t, fetcher, nil, nil)

	require.NoError(t, uc.RefreshFromIconURLs(context.Background(), "https://example.com/a", []string{"https://example.com/favicon.ico"}))
	select {
	case deadline := <-deadlines:
		remaining := time.Until(deadline)
		require.Greater(t, remaining, 4*time.Second)
		require.LessOrEqual(t, remaining, refreshOpTimeout)
	case <-time.After(10 * time.Second):
		t.Fatal("fetch did not observe the operation deadline")
	}
}

// TestRefreshLastWaiterCancelAbortsOp proves a lone waiter that leaves
// cancels the shared operation instead of leaking it.
func TestRefreshLastWaiterCancelAbortsOp(t *testing.T) {
	observedCancel := make(chan struct{}, 1)
	entered := make(chan struct{}, 1)
	fetcher := refreshFetcherMock(t, func(ctx context.Context, req appport.FaviconFetchRequest) (*appport.FaviconFetchedIcon, error) {
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			observedCancel <- struct{}{}
			return nil, ctx.Err()
		case <-time.After(10 * time.Second):
			return fetchedIcon(req.IconURL), nil
		}
	})
	uc := refreshTestUC(t, fetcher, nil, nil)

	waiter, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- uc.RefreshFromIconURLs(waiter, "https://example.com/a", []string{"https://example.com/favicon.ico"})
	}()
	// Cancel only after the executor entered the fetch: len(active) == 1
	// alone cannot prove the executor is blocked inside Fetch, and an early
	// cancel could race operation creation instead of aborting shared work.
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("executor did not enter the fetch")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(10 * time.Second):
		t.Fatal("canceled lone waiter did not return")
	}
	select {
	case <-observedCancel:
	case <-time.After(10 * time.Second):
		t.Fatal("operation was not canceled with its last waiter")
	}
	require.Eventually(t, func() bool {
		uc.shared.mu.Lock()
		defer uc.shared.mu.Unlock()
		return len(uc.shared.active) == 0
	}, 10*time.Second, 5*time.Millisecond, "canceled operation must release its slot")
}

// TestFaviconUseCase_CloseCancelsAndDrains proves shutdown wiring: Close
// aborts owned background work and returns once nothing is active.
func TestFaviconUseCase_CloseCancelsAndDrains(t *testing.T) {
	background, cancel := context.WithCancel(context.Background())
	defer cancel()
	sawCancel := make(chan struct{}, 1)
	fetcher := refreshFetcherMock(t, func(ctx context.Context, req appport.FaviconFetchRequest) (*appport.FaviconFetchedIcon, error) {
		select {
		case <-ctx.Done():
			sawCancel <- struct{}{}
			return nil, ctx.Err()
		case <-time.After(10 * time.Second):
			return fetchedIcon(req.IconURL), nil
		}
	})
	uc := refreshTestUC(t, fetcher, nil, background)

	done := make(chan error, 1)
	go func() {
		done <- uc.RefreshFromIconURLs(context.Background(), "https://example.com/a", []string{"https://example.com/favicon.ico"})
	}()
	require.Eventually(t, func() bool {
		uc.shared.mu.Lock()
		defer uc.shared.mu.Unlock()
		return len(uc.shared.active) == 1
	}, 10*time.Second, 5*time.Millisecond)

	closed := make(chan struct{})
	go func() {
		uc.Close()
		close(closed)
	}()
	select {
	case <-sawCancel:
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not cancel background work")
	}
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not drain")
	}
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("refresh caller did not return after Close")
	}
	// Repeat Close must be safe (idempotent, no error to report).
	uc.Close()
}

// TestRefreshMissClassification_HTTPClasses proves only genuine absence
// (404/410) is negatively cached: auth/transient failures refetch, and a
// repeated miss within TTL issues no fetch.
func TestRefreshMissClassification_HTTPClasses(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	statusByURL := map[string]int{
		"https://example.com/gone.ico":       404,
		"https://example.com/removed.ico":    410,
		"https://example.com/denied.ico":     403,
		"https://example.com/login.ico":      401,
		"https://example.com/broken.ico":     500,
		"https://example.com/unstable.ico":   503,
		"https://example.com/validation.ico": 0,
	}
	var calls atomic.Int64
	var candidateCalls atomic.Int64
	fetcher := refreshFetcherMock(t, func(_ context.Context, req appport.FaviconFetchRequest) (*appport.FaviconFetchedIcon, error) {
		calls.Add(1)
		// The all-miss fallback runs a candidate-less discovery fetch;
		// assertions below count candidate batches only.
		if req.IconURL != "" {
			candidateCalls.Add(1)
		}
		status := statusByURL[req.IconURL]
		if status == 0 {
			return nil, appport.ErrFaviconMiss
		}
		return nil, &appport.FaviconFetchError{StatusCode: status}
	})
	uc := refreshTestUC(t, fetcher, &now, nil)
	page := "https://example.com/a"

	refresh := func(icon string) error {
		return uc.RefreshFromIconURLs(context.Background(), page, []string{icon})
	}
	for icon := range statusByURL {
		err := refresh(icon)
		require.ErrorIs(t, err, ErrFaviconMiss, "every class must preserve miss matching for %s", icon)
	}
	firstCalls := candidateCalls.Load()
	require.EqualValues(t, len(statusByURL), firstCalls)

	// Genuine absence is cached: 404/410 refetch nothing within TTL.
	require.ErrorIs(t, refresh("https://example.com/gone.ico"), ErrFaviconMiss)
	require.ErrorIs(t, refresh("https://example.com/removed.ico"), ErrFaviconMiss)
	require.Equal(t, firstCalls, candidateCalls.Load(), "cached genuine absence must not refetch")

	// Auth/transient/validation failures are never cached: they refetch.
	require.ErrorIs(t, refresh("https://example.com/denied.ico"), ErrFaviconMiss)
	require.ErrorIs(t, refresh("https://example.com/broken.ico"), ErrFaviconMiss)
	require.ErrorIs(t, refresh("https://example.com/validation.ico"), ErrFaviconMiss)
	require.Equal(t, firstCalls+3, candidateCalls.Load())

	// After TTL expiry the genuine absence is eligible again.
	now = now.Add(faviconMissTTL + time.Second)
	require.ErrorIs(t, refresh("https://example.com/gone.ico"), ErrFaviconMiss)
	require.Equal(t, firstCalls+4, candidateCalls.Load())
}

// TestRefreshInvalidate_DropsStaleCompletion proves the epoch rule: an
// invalidation racing an in-flight fetch prevents the stale bytes from
// being stored.
func TestRefreshInvalidate_DropsStaleCompletion(t *testing.T) {
	release := make(chan struct{})
	fetcher := refreshFetcherMock(t, func(_ context.Context, req appport.FaviconFetchRequest) (*appport.FaviconFetchedIcon, error) {
		<-release
		return fetchedIcon(req.IconURL), nil
	})
	repo := newFaviconRepoState()
	uc := NewFaviconUseCase(FaviconDeps{
		Repository:   mockFaviconRepository(t, repo),
		BlobStore:    mockFaviconBlobStore(t, newFaviconBlobStoreState()),
		Converter:    mockFaviconConverter(t, &faviconConverterState{}),
		Scheduler:    mockFaviconScheduler(t, &faviconSchedulerState{seen: map[favicon.Key]bool{}}),
		Invalidators: mockFaviconInvalidators(t, &faviconInvalidatorsState{}),
		Fetcher:      fetcher,
		Now:          time.Now,
		Background:   context.Background(),
	})

	page := "https://example.com/a"
	done := make(chan error, 1)
	go func() {
		done <- uc.RefreshFromIconURLs(context.Background(), page, []string{"https://example.com/favicon.ico"})
	}()
	require.Eventually(t, func() bool {
		uc.shared.mu.Lock()
		defer uc.shared.mu.Unlock()
		return len(uc.shared.active) == 1
	}, 10*time.Second, 5*time.Millisecond)

	key, ok := favicon.CanonicalKey(page)
	require.True(t, ok)
	require.NoError(t, uc.Invalidate(context.Background(), key))
	close(release)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("refresh did not complete after release")
	}
	require.Empty(t, repo.byKey, "stale completion must not repopulate invalidated entries")
}

// TestRefreshJoinerSharesCreatorEpoch proves concurrent joiners never
// invalidate the operation they joined: overlapping identical refreshes
// both succeed and the fetched icon is stored.
func TestRefreshJoinerSharesCreatorEpoch(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int64
	fetcher := refreshFetcherMock(t, func(_ context.Context, req appport.FaviconFetchRequest) (*appport.FaviconFetchedIcon, error) {
		calls.Add(1)
		<-release
		return fetchedIcon(req.IconURL), nil
	})
	repo := newFaviconRepoState()
	uc := NewFaviconUseCase(FaviconDeps{
		Repository:   mockFaviconRepository(t, repo),
		BlobStore:    mockFaviconBlobStore(t, newFaviconBlobStoreState()),
		Converter:    mockFaviconConverter(t, &faviconConverterState{}),
		Scheduler:    mockFaviconScheduler(t, &faviconSchedulerState{seen: map[favicon.Key]bool{}}),
		Invalidators: mockFaviconInvalidators(t, &faviconInvalidatorsState{}),
		Fetcher:      fetcher,
		Now:          time.Now,
		Background:   context.Background(),
	})

	page := "https://example.com/a"
	candidates := []string{"https://example.com/favicon.ico"}
	key, _, ok := refreshRequestKey(page, candidates)
	require.True(t, ok)
	first := make(chan error, 1)
	go func() {
		first <- uc.RefreshFromIconURLs(context.Background(), page, candidates)
	}()
	require.Eventually(t, func() bool {
		uc.shared.mu.Lock()
		defer uc.shared.mu.Unlock()
		return len(uc.shared.active) == 1
	}, 10*time.Second, 5*time.Millisecond)
	second := make(chan error, 1)
	go func() {
		second <- uc.RefreshFromIconURLs(context.Background(), page, candidates)
	}()
	require.Eventually(t, func() bool {
		uc.shared.mu.Lock()
		defer uc.shared.mu.Unlock()
		call, ok := uc.shared.active[key]
		return ok && call.waiters == 2
	}, 10*time.Second, 5*time.Millisecond, "joiner must share the creator operation")
	close(release)
	select {
	case err := <-first:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("creator did not complete")
	}
	select {
	case err := <-second:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("joiner did not complete")
	}
	require.EqualValues(t, 1, calls.Load())
	require.NotEmpty(t, repo.byKey, "shared success must be stored, not dropped by a joiner epoch bump")
}

// TestRefreshPartialMissDoesNotPoisonLaterCandidates proves request-level
// negative caching happens only for fully missed requests: a 404 on the
// first candidate followed by a successful second candidate stores the
// icon and records no miss.
func TestRefreshPartialMissDoesNotPoisonLaterCandidates(t *testing.T) {
	var calls atomic.Int64
	fetcher := refreshFetcherMock(t, func(_ context.Context, req appport.FaviconFetchRequest) (*appport.FaviconFetchedIcon, error) {
		calls.Add(1)
		if req.IconURL == "https://example.com/gone.ico" {
			return nil, &appport.FaviconFetchError{StatusCode: 404}
		}
		return fetchedIcon(req.IconURL), nil
	})
	repo := newFaviconRepoState()
	uc := NewFaviconUseCase(FaviconDeps{
		Repository:   mockFaviconRepository(t, repo),
		BlobStore:    mockFaviconBlobStore(t, newFaviconBlobStoreState()),
		Converter:    mockFaviconConverter(t, &faviconConverterState{}),
		Scheduler:    mockFaviconScheduler(t, &faviconSchedulerState{seen: map[favicon.Key]bool{}}),
		Invalidators: mockFaviconInvalidators(t, &faviconInvalidatorsState{}),
		Fetcher:      fetcher,
		Now:          time.Now,
		Background:   context.Background(),
	})

	page := "https://example.com/a"
	candidates := []string{"https://example.com/gone.ico", "https://example.com/good.ico"}
	require.NoError(t, uc.RefreshFromIconURLs(context.Background(), page, candidates))
	require.NotEmpty(t, repo.byKey, "successful candidate must be stored")
	key, _, ok := refreshRequestKey(page, candidates)
	require.True(t, ok)
	require.False(t, uc.cachedMiss(key), "partial miss must not populate the request miss cache")
}

// TestRefreshCanceledCallNotJoined proves a waiter arriving after the last
// waiter canceled starts a fresh operation instead of observing the dying
// call.
func TestRefreshCanceledCallNotJoined(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int64
	fetcher := refreshFetcherMock(t, func(ctx context.Context, req appport.FaviconFetchRequest) (*appport.FaviconFetchedIcon, error) {
		calls.Add(1)
		select {
		case <-release:
			return fetchedIcon(req.IconURL), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	uc := refreshTestUC(t, fetcher, nil, nil)

	page := "https://example.com/a"
	candidates := []string{"https://example.com/favicon.ico"}
	leaving, cancel := context.WithCancel(context.Background())
	left := make(chan error, 1)
	go func() {
		left <- uc.RefreshFromIconURLs(leaving, page, candidates)
	}()
	require.Eventually(t, func() bool {
		uc.shared.mu.Lock()
		defer uc.shared.mu.Unlock()
		return len(uc.shared.active) == 1
	}, 10*time.Second, 5*time.Millisecond)
	cancel()
	select {
	case err := <-left:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(10 * time.Second):
		t.Fatal("canceled waiter did not return")
	}
	// The dying call still occupies the map entry until its executor
	// finishes; the next waiter must start fresh rather than join it.
	fresh := make(chan error, 1)
	go func() {
		fresh <- uc.RefreshFromIconURLs(context.Background(), page, candidates)
	}()
	require.Eventually(t, func() bool {
		return calls.Load() == 2
	}, 10*time.Second, 5*time.Millisecond, "late joiner must start a fresh operation")
	close(release)
	select {
	case err := <-fresh:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("fresh operation did not complete")
	}
}

// TestRefreshCloseSealsAdmission proves no refresh can start after Close
// began: late callers receive a distinct shutdown error instead of racing
// the drain.
func TestRefreshCloseSealsAdmission(t *testing.T) {
	uc := refreshTestUC(t, refreshFetcherMock(t, func(_ context.Context, req appport.FaviconFetchRequest) (*appport.FaviconFetchedIcon, error) {
		return fetchedIcon(req.IconURL), nil
	}), nil, nil)

	uc.Close()
	err := uc.RefreshFromIconURLs(context.Background(), "https://example.com/a", []string{"https://example.com/favicon.ico"})
	require.ErrorIs(t, err, appport.ErrFaviconShutdown)
	require.NotErrorIs(t, err, ErrFaviconMiss, "shutdown must stay distinct from a miss")
}

// TestRefreshMissOrderBounded proves the negative-cache order backlog
// cannot grow without bound under distinct misses.
func TestRefreshMissOrderBounded(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	fetcher := refreshFetcherMock(t, func(_ context.Context, _ appport.FaviconFetchRequest) (*appport.FaviconFetchedIcon, error) {
		return nil, &appport.FaviconFetchError{StatusCode: 404}
	})
	uc := refreshTestUC(t, fetcher, &now, nil)

	for i := range 3 * maxMissEntries {
		icon := "https://example.com/missing-" + string(rune('a'+i%26)) + "-" + string(rune('0'+i/26%10)) + ".ico"
		require.ErrorIs(t, uc.RefreshFromIconURLs(context.Background(), "https://example.com/a", []string{icon}), ErrFaviconMiss)
	}
	uc.shared.mu.Lock()
	defer uc.shared.mu.Unlock()
	require.LessOrEqual(t, len(uc.shared.misses), maxMissEntries)
	require.LessOrEqual(t, len(uc.shared.order), 4*maxMissEntries, "order backlog must stay bounded")
}

// TestRefreshSharing_ScopedToUseCaseInstance proves profile isolation:
// identical requests on different use-case instances never share fetches.
func TestRefreshSharing_ScopedToUseCaseInstance(t *testing.T) {
	var calls atomic.Int64
	newUC := func() *FaviconUseCase {
		fetcher := refreshFetcherMock(t, func(_ context.Context, req appport.FaviconFetchRequest) (*appport.FaviconFetchedIcon, error) {
			calls.Add(1)
			return fetchedIcon(req.IconURL), nil
		})
		return refreshTestUC(t, fetcher, nil, nil)
	}
	first, second := newUC(), newUC()
	page := "https://example.com/a"
	candidates := []string{"https://example.com/favicon.ico"}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, uc := range []*FaviconUseCase{first, second} {
		wg.Add(1)
		go func(uc *FaviconUseCase) {
			defer wg.Done()
			errs <- uc.RefreshFromIconURLs(context.Background(), page, candidates)
		}(uc)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.EqualValues(t, 2, calls.Load(), "sharing must not cross use-case instances")
}

// TestRefreshMixedStatusBatchNotCached proves a batch mixing a transient
// failure with a genuine absence never populates the negative cache: the
// last miss alone must not decide cacheability.
func TestRefreshMixedStatusBatchNotCached(t *testing.T) {
	var calls atomic.Int64
	fetcher := refreshFetcherMock(t, func(_ context.Context, req appport.FaviconFetchRequest) (*appport.FaviconFetchedIcon, error) {
		calls.Add(1)
		if req.IconURL == "https://example.com/denied.ico" {
			return nil, &appport.FaviconFetchError{StatusCode: 403}
		}
		return nil, &appport.FaviconFetchError{StatusCode: 404}
	})
	uc := refreshTestUC(t, fetcher, nil, nil)

	page := "https://example.com/a"
	candidates := []string{"https://example.com/denied.ico", "https://example.com/gone.ico"}
	require.ErrorIs(t, uc.RefreshFromIconURLs(context.Background(), page, candidates), ErrFaviconMiss)
	key, _, ok := refreshRequestKey(page, candidates)
	require.True(t, ok)
	require.False(t, uc.cachedMiss(key), "mixed 403+404 batch must not be negatively cached")
	require.ErrorIs(t, uc.RefreshFromIconURLs(context.Background(), page, candidates), ErrFaviconMiss)
	require.EqualValues(t, 4, calls.Load(), "second refresh must retry instead of reading a cached absence")
}

// TestRefreshClosedWinsOverCachedMiss proves sealed admission takes
// precedence over a cached absence: after Close, refresh reports shutdown
// even for a key holding a genuine 404.
func TestRefreshClosedWinsOverCachedMiss(t *testing.T) {
	var calls atomic.Int64
	fetcher := refreshFetcherMock(t, func(_ context.Context, _ appport.FaviconFetchRequest) (*appport.FaviconFetchedIcon, error) {
		calls.Add(1)
		return nil, &appport.FaviconFetchError{StatusCode: 404}
	})
	uc := refreshTestUC(t, fetcher, nil, nil)

	page := "https://example.com/a"
	candidates := []string{"https://example.com/gone.ico"}
	key, _, ok := refreshRequestKey(page, candidates)
	require.True(t, ok)
	uc.noteMiss(key, &appport.FaviconFetchError{StatusCode: 404})
	require.True(t, uc.cachedMiss(key))
	uc.Close()
	err := uc.RefreshFromIconURLs(context.Background(), page, candidates)
	require.ErrorIs(t, err, appport.ErrFaviconShutdown)
	require.NotErrorIs(t, err, ErrFaviconMiss, "shutdown must stay distinct from a miss")
	require.Zero(t, calls.Load(), "sealed refresh must not reach the fetcher")
}

// TestCompactMissOrderDedupsReinsertedKeys proves compaction collapses
// stale duplicates: an evicted key that is reinserted keeps exactly one
// order entry, so the backlog cannot grow across expire/reinsert cycles.
func TestCompactMissOrderDedupsReinsertedKeys(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	uc := refreshTestUC(t, refreshFetcherMock(t, func(_ context.Context, _ appport.FaviconFetchRequest) (*appport.FaviconFetchedIcon, error) {
		return nil, &appport.FaviconFetchError{StatusCode: 404}
	}), &now, nil)

	uc.shared.mu.Lock()
	uc.shared.misses["live"] = refreshMiss{expiresAt: now.Add(faviconMissTTL)}
	// Simulate eviction (map entry removed, order name left behind) plus
	// reinsertion: the same key appears twice in order.
	uc.shared.order = []string{"stale-a", "live", "stale-b", "live"}
	uc.compactMissOrderLocked()
	order := append([]string(nil), uc.shared.order...)
	uc.shared.mu.Unlock()
	require.Equal(t, []string{"live"}, order, "compaction must keep one entry per live key")
}

// TestRefreshRequestKey_IPv6DefaultPortEquivalence proves bracketed IPv6
// hosts with an explicit default port share identity with the bare form:
// without bracket-aware parsing the two spellings hash differently and
// bypass in-flight sharing.
func TestRefreshRequestKey_IPv6DefaultPortEquivalence(t *testing.T) {
	page := "https://[2001:db8::1]/a"
	withPort, withPortAccepted, ok := refreshRequestKey(page, []string{"https://[2001:db8::1]:443/favicon.ico"})
	require.True(t, ok)
	bare, bareAccepted, ok := refreshRequestKey(page, []string{"https://[2001:db8::1]/favicon.ico"})
	require.True(t, ok)
	require.Equal(t, bare, withPort, "explicit default port must not change the request key")
	require.Equal(t, bareAccepted, withPortAccepted)
}
