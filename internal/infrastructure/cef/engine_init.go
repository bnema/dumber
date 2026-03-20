package cef

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/logging"
)

// NewEngine initializes the CEF runtime and returns a ready-to-use Engine.
func NewEngine(ctx context.Context, cfg config.CEFEngineConfig) (*Engine, error) {
	logger := logging.FromContext(ctx)
	// 1. Initialize CEF.
	settings := purecef.DefaultSettings()
	settings.ExternalMessagePump = parseBoolEnv("DUMBER_CEF_EXTERNAL_PUMP", settings.ExternalMessagePump)
	if cfg.CEFDir != "" {
		settings.CEFDir = cfg.CEFDir
	}
	if cfg.LogSeverity != 0 {
		settings.LogSeverity = cfg.LogSeverity
	}

	// NOTE: MaybeExitSubprocess is called in main() before Cobra, so that
	// CEF subprocess args (--type=renderer) are handled before arg stripping.

	// Use a dedicated helper binary for CEF subprocesses (renderer, GPU,
	// utility). The main dumber binary has heavy Go initialization that
	// corrupts process state before CEF gets control. The helper binary
	// calls cef_execute_process immediately and exits cleanly.
	helperPath := findHelperBinary()
	if helperPath != "" {
		settings.BrowserSubprocessPath = helperPath
		logger.Debug().Str("path", helperPath).Msg("cef: using subprocess helper")
	} else {
		logger.Warn().Msg("cef: subprocess helper not found, falling back to main binary")
	}

	// Chromium flags injected temporarily into os.Args for cef_initialize,
	// then restored so GTK doesn't choke on unknown flags.
	//
	// --no-zygote: Disable the zygote process which uses fork(). Go's runtime
	//   does not survive fork cleanly, causing renderer crashes. Without zygote,
	//   CEF launches each subprocess as a fresh exec of the helper binary.
	savedArgs := os.Args
	os.Args = appendIfMissing(os.Args, "--no-zygote")

	// 2. Build the engine early so the App can reference it for the pump callback.
	eng := &Engine{
		ctx:                 ctx,
		externalMessagePump: settings.ExternalMessagePump,
		manualPumpInterval:  parseInt64Env("DUMBER_CEF_MANUAL_PUMP_MS", 10),
	}

	logger.Info().
		Bool("external_message_pump", settings.ExternalMessagePump).
		Int64("manual_pump_interval_ms", eng.manualPumpInterval).
		Msg("cef: configured message pump mode")

	// Create the CEF App with a BrowserProcessHandler that drives the
	// adaptive message pump via OnScheduleMessagePumpWork.
	app := newDumberApp(eng)

	logger.Debug().Msg("cef: calling InitWithApp")
	if err := purecef.InitWithApp(settings, app); err != nil {
		os.Args = savedArgs
		return nil, fmt.Errorf("cef.InitWithApp: %w", err)
	}
	os.Args = savedArgs
	logger.Debug().Msg("cef: InitWithApp returned OK")

	// 3. Load GL.
	gl, err := newGLLoader()
	if err != nil {
		purecef.Shutdown()
		return nil, fmt.Errorf("GL loader: %w", err)
	}

	// 4. Create factory + pool and wire them into the engine.
	scale := int32(1) // TODO: detect from GDK
	factory := newWebViewFactory(eng, gl, scale)
	pool := newWebViewPool(factory)

	eng.gl = gl
	eng.factory = factory
	eng.pool = pool

	return eng, nil
}

// appendIfMissing appends flag to args if it's not already present.
func appendIfMissing(args []string, flag string) []string {
	for _, a := range args {
		if a == flag {
			return args
		}
	}
	return append(args, flag)
}

// findHelperBinary looks for the cef-helper binary next to the running
// executable. Returns empty string if not found.
func findHelperBinary() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(exe)
	helper := filepath.Join(dir, "cef-helper")
	if _, err := os.Stat(helper); err == nil {
		return helper
	}
	return ""
}

func parseBoolEnv(name string, fallback bool) bool {
	value, ok := os.LookupEnv(name)
	if !ok || value == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func parseInt64Env(name string, fallback int64) int64 {
	value, ok := os.LookupEnv(name)
	if !ok || value == "" {
		return fallback
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
