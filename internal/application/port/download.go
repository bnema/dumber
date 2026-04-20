package port

import "context"

// DownloadEventType represents the type of download event.
type DownloadEventType int

const (
	// DownloadEventStarted indicates a download has begun.
	DownloadEventStarted DownloadEventType = iota
	// DownloadEventProgress indicates download progress has advanced.
	DownloadEventProgress
	// DownloadEventFinished indicates a download completed successfully.
	DownloadEventFinished
	// DownloadEventFailed indicates a download failed.
	DownloadEventFailed
)

// DownloadEvent contains information about a download event.
type DownloadEvent struct {
	Type          DownloadEventType
	Filename      string
	Destination   string
	Progress      float64 // Set when Type is DownloadEventProgress, normalized to 0.0-1.0.
	BytesReceived int64   // Best-effort received byte count for progress updates.
	BytesTotal    int64   // Best-effort total byte count for progress updates when known.
	Error         error   // Set when Type is DownloadEventFailed
}

// DownloadEventHandler receives download event notifications.
type DownloadEventHandler interface {
	OnDownloadEvent(ctx context.Context, event DownloadEvent)
}

// DownloadResponse provides access to response metadata for downloads.
// This abstracts the WebKit URIResponse to allow testing without CGO.
type DownloadResponse interface {
	GetMimeType() string
	GetSuggestedFilename() string
	GetUri() string
}
