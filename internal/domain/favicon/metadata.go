package favicon

import "time"

type Source string

const (
	SourceEngine        Source = "engine"
	SourcePageDiscovery Source = "page_discovery"
	SourceDuckDuckGo    Source = "duckduckgo"
	SourceInternal      Source = "internal"
	SourceManual        Source = "manual"
)

type Metadata struct {
	Key           Key
	SourceURL     string
	PageURL       string
	Source        Source
	ContentHash   string
	ContentType   string
	UpdatedAt     time.Time
	LastCheckedAt time.Time
}
