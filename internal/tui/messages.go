package tui

// Async safe message helpers for Bubble Tea programs.

// ErrorMsg wraps an error for unified handling.
type ErrorMsg struct {
	Err error
}

// InfoMsg is a lightweight status update.
type InfoMsg string

// DoneMsg signals completion of a long-running action.
type DoneMsg struct{}
