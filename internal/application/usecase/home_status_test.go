package usecase

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/application/port/mocks"
	"bnema.dev/dumber/internal/domain/entity"
)

type homeStatusFixture struct {
	uc     *HomeUseCase
	layout *entity.HomeLayout
	prober *mocks.MockHomeStatusProber
	now    *time.Time
}

func newHomeStatusFixture(t *testing.T) *homeStatusFixture {
	t.Helper()
	off := false
	layout := entity.HomeLayout{
		Settings: entity.HomeSettings{Status: true},
		Groups: []entity.HomeGroup{{Name: "G", Services: []entity.HomeService{
			{ID: "media", Name: "Media", URL: "https://media.local"},
			{ID: "quiet", Name: "Quiet", URL: "https://quiet.local", Status: &off},
			{ID: "nourl", Name: "No URL"},
		}}},
	}
	layout.Normalize()
	require.Empty(t, layout.Validate())

	f := &homeStatusFixture{layout: &layout, prober: mocks.NewMockHomeStatusProber(t)}
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	f.now = &now
	source := mocks.NewMockHomeConfigSource(t)
	source.EXPECT().Load(mock.Anything).RunAndReturn(func(context.Context) (port.HomeConfigSnapshot, error) {
		return port.HomeConfigSnapshot{Layout: *f.layout, Exists: true}, nil
	}).Maybe()
	f.uc = NewHomeUseCase(HomeDeps{Source: source, Status: f.prober, Now: func() time.Time { return *f.now }})
	return f
}

func TestHomeStatusResponseMapping(t *testing.T) {
	tests := []struct {
		name string
		res  port.HomeProbeResult
		err  error
		want dto.HomeStatusResponse
	}{
		{"200", port.HomeProbeResult{StatusCode: 200, Latency: 12 * time.Millisecond},
			nil, dto.HomeStatusResponse{State: dto.HomeServiceStateUp, Code: 200, LatencyMS: 12}},
		{"302", port.HomeProbeResult{StatusCode: 302}, nil, dto.HomeStatusResponse{State: dto.HomeServiceStateUp, Code: 302}},
		{"401", port.HomeProbeResult{StatusCode: 401}, nil, dto.HomeStatusResponse{State: dto.HomeServiceStateUp, Code: 401}},
		{"403", port.HomeProbeResult{StatusCode: 403}, nil, dto.HomeStatusResponse{State: dto.HomeServiceStateUp, Code: 403}},
		{"503", port.HomeProbeResult{StatusCode: 503}, nil, dto.HomeStatusResponse{State: dto.HomeServiceStateError, Code: 503}},
		{"tls", port.HomeProbeResult{TLSUnverified: true, Latency: 3 * time.Millisecond},
			nil, dto.HomeStatusResponse{State: dto.HomeServiceStateUp, TLSUnverified: true, LatencyMS: 3}},
		{"failed", port.HomeProbeResult{}, errors.New("refused"), dto.HomeStatusResponse{State: dto.HomeServiceStateDown}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.want.ServiceID = "s"
			assert.Equal(t, tt.want, homeStatusResponse("s", tt.res, tt.err))
		})
	}
}

func TestCheckHomeStatus_RejectsUnknownAndDisabled(t *testing.T) {
	f := newHomeStatusFixture(t)
	for _, id := range []string{"missing", "quiet", "nourl"} {
		_, err := f.uc.CheckHomeStatus(context.Background(), id)
		require.Error(t, err, id)
	}
}

func TestCheckHomeStatus_CachesFor30s(t *testing.T) {
	f := newHomeStatusFixture(t)
	f.prober.EXPECT().Probe(mock.Anything, "https://media.local", homeStatusTimeout).
		Return(port.HomeProbeResult{StatusCode: 200}, nil).Once()
	ctx := context.Background()

	resp, err := f.uc.CheckHomeStatus(ctx, "media")
	require.NoError(t, err)
	assert.Equal(t, dto.HomeServiceStateUp, resp.State)

	*f.now = f.now.Add(29 * time.Second)
	_, err = f.uc.CheckHomeStatus(ctx, "media")
	require.NoError(t, err)

	*f.now = f.now.Add(2 * time.Second)
	f.prober.EXPECT().Probe(mock.Anything, "https://media.local", homeStatusTimeout).
		Return(port.HomeProbeResult{}, errors.New("refused")).Once()
	resp, err = f.uc.CheckHomeStatus(ctx, "media")
	require.NoError(t, err)
	assert.Equal(t, dto.HomeServiceStateDown, resp.State)
}

func TestCheckHomeStatus_URLChangeBypassesCache(t *testing.T) {
	f := newHomeStatusFixture(t)
	f.prober.EXPECT().Probe(mock.Anything, "https://media.local", homeStatusTimeout).
		Return(port.HomeProbeResult{StatusCode: 200}, nil).Once()
	f.prober.EXPECT().Probe(mock.Anything, "https://media2.local", homeStatusTimeout).
		Return(port.HomeProbeResult{StatusCode: 503}, nil).Once()
	ctx := context.Background()
	_, err := f.uc.CheckHomeStatus(ctx, "media")
	require.NoError(t, err)
	f.layout.Groups[0].Services[0].URL = "https://media2.local"
	resp, err := f.uc.CheckHomeStatus(ctx, "media")
	require.NoError(t, err)
	assert.Equal(t, dto.HomeServiceStateError, resp.State)
}

