package cef

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bnema/dumber/internal/logging"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestStartupPresentationHooksWireEveryUpstreamFirstPresentationCallback(t *testing.T) {
	var output bytes.Buffer
	trace := logging.NewCEFStartupTrace()
	trace.SetBackend("gdk-dmabuf")
	logger := zerolog.New(&output)
	trace.SetLogger(&logger)
	for _, milestone := range []string{
		"process_entry",
		"config_complete",
		"cef_library_load_begin",
		"cef_initialized",
		"browser_create_requested",
	} {
		require.Truef(t, trace.Mark(milestone), "seed milestone %q", milestone)
	}

	hooks := startupPresentationHooks(trace)
	require.NotNil(t, hooks.OnFirstAcceleratedPaint)
	require.NotNil(t, hooks.OnFirstDMABUFTextureSwap)
	require.NotNil(t, hooks.OnFirstPresentation)
	require.NotNil(t, hooks.OnDMABUFUnsupported)

	// The unsupported fact may arrive before the successful presentation path.
	// Each callback is invoked twice to make no-op, swapped, and duplicate bodies
	// observable through the exact trace output below.
	hooks.OnDMABUFUnsupported()
	hooks.OnDMABUFUnsupported()
	hooks.OnFirstAcceleratedPaint()
	hooks.OnFirstAcceleratedPaint()
	hooks.OnFirstDMABUFTextureSwap()
	hooks.OnFirstDMABUFTextureSwap()
	hooks.OnFirstPresentation()
	hooks.OnFirstPresentation()

	var milestones []string
	summaryCount := 0
	var incompleteReason string
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var event struct {
			Message          string `json:"message"`
			Milestone        string `json:"milestone"`
			IncompleteReason string `json:"incomplete_reason"`
		}
		require.NoError(t, json.Unmarshal([]byte(line), &event))
		switch event.Message {
		case "startup_trace: milestone":
			milestones = append(milestones, event.Milestone)
		case "startup_trace: first presentation":
			summaryCount++
			incompleteReason = event.IncompleteReason
		}
	}

	require.Equal(t, []string{
		"process_entry",
		"config_complete",
		"cef_library_load_begin",
		"cef_initialized",
		"browser_create_requested",
		"first_accelerated_paint_received",
		"first_dmabuf_texture_swap",
		"first_gtk_presentation",
	}, milestones)
	require.Equal(t, "dmabuf_texture_swap_unavailable", incompleteReason)
	require.Equal(t, 1, summaryCount)
}
