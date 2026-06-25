package usecase

import (
	"context"
	"fmt"
	"math"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
)

// NavigateUseCase handles URL navigation with zoom application.
type NavigateUseCase struct {
	defaultZoom float64
}

// NewNavigateUseCase creates a new navigation use case.
// defaultZoom is the zoom level to use when no per-domain zoom is saved (typically from config).
func NewNavigateUseCase(defaultZoom float64) *NavigateUseCase {
	if defaultZoom <= 0 || math.IsNaN(defaultZoom) || math.IsInf(defaultZoom, 0) {
		defaultZoom = entity.ZoomDefault
	}

	return &NavigateUseCase{defaultZoom: defaultZoom}
}

// NavigateInput contains parameters for navigation.
type NavigateInput struct {
	URL     string
	PaneID  string
	WebView port.WebView
}

// NavigateOutput contains the result of navigation.
type NavigateOutput struct {
	AppliedZoom float64
}

// Execute navigates to a URL.
// Zoom is applied later via LoadCommitted event in ContentCoordinator.
func (uc *NavigateUseCase) Execute(ctx context.Context, input NavigateInput) (*NavigateOutput, error) {
	log := logging.FromContext(ctx)
	log.Debug().
		Str("url", logging.RedactURL(input.URL)).
		Str("pane_id", input.PaneID).
		Msg("navigating to URL")

	// Navigate to URL (zoom will be applied on LoadCommitted to avoid shift)
	// History is recorded on LoadCommitted when URI is guaranteed correct
	if err := input.WebView.LoadURI(ctx, input.URL); err != nil {
		return nil, fmt.Errorf("failed to load URL: %w", err)
	}

	log.Info().
		Str("url", logging.RedactURL(input.URL)).
		Msg("navigation initiated")

	return &NavigateOutput{
		AppliedZoom: uc.defaultZoom,
	}, nil
}

// Reload reloads the current page.
func (uc *NavigateUseCase) Reload(ctx context.Context, webview port.WebView, bypassCache bool) error {
	log := logging.FromContext(ctx).With().Float64("default_zoom", uc.defaultZoom).Logger()
	log.Debug().Bool("bypass_cache", bypassCache).Msg("reloading page")

	if bypassCache {
		return webview.ReloadBypassCache(ctx)
	}
	return webview.Reload(ctx)
}

// Stop stops the current page load.
func (uc *NavigateUseCase) Stop(ctx context.Context, webview port.WebView) error {
	log := logging.FromContext(ctx).With().Float64("default_zoom", uc.defaultZoom).Logger()
	log.Debug().Msg("stopping page load")
	return webview.Stop(ctx)
}
