package usecase

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
)

// ReadSystemviewConfigUseCase exposes read-side systemview config payloads.
type ReadSystemviewConfigUseCase struct {
	reader port.SystemviewConfigReader
}

// NewReadSystemviewConfigUseCase creates a new read systemview config use case.
func NewReadSystemviewConfigUseCase(reader port.SystemviewConfigReader) *ReadSystemviewConfigUseCase {
	return &ReadSystemviewConfigUseCase{reader: reader}
}

// Current returns the current systemview config payload.
func (uc *ReadSystemviewConfigUseCase) Current(ctx context.Context) (port.SystemviewConfigPayload, error) {
	if uc == nil || uc.reader == nil {
		return port.SystemviewConfigPayload{}, fmt.Errorf("systemview config reader is nil")
	}
	return uc.reader.Current(ctx)
}

// Default returns the default systemview config payload.
func (uc *ReadSystemviewConfigUseCase) Default(ctx context.Context) (port.SystemviewConfigPayload, error) {
	if uc == nil || uc.reader == nil {
		return port.SystemviewConfigPayload{}, fmt.Errorf("systemview config reader is nil")
	}
	return uc.reader.Default(ctx)
}
