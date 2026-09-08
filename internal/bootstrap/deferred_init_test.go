package bootstrap

import (
	"context"
	"sync"
	"testing"

	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunDeferredInit_CEFExecutesNoWebKitDiagnostics verifies that automatic
// CEF startup runs no unrelated WebKit pkg-config or GStreamer media
// diagnostics. Explicit diagnostic commands invoke the Check* functions
// directly and are unaffected.
func TestRunDeferredInit_CEFExecutesNoWebKitDiagnostics(t *testing.T) {
	t.Setenv("DUMBER_ENGINE", config.EngineTypeCEF)
	cfg := config.DefaultConfig()
	cfg.Engine.Type = config.EngineTypeCEF

	// The probe fires inside each check goroutine, so a zero count proves
	// engine selection skipped the checks without invoking them. Not
	// parallel-safe with other probe users by design; restored via defer.
	var mu sync.Mutex
	var invoked []string
	deferredInitProbeForTest = func(kind string) {
		mu.Lock()
		defer mu.Unlock()
		invoked = append(invoked, kind)
	}
	defer func() { deferredInitProbeForTest = nil }()

	result := RunDeferredInit(DeferredInitInput{Ctx: context.Background(), Config: cfg})

	assert.NoError(t, result.RuntimeErr, "CEF startup must not run WebKit runtime diagnostics")
	assert.NoError(t, result.MediaErr, "CEF startup must not run GStreamer media diagnostics")
	assert.Empty(t, invoked, "CEF startup must not invoke WebKit runtime/media diagnostics at all")
}

// TestCheckFunctions_RemainAvailableForExplicitCommands verifies the
// underlying checks stay callable for explicit diagnostic commands after
// RunDeferredInit stops invoking them automatically for CEF.
func TestCheckFunctions_RemainAvailableForExplicitCommands(t *testing.T) {
	t.Setenv("DUMBER_ENGINE", config.EngineTypeCEF)
	cfg := config.DefaultConfig()
	cfg.Engine.Type = config.EngineTypeCEF

	// The functions must exist and be invocable regardless of engine; their
	// results depend on host tooling, so only invocation (not success) is
	// asserted here.
	_ = CheckRuntimeRequirements(context.Background(), cfg)
	_ = CheckMediaRequirements(context.Background(), cfg)
	require.NotNil(t, cfg)
}
