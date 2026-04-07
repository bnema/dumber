package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
)

const layerShellLibrarySubstring = "libgtk4-layer-shell"

var layerShellLibraryCandidates = []string{
	"/usr/lib/libgtk4-layer-shell.so.0",
	"/usr/lib64/libgtk4-layer-shell.so.0",
	"/usr/lib/aarch64-linux-gnu/libgtk4-layer-shell.so.0",
	"/usr/lib/x86_64-linux-gnu/libgtk4-layer-shell.so.0",
	"/lib/x86_64-linux-gnu/libgtk4-layer-shell.so.0",
}

func ShouldPreloadLayerShell(env map[string]string) bool {
	if env["WAYLAND_DISPLAY"] == "" {
		return false
	}

	return !LayerShellPreloadPresent(env)
}

func LayerShellPreloadPresent(env map[string]string) bool {
	return strings.Contains(env["LD_PRELOAD"], layerShellLibrarySubstring)
}

func CurrentEnvMap() map[string]string {
	env := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		env[key] = value
	}
	return env
}

func FindLayerShellLibrary(env map[string]string, exists func(path string) bool) string {
	for _, path := range layerShellLibrarySearchPaths(env) {
		if exists(path) {
			return path
		}
	}

	return ""
}

func LayerShellLibraryPath() string {
	return FindLayerShellLibrary(CurrentEnvMap(), func(path string) bool {
		info, err := os.Stat(filepath.Clean(path))
		return err == nil && !info.IsDir()
	})
}

func layerShellLibrarySearchPaths(env map[string]string) []string {
	paths := make([]string, 0, len(layerShellLibraryCandidates)+4)
	paths = append(paths, layerShellLibraryCandidates...)

	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		seen[path] = struct{}{}
	}

	for _, dir := range strings.FieldsFunc(env["LD_LIBRARY_PATH"], func(r rune) bool { return r == ':' }) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, "libgtk4-layer-shell.so.0")
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		paths = append(paths, candidate)
	}

	return paths
}

func LayerShellPreloadEnv(env map[string]string, libraryPath string) map[string]string {
	cloned := make(map[string]string, len(env)+1)
	for key, value := range env {
		cloned[key] = value
	}

	if libraryPath == "" {
		return cloned
	}

	if current := cloned["LD_PRELOAD"]; current != "" {
		cloned["LD_PRELOAD"] = libraryPath + " " + current
		return cloned
	}

	cloned["LD_PRELOAD"] = libraryPath
	return cloned
}
