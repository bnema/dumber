package usecase

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
)

func TestCheckMediaDiagnoseNeverReturnsNil(t *testing.T) {
	diag := portmocks.NewMockMediaDiagnostics(t)
	diag.EXPECT().RunDiagnostics(mock.Anything).Return(nil)

	got := NewCheckMediaUseCase(diag).Diagnose(context.Background())

	require.NotNil(t, got)
	assert.False(t, got.GStreamerAvailable)
}

func TestCheckMediaExecuteFailsWithoutGStreamer(t *testing.T) {
	diag := portmocks.NewMockMediaDiagnostics(t)
	diag.EXPECT().RunDiagnostics(mock.Anything).Return(nil)

	_, err := NewCheckMediaUseCase(diag).Execute(context.Background(), CheckMediaInput{})

	require.Error(t, err)
}

func TestCheckMediaExecuteReturnsSummary(t *testing.T) {
	diag := portmocks.NewMockMediaDiagnostics(t)
	diag.EXPECT().RunDiagnostics(mock.Anything).Return(&port.MediaDiagnosticsResult{
		GStreamerAvailable: true,
		HWAccelAvailable:   true,
		Warnings:           []string{"no av1"},
	})

	out, err := NewCheckMediaUseCase(diag).Execute(context.Background(), CheckMediaInput{})

	require.NoError(t, err)
	assert.True(t, out.HWAccelAvailable)
	assert.Equal(t, []string{"no av1"}, out.Warnings)
}
