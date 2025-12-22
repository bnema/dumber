package entity

// PurgeTargetType identifies what kind of purgeable item this is.
type PurgeTargetType int

const (
	PurgeTargetConfig PurgeTargetType = iota
	PurgeTargetData
	PurgeTargetState
	PurgeTargetCache
	PurgeTargetFilterJSON
	PurgeTargetFilterStore
	PurgeTargetFilterCache
	PurgeTargetDesktopFile
	PurgeTargetIcon
)

// PurgeTarget represents something that can be purged.
type PurgeTarget struct {
	Type        PurgeTargetType
	Path        string
	Description string
	Size        int64
	Exists      bool
}

// PurgeResult represents the outcome of purging a single target.
type PurgeResult struct {
	Target  PurgeTarget
	Success bool
	Error   error
}
