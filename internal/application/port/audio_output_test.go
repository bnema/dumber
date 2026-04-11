package port_test

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/assert"
)

// TestAudioOutputPortContract verifies that the Audio Output port contract
// matches the approved design exactly.
func TestAudioOutputPortContract(t *testing.T) {
	t.Run("AudioStreamFormat is a struct with correct fields", func(t *testing.T) {
		format := port.AudioStreamFormat{
			SampleRate:      48000,
			ChannelCount:    2,
			FramesPerBuffer: 512,
		}

		assert.Equal(t, 48000, format.SampleRate)
		assert.Equal(t, 2, format.ChannelCount)
		assert.Equal(t, 512, format.FramesPerBuffer)
	})

	t.Run("AudioOutputStream interface has Write with [][]float32", func(_ *testing.T) {
		var _ port.AudioOutputStream = (*mockAudioOutputStream)(nil)
	})

	t.Run("AudioOutputFactory has NewStream with context and format", func(_ *testing.T) {
		var _ port.AudioOutputFactory = (*mockAudioOutputFactory)(nil)
	})
}

// mockAudioOutputStream is a test mock for the AudioOutputStream interface
type mockAudioOutputStream struct{}

func (*mockAudioOutputStream) Write(_ [][]float32) error {
	return nil
}

func (*mockAudioOutputStream) Close() error {
	return nil
}

// mockAudioOutputFactory is a test mock for the AudioOutputFactory interface
type mockAudioOutputFactory struct{}

func (*mockAudioOutputFactory) NewStream(_ context.Context, _ port.AudioStreamFormat) (port.AudioOutputStream, error) {
	return &mockAudioOutputStream{}, nil
}
