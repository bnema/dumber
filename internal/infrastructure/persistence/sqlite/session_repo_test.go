package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite"
	"github.com/bnema/dumber/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testCtx() context.Context {
	logger := logging.NewFromConfigValues("debug", "console")
	return logging.WithContext(context.Background(), logger)
}

func TestSessionRepository_CRUD_NilProcessID(t *testing.T) {
	ctx := testCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewSessionRepository(db)

	startedAt := time.Date(2025, 12, 22, 7, 0, 0, 0, time.UTC)
	s := &entity.Session{ID: "20251222_070000_nilp", Type: entity.SessionTypeBrowser, StartedAt: startedAt}
	require.NoError(t, repo.Save(ctx, s))

	got, err := repo.FindByID(ctx, s.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Nil(t, got.ProcessID)

	active, err := repo.GetActive(ctx)
	require.NoError(t, err)
	require.NotNil(t, active)
	assert.Equal(t, s.ID, active.ID)
	assert.Nil(t, active.ProcessID)
}

func TestSessionRepository_CRUD(t *testing.T) {
	ctx := testCtx()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")

	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewSessionRepository(db)

	startedAt := time.Date(2025, 12, 22, 8, 0, 0, 0, time.UTC)
	pid := 4321
	s := &entity.Session{ID: "20251222_080000_abcd", Type: entity.SessionTypeBrowser, StartedAt: startedAt, ProcessID: &pid}
	require.NoError(t, repo.Save(ctx, s))

	got, err := repo.FindByID(ctx, s.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, s.ID, got.ID)
	assert.Equal(t, s.Type, got.Type)
	assert.True(t, got.StartedAt.Equal(startedAt))
	require.NotNil(t, got.ProcessID)
	assert.Equal(t, pid, *got.ProcessID)
	assert.Nil(t, got.EndedAt)

	active, err := repo.GetActive(ctx)
	require.NoError(t, err)
	require.NotNil(t, active)
	assert.Equal(t, s.ID, active.ID)
	require.NotNil(t, active.ProcessID)
	assert.Equal(t, pid, *active.ProcessID)

	endedAt := time.Date(2025, 12, 22, 9, 0, 0, 0, time.UTC)
	require.NoError(t, repo.MarkEnded(ctx, s.ID, endedAt))

	active2, err := repo.GetActive(ctx)
	require.NoError(t, err)
	assert.Nil(t, active2)

	got2, err := repo.FindByID(ctx, s.ID)
	require.NoError(t, err)
	require.NotNil(t, got2.EndedAt)
	assert.True(t, got2.EndedAt.Equal(endedAt))

	recent, err := repo.GetRecent(ctx, 10)
	require.NoError(t, err)
	require.Len(t, recent, 1)
	assert.Equal(t, s.ID, recent[0].ID)
	require.NotNil(t, recent[0].ProcessID)
	assert.Equal(t, pid, *recent[0].ProcessID)
}
