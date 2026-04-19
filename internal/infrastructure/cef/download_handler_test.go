package cef

import (
	"context"
	"fmt"
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
)

type stubDownloadItem struct {
	id              uint32
	url             string
	suggested       string
	mimeType        string
	fullPath        string
	complete        bool
	canceled        bool
	interrupted     bool
	reason          purecef.DownloadInterruptReason
	percentComplete int32
	totalBytes      int64
	receivedBytes   int64
}

func (s stubDownloadItem) IsValid() bool       { return true }
func (s stubDownloadItem) IsInProgress() bool  { return !s.complete && !s.canceled && !s.interrupted }
func (s stubDownloadItem) IsComplete() bool    { return s.complete }
func (s stubDownloadItem) IsCanceled() bool    { return s.canceled }
func (s stubDownloadItem) IsInterrupted() bool { return s.interrupted }
func (s stubDownloadItem) GetInterruptReason() purecef.DownloadInterruptReason {
	return s.reason
}
func (s stubDownloadItem) GetCurrentSpeed() int64        { return 0 }
func (s stubDownloadItem) GetPercentComplete() int32     { return s.percentComplete }
func (s stubDownloadItem) GetTotalBytes() int64          { return s.totalBytes }
func (s stubDownloadItem) GetReceivedBytes() int64       { return s.receivedBytes }
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
	// Repeated completion updates should not emit duplicate finished events.
	handler.onDownloadUpdated(ctx, item, nil)

	require.Len(t, events.events, 2)
	require.Equal(t, port.DownloadEventFinished, events.events[1].Type)
	require.Equal(t, "image.iso", events.events[1].Filename)

	// Active map should be cleaned up after terminal event.
	handler.mu.Lock()
	_, inActive := handler.active[7]
	_, inFinished := handler.finished[7]
	handler.mu.Unlock()
	require.False(t, inActive, "completed download should be removed from active map")
	require.True(t, inFinished, "completed download should be in finished set")
}

func TestCanDownload_AllowsAllMethods(t *testing.T) {
	preparer := usecase.NewPrepareDownloadUseCase(nil)
	handler := newDownloadHandler("/tmp/downloads", nil, preparer)

	require.True(t, handler.canDownload(nil, "https://example.com/file.zip", "GET"))
	require.True(t, handler.canDownload(nil, "https://example.com/file.zip", "POST"))
	require.True(t, handler.canDownload(nil, "https://example.com/file.zip", "PUT"))
	require.True(t, handler.canDownload(nil, "https://example.com/file.zip", ""))
}

func TestMarkFinished_SuppressesDuplicatesAfterCleanup(t *testing.T) {
	preparer := usecase.NewPrepareDownloadUseCase(nil)
	handler := newDownloadHandler("/tmp/downloads", nil, preparer)

	// Simulate a download that was tracked through onBeforeDownload.
	handler.mu.Lock()
	handler.active[42] = cefDownloadState{filename: "test.bin", destination: "/tmp/test.bin", lastProgressPercent: -1}
	handler.mu.Unlock()

	require.True(t, handler.markFinished(42), "first terminal event should succeed")

	// Simulate a concurrent update re-populating active before a duplicate terminal update.
	handler.mu.Lock()
	handler.active[42] = cefDownloadState{filename: "test.bin", destination: "/tmp/test.bin", lastProgressPercent: -1}
	handler.mu.Unlock()

	require.False(t, handler.markFinished(42), "second terminal event should be suppressed")

	// Active map should not contain the entry.
	handler.mu.Lock()
	_, inActive := handler.active[42]
	handler.mu.Unlock()
	require.False(t, inActive)
}

func TestCEFDownloadHandlerEmitsProgressOncePerPercent(t *testing.T) {
	ctx := context.Background()
	preparer := usecase.NewPrepareDownloadUseCase(nil)
	events := &captureDownloadEvents{}
	handler := newDownloadHandler("/tmp/downloads", events, preparer)
	callback := &stubBeforeDownloadCallback{}

	item := stubDownloadItem{
		id:        11,
		url:       "https://example.com/archive.zip",
		suggested: "archive.zip",
		mimeType:  "application/zip",
	}

	require.True(t, handler.onBeforeDownload(ctx, nil, item, item.suggested, callback))

	item.fullPath = callback.path
	item.percentComplete = 12
	item.receivedBytes = 12
	item.totalBytes = 100
	handler.onDownloadUpdated(ctx, item, nil)
	handler.onDownloadUpdated(ctx, item, nil)

	item.percentComplete = 13
	item.receivedBytes = 13
	handler.onDownloadUpdated(ctx, item, nil)

	require.Len(t, events.events, 3)
	require.Equal(t, port.DownloadEventStarted, events.events[0].Type)
	require.Equal(t, port.DownloadEventProgress, events.events[1].Type)
	require.InEpsilon(t, 0.12, events.events[1].Progress, 0.0001)
	require.Equal(t, port.DownloadEventProgress, events.events[2].Type)
	require.InEpsilon(t, 0.13, events.events[2].Progress, 0.0001)
}

func TestCEFDownloadInterruptedErrorIncludesCodeAndReason(t *testing.T) {
	ctx := context.Background()
	preparer := usecase.NewPrepareDownloadUseCase(nil)
	events := &captureDownloadEvents{}
	handler := newDownloadHandler("/tmp/downloads", events, preparer)
	callback := &stubBeforeDownloadCallback{}

	item := stubDownloadItem{
		id:        9,
		url:       "https://example.com/ubuntu.iso",
		suggested: "ubuntu.iso",
		mimeType:  "application/x-iso9660-image",
	}

	require.True(t, handler.onBeforeDownload(ctx, nil, item, item.suggested, callback))

	item.fullPath = callback.path
	item.interrupted = true
	item.reason = purecef.DownloadInterruptReasonServerBadContent
	handler.onDownloadUpdated(ctx, item, nil)

	require.Len(t, events.events, 2)
	require.Error(t, events.events[1].Error)
	expectedCode := fmt.Sprintf("code=%d", purecef.DownloadInterruptReasonServerBadContent)
	require.ErrorContains(t, events.events[1].Error, expectedCode)
	require.ErrorContains(t, events.events[1].Error, "reason=server_bad_content")
}