func TestCheckHomeStatus_SharesInflightProbe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newHomeStatusFixture(t)
		release := make(chan struct{})
		var calls atomic.Int32
		f.prober.EXPECT().Probe(mock.Anything, "https://media.local", homeStatusTimeout).
			RunAndReturn(func(context.Context, string, time.Duration) (port.HomeProbeResult, error) {
				calls.Add(1)
				<-release
				return port.HomeProbeResult{StatusCode: 204}, nil
			}).Once()

		const n = 5
		var wg sync.WaitGroup
		results := make([]*dto.HomeStatusResponse, n)
		for i := range n {
			wg.Go(func() {
				resp, err := f.uc.CheckHomeStatus(context.Background(), "media")
				assert.NoError(t, err)
				results[i] = resp
			})
		}
		synctest.Wait()
		f.uc.statusState.mu.Lock()
		require.Len(t, f.uc.statusState.inflight, 1)
		f.uc.statusState.mu.Unlock()
		close(release)
		wg.Wait()
		assert.Equal(t, int32(1), calls.Load())
		for _, r := range results {
			require.NotNil(t, r)
			assert.Equal(t, 204, r.Code)
		}
	})
}

func TestCheckHomeStatus_CallerCancelDoesNotKillSharedProbe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newHomeStatusFixture(t)
		release := make(chan struct{})
		f.prober.EXPECT().Probe(mock.Anything, "https://media.local", homeStatusTimeout).
			RunAndReturn(func(ctx context.Context, _ string, _ time.Duration) (port.HomeProbeResult, error) {
				<-release
				return port.HomeProbeResult{StatusCode: 200}, ctx.Err()
			}).Once()

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error)
		go func() {
			_, err := f.uc.CheckHomeStatus(ctx, "media")
			done <- err
		}()
		synctest.Wait()
		f.uc.statusState.mu.Lock()
		require.Len(t, f.uc.statusState.inflight, 1)
		f.uc.statusState.mu.Unlock()
		cancel()
		require.ErrorIs(t, <-done, context.Canceled)
		close(release)

		resp, err := f.uc.CheckHomeStatus(context.Background(), "media")
		require.NoError(t, err)
		assert.Equal(t, dto.HomeServiceStateUp, resp.State)
	})
}

func TestCheckHomeStatus_PanicReleasesWaiters(t *testing.T) {
	f := newHomeStatusFixture(t)
	f.prober.EXPECT().Probe(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(context.Context, string, time.Duration) (port.HomeProbeResult, error) {
			panic("boom")
		}).Once()
	resp, err := f.uc.CheckHomeStatus(context.Background(), "media")
	require.NoError(t, err)
	assert.Equal(t, dto.HomeServiceStateDown, resp.State)
}

func TestCheckHomeStatus_PrunesRemovedServices(t *testing.T) {
	f := newHomeStatusFixture(t)
	f.prober.EXPECT().Probe(mock.Anything, mock.Anything, mock.Anything).
		Return(port.HomeProbeResult{StatusCode: 200}, nil).Once()
	_, err := f.uc.CheckHomeStatus(context.Background(), "media")
	require.NoError(t, err)

	f.layout.Groups[0].Services = f.layout.Groups[0].Services[1:]
	f.layout.Groups[0].Services = append(f.layout.Groups[0].Services,
		entity.HomeService{ID: "other", Name: "Other", URL: "https://other.local"})
	// A changed content hash triggers the prune scan.
	src := mocks.NewMockHomeConfigSource(t)
	src.EXPECT().Load(mock.Anything).Return(port.HomeConfigSnapshot{Layout: *f.layout, ContentHash: [32]byte{1}}, nil)
	f.uc.source = src
	f.prober.EXPECT().Probe(mock.Anything, "https://other.local", mock.Anything).
		Return(port.HomeProbeResult{StatusCode: 200}, nil).Once()
	_, err = f.uc.CheckHomeStatus(context.Background(), "other")
	require.NoError(t, err)

	f.uc.statusState.mu.Lock()
	defer f.uc.statusState.mu.Unlock()
	assert.NotContains(t, f.uc.statusState.cache, "media")
}

func TestCheckHomeStatus_URLChangeDoesNotJoinOldFlight(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newHomeStatusFixture(t)
		release := make(chan struct{})
		f.prober.EXPECT().Probe(mock.Anything, "https://media.local", homeStatusTimeout).
			RunAndReturn(func(context.Context, string, time.Duration) (port.HomeProbeResult, error) {
				<-release
				return port.HomeProbeResult{StatusCode: 200}, nil
			}).Once()
		f.prober.EXPECT().Probe(mock.Anything, "https://media2.local", homeStatusTimeout).
			Return(port.HomeProbeResult{StatusCode: 503}, nil).Once()

		oldDone := make(chan *dto.HomeStatusResponse)
		go func() {
			resp, _ := f.uc.CheckHomeStatus(context.Background(), "media")
			oldDone <- resp
		}()
		synctest.Wait()
		f.uc.statusState.mu.Lock()
		require.Len(t, f.uc.statusState.inflight, 1)
		f.uc.statusState.mu.Unlock()

		f.uc.statusState.mu.Lock()
		f.layout.Groups[0].Services[0].URL = "https://media2.local"
		f.uc.statusState.mu.Unlock()
		resp, err := f.uc.CheckHomeStatus(context.Background(), "media")
		require.NoError(t, err)
		assert.Equal(t, 503, resp.Code)

		close(release)
		assert.Equal(t, 200, (<-oldDone).Code)
	})
}

func TestCheckHomeStatus_NoProberConfigured(t *testing.T) {
	source := mocks.NewMockHomeConfigSource(t)
	_, err := NewHomeUseCase(HomeDeps{Source: source}).CheckHomeStatus(context.Background(), "media")
	require.EqualError(t, err, "home status not configured")
}
