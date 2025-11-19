package cli

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/webext"
	"github.com/spf13/cobra"
)

func newExtensionsAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <path-or-url>",
		Short: "Add/install an extension from a local path or URL (.zip)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			source := args[0]

			cli, err := NewCLI()
			if err != nil {
				return fmt.Errorf("failed to initialize CLI: %w", err)
			}
			defer cli.Close()

			manager, err := buildExtensionManager(cli)
			if err != nil {
				return err
			}

			dataDir, err := config.GetDataDir()
			if err != nil {
				return fmt.Errorf("failed to locate data dir: %w", err)
			}
			extensionsDir := filepath.Join(dataDir, "extensions")

			srcDir, cleanup, err := prepareSourceDir(source)
			if cleanup != nil {
				defer cleanup()
			}
			if err != nil {
				return err
			}

			manifestPath := filepath.Join(srcDir, "manifest.json")
			manifest, err := webext.LoadManifest(manifestPath)
			if err != nil {
				return fmt.Errorf("invalid manifest: %w", err)
			}

			extID := deriveExtensionID(manifest, srcDir)
			destDir := filepath.Join(extensionsDir, extID)
			if _, err := os.Stat(destDir); err == nil {
				return fmt.Errorf("extension already exists at %s", destDir)
			}

			if err := copyDir(srcDir, destDir); err != nil {
				return fmt.Errorf("failed to install extension: %w", err)
			}

			// Load the new extension to persist into DB.
			if err := manager.LoadExtensions(extensionsDir); err != nil {
				return fmt.Errorf("failed to load installed extension: %w", err)
			}

			fmt.Printf("Installed %s (%s) as %s\n", manifest.Name, manifest.Version, extID)
			return nil
		},
	}

	return cmd
}

func prepareSourceDir(source string) (string, func(), error) {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		tmpFile, err := os.CreateTemp("", "dumber-ext-*.zip")
		if err != nil {
			return "", nil, err
		}
		urlCleanup := func() { os.Remove(tmpFile.Name()) }

		if err := downloadToFile(source, tmpFile); err != nil {
			urlCleanup()
			return "", nil, err
		}
		tmpFile.Close()

		dir, cleanup, err := unzipToTemp(tmpFile.Name())
		if err != nil {
			urlCleanup()
			return "", nil, err
		}
		return dir, func() {
			cleanup()
			urlCleanup()
		}, nil
	}

	// Local path
	info, err := os.Stat(source)
	if err != nil {
		return "", nil, err
	}
	if info.IsDir() {
		return source, nil, nil
	}
	if strings.HasSuffix(strings.ToLower(source), ".zip") {
		dir, cleanup, err := unzipToTemp(source)
		return dir, cleanup, err
	}

	return "", nil, errors.New("source must be a directory or .zip file")
}

func downloadToFile(url string, f *os.File) error {
	resp, err := http.Get(url) //nolint:gosec // user-provided URL expected
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	_, err = io.Copy(f, resp.Body)
	return err
}

func unzipToTemp(zipPath string) (string, func(), error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", nil, err
	}
	defer r.Close()

	tmpDir, err := os.MkdirTemp("", "dumber-ext-")
	if err != nil {
		return "", nil, err
	}

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		dstPath := filepath.Join(tmpDir, f.Name)
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return "", nil, err
		}

		rc, err := f.Open()
		if err != nil {
			return "", nil, err
		}
		dst, err := os.Create(dstPath)
		if err != nil {
			rc.Close()
			return "", nil, err
		}
		if _, err := io.Copy(dst, rc); err != nil {
			rc.Close()
			dst.Close()
			return "", nil, err
		}
		rc.Close()
		dst.Chmod(f.Mode())
		dst.Close()
	}

	return tmpDir, func() { os.RemoveAll(tmpDir) }, nil
}

func deriveExtensionID(manifest *webext.Manifest, srcDir string) string {
	if manifest != nil && manifest.Name != "" {
		return slugify(manifest.Name)
	}
	return slugify(filepath.Base(srcDir))
}

func slugify(s string) string {
	s = strings.ToLower(s)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "ext"
	}
	return s
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		dstFile, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(dstFile, srcFile); err != nil {
			dstFile.Close()
			return err
		}
		if err := dstFile.Chmod(info.Mode()); err != nil {
			dstFile.Close()
			return err
		}
		return dstFile.Close()
	})
}
