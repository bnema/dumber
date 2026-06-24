package webkit

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestResolvedImageSaver_SaveResolvedImage(t *testing.T) {
	downloadDir := t.TempDir()
	destPath := filepath.Join(downloadDir, "image.png")
	preparer := portmocks.NewMockDownloadPreparer(t)
	preparer.EXPECT().
		Execute(mock.Anything, mock.MatchedBy(func(input port.DownloadPrepareInput) bool {
			return input.DownloadDir == downloadDir &&
				input.Response.GetUri() == "https://example.com/assets/image" &&
				input.Response.GetMimeType() == "image/jpeg"
		})).
		Return(&port.DownloadPrepareOutput{Filename: "image.png", DestinationPath: destPath}).
		Once()
	saver := NewResolvedImageSaver(preparer, downloadDir)

	err := saver.SaveResolvedImage(context.Background(), entity.ImageData{Bytes: []byte{1, 2, 3, 4}, MimeType: "image/jpeg"}, port.MenuContext{ImageURI: "https://example.com/assets/image"})
	require.NoError(t, err)
	require.Equal(t, []byte{1, 2, 3, 4}, readFile(t, destPath))
}

func TestResolvedImageSaver_SaveResolvedImageRequiresDestination(t *testing.T) {
	preparer := portmocks.NewMockDownloadPreparer(t)
	preparer.EXPECT().
		Execute(mock.Anything, mock.Anything).
		Return(&port.DownloadPrepareOutput{Filename: "image.png"}).
		Once()
	saver := NewResolvedImageSaver(preparer, t.TempDir())

	err := saver.SaveResolvedImage(context.Background(), entity.ImageData{Bytes: []byte{1}}, port.MenuContext{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "destination path")
}

func TestResolvedImageSaver_SaveResolvedImageRejectsEmptyBytes(t *testing.T) {
	preparer := portmocks.NewMockDownloadPreparer(t)
	saver := NewResolvedImageSaver(preparer, t.TempDir())

	err := saver.SaveResolvedImage(context.Background(), entity.ImageData{}, port.MenuContext{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty image data")
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}
