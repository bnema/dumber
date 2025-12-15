// Package usecase contains application use cases that orchestrate domain logic.
package usecase

import (
	"context"
	"fmt"
	"net/url"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/logging"
)

// ManageZoomUseCase handles per-domain zoom level operations.
type ManageZoomUseCase struct {
	zoomRepo    repository.ZoomRepository
	defaultZoom float64
}

// NewManageZoomUseCase creates a new zoom management use case.
// defaultZoom is the zoom level to use when resetting (typically from config).
func NewManageZoomUseCase(zoomRepo repository.ZoomRepository, defaultZoom float64) *ManageZoomUseCase {
	if defaultZoom <= 0 {
		defaultZoom = entity.ZoomDefault
	}
	return &ManageZoomUseCase{
		zoomRepo:    zoomRepo,
		defaultZoom: defaultZoom,
	}
}

// DefaultZoom returns the configured default zoom level.
func (uc *ManageZoomUseCase) DefaultZoom() float64 {
	return uc.defaultZoom
}

// GetZoom retrieves the zoom level for a domain.
// Returns the configured default zoom level if none is set.
func (uc *ManageZoomUseCase) GetZoom(ctx context.Context, domain string) (*entity.ZoomLevel, error) {
	log := logging.FromContext(ctx)
	log.Debug().Str("domain", domain).Msg("getting zoom level")

	zoom, err := uc.zoomRepo.Get(ctx, domain)
	if err != nil {
		return nil, fmt.Errorf("failed to get zoom level: %w", err)
	}

	if zoom == nil {
		zoom = entity.NewZoomLevel(domain, uc.defaultZoom)
		log.Debug().Str("domain", domain).Float64("zoom", zoom.ZoomFactor).Msg("using default zoom")
	}

	return zoom, nil
}

// SetZoom saves a zoom level for a domain.
func (uc *ManageZoomUseCase) SetZoom(ctx context.Context, domain string, factor float64) error {
	log := logging.FromContext(ctx)
	log.Debug().Str("domain", domain).Float64("factor", factor).Msg("setting zoom level")

	zoom := entity.NewZoomLevel(domain, factor)
	if err := uc.zoomRepo.Set(ctx, zoom); err != nil {
		return fmt.Errorf("failed to set zoom level: %w", err)
	}

	log.Info().Str("domain", domain).Float64("factor", zoom.ZoomFactor).Msg("zoom level saved")
	return nil
}

// ResetZoom removes the custom zoom level for a domain.
func (uc *ManageZoomUseCase) ResetZoom(ctx context.Context, domain string) error {
	log := logging.FromContext(ctx)
	log.Debug().Str("domain", domain).Msg("resetting zoom level")

	if err := uc.zoomRepo.Delete(ctx, domain); err != nil {
		return fmt.Errorf("failed to reset zoom level: %w", err)
	}

	log.Info().Str("domain", domain).Msg("zoom level reset to default")
	return nil
}

// ZoomIn increases the zoom level by one step (0.1).
func (uc *ManageZoomUseCase) ZoomIn(ctx context.Context, domain string, current float64) (*entity.ZoomLevel, error) {
	log := logging.FromContext(ctx)

	zoom := entity.NewZoomLevel(domain, current)
	zoom.ZoomIn()

	log.Debug().
		Str("domain", domain).
		Float64("from", current).
		Float64("to", zoom.ZoomFactor).
		Msg("zooming in")

	if err := uc.zoomRepo.Set(ctx, zoom); err != nil {
		return nil, fmt.Errorf("failed to save zoom level: %w", err)
	}

	return zoom, nil
}

// ZoomOut decreases the zoom level by one step (0.1).
func (uc *ManageZoomUseCase) ZoomOut(ctx context.Context, domain string, current float64) (*entity.ZoomLevel, error) {
	log := logging.FromContext(ctx)

	zoom := entity.NewZoomLevel(domain, current)
	zoom.ZoomOut()

	log.Debug().
		Str("domain", domain).
		Float64("from", current).
		Float64("to", zoom.ZoomFactor).
		Msg("zooming out")

	if err := uc.zoomRepo.Set(ctx, zoom); err != nil {
		return nil, fmt.Errorf("failed to save zoom level: %w", err)
	}

	return zoom, nil
}

// ApplyToWebView loads the saved zoom level and applies it to a webview.
func (uc *ManageZoomUseCase) ApplyToWebView(ctx context.Context, webview port.WebView, domain string) error {
	log := logging.FromContext(ctx)

	zoom, err := uc.GetZoom(ctx, domain)
	if err != nil {
		return err
	}

	log.Debug().
		Str("domain", domain).
		Float64("factor", zoom.ZoomFactor).
		Msg("applying zoom to webview")

	if err := webview.SetZoomLevel(ctx, zoom.ZoomFactor); err != nil {
		return fmt.Errorf("failed to set zoom level: %w", err)
	}
	return nil
}

// GetAll retrieves all saved zoom levels.
func (uc *ManageZoomUseCase) GetAll(ctx context.Context) ([]*entity.ZoomLevel, error) {
	log := logging.FromContext(ctx)
	log.Debug().Msg("getting all zoom levels")

	levels, err := uc.zoomRepo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get zoom levels: %w", err)
	}

	log.Debug().Int("count", len(levels)).Msg("retrieved zoom levels")
	return levels, nil
}

// ExtractDomain extracts the host from a URL string.
func ExtractDomain(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("URL has no host: %s", rawURL)
	}
	return u.Host, nil
}
