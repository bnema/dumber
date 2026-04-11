// Package factory provides audio backend selection with fallback support.
package factory

import (
	"context"
	"errors"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/audio/null"
	"github.com/bnema/dumber/internal/infrastructure/audio/pipewire"
)

// Selector chooses between a primary and fallback audio backend.
// If the primary backend fails to create a stream, it falls back to the fallback.
type Selector struct {
	Primary  port.AudioOutputFactory
	Fallback port.AudioOutputFactory
}

// NewStream attempts to create a stream from the primary backend.
// If that fails, it falls back to the fallback backend.
// Returns an error only if both backends fail.
func (s *Selector) NewStream(ctx context.Context, format port.AudioStreamFormat) (port.AudioOutputStream, error) {
	if s.Primary != nil {
		stream, err := s.Primary.NewStream(ctx, format)
		if err == nil {
			return stream, nil
		}

		// Primary failed, try fallback
		if s.Fallback != nil {
			fallbackStream, fallbackErr := s.Fallback.NewStream(ctx, format)
			if fallbackErr == nil {
				return fallbackStream, nil
			}
			return nil, fmt.Errorf("audio backends failed: %w", errors.Join(err, fallbackErr))
		}
		return nil, fmt.Errorf("primary failed: %w; no fallback configured", err)
	}

	// No primary, try fallback only
	if s.Fallback != nil {
		return s.Fallback.NewStream(ctx, format)
	}

	return nil, errors.New("no audio backend configured")
}

// NewAudioOutputFactory creates the primary audio output factory with PipeWire
// as the primary backend and null as the fallback.
// If PipeWire stream creation fails at runtime, the Selector falls back to the
// null backend automatically.
func NewAudioOutputFactory() port.AudioOutputFactory {
	pwFactory, err := pipewire.NewFactory()
	if err != nil {
		// Unexpected failure; use null fallback only
		return &null.Factory{}
	}

	return &Selector{
		Primary:  pwFactory,
		Fallback: &null.Factory{},
	}
}
