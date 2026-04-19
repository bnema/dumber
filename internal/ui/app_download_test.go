package ui

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/ui/component"
)

func TestDownloadEventAdapterToastSpecLifecycle(t *testing.T) {
	adapter := newDownloadEventAdapter(nil)
	event := port.DownloadEvent{
		Filename:    "archlinux.iso",
		Destination: "/tmp/archlinux.iso",
	}

	started := adapter.toastSpecForEvent(port.DownloadEvent{
		Type:        port.DownloadEventStarted,
		Filename:    event.Filename,
		Destination: event.Destination,
	})
	require.True(t, started.show)
	require.Equal(t, component.ToastInfo, started.level)
	require.Equal(t, 0, started.duration)
	require.Equal(t, "Download started: archlinux.iso (0%)", started.message)

	progress := adapter.toastSpecForEvent(port.DownloadEvent{
		Type:        port.DownloadEventProgress,
		Filename:    event.Filename,
		Destination: event.Destination,
		Progress:    0.37,
	})
	require.True(t, progress.show)
	require.Equal(t, component.ToastInfo, progress.level)
	require.Equal(t, 0, progress.duration)
	require.Equal(t, "Downloading: archlinux.iso (37%)", progress.message)

	finished := adapter.toastSpecForEvent(port.DownloadEvent{
		Type:        port.DownloadEventFinished,
		Filename:    event.Filename,
		Destination: event.Destination,
	})
	require.True(t, finished.show)
	require.Equal(t, component.ToastSuccess, finished.level)
	require.Equal(t, downloadToastTerminalDuration, finished.duration)
	require.Equal(t, "Download complete: archlinux.iso", finished.message)
}

func TestDownloadEventAdapterToastSpecSuppressesDuplicateProgress(t *testing.T) {
	adapter := newDownloadEventAdapter(nil)
	event := port.DownloadEvent{
		Filename:    "ubuntu.iso",
		Destination: "/tmp/ubuntu.iso",
	}

	adapter.toastSpecForEvent(port.DownloadEvent{
		Type:        port.DownloadEventStarted,
		Filename:    event.Filename,
		Destination: event.Destination,
	})

	first := adapter.toastSpecForEvent(port.DownloadEvent{
		Type:        port.DownloadEventProgress,
		Filename:    event.Filename,
		Destination: event.Destination,
		Progress:    0.12,
	})
	require.True(t, first.show)

	duplicate := adapter.toastSpecForEvent(port.DownloadEvent{
		Type:        port.DownloadEventProgress,
		Filename:    event.Filename,
		Destination: event.Destination,
		Progress:    0.12,
	})
	require.False(t, duplicate.show)
}

func TestDownloadProgressPercentRoundsReasonably(t *testing.T) {
	require.Equal(t, 0, downloadProgressPercent(0))
	require.Equal(t, 1, downloadProgressPercent(0.0051))
	require.Equal(t, 37, downloadProgressPercent(0.37))
	require.Equal(t, 100, downloadProgressPercent(1.2))
}
