package cef

import (
	"context"
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
)

type stubDownloadItem struct {
	id        uint32
	url       string
	suggested string
	mimeType  string
	fullPath  string
	complete  bool
	canceled  bool
}

func (s stubDownloadItem) IsValid() bool       { return true }
func (s stubDownloadItem) IsInProgress() bool  { return !s.complete && !s.canceled }
func (s stubDownloadItem) IsComplete() bool    { return s.complete }
func (s stubDownloadItem) IsCanceled() bool    { return s.canceled }
func (s stubDownloadItem) IsInterrupted() bool { return false }
func (s stubDownloadItem) GetInterruptReason() purecef.DownloadInterruptReason {
	return 0
}
func (s stubDownloadItem) GetCurrentSpeed() int64        { return 0 }
func (s stubDownloadItem) GetPercentComplete() int32     { return 0 }
func (s stubDownloadItem) GetTotalBytes() int64          { return 0 }
func (s stubDownloadItem) GetReceivedBytes() int64       { return 0 }
func (s stubDownloadItem) GetStartTime() uintptr         { return 0 }
func (s stubDownloadItem) GetEndTime() uintptr           { return 0 }
func (s stubDownloadItem) GetFullPath() string           { return s.fullPath }
func (s stubDownloadItem) GetID() uint32                 { return s.id }
func (s stubDownloadItem) GetURL() string                { return s.url }
func (s stubDownloadItem) GetOriginalURL() string        { return s.url }
func (s stubDownloadItem) GetSuggestedFileName() string  { return s.suggested }
func (s stubDownloadItem) GetContentDisposition() string { return "" }
func (s stubDownloadItem) GetMimeType() string           { return s.mimeType }
func (s stubDownloadItem) IsPaused() bool                { return false }

type stubBeforeDownloadCallback struct {
	path       string
	showDialog int32
}

func (s *stubBeforeDownloadCallback) Cont(downloadPath string, showDialog int32) {
	s.path = downloadPath
	s.showDialog = showDialog
}

type captureDownloadEvents struct {
	events []port.DownloadEvent
}

func (c *captureDownloadEvents) OnDownloadEvent(_ context.Context, event port.DownloadEvent) {
	c.events = append(c.events, event)
}

func TestCEFDownloadHandlerLifecycle(t *testing.T) {
	ctx := context.Background()
	preparer := usecase.NewPrepareDownloadUseCase(nil)
	events := &captureDownloadEvents{}
	handler := newDownloadHandler("/tmp/downloads", events, preparer)
	callback := &stubBeforeDownloadCallback{}

	item := stubDownloadItem{
		id:        7,
		url:       "https://example.com/image.iso",
		suggested: "image.iso",
		mimeType:  "application/x-iso9660-image",
	}

	ok := handler.onBeforeDownload(ctx, nil, item, item.suggested, callback)

	require.True(t, ok)
	require.Equal(t, "/tmp/downloads/image.iso", callback.path)
	require.Len(t, events.events, 1)
	require.Equal(t, port.DownloadEventStarted, events.events[0].Type)

	item.complete = true
	item.fullPath = callback.path
	handler.onDownloadUpdated(ctx, item, nil)
	handler.onDownloadUpdated(ctx, item, nil)

	require.Len(t, events.events, 2)
	require.Equal(t, port.DownloadEventFinished, events.events[1].Type)
	require.Equal(t, "image.iso", events.events[1].Filename)
}
