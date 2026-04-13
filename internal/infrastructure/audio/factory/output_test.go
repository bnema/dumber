package factory

import (
	"context"
	"errors"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/audio/null"
)

// failingFactory is a factory that always returns an error
type failingFactory struct {
	err error
}

func (f *failingFactory) NewStream(_ context.Context, _ port.AudioStreamFormat) (port.AudioOutputStream, error) {
	return nil, f.err
}

func TestSelector_PrimaryFails_FallsBackToFallback(t *testing.T) {
	// Arrange
	primaryErr := errors.New("primary backend unavailable")
	primary := &failingFactory{err: primaryErr}
	fallback := &null.Factory{}

	selector := &Selector{
		Primary:  primary,
		Fallback: fallback,
	}

	format := port.AudioStreamFormat{
		SampleRate:      48000,
		ChannelCount:    2,
		FramesPerBuffer: 512,
	}
	ctx := context.Background()

	// Act
	stream, err := selector.NewStream(ctx, format)

	// Assert
	if err != nil {
		t.Fatalf("Expected no error when fallback succeeds, got: %v", err)
	}
	if stream == nil {
		t.Fatal("Expected non-nil stream from fallback")
	}

	// Verify the stream works (it's the null stream)
	samples := make([][]float32, format.ChannelCount)
	for i := range samples {
		samples[i] = make([]float32, format.FramesPerBuffer)
	}

	if err := stream.Write(samples); err != nil {
		t.Errorf("Write failed: %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestSelector_PrimarySucceeds_UsesPrimary(t *testing.T) {
	// Arrange
	primary := &null.Factory{} // Using null as primary for simplicity
	fallback := &failingFactory{err: errors.New("fallback should not be called")}

	selector := &Selector{
		Primary:  primary,
		Fallback: fallback,
	}

	format := port.AudioStreamFormat{
		SampleRate:      48000,
		ChannelCount:    2,
		FramesPerBuffer: 512,
	}
	ctx := context.Background()

	// Act
	stream, err := selector.NewStream(ctx, format)

	// Assert
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if stream == nil {
		t.Fatal("Expected non-nil stream from primary")
	}

	// Verify the stream works
	samples := make([][]float32, format.ChannelCount)
	for i := range samples {
		samples[i] = make([]float32, format.FramesPerBuffer)
	}

	if err := stream.Write(samples); err != nil {
		t.Errorf("Write failed: %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestSelector_BothFail_ReturnsError(t *testing.T) {
	// Arrange
	primaryErr := errors.New("primary backend unavailable")
	fallbackErr := errors.New("fallback backend unavailable")

	selector := &Selector{
		Primary:  &failingFactory{err: primaryErr},
		Fallback: &failingFactory{err: fallbackErr},
	}

	format := port.AudioStreamFormat{
		SampleRate:      48000,
		ChannelCount:    2,
		FramesPerBuffer: 512,
	}
	ctx := context.Background()

	// Act
	stream, err := selector.NewStream(ctx, format)

	// Assert
	if err == nil {
		t.Fatal("Expected error when both backends fail")
	}
	if stream != nil {
		t.Error("Expected nil stream when both backends fail")
	}
}

// TestNewAudioOutputFactory_ReturnsWorkingFactory verifies that
// NewAudioOutputFactory returns a working Selector with PipeWire primary
// and null fallback.
func TestNewAudioOutputFactory_ReturnsWorkingFactory(t *testing.T) {
	factory := NewAudioOutputFactory()
	if factory == nil {
		t.Fatal("Expected non-nil factory")
	}

	// Verify the factory produces a working stream
	format := port.AudioStreamFormat{
		SampleRate:      48000,
		ChannelCount:    2,
		FramesPerBuffer: 512,
	}
	ctx := context.Background()

	stream, err := factory.NewStream(ctx, format)
	if err != nil {
		t.Fatalf("Expected no error creating stream, got: %v", err)
	}
	if stream == nil {
		t.Fatal("Expected non-nil stream")
	}

	// Verify the stream works
	samples := make([][]float32, format.ChannelCount)
	for i := range samples {
		samples[i] = make([]float32, format.FramesPerBuffer)
	}

	if err := stream.Write(samples); err != nil {
		t.Errorf("Write failed: %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}
