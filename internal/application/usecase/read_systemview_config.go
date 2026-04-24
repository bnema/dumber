package usecase

import (
	"context"
	"errors"

	"github.com/bnema/dumber/internal/application/port"
)

// ReadSystemviewConfigUseCase exposes read-side systemview config payloads.
type ReadSystemviewConfigUseCase struct {
	reader port.SystemviewConfigReader
}

// NewReadSystemviewConfigUseCase creates a new read systemview config use case.
func NewReadSystemviewConfigUseCase(reader port.SystemviewConfigReader) *ReadSystemviewConfigUseCase {
	if reader == nil {
		panic("NewReadSystemviewConfigUseCase: reader is nil")
	}
	return &ReadSystemviewConfigUseCase{reader: reader}
}

// ErrNilSystemviewConfigReader is returned when the config reader dependency is nil.
var ErrNilSystemviewConfigReader = errors.New("systemview config reader is nil")

// Current returns the current systemview config payload.
func (uc *ReadSystemviewConfigUseCase) Current(ctx context.Context) (port.SystemviewConfigPayload, error) {
	if uc == nil || uc.reader == nil {
		return port.SystemviewConfigPayload{}, ErrNilSystemviewConfigReader
	}
	return uc.reader.Current(ctx)
}

// Default returns the default systemview config payload.
func (uc *ReadSystemviewConfigUseCase) Default(ctx context.Context) (port.SystemviewConfigPayload, error) {
	if uc == nil || uc.reader == nil {
		return port.SystemviewConfigPayload{}, ErrNilSystemviewConfigReader
	}
	return uc.reader.Default(ctx)
}
