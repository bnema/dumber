package clipboard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/require"
)

type fakeToolkitClipboard struct {
	writeTextCalls  int
	writeImageCalls int
	text            string
	image           port.ImageData
}

func (f *fakeToolkitClipboard) WriteText(_ context.Context, text string) error {
	f.writeTextCalls++
	f.text = text
	return nil
}

func (f *fakeToolkitClipboard) WriteImage(_ context.Context, image port.ImageData) error {
	f.writeImageCalls++
	f.image = image
	return nil
}

func TestNew_PrefersToolkitClipboard(t *testing.T) {
	oldToolkitFactory := newToolkitClipboard
	oldLookPath := lookPath
	t.Cleanup(func() {
		newToolkitClipboard = oldToolkitFactory
		lookPath = oldLookPath
	})

	toolkitChecked := false
	fake := &fakeToolkitClipboard{}
	newToolkitClipboard = func() toolkitClipboard {
		toolkitChecked = true
		return fake
	}
	lookPath = func(_ string) (string, error) {
		require.True(t, toolkitChecked, "toolkit clipboard should be checked before system tools")
		return "", errors.New("unexpected call")
	}

	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	t.Setenv("DISPLAY", ":0")

	adapter := New().(*Adapter)
	require.NoError(t, adapter.WriteImage(context.Background(), port.ImageData{Bytes: []byte{1, 2, 3}}))
	require.Equal(t, 1, fake.writeImageCalls)
	require.Equal(t, []byte{1, 2, 3}, fake.image.Bytes)
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

	fake := &fakeToolkitClipboard{}
	newToolkitClipboard = func() toolkitClipboard { return fake }
	lookPath = func(_ string) (string, error) { return "/usr/bin/clipboard", nil }

	err := New().(*Adapter).WriteImage(context.Background(), port.ImageData{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty image data")
	require.Zero(t, fake.writeImageCalls)
}

func TestAdapter_WriteImageWithCommandUsesImageMimeType(t *testing.T) {
	testCases := []struct {
		name     string
		image    port.ImageData
		expected string
	}{
		{
			name:     "uses provided MIME type",
			image:    port.ImageData{Bytes: []byte{1, 2, 3}, MimeType: "image/jpeg"},
			expected: "image/jpeg",
		},
		{
			name:     "falls back to PNG",
			image:    port.ImageData{Bytes: []byte{1, 2, 3}},
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
