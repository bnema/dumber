package usecase

import (
	"context"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// PrepareWebUIThemeUseCase handles theme CSS injection for internal web pages.
type PrepareWebUIThemeUseCase struct {
	injector port.ContentInjector
}

// NewPrepareWebUIThemeUseCase creates a new theme preparation use case.
func NewPrepareWebUIThemeUseCase(injector port.ContentInjector) *PrepareWebUIThemeUseCase {
	return &PrepareWebUIThemeUseCase{
		injector: injector,
	}
}

// PrepareWebUIThemeInput contains parameters for theme preparation.
type PrepareWebUIThemeInput struct {
	// CSSVars is the CSS custom property declarations to inject.
	// Should be generated from palette.ToWebCSSVars().
	CSSVars string
}

// Execute injects theme CSS variables into the web view.
func (uc *PrepareWebUIThemeUseCase) Execute(ctx context.Context, input PrepareWebUIThemeInput) error {
	log := logging.FromContext(ctx)
	log.Debug().Msg("preparing WebUI theme CSS")

	if uc.injector == nil {
		log.Warn().Msg("content injector is nil, skipping theme injection")
		return nil
	}

	if input.CSSVars == "" {
		log.Debug().Msg("no CSS vars to inject")
		return nil
	}

	if err := uc.injector.InjectThemeCSS(ctx, input.CSSVars); err != nil {
		log.Error().Err(err).Msg("failed to inject theme CSS")
		return err
	}

	log.Debug().Msg("theme CSS injected successfully")
	return nil
}
