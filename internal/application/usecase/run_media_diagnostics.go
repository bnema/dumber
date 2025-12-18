// Package usecase contains application business logic.
package usecase

import (
	"context"

	"github.com/bnema/dumber/internal/application/port"
)

// RunMediaDiagnosticsUseCase retrieves detailed media playback diagnostics.
type RunMediaDiagnosticsUseCase struct {
	diagnostics port.MediaDiagnostics
}

func NewRunMediaDiagnosticsUseCase(diagnostics port.MediaDiagnostics) *RunMediaDiagnosticsUseCase {
	return &RunMediaDiagnosticsUseCase{diagnostics: diagnostics}
}

type RunMediaDiagnosticsInput struct{}

type RunMediaDiagnosticsOutput struct {
	GStreamerAvailable bool
	HasVAPlugin        bool
	HasVAAPIPlugin     bool
	HasNVCodecPlugin   bool

	AV1Decoders  []string
	H264Decoders []string
	H265Decoders []string
	VP9Decoders  []string

	VAAPIAvailable bool
	VAAPIDriver    string
	VAAPIVersion   string

	HWAccelAvailable bool
	AV1HWAvailable   bool
	Warnings         []string
}

func (uc *RunMediaDiagnosticsUseCase) Execute(ctx context.Context, _ RunMediaDiagnosticsInput) (*RunMediaDiagnosticsOutput, error) {
	result := uc.diagnostics.RunDiagnostics(ctx)
	if result == nil {
		return &RunMediaDiagnosticsOutput{}, nil
	}

	return &RunMediaDiagnosticsOutput{
		GStreamerAvailable: result.GStreamerAvailable,
		HasVAPlugin:        result.HasVAPlugin,
		HasVAAPIPlugin:     result.HasVAAPIPlugin,
		HasNVCodecPlugin:   result.HasNVCodecPlugin,

		AV1Decoders:  result.AV1Decoders,
		H264Decoders: result.H264Decoders,
		H265Decoders: result.H265Decoders,
		VP9Decoders:  result.VP9Decoders,

		VAAPIAvailable: result.VAAPIAvailable,
		VAAPIDriver:    result.VAAPIDriver,
		VAAPIVersion:   result.VAAPIVersion,

		HWAccelAvailable: result.HWAccelAvailable,
		AV1HWAvailable:   result.AV1HWAvailable,
		Warnings:         result.Warnings,
	}, nil
}
