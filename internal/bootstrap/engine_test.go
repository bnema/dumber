package bootstrap

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bnema/dumber/internal/infrastructure/cef"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/runtimeprofile"
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
	_, err := BuildEngine(EngineInput{Config: cfg, Ctx: context.Background(), RuntimeProfile: testRuntimeProfile(t)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cef.InitWithApp")
}

func TestBuildEngine_CEF_ReturnsErrorForUnsupportedCookiePolicy(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Engine.Type = "cef"
	cfg.Engine.CookiePolicy = config.CookiePolicyNever
	_, err := BuildEngine(EngineInput{Config: cfg, Ctx: context.Background(), RuntimeProfile: testRuntimeProfile(t)})
	require.ErrorIs(t, err, cef.ErrCookiePolicyUnsupported)
}

func TestBuildEngine_CEF_AllowsNoThirdPartyCookiePolicy(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Engine.Type = "cef"
	cfg.Engine.CookiePolicy = config.CookiePolicyNoThirdParty
	cfg.Engine.CEF.CEFDir = t.TempDir()
	_, err := BuildEngine(EngineInput{Config: cfg, Ctx: context.Background(), RuntimeProfile: testRuntimeProfile(t)})
	require.NotErrorIs(t, err, cef.ErrCookiePolicyUnsupported)
}

func TestBuildEngine_ReceivesResolvedProfile(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Engine.Type = config.EngineTypeCEF
	cfg.Engine.CEF.CEFDir = t.TempDir()

	_, err := BuildEngine(EngineInput{Config: cfg, Ctx: context.Background(), RuntimeProfile: testRuntimeProfile(t)})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "missing runtime profile")
}

func testRuntimeProfile(t *testing.T) runtimeprofile.Profile {
	t.Helper()
	root := t.TempDir()
	profile, err := runtimeprofile.Resolve(runtimeprofile.ResolveInput{
		Env:    func(string) string { return "" },
		Engine: config.EngineTypeCEF,
		CWD:    func() (string, error) { return root, nil },
		Base: runtimeprofile.BasePaths{
			ConfigHome: filepath.Join(root, "config"),
			DataHome:   filepath.Join(root, "data"),
			StateHome:  filepath.Join(root, "state"),
			CacheHome:  filepath.Join(root, "cache"),
		},
	})
	require.NoError(t, err)
	return profile
}
