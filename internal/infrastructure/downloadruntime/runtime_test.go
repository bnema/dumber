package downloadruntime

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
)

type stubResponse struct {
	mimeType string
	uri      string
}

func (s stubResponse) GetMimeType() string        { return s.mimeType }
func (stubResponse) GetSuggestedFilename() string { return "" }
func (s stubResponse) GetUri() string             { return s.uri }

type captureEvents struct {
	events []port.DownloadEvent
}

func (c *captureEvents) OnDownloadEvent(_ context.Context, event port.DownloadEvent) {
	c.events = append(c.events, event)
}

func TestRuntimeResolveDestinationAndEvents(t *testing.T) {
	ctx := context.Background()
	events := &captureEvents{}
	runtime := New(t.TempDir(), events, usecase.NewPrepareDownloadUseCase(nil))

	output, err := runtime.ResolveDestination(ctx, "artifact", stubResponse{mimeType: "application/pdf"})

	require.NoError(t, err)
	require.Equal(t, "artifact.pdf", output.Filename)
	runtime.EmitStarted(ctx, output)
	runtime.EmitProgress(ctx, output.Filename, output.DestinationPath, 0.42, 42, 100)
	runtime.EmitFinished(ctx, output.Filename, output.DestinationPath)

	require.Len(t, events.events, 3)
	require.Equal(t, port.DownloadEventStarted, events.events[0].Type)
	require.Equal(t, port.DownloadEventProgress, events.events[1].Type)
	require.InEpsilon(t, 0.42, events.events[1].Progress, 0.0001)
	require.EqualValues(t, 42, events.events[1].BytesReceived)
	require.EqualValues(t, 100, events.events[1].BytesTotal)
	require.Equal(t, port.DownloadEventFinished, events.events[2].Type)
}

func TestNewRuntime_NilPreparer_NoPanic(t *testing.T) {
	require.NotPanics(t, func() {
		_ = New("/tmp/downloads", nil, nil)
	})
}

func TestRuntimeResolveDestination_NilPreparerReturnsError(t *testing.T) {
	runtime := New(t.TempDir(), nil, nil)

	output, err := runtime.ResolveDestination(context.Background(), "artifact", stubResponse{})

	require.Nil(t, output)
	require.Error(t, err)
	require.ErrorContains(t, err, "download preparer is required")
}
