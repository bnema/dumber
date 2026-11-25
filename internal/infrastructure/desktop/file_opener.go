package desktop

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"bnema.dev/dumber/internal/application/port"
)

// FileOpener opens local files with xdg-open.
type FileOpener struct{}

var _ port.FileOpener = FileOpener{}

// OpenFile starts xdg-open detached on an absolute local path. Callers must
// pass a native path, never one supplied by a page.
func (FileOpener) OpenFile(_ context.Context, path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("open file: path %q is not absolute", path)
	}
	cmd := exec.Command("xdg-open", path)
	cmd.Env = sanitizedChildEnv(os.Environ())
	if err := startDetachedProcess(cmd); err != nil {
		return fmt.Errorf("xdg-open %s: %w", path, err)
	}
	return nil
}
