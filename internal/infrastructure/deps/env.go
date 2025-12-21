package deps

import (
	"os"
	"path/filepath"
	"strings"
)

// ApplyPrefixEnv prepends environment variables derived from prefix.
//
// This is used to support manual runtime installs under /opt.
func ApplyPrefixEnv(prefix string) {
	if strings.TrimSpace(prefix) == "" {
		return
	}

	for key, values := range prefixEnv(prefix) {
		_ = os.Setenv(key, prependPathList(os.Getenv(key), values...))
	}
}

// CommandEnvWithPrefix returns an environment suitable for exec.Cmd.Env.
// It prepends prefix-derived paths to the current environment.
func CommandEnvWithPrefix(prefix string) []string {
	if strings.TrimSpace(prefix) == "" {
		return os.Environ()
	}

	updates := prefixEnv(prefix)
	base := os.Environ()

	// Convert base env to map.
	m := make(map[string]string, len(base))
	for _, kv := range base {
		k, v, ok := strings.Cut(kv, "=")
		if ok {
			m[k] = v
		}
	}

	for k, values := range updates {
		m[k] = prependPathList(m[k], values...)
	}

	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

func prefixEnv(prefix string) map[string][]string {
	prefix = filepath.Clean(prefix)

	pkgConfig := []string{
		filepath.Join(prefix, "lib", "pkgconfig"),
		filepath.Join(prefix, "lib64", "pkgconfig"),
		filepath.Join(prefix, "share", "pkgconfig"),
		filepath.Join(prefix, "lib", "x86_64-linux-gnu", "pkgconfig"),
		filepath.Join(prefix, "lib64", "x86_64-linux-gnu", "pkgconfig"),
	}

	ldLibrary := []string{
		filepath.Join(prefix, "lib"),
		filepath.Join(prefix, "lib64"),
		filepath.Join(prefix, "lib", "x86_64-linux-gnu"),
		filepath.Join(prefix, "lib64", "x86_64-linux-gnu"),
	}

	giTypelib := []string{
		filepath.Join(prefix, "lib", "girepository-1.0"),
		filepath.Join(prefix, "lib64", "girepository-1.0"),
		filepath.Join(prefix, "lib", "x86_64-linux-gnu", "girepository-1.0"),
		filepath.Join(prefix, "lib64", "x86_64-linux-gnu", "girepository-1.0"),
	}

	xdgData := []string{
		filepath.Join(prefix, "share"),
	}

	return map[string][]string{
		"PKG_CONFIG_PATH": pkgConfig,
		"LD_LIBRARY_PATH": ldLibrary,
		"GI_TYPELIB_PATH": giTypelib,
		"XDG_DATA_DIRS":   xdgData,
	}
}

func prependPathList(existing string, values ...string) string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values)+4)

	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}

	for _, v := range values {
		add(v)
	}

	if existing != "" {
		for _, v := range strings.Split(existing, ":") {
			add(v)
		}
	}

	return strings.Join(out, ":")
}
