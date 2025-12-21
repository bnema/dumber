package entity

import (
	"errors"
	"time"
)

// SessionID uniquely identifies an application session.
// For now it matches the log session ID format (YYYYMMDD_HHMMSS_xxxx).
type SessionID string

// SessionType distinguishes long-running browser sessions from ephemeral CLI invocations.
type SessionType string

const (
	SessionTypeBrowser SessionType = "browser"
	SessionTypeCLI     SessionType = "cli"
)

// Session captures metadata about a dumber run.
// A browser session is expected to have a corresponding log file.
type Session struct {
	ID        SessionID
	Type      SessionType
	StartedAt time.Time
	EndedAt   *time.Time
}

func (s *Session) ShortID() string {
	id := string(s.ID)
	if len(id) < 4 {
		return id
	}
	return id[len(id)-4:]
}

func (s *Session) IsActive() bool {
	return s != nil && s.EndedAt == nil
}

func (s *Session) End(endedAt time.Time) {
	endedAt = endedAt.UTC()
	s.EndedAt = &endedAt
}

func (s *Session) Validate() error {
	if s == nil {
		return ErrInvalidSession
	}
	if s.ID == "" {
		return ErrInvalidSession
	}
	if s.Type != SessionTypeBrowser && s.Type != SessionTypeCLI {
		return ErrInvalidSession
	}
	if s.StartedAt.IsZero() {
		return ErrInvalidSession
	}
	return nil
}

var ErrInvalidSession = errors.New("invalid session")
