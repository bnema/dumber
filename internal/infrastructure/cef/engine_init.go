package cef

import (
	"context"
	"fmt"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/jwijenbergh/puregotk/v4/glib"
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

	if err := purecef.Init(settings); err != nil {
		return nil, fmt.Errorf("cef.Init: %w", err)
	}

	// 2. Pump CEF messages via the glib idle loop so CEF work executes on the
	//    GTK main thread alongside normal UI processing.
	cb := glib.SourceFunc(func(_ uintptr) bool {
		purecef.DoMessageLoopWork()
		return true // keep firing
	})
	glib.IdleAdd(&cb, 0)

	// 3. Load GL.
	gl, err := newGLLoader()
	if err != nil {
		purecef.Shutdown()
		return nil, fmt.Errorf("GL loader: %w", err)
	}

	// 4. Create factory + pool.
	scale := int32(1) // TODO: detect from GDK
	factory := newWebViewFactory(gl, scale)
	pool := newWebViewPool(factory)

	return &Engine{
		ctx:     ctx,
		gl:      gl,
		factory: factory,
		pool:    pool,
		logger:  logger,
	}, nil
}
