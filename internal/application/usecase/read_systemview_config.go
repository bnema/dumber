package usecase

import (
	"context"
	"errors"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
)

// ReadSystemviewConfigUseCase exposes read-side systemview config payloads.
type ReadSystemviewConfigUseCase struct {
	reader             port.SystemviewConfigReader
	resolvedAppearance *systemviewResolvedAppearanceResolver
}

type systemviewResolvedAppearanceResolver struct {
	externalTheme port.ExternalThemeSource
	colorResolver port.ColorSchemeResolver
	prefersDark   func() bool
}

// ReadSystemviewConfigOption customizes ReadSystemviewConfigUseCase behavior.
type ReadSystemviewConfigOption func(*ReadSystemviewConfigUseCase)

// WithSystemviewResolvedAppearance overlays the current config payload with a display-only
// resolved appearance snapshot. It uses an isolated theme resolver per read so opening
// dumb://config cannot mutate the live browser theme resolver's last-good cache.
func WithSystemviewResolvedAppearance(
	externalTheme port.ExternalThemeSource,
	colorResolver port.ColorSchemeResolver,
	prefersDark func() bool,
) ReadSystemviewConfigOption {
	return func(uc *ReadSystemviewConfigUseCase) {
		uc.resolvedAppearance = &systemviewResolvedAppearanceResolver{
			externalTheme: externalTheme,
			colorResolver: colorResolver,
			prefersDark:   prefersDark,
		}
	}
}

// NewReadSystemviewConfigUseCase creates a new read systemview config use case.
func NewReadSystemviewConfigUseCase(reader port.SystemviewConfigReader, opts ...ReadSystemviewConfigOption) *ReadSystemviewConfigUseCase {
	if reader == nil {
		panic("NewReadSystemviewConfigUseCase: reader is nil")
	}
	uc := &ReadSystemviewConfigUseCase{reader: reader}
	for _, opt := range opts {
		if opt != nil {
			opt(uc)
		}
	}
	return uc
}

// ErrNilSystemviewConfigReader is returned when the config reader dependency is nil.
var ErrNilSystemviewConfigReader = errors.New("systemview config reader is nil")

// Current returns the current systemview config payload.
func (uc *ReadSystemviewConfigUseCase) Current(ctx context.Context) (dto.SystemviewConfigPayload, error) {
	if uc == nil || uc.reader == nil {
		return dto.SystemviewConfigPayload{}, ErrNilSystemviewConfigReader
	}
	payload, err := uc.reader.Current(ctx)
	if err != nil {
		return dto.SystemviewConfigPayload{}, err
	}
	return uc.withResolvedAppearance(ctx, payload), nil
}

// Default returns the default systemview config payload.
func (uc *ReadSystemviewConfigUseCase) Default(ctx context.Context) (dto.SystemviewConfigPayload, error) {
	if uc == nil || uc.reader == nil {
		return dto.SystemviewConfigPayload{}, ErrNilSystemviewConfigReader
	}
	return uc.reader.Default(ctx)
}

func (uc *ReadSystemviewConfigUseCase) withResolvedAppearance(
	ctx context.Context,
	payload dto.SystemviewConfigPayload,
) dto.SystemviewConfigPayload {
	if uc == nil || uc.resolvedAppearance == nil {
		return payload
	}
	resolved, ok := uc.resolvedAppearance.resolve(ctx, payload)
	if !ok {
		return payload
	}
	appearance := dto.WebUIAppearanceWithResolvedTheme(payload.Appearance, resolved)
	payload.ResolvedAppearance = &appearance
	return payload
}

func (r *systemviewResolvedAppearanceResolver) resolve(
	ctx context.Context,
	payload dto.SystemviewConfigPayload,
) (entity.ResolvedTheme, bool) {
	if r == nil {
		return entity.ResolvedTheme{}, false
	}
	resolver := NewResolveThemeUseCase(r.externalTheme)
	out, err := resolver.Execute(ctx, ResolveThemeInputFromConfig(
		&payload.Appearance,
		payload.DefaultUIScale,
		nil,
		r.preference(payload.Appearance.ColorScheme),
	))
	if err != nil {
		return entity.ResolvedTheme{}, false
	}
	return out.Theme, true
}

func (r *systemviewResolvedAppearanceResolver) preference(colorScheme string) port.ColorSchemePreference {
	fallback := port.ColorSchemePreference{PrefersDark: true, Source: "systemviews"}
	if r.colorResolver != nil {
		fallback = r.colorResolver.Resolve()
	} else if r.prefersDark != nil {
		fallback.PrefersDark = r.prefersDark()
	}
	return ColorSchemePreferenceFromConfig(colorScheme, fallback)
}
