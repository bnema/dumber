package cef

import (
	"context"
	"fmt"
	"math"
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZoomConversionsRoundTrip(t *testing.T) {
	factors := []float64{0.5, 0.8, 1.0, 1.1, 1.2, 1.4, 2.0}
	for _, factor := range factors {
		t.Run(fmt.Sprintf("factor_%.1f", factor), func(t *testing.T) {
			level := cefZoomFromFactor(factor)
			got := factorFromCEFZoom(level)
			assert.InDelta(t, factor, got, 1e-9, "round-trip mismatch for factor %.6f: level %.6f -> factor %.12f", factor, level, got)
		})
	}
}

func TestZoomConversionsApplyUserZoomAndCompensation(t *testing.T) {
	tests := []struct {
		name         string
		userZoom     float64
		compensation float64
		wantInternal float64
	}{
		{
			name:         "unit_output_needs_no_compensation",
			userZoom:     1.0,
			compensation: 1.0,
			wantInternal: 1.0,
		},
		{
			name:         "user_zoom_on_unit_output",
			userZoom:     1.3,
			compensation: 1.0,
			wantInternal: 1.3,
		},
		{
			name:         "fractional_output_compensates_100_percent",
			userZoom:     1.0,
			compensation: 1.25,
			wantInternal: 1.25,
		},
		{
			name:         "user_zoom_times_fractional_output",
			userZoom:     1.3,
			compensation: 1.25,
			wantInternal: 1.625,
		},
		{
			name:         "application_scale_composed_compensation",
			userZoom:     1.0,
			compensation: 1.75,
			wantInternal: 1.75,
		},
		{
			name:         "integer_output_doubles_internal_factor",
			userZoom:     2.0,
			compensation: 2.0,
			wantInternal: 4.0,
		},
		{
			name:         "half_user_zoom_stays_compensated",
			userZoom:     0.5,
			compensation: 1.25,
			wantInternal: 0.625,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level := cefZoomFromPageZoom(tt.userZoom, tt.compensation)
			assert.InDelta(t, tt.wantInternal, factorFromCEFZoom(level), 1e-9, "internal CEF factor")
			assert.InDelta(t, tt.userZoom, pageZoomFromCEFLevel(level, tt.compensation), 1e-9, "inverse must return user zoom")
		})
	}
}

func TestZoomConversionsNormalizeInvalidCompensation(t *testing.T) {
	for _, compensation := range []float64{0, -1.5, math.NaN(), math.Inf(1)} {
		level := cefZoomFromPageZoom(1.3, compensation)
		assert.InDelta(t, 1.3, factorFromCEFZoom(level), 1e-9, "invalid compensation %v must behave as 1", compensation)
		assert.InDelta(t, 1.3, pageZoomFromCEFLevel(level, compensation), 1e-9)
	}
}

func TestZoomReapplyGuardTracksAppliedCompensation(t *testing.T) {
	wv := &WebView{}

	assert.True(t, wv.shouldReapplyZoomForCompensation(1.25), "unknown applied compensation must reapply")
	assert.Zero(t, wv.appliedZoomCompensation())

	wv.recordAppliedZoomCompensation(1.25)

	assert.False(t, wv.shouldReapplyZoomForCompensation(1.25), "unchanged compensation must not reapply")
	assert.True(t, wv.shouldReapplyZoomForCompensation(2.0), "compensation change 1.25 -> 2 must reapply")
	assert.InDelta(t, 1.25, wv.appliedZoomCompensation(), 1e-9)
}

func TestPageZoomCompensationDefaultsToOneWithoutBridge(t *testing.T) {
	wv := &WebView{}
	assert.InDelta(t, 1.0, wv.pageZoomCompensation(), 1e-9)

	wv.zoomCompensation = zoomCompensationStub{compensation: 1.75}
	assert.InDelta(t, 1.75, wv.pageZoomCompensation(), 1e-9)

	wv.zoomCompensation = zoomCompensationStub{compensation: math.NaN()}
	assert.InDelta(t, 1.0, wv.pageZoomCompensation(), 1e-9)
}

