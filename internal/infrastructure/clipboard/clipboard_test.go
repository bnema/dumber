package clipboard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	clipboardmocks "github.com/bnema/dumber/internal/infrastructure/clipboard/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type toolkitClipboardRecorder struct {
	*clipboardmocks.MockToolkitClipboard

	writeTextCalls  int
	readTextCalls   int
	writeImageCalls int
	text            string
	image           entity.ImageData
	writeTextErr    error
	readTextErr     error
	writeImageErr   error
}

func newToolkitClipboardRecorder(t *testing.T) *toolkitClipboardRecorder {
	t.Helper()

	recorder := &toolkitClipboardRecorder{MockToolkitClipboard: clipboardmocks.NewMockToolkitClipboard(t)}
	recorder.EXPECT().WriteText(mock.Anything, mock.Anything).RunAndReturn(recorder.writeText).Maybe()
	recorder.EXPECT().ReadText(mock.Anything).RunAndReturn(recorder.readText).Maybe()
	recorder.EXPECT().WriteImage(mock.Anything, mock.Anything).RunAndReturn(recorder.writeImage).Maybe()
	return recorder
}

func (r *toolkitClipboardRecorder) writeText(_ context.Context, text string) error {
	r.writeTextCalls++
	r.text = text
	return r.writeTextErr
}

func (r *toolkitClipboardRecorder) readText(_ context.Context) (string, error) {
	r.readTextCalls++
	return r.text, r.readTextErr
}

func (r *toolkitClipboardRecorder) writeImage(_ context.Context, image entity.ImageData) error {
	r.writeImageCalls++
	r.image = image
	return r.writeImageErr
}

func TestAdapter_ReadTextFallsBackToToolkitClipboardWhenNoSystemTool(t *testing.T) {
	toolkit := newToolkitClipboardRecorder(t)
	toolkit.text = "toolkit text"
	adapter := &Adapter{toolkit: toolkit}

	text, err := adapter.ReadText(context.Background())

	require.NoError(t, err)
	require.Equal(t, "toolkit text", text)
	require.Equal(t, 1, toolkit.readTextCalls)
}

func TestAdapter_HasTextReturnsTrueWhenToolkitFallbackHasText(t *testing.T) {
	toolkit := newToolkitClipboardRecorder(t)
	toolkit.text = "toolkit text"
	adapter := &Adapter{toolkit: toolkit}

	hasText, err := adapter.HasText(context.Background())

	require.NoError(t, err)
	require.True(t, hasText)
	require.Equal(t, 1, toolkit.readTextCalls)
}

func TestAdapter_WriteTextReturnsToolkitErrorWhenNoSystemTool(t *testing.T) {
	sentinel := errors.New("toolkit text failed")
	toolkit := newToolkitClipboardRecorder(t)
	toolkit.writeTextErr = sentinel
	adapter := &Adapter{toolkit: toolkit}

	err := adapter.WriteText(context.Background(), "hello")

	require.Equal(t, sentinel, err)
	require.Equal(t, 1, toolkit.writeTextCalls)
	require.Equal(t, "hello", toolkit.text)
}

func TestAdapter_WriteImageReturnsToolkitErrorWhenNoSystemTool(t *testing.T) {
	sentinel := errors.New("toolkit image failed")
	toolkit := newToolkitClipboardRecorder(t)
	toolkit.writeImageErr = sentinel
	adapter := &Adapter{toolkit: toolkit}

	err := adapter.WriteImage(context.Background(), entity.ImageData{Bytes: []byte{1}, MimeType: "image/png"})

	require.Equal(t, sentinel, err)
	require.Equal(t, 1, toolkit.writeImageCalls)
	require.Equal(t, entity.ImageData{Bytes: []byte{1}, MimeType: "image/png"}, toolkit.image)
}

func TestNew_FallsBackToToolkitClipboardWhenSystemToolsUnavailable(t *testing.T) {
	oldToolkitFactory := newToolkitClipboard
	oldLookPath := lookPath
	t.Cleanup(func() {
		newToolkitClipboard = oldToolkitFactory
		lookPath = oldLookPath
	})

	toolkit := newToolkitClipboardRecorder(t)
	newToolkitClipboard = func() toolkitClipboard {
		return toolkit
	}
	lookPath = func(_ string) (string, error) {
		return "", errors.New("missing")
	}

	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	t.Setenv("DISPLAY", ":0")

	adapter := New().(*Adapter)
	require.NoError(t, adapter.WriteImage(context.Background(), entity.ImageData{Bytes: []byte{1, 2, 3}}))
	require.Equal(t, 1, toolkit.writeImageCalls)
	require.Equal(t, []byte{1, 2, 3}, toolkit.image.Bytes)
}

