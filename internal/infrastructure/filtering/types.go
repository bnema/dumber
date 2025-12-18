package filtering

import "time"

// FilterState represents the current state of the content filter system.
type FilterState string

const (
	// StateUninitialized means the filter manager has not been initialized yet.
	StateUninitialized FilterState = "uninitialized"
	// StateLoading means filters are being downloaded or compiled.
	StateLoading FilterState = "loading"
	// StateActive means filters are loaded and active.
	StateActive FilterState = "active"
	// StateDisabled means filtering is disabled by configuration.
	StateDisabled FilterState = "disabled"
	// StateError means an error occurred during filter loading.
	StateError FilterState = "error"
)

// FilterStatus represents the current status of the content filter system.
// Used for UI toast notifications and status reporting.
type FilterStatus struct {
	State   FilterState
	Message string
	Version string
}

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

// FilterFiles defines the files to download from GitHub releases.
var FilterFiles = struct {
	Manifest string
	Combined []string
}{
	Manifest: "manifest.json",
	Combined: []string{
		"combined-part1.json",
		"combined-part2.json",
		"combined-part3.json",
	},
}

// FilterIdentifier is the identifier used when storing/loading compiled filters.
const FilterIdentifier = "ublock-combined"

// GitHubReleaseURL is the base URL for downloading filter files.
const GitHubReleaseURL = "https://github.com/bnema/ublock-webkit-filters/releases/latest/download"
