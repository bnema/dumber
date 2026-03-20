package bootstrap

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/stretchr/testify/require"
)

func TestBuildEngine_UnknownType(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Engine.Type = "unknown"
	_, err := BuildEngine(EngineInput{Config: cfg, Ctx: context.Background()})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown engine type")
}

func TestBuildEngine_CEF_ReturnsErrorWhenRuntimeUnavailable(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Engine.Type = "cef"
	cfg.Engine.CEF.CEFDir = t.TempDir()
	_, err := BuildEngine(EngineInput{Config: cfg, Ctx: context.Background()})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cef: open library")
}
