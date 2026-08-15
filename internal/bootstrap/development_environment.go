package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	developmentEnvironmentName             = "dev"
	developmentPrivateMode     os.FileMode = 0o700
)

// ApplyDevelopmentEnvironment isolates the process from the user's XDG and
// home directories when ENV=dev. It must run before GTK, CEF, or any adapter
// can initialize and read process environment settings.
func ApplyDevelopmentEnvironment() error {
	if os.Getenv("ENV") != developmentEnvironmentName {
		return nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve development environment root: %w", err)
	}

	devDir := filepath.Join(cwd, ".dev")
	if err := ensureDevelopmentDirectory(devDir, false); err != nil {
		return err
	}

	root := filepath.Join(devDir, "dumber")
	paths := map[string]string{
		"HOME":            filepath.Join(root, "home"),
		"XDG_CONFIG_HOME": filepath.Join(root, "config"),
		"XDG_DATA_HOME":   filepath.Join(root, "data"),
		"XDG_STATE_HOME":  filepath.Join(root, "state"),
		"XDG_CACHE_HOME":  filepath.Join(root, "cache"),
	}
	for _, path := range developmentPrivateDirectories(root, paths) {
		if err := ensureDevelopmentDirectory(path, true); err != nil {
			return err
		}
	}
	for name, path := range paths {
		if err := os.Setenv(name, path); err != nil {
			return fmt.Errorf("set %s for development environment: %w", name, err)
		}
	}

	return nil
}

// developmentPrivateDirectories returns every currently known path that Dumber
// creates below the development sandbox. Creating and validating them before
// GTK, CEF, logging, or the browser profile starts prevents those subsystems
// from following a pre-existing sandbox symlink.
func developmentPrivateDirectories(root string, paths map[string]string) []string {
	enginesDir := filepath.Join(root, "engines")
	cefDir := filepath.Join(enginesDir, "cef")
	webkitDir := filepath.Join(enginesDir, "webkit")
	runtimeDir := filepath.Join(root, "runtime")
	return []string{
		root,
		paths["HOME"],
		paths["XDG_CONFIG_HOME"],
		paths["XDG_DATA_HOME"],
		paths["XDG_STATE_HOME"],
		paths["XDG_CACHE_HOME"],
		filepath.Join(root, "logs"),
		enginesDir,
		cefDir,
		filepath.Join(cefDir, "data"),
		filepath.Join(cefDir, "runtime"),
		filepath.Join(cefDir, "logs"),
		webkitDir,
		filepath.Join(webkitDir, "data"),
		filepath.Join(webkitDir, "cache"),
		filepath.Join(webkitDir, "runtime"),
		filepath.Join(webkitDir, "logs"),
		runtimeDir,
		filepath.Join(runtimeDir, "cef"),
		filepath.Join(runtimeDir, "webkit"),
	}
}

func ensureDevelopmentDirectory(path string, private bool) error {
	if err := os.MkdirAll(path, developmentPrivateMode); err != nil {
		return fmt.Errorf("create development environment directory %q: %w", path, err)
	}

	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect development environment directory %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("development environment path %q must be a directory, not %s", path, info.Mode().Type())
	}
	if !private {
		return nil
	}
	if err = os.Chmod(path, developmentPrivateMode); err != nil {
		return fmt.Errorf("restrict development environment directory %q: %w", path, err)
	}
	info, err = os.Stat(path)
	if err != nil {
		return fmt.Errorf("verify development environment directory %q: %w", path, err)
	}
	if info.Mode().Perm() != developmentPrivateMode.Perm() {
		return fmt.Errorf("development environment directory %q permissions are %04o, want 0700", path, info.Mode().Perm())
	}
	return nil
}