// withSynchronousCEFTasks replaces delayed task posting with a recorder so tests
// can inspect and run zoom tasks without a CEF message loop.
func withSynchronousCEFTasks(t *testing.T) *[]purecef.Task {
	t.Helper()
	oldNewTask := cefNewTask
	oldPostDelayedTask := cefPostDelayedTask
	t.Cleanup(func() {
		cefNewTask = oldNewTask
		cefPostDelayedTask = oldPostDelayedTask
	})

	cefNewTask = func(task purecef.Task) purecef.Task { return task }
	scheduled := &[]purecef.Task{}
	cefPostDelayedTask = func(threadID purecef.ThreadID, task purecef.Task, _ int64) int32 {
		require.Equal(t, purecef.ThreadIDTidUi, threadID)
		require.NotNil(t, task)
		*scheduled = append(*scheduled, task)
		return 1
	}
	return scheduled
}

func TestSetZoomLevelAppliesUserZoomThroughCompensation(t *testing.T) {
	scheduled := withSynchronousCEFTasks(t)

	host := &viewportSyncOrderHost{}
	wv := &WebView{
		ctx:              context.Background(),
		host:             host,
		zoomCompensation: zoomCompensationStub{compensation: 1.25},
	}

	require.NoError(t, wv.SetZoomLevel(context.Background(), 1.3))

	assert.InDelta(t, cefZoomFromFactor(1.625), host.zoomLevel, 1e-9, "internal CEF level must include user zoom x compensation")
	assert.InDelta(t, 1.3, wv.GetZoomLevel(), 1e-9, "user zoom must stay user-facing")
	assert.InDelta(t, 1.25, wv.appliedZoomCompensation(), 1e-9)
	assert.False(t, wv.shouldReapplyZoomForCompensation(1.25))
	assert.NotEmpty(t, *scheduled)

	// The diagnostic readback decodes the same CEF level back to user zoom with
	// the compensation of its own application.
	for _, task := range *scheduled {
		task.Execute()
	}
	assert.InDelta(t, 1.3, pageZoomFromCEFLevel(host.zoomLevel, 1.25), 1e-9)
}

func TestSetZoomLevelRejectsDestroyedViewAndMissingHost(t *testing.T) {
	destroyed := &WebView{ctx: context.Background()}
	destroyed.destroyed.Store(true)
	require.ErrorIs(t, destroyed.SetZoomLevel(context.Background(), 1.2), errDestroyed)

	require.ErrorIs(t, (&WebView{ctx: context.Background()}).SetZoomLevel(context.Background(), 1.2), errNoBrowser)
}

func TestScheduleZoomReadbackSkipsSupersededApplication(t *testing.T) {
	scheduled := withSynchronousCEFTasks(t)
	host := &viewportSyncOrderHost{}
	wv := &WebView{ctx: context.Background(), host: host}

	wv.recordAppliedZoomCompensation(1.25)
	wv.scheduleZoomReadback(1.3, cefZoomFromPageZoom(1.3, 1.25), 1.25)
	require.Len(t, *scheduled, 2)
	stale := (*scheduled)[0]

	// A newer application supersedes the pending readback: decoding the newer CEF
	// level with the older compensation would report a wrong user zoom.
	wv.recordAppliedZoomCompensation(2.0)
	stale.Execute()
	assert.Zero(t, host.getZoomLevelCalls, "superseded application must not be diagnosed")

	wv.scheduleZoomReadback(1.3, cefZoomFromPageZoom(1.3, 2.0), 2.0)
	require.Len(t, *scheduled, 4)
	(*scheduled)[2].Execute()
	assert.Equal(t, 1, host.getZoomLevelCalls, "current application must be diagnosed")
}
