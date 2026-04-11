package null

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
)

func TestFactory_NewStream_ReturnsWritableClosableStream(t *testing.T) {
	// Arrange
	factory := &Factory{}
	format := port.AudioStreamFormat{
		SampleRate:      48000,
		ChannelCount:    2,
		FramesPerBuffer: 512,
	}
	ctx := context.Background()

	// Act
	stream, err := factory.NewStream(ctx, format)
	if err != nil {
		t.Fatalf("NewStream failed: %v", err)
	}
	if stream == nil {
		t.Fatal("NewStream returned nil stream")
	}

	// Assert: stream should accept writes
	samples := make([][]float32, format.ChannelCount)
	for i := range samples {
		samples[i] = make([]float32, format.FramesPerBuffer)
	}

	if err := stream.Write(samples); err != nil {
		t.Errorf("Write failed: %v", err)
	}

	// Assert: stream should be closable
	if err := stream.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestStream_Write_AcceptsMultipleCalls(t *testing.T) {
	// Arrange
	factory := &Factory{}
	format := port.AudioStreamFormat{
		SampleRate:      44100,
		ChannelCount:    2,
		FramesPerBuffer: 1024,
	}
	ctx := context.Background()

	stream, err := factory.NewStream(ctx, format)
	if err != nil {
		t.Fatalf("NewStream failed: %v", err)
	}
	defer stream.Close()

	samples := make([][]float32, format.ChannelCount)
	for i := range samples {
		samples[i] = make([]float32, format.FramesPerBuffer)
	}

	// Act & Assert: multiple writes should succeed
	for i := 0; i < 3; i++ {
		if err := stream.Write(samples); err != nil {
			t.Errorf("Write #%d failed: %v", i+1, err)
		}
	}
}

func TestStream_Close_Idempotent(t *testing.T) {
	// Arrange
	factory := &Factory{}
	format := port.AudioStreamFormat{
		SampleRate:      48000,
		ChannelCount:    1,
		FramesPerBuffer: 256,
	}
	ctx := context.Background()

	stream, err := factory.NewStream(ctx, format)
	if err != nil {
		t.Fatalf("NewStream failed: %v", err)
	}

	// Act & Assert: multiple closes should succeed
	if err := stream.Close(); err != nil {
		t.Errorf("First Close failed: %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Errorf("Second Close failed: %v", err)
	}
}
