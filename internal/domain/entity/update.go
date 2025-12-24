// Package entity defines domain entities for dumber.
package entity

import "time"

// UpdateInfo holds information about an available update.
type UpdateInfo struct {
	// CurrentVersion is the version of the running binary.
	CurrentVersion string
	// LatestVersion is the latest available version from GitHub.
	LatestVersion string
	// IsNewer is true if LatestVersion is newer than CurrentVersion.
	IsNewer bool
	// ReleaseURL is the URL to the GitHub release page.
	ReleaseURL string
	// DownloadURL is the direct download URL for the binary archive.
	DownloadURL string
	// PublishedAt is when the release was published.
	PublishedAt time.Time
	// ReleaseNotes contains the release changelog (optional).
	ReleaseNotes string
}

// UpdateStatus represents the current state of the update process.
type UpdateStatus int

const (
	// UpdateStatusUnknown means update status hasn't been checked yet.
	UpdateStatusUnknown UpdateStatus = iota
	// UpdateStatusUpToDate means the current version is the latest.
	UpdateStatusUpToDate
	// UpdateStatusAvailable means a newer version is available.
	UpdateStatusAvailable
	// UpdateStatusDownloading means the update is being downloaded.
	UpdateStatusDownloading
	// UpdateStatusReady means the update is downloaded and staged for exit.
	UpdateStatusReady
	// UpdateStatusFailed means the update check or download failed.
	UpdateStatusFailed
)

// String returns a human-readable string for the update status.
func (s UpdateStatus) String() string {
	switch s {
	case UpdateStatusUnknown:
		return "unknown"
	case UpdateStatusUpToDate:
		return "up-to-date"
	case UpdateStatusAvailable:
		return "available"
	case UpdateStatusDownloading:
		return "downloading"
	case UpdateStatusReady:
		return "ready"
	case UpdateStatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}