func TestAdapter_WriteImagePrefersToolkitClipboardWhenAvailable(t *testing.T) {
	oldToolkitFactory := newToolkitClipboard
	t.Cleanup(func() {
		newToolkitClipboard = oldToolkitFactory
	})

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "wl-copy")
	stdinPath := filepath.Join(dir, "stdin.bin")
	script := "#!/bin/sh\ncat > \"" + stdinPath + "\"\n"
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	toolkit := newToolkitClipboardRecorder(t)
	newToolkitClipboard = func() toolkitClipboard { return toolkit }
	adapter := &Adapter{copyCmd: scriptPath}
	image := entity.ImageData{Bytes: []byte{1, 2, 3}, MimeType: "image/png"}

	require.NoError(t, adapter.WriteImage(context.Background(), image))
	require.Equal(t, 1, toolkit.writeImageCalls)
	require.Equal(t, image, toolkit.image)

	_, err := os.Stat(stdinPath)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
}

func TestAdapter_WriteImageFallsBackToSystemClipboardWhenToolkitUnavailable(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "wl-copy")
	stdinPath := filepath.Join(dir, "stdin.bin")
	script := "#!/bin/sh\ncat > \"" + stdinPath + "\"\n"
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	adapter := &Adapter{copyCmd: scriptPath}
	image := entity.ImageData{Bytes: []byte{1, 2, 3}, MimeType: "image/png"}

	require.NoError(t, adapter.WriteImage(context.Background(), image))

	stdin, err := os.ReadFile(stdinPath)
	require.NoError(t, err)
	require.Equal(t, image.Bytes, stdin)
}

func TestNew_FallsBackWaylandBeforeX11(t *testing.T) {
	oldToolkitFactory := newToolkitClipboard
	oldLookPath := lookPath
	t.Cleanup(func() {
		newToolkitClipboard = oldToolkitFactory
		lookPath = oldLookPath
	})

	newToolkitClipboard = func() toolkitClipboard { return nil }
	lookups := make([]string, 0, 2)
	lookPath = func(name string) (string, error) {
		lookups = append(lookups, name)
		switch name {
		case "wl-copy":
			return "", errors.New("wl-copy missing")
		case "xclip":
			return "/usr/bin/xclip", nil
		default:
			return "", errors.New("unexpected lookPath call")
		}
	}

	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	t.Setenv("DISPLAY", ":0")

	adapter := New().(*Adapter)
	require.Equal(t, "/usr/bin/xclip", adapter.copyCmd)
	require.Equal(t, "/usr/bin/xclip", adapter.pasteCmd)
	require.Equal(t, []string{"wl-copy", "xclip"}, lookups)
}

func TestAdapter_WriteImageRejectsEmptyBytes(t *testing.T) {
	oldToolkitFactory := newToolkitClipboard
	oldLookPath := lookPath
	t.Cleanup(func() {
		newToolkitClipboard = oldToolkitFactory
		lookPath = oldLookPath
	})

	toolkit := newToolkitClipboardRecorder(t)
	newToolkitClipboard = func() toolkitClipboard { return toolkit }
	lookPath = func(_ string) (string, error) { return "/usr/bin/clipboard", nil }

	err := New().(*Adapter).WriteImage(context.Background(), entity.ImageData{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty image data")
	require.Zero(t, toolkit.writeImageCalls)
}

func TestAdapter_WriteImageWithCommandUsesImageMimeType(t *testing.T) {
	testCases := []struct {
		name     string
		image    entity.ImageData
		expected string
	}{
		{
			name:     "uses provided MIME type",
			image:    entity.ImageData{Bytes: []byte{1, 2, 3}, MimeType: "image/jpeg"},
			expected: "image/jpeg",
		},
		{
			name:     "falls back to PNG",
			image:    entity.ImageData{Bytes: []byte{1, 2, 3}},
			expected: "image/png",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			scriptPath := filepath.Join(dir, "wl-copy")
			argsPath := filepath.Join(dir, "args.txt")
			stdinPath := filepath.Join(dir, "stdin.bin")
			script := "#!/bin/sh\nprintf '%s\n' \"$@\" > \"" + argsPath + "\"\ncat > \"" + stdinPath + "\"\n"
			require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

			adapter := &Adapter{copyCmd: scriptPath}
			require.NoError(t, adapter.WriteImage(context.Background(), tt.image))

			args, err := os.ReadFile(argsPath)
			require.NoError(t, err)
			require.Equal(t, []string{"--type", tt.expected}, strings.Split(strings.TrimSpace(string(args)), "\n"))

			stdin, err := os.ReadFile(stdinPath)
			require.NoError(t, err)
			require.Equal(t, tt.image.Bytes, stdin)
		})
	}
}
