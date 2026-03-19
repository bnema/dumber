package cef

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/rs/zerolog"

	"github.com/bnema/dumber/internal/infrastructure/config"
)

// NewEngine initializes the CEF runtime and returns a ready-to-use Engine.
func NewEngine(ctx context.Context, cfg config.CEFEngineConfig, logger zerolog.Logger) (*Engine, error) {
	// 1. Initialize CEF.
	settings := purecef.DefaultSettings()
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
	// --in-process-gpu: GPU subprocess fails (error_code=1002) even with the
	//   helper because the Go runtime creates FDs the GPU process doesn't expect.
	// --no-zygote: Disable the zygote process which uses fork(). Go's runtime
	//   does not survive fork cleanly, causing renderer crashes. Without zygote,
	//   CEF launches each subprocess as a fresh exec of the helper binary.
	savedArgs := os.Args
	os.Args = appendIfMissing(os.Args, "--in-process-gpu")
	os.Args = appendIfMissing(os.Args, "--no-zygote")

	logger.Debug().Msg("cef: calling Init")
	if err := purecef.Init(settings); err != nil {
		os.Args = savedArgs
		return nil, fmt.Errorf("cef.Init: %w", err)
	}
	os.Args = savedArgs
	logger.Debug().Msg("cef: Init returned OK")

	// 2. Load GL.
	gl, err := newGLLoader()
	if err != nil {
		purecef.Shutdown()
		return nil, fmt.Errorf("GL loader: %w", err)
	}

	// 3. Create factory + pool.
	scale := int32(1) // TODO: detect from GDK
	factory := newWebViewFactory(gl, scale)
	pool := newWebViewPool(factory)

	// 4. Build the engine. The CEF message pump is started later from
	//    OnToolkitReady, after GTK/libadwaita have fully initialized.
	//    Starting it here (before the GTK main loop) causes a SIGSEGV with
	//    --in-process-gpu because Chromium GPU bootstrap runs too early.
	eng := &Engine{
		ctx:     ctx,
		gl:      gl,
		factory: factory,
		pool:    pool,
		logger:  logger,
	}

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
