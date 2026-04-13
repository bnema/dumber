package clipboard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/require"
)

type fakeToolkitClipboard struct {
	writeTextCalls  int
	writeImageCalls int
	text            string
	image           entity.ImageData
}

func (f *fakeToolkitClipboard) WriteText(_ context.Context, text string) error {
	f.writeTextCalls++
	f.text = text
	return nil
}

func (f *fakeToolkitClipboard) WriteImage(_ context.Context, image entity.ImageData) error {
	f.writeImageCalls++
	f.image = image
	return nil
}

func TestNew_FallsBackToToolkitClipboardWhenSystemToolsUnavailable(t *testing.T) {
	oldToolkitFactory := newToolkitClipboard
	oldLookPath := lookPath
	t.Cleanup(func() {
		newToolkitClipboard = oldToolkitFactory
		lookPath = oldLookPath
	})

	fake := &fakeToolkitClipboard{}
	newToolkitClipboard = func() toolkitClipboard {
		return fake
	}
	lookPath = func(_ string) (string, error) {
		return "", errors.New("missing")
	}

	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	t.Setenv("DISPLAY", ":0")

	adapter := New().(*Adapter)
	require.NoError(t, adapter.WriteImage(context.Background(), entity.ImageData{Bytes: []byte{1, 2, 3}}))
	require.Equal(t, 1, fake.writeImageCalls)
	require.Equal(t, []byte{1, 2, 3}, fake.image.Bytes)
}

func TestAdapter_WriteImagePrefersSystemClipboardOverToolkit(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "wl-copy")
	argsPath := filepath.Join(dir, "args.txt")
	stdinPath := filepath.Join(dir, "stdin.bin")
	script := "#!/bin/sh\nprintf '%s\n' \"$@\" > \"" + argsPath + "\"\ncat > \"" + stdinPath + "\"\n"
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	fake := &fakeToolkitClipboard{}
	adapter := &Adapter{toolkit: fake, copyCmd: scriptPath}
	image := entity.ImageData{Bytes: []byte{1, 2, 3}, MimeType: "image/png"}

	require.NoError(t, adapter.WriteImage(context.Background(), image))
	require.Zero(t, fake.writeImageCalls)

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

	fake := &fakeToolkitClipboard{}
	newToolkitClipboard = func() toolkitClipboard { return fake }
	lookPath = func(_ string) (string, error) { return "/usr/bin/clipboard", nil }

	err := New().(*Adapter).WriteImage(context.Background(), entity.ImageData{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty image data")
	require.Zero(t, fake.writeImageCalls)
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
