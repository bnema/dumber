package entity_test

import (
	"testing"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSession_ShortID(t *testing.T) {
	s := &entity.Session{ID: entity.SessionID("20251217_205106_a7b3")}
	assert.Equal(t, "a7b3", s.ShortID())

	s2 := &entity.Session{ID: entity.SessionID("abc")}
	assert.Equal(t, "abc", s2.ShortID())
}

func TestSession_Validate(t *testing.T) {
	now := time.Now()

	valid := &entity.Session{ID: "20251217_205106_a7b3", Type: entity.SessionTypeBrowser, StartedAt: now}
	require.NoError(t, valid.Validate())

	missingID := &entity.Session{Type: entity.SessionTypeBrowser, StartedAt: now}
	require.ErrorIs(t, missingID.Validate(), entity.ErrInvalidSession)

	badType := &entity.Session{ID: "20251217_205106_a7b3", Type: "nope", StartedAt: now}
	require.ErrorIs(t, badType.Validate(), entity.ErrInvalidSession)

	missingStarted := &entity.Session{ID: "20251217_205106_a7b3", Type: entity.SessionTypeBrowser}
	require.ErrorIs(t, missingStarted.Validate(), entity.ErrInvalidSession)
}

func TestSession_End(t *testing.T) {
	s := &entity.Session{ID: "x", Type: entity.SessionTypeBrowser, StartedAt: time.Now()}
	assert.True(t, s.IsActive())

	endedAt := time.Date(2025, 12, 22, 0, 0, 0, 0, time.FixedZone("X", 3600))
	s.End(endedAt)
	assert.False(t, s.IsActive())
	if assert.NotNil(t, s.EndedAt) {
		assert.True(t, s.EndedAt.Equal(endedAt.UTC()))
	}
}
