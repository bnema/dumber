package bootstrap

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/runtimeprofile"
	"github.com/stretchr/testify/require"
)

func TestNewSessionSpawnerRejectsDetachedRestoreForEphemeralProfile(t *testing.T) {
	spawner := NewSessionSpawner(context.Background(), runtimeprofile.Profile{
		Engine:              "cef",
		InstanceAppIDSuffix: "ephemeral-test",
	})

	err := spawner.SpawnWithSession(entity.SessionID("saved"))
	require.ErrorContains(t, err, "unavailable with --ephemeral")
	require.ErrorContains(t, err, "use --profile")
}
