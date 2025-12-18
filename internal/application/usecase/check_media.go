// Package usecase contains application business logic.
package usecase

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// CheckMediaUseCase validates media playback requirements at startup.
type CheckMediaUseCase struct {
	diagnostics port.MediaDiagnostics
}

// NewCheckMediaUseCase creates a new CheckMediaUseCase.
func NewCheckMediaUseCase(diagnostics port.MediaDiagnostics) *CheckMediaUseCase {
	return &CheckMediaUseCase{
		diagnostics: diagnostics,
	}
}

// CheckMediaInput contains options for the media check.
type CheckMediaInput struct {
	ShowDiagnostics bool // Log diagnostics warnings
}

// CheckMediaOutput contains the result of the media check.
type CheckMediaOutput struct {
	GStreamerAvailable bool
	HWAccelAvailable   bool
	AV1HWAvailable     bool
	Warnings           []string
}

// Execute checks media playback requirements.
// Returns error if GStreamer is not installed (fatal).
// Returns warnings for missing hardware acceleration (non-fatal).
func (uc *CheckMediaUseCase) Execute(ctx context.Context, input CheckMediaInput) (*CheckMediaOutput, error) {
	log := logging.FromContext(ctx)

	result := uc.diagnostics.RunDiagnostics(ctx)

	// GStreamer is required - fail early if not installed
	if !result.GStreamerAvailable {
		return nil, fmt.Errorf("GStreamer not installed - video playback requires GStreamer. Install: gst-plugins-base gst-plugins-good gst-plugins-bad gst-plugins-ugly gst-libav")
	}

	// Log warnings if requested
	if input.ShowDiagnostics {
		for _, warning := range result.Warnings {
			log.Warn().Str("component", "media").Msg(warning)
		}
		if result.AV1HWAvailable {
			log.Info().Str("component", "media").Msg("AV1 hardware decoding available (preferred codec)")
		}
	}

	return &CheckMediaOutput{
		GStreamerAvailable: result.GStreamerAvailable,
		HWAccelAvailable:   result.HWAccelAvailable,
		AV1HWAvailable:     result.AV1HWAvailable,
		Warnings:           result.Warnings,
	}, nil
}
