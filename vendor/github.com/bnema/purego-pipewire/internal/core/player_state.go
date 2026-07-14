package core

// PlayerState represents the current state of the player
type PlayerState int32

const (
	PlayerStateIdle PlayerState = iota
	PlayerStateStarting
	PlayerStatePlaying
	PlayerStatePaused
	PlayerStateStopped
	PlayerStateClosing
	PlayerStateClosed
	PlayerStateError
)
