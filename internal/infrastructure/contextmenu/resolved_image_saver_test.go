package contextmenu

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/require"
)

type fakeDownloadPreparer struct {
	input  port.DownloadPrepareInput
	output *port.DownloadPrepareOutput
}

func (f *fakeDownloadPreparer) Execute(_ context.Context, input port.DownloadPrepareInput) *port.DownloadPrepareOutput {
	f.input = input
	return f.output
}

func TestResolvedImageSaver_SaveResolvedImage(t *testing.T) {
	downloadDir := t.TempDir()
	destPath := filepath.Join(downloadDir, "image.png")
	preparer := &fakeDownloadPreparer{output: &port.DownloadPrepareOutput{Filename: "image.png", DestinationPath: destPath}}
	saver := NewResolvedImageSaver(preparer, downloadDir)

	err := saver.SaveResolvedImage(context.Background(), port.ImageData{Bytes: []byte{1, 2, 3, 4}, MimeType: "image/jpeg"}, port.MenuContext{ImageURI: "https://example.com/assets/image"})
	require.NoError(t, err)
	require.Equal(t, downloadDir, preparer.input.DownloadDir)
	require.Equal(t, "https://example.com/assets/image", preparer.input.Response.GetUri())
	require.Equal(t, "image/jpeg", preparer.input.Response.GetMimeType())
	require.Equal(t, []byte{1, 2, 3, 4}, readFile(t, destPath))
}

func TestResolvedImageSaver_SaveResolvedImageRequiresDestination(t *testing.T) {
	preparer := &fakeDownloadPreparer{output: &port.DownloadPrepareOutput{Filename: "image.png"}}
	saver := NewResolvedImageSaver(preparer, t.TempDir())

	err := saver.SaveResolvedImage(context.Background(), port.ImageData{Bytes: []byte{1}}, port.MenuContext{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "destination path")
}

func TestResolvedImageSaver_SaveResolvedImageRejectsEmptyBytes(t *testing.T) {
	preparer := &fakeDownloadPreparer{output: &port.DownloadPrepareOutput{Filename: "image.png", DestinationPath: filepath.Join(t.TempDir(), "image.png")}}
	saver := NewResolvedImageSaver(preparer, t.TempDir())

	err := saver.SaveResolvedImage(context.Background(), port.ImageData{}, port.MenuContext{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty image data")
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}
