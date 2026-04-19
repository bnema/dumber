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

func (s stubResponse) GetMimeType() string          { return s.mimeType }
func (s stubResponse) GetSuggestedFilename() string { return "" }
func (s stubResponse) GetUri() string               { return s.uri }

type captureEvents struct {
	events []port.DownloadEvent
}

func (c *captureEvents) OnDownloadEvent(_ context.Context, event port.DownloadEvent) {
	c.events = append(c.events, event)
}

func TestRuntimeResolveDestinationAndEvents(t *testing.T) {
	ctx := context.Background()
	events := &captureEvents{}
	runtime := New("/tmp/downloads", events, usecase.NewPrepareDownloadUseCase(nil))

	output, err := runtime.ResolveDestination(ctx, "artifact", stubResponse{mimeType: "application/pdf"})

	require.NoError(t, err)
	require.Equal(t, "artifact.pdf", output.Filename)
	runtime.EmitStarted(ctx, output)
	runtime.EmitFinished(ctx, output.Filename, output.DestinationPath)

	require.Len(t, events.events, 2)
	require.Equal(t, port.DownloadEventStarted, events.events[0].Type)
	require.Equal(t, port.DownloadEventFinished, events.events[1].Type)
}
