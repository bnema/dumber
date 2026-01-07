package port

import "context"

// DownloadEventType represents the type of download event.
type DownloadEventType int

const (
	// DownloadEventStarted indicates a download has begun.
	DownloadEventStarted DownloadEventType = iota
	// DownloadEventFinished indicates a download completed successfully.
	DownloadEventFinished
	// DownloadEventFailed indicates a download failed.
	DownloadEventFailed
)

// DownloadEvent contains information about a download event.
type DownloadEvent struct {
	Type        DownloadEventType
	Filename    string
	Destination string
	Error       error // Set when Type is DownloadEventFailed
}

// DownloadEventHandler receives download event notifications.
type DownloadEventHandler interface {
	OnDownloadEvent(ctx context.Context, event DownloadEvent)
}
