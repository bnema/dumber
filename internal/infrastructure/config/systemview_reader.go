package config

import (
	"context"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
)

type systemviewConfigReader struct {
	hwSurveyor port.HardwareSurveyor
}

var _ port.SystemviewConfigReader = (*systemviewConfigReader)(nil)

// NewSystemviewConfigReader creates a read-side config payload reader.
func NewSystemviewConfigReader(hwSurveyor port.HardwareSurveyor) port.SystemviewConfigReader {
	return &systemviewConfigReader{hwSurveyor: hwSurveyor}
}

func (r *systemviewConfigReader) Current(ctx context.Context) (dto.SystemviewConfigPayload, error) {
	return r.build(ctx, Get())
}

func (r *systemviewConfigReader) Default(ctx context.Context) (dto.SystemviewConfigPayload, error) {
	return r.build(ctx, DefaultConfig())
}

func (r *systemviewConfigReader) build(ctx context.Context, cfg *Config) (dto.SystemviewConfigPayload, error) {
	if cfg == nil {
		return dto.SystemviewConfigPayload{}, nil
	}

	var hw *port.HardwareInfo
	if r.hwSurveyor != nil {
		survey := r.hwSurveyor.Survey(ctx)
		hw = &survey
	}

	return BuildSystemviewConfigPayload(cfg, hw), nil
}
