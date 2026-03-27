package filtering

import (
	"time"

	"github.com/bnema/dumber/internal/application/port"
)

// FilterState is an alias for port.FilterState to avoid breaking existing references.
type FilterState = port.FilterState

// FilterStatus is an alias for port.FilterStatus so filtering.Manager satisfies port.FilterManager.
type FilterStatus = port.FilterStatus

const (
	// StateUninitialized means the filter manager has not been initialized yet.
	StateUninitialized FilterState = port.FilterStateUninitialized
	// StateLoading means filters are being downloaded or compiled.
	StateLoading FilterState = port.FilterStateLoading
	// StateActive means filters are loaded and active.
	StateActive FilterState = port.FilterStateActive
	// StateDisabled means filtering is disabled by configuration.
	StateDisabled FilterState = port.FilterStateDisabled
	// StateError means an error occurred during filter loading.
	StateError FilterState = port.FilterStateError
)

// Manifest represents the manifest.json from ublock-webkit-filters releases.
// It contains metadata about the filter lists and their rule counts.
type Manifest struct {
	Version     string              `json:"version"`
	GeneratedAt time.Time           `json:"generated_at"`
	Lists       map[string]ListInfo `json:"lists"`
	Combined    CombinedInfo        `json:"combined"`
}

// ListInfo contains metadata for a single filter list.
type ListInfo struct {
	Name         string `json:"name"`
	SourceURL    string `json:"source_url"`
	RulesCount   int    `json:"rules_count"`
	SkippedCount int    `json:"skipped_count"`
}

// CombinedInfo contains metadata for the combined (merged) filter set.
type CombinedInfo struct {
	TotalRules int      `json:"total_rules"`
	Files      []string `json:"files"`
}

// FilterFiles defines the well-known file names for GitHub releases.
var FilterFiles = struct {
	Manifest string
}{
	Manifest: "manifest.json",
}

// FilterIdentifier is the identifier used when storing/loading compiled filters.
const FilterIdentifier = "ublock-combined"

// GitHubReleaseURL is the base URL for downloading filter files.
const GitHubReleaseURL = "https://github.com/bnema/ublock-webkit-filters/releases/latest/download"
