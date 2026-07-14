package logging

import (
	"bytes"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestStartupTraceAcceptsOnlyOrderedOneShotMilestones(t *testing.T) {
	now := time.Unix(100, 0)
	trace := newStartupTrace(func() time.Time { return now })

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
		require.Truef(t, trace.Mark(name), "milestone %s should be accepted", name)
	}
	require.False(t, trace.Mark("first_gtk_presentation"))
}

func TestStartupTraceEmitsOneNormalSummaryAtFirstPresentation(t *testing.T) {
	var output bytes.Buffer
	logger := zerolog.New(&output)
	now := time.Unix(100, 0)
	trace := newStartupTrace(func() time.Time { return now })
	trace.SetBackend("gdk-dmabuf")
	trace.SetLogger(&logger)

	for _, name := range startupMilestoneOrder {
		now = now.Add(time.Millisecond)
		require.True(t, trace.Mark(name))
	}
	trace.Mark("first_gtk_presentation")

	require.Contains(t, output.String(), `"message":"startup_trace: first presentation"`)
	require.Contains(t, output.String(), `"backend":"gdk-dmabuf"`)
	require.Equal(t, 1, bytes.Count(output.Bytes(), []byte(`"message":"startup_trace: first presentation"`)))
}
