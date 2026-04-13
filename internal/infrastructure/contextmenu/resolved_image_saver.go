package contextmenu

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bnema/dumber/internal/application/port"
)

const (
	resolvedImageDirPerm  = 0o755
	resolvedImageFilePerm = 0o644
)

// ResolvedImageSaver prepares a download destination and writes resolved image bytes to disk.
type ResolvedImageSaver struct {
	preparer    port.DownloadPreparer
	downloadDir string
	menuCtx     port.MenuContext
}

// NewResolvedImageSaver creates a new ResolvedImageSaver helper.
func NewResolvedImageSaver(preparer port.DownloadPreparer, downloadDir string, menuCtx port.MenuContext) *ResolvedImageSaver {
	return &ResolvedImageSaver{
		preparer:    preparer,
		downloadDir: downloadDir,
		menuCtx:     menuCtx,
	}
}

// SaveResolvedImage resolves a destination path and writes the image bytes.
func (s *ResolvedImageSaver) SaveResolvedImage(ctx context.Context, image port.ImageData) error {
	if s == nil || s.preparer == nil {
		return fmt.Errorf("resolved image saver: download preparer not available")
	}

	output := s.preparer.Execute(ctx, port.DownloadPrepareInput{
		DownloadDir: s.downloadDir,
		// Leave SuggestedFilename empty so DownloadPreparer can fall back to
		// URI-derived naming when no explicit filename is available.
		Response: downloadResponse{
			mimeType: "image/png",
			uri:      s.menuCtx.ImageURI,
		},
	})
	if output == nil || output.DestinationPath == "" {
		return fmt.Errorf("resolved image saver: destination path not prepared")
	}
	if len(image.Bytes) == 0 {
		return fmt.Errorf("resolved image saver: empty image data")
	}

	if err := os.MkdirAll(filepath.Dir(output.DestinationPath), resolvedImageDirPerm); err != nil {
		return err
	}
	if err := os.WriteFile(output.DestinationPath, image.Bytes, resolvedImageFilePerm); err != nil {
		return err
	}
	return nil
}

type downloadResponse struct {
	mimeType          string
	suggestedFilename string
	uri               string
}

func (r downloadResponse) GetMimeType() string          { return r.mimeType }
func (r downloadResponse) GetSuggestedFilename() string { return r.suggestedFilename }
func (r downloadResponse) GetUri() string               { return r.uri }
