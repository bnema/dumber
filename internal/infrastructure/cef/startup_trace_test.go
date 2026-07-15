package cef

import (
	"bytes"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestActivateStartupTraceSeedsNeutralProcessAndConfigTiming(t *testing.T) {
	processStartupTrace.Lock()
	previous := processStartupTrace.trace
	processStartupTrace.trace = nil
	processStartupTrace.Unlock()
	t.Cleanup(func() {
		processStartupTrace.Lock()
		processStartupTrace.trace = previous
		processStartupTrace.Unlock()
	})

	processEntry := time.Unix(100, 0)
	configComplete := processEntry.Add(15 * time.Millisecond)
	var output bytes.Buffer
	logger := zerolog.New(&output).Level(zerolog.DebugLevel)

	ActivateStartupTrace(processEntry, configComplete, &logger)
	trace := activeStartupTrace()

	require.NotNil(t, trace)
	require.Equal(t, []string{"process_entry", "config_complete"}, []string{trace.milestones[0].Name, trace.milestones[1].Name})
	require.Equal(t, int64(0), trace.milestones[0].Elapsed.Milliseconds())
	require.Equal(t, int64(15), trace.milestones[1].Elapsed.Milliseconds())
	require.Contains(t, output.String(), `"milestone":"process_entry"`)
	require.Contains(t, output.String(), `"milestone":"config_complete"`)
	require.NotContains(t, output.String(), `"message":"startup_trace: first presentation"`)
}

func TestStartupTraceAcceptsOnlyOrderedOneShotMilestones(t *testing.T) {
	now := time.Unix(100, 0)
	trace := newStartupTrace(func() time.Time { return now }, now)

	require.True(t, trace.Mark("process_entry"))
	require.False(t, trace.Mark("cef_initialized"), "out-of-order transition must be rejected")
	require.False(t, trace.Mark("process_entry"), "duplicate transition must be rejected")

	for _, name := range []string{
		"config_complete",
		"cef_library_load_begin",
		"cef_initialized",
		"browser_create_requested",
		"first_accelerated_paint_received",
		"first_dmabuf_texture_swap",
		"first_gtk_presentation",
	} {
		now = now.Add(time.Millisecond)
		if name == "first_gtk_presentation" {
			require.Truef(t, trace.MarkGTKAfterPaint(), "milestone %s should be accepted", name)
		} else {
			require.Truef(t, trace.Mark(name), "milestone %s should be accepted", name)
		}
	}
	require.False(t, trace.Mark("first_gtk_presentation"))
}

func TestStartupTracePreservesProcessAndConfigTimestamps(t *testing.T) {
	processEntry := time.Unix(100, 0)
	configComplete := processEntry.Add(1500 * time.Microsecond)
	now := configComplete.Add(900 * time.Microsecond)
	trace := newStartupTrace(func() time.Time { return now }, processEntry)

	require.True(t, trace.markAt("process_entry", processEntry))
	require.True(t, trace.markAt("config_complete", configComplete))

	require.Equal(t, int64(0), trace.milestones[0].Elapsed.Milliseconds())
	require.Equal(t, int64(0), trace.milestones[0].Delta.Milliseconds())
	require.Equal(t, int64(1), trace.milestones[1].Elapsed.Milliseconds())
	require.Equal(t, int64(1), trace.milestones[1].Delta.Milliseconds())
}

func TestStartupTraceReservesFirstGTKPresentationForAfterPaint(t *testing.T) {
	now := time.Unix(100, 0)
	trace := newStartupTrace(func() time.Time { return now }, now)

	for _, name := range startupMilestoneOrder[:len(startupMilestoneOrder)-1] {
		now = now.Add(time.Millisecond)
		require.True(t, trace.Mark(name))
	}

	require.Len(t, trace.milestones, len(startupMilestoneOrder)-1)
	require.False(t, trace.Mark("first_gtk_presentation"), "only the CEF-to-GTK after-paint hook can record the reserved milestone")
	require.False(t, trace.summaryEmitted)
	require.True(t, trace.MarkGTKAfterPaint(), "only the after-paint hook may record this milestone")
}

func TestStartupTraceEmitsOneNormalSummaryAtFirstPresentation(t *testing.T) {
	var output bytes.Buffer
	logger := zerolog.New(&output)
	now := time.Unix(100, 0)
	trace := newStartupTrace(func() time.Time { return now }, now)
	trace.SetBackend("gdk-dmabuf")
	trace.SetLogger(&logger)

	for _, name := range startupMilestoneOrder[:len(startupMilestoneOrder)-1] {
		now = now.Add(time.Millisecond)
		require.True(t, trace.Mark(name))
	}
	now = now.Add(time.Millisecond)
	require.True(t, trace.MarkGTKAfterPaint())
	trace.MarkGTKAfterPaint()

	require.Contains(t, output.String(), `"message":"startup_trace: first presentation"`)
	require.Contains(t, output.String(), `"backend":"gdk-dmabuf"`)
	require.Equal(t, 1, bytes.Count(output.Bytes(), []byte(`"message":"startup_trace: first presentation"`)))
}
