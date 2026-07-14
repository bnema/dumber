package egl

import (
	"sort"
	"strings"
)

const ExtensionDMABUFImport = "EGL_EXT_image_dma_buf_import"

// Extensions is a parsed EGL extension list.
type Extensions map[string]struct{}

// ParseExtensions parses the space-separated string returned by eglQueryString.
func ParseExtensions(s string) Extensions {
	exts := make(Extensions)
	for _, name := range strings.Fields(s) {
		exts[name] = struct{}{}
	}
	return exts
}

func (e Extensions) Has(name string) bool {
	if e == nil || name == "" {
		return false
	}
	_, ok := e[name]
	return ok
}

func (e Extensions) Missing(required ...string) []string {
	var missing []string
	for _, name := range required {
		if !e.Has(name) {
			missing = append(missing, name)
		}
	}
	return missing
}

func (e Extensions) Names() []string {
	names := make([]string, 0, len(e))
	for name := range e {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
