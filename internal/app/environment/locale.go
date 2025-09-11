package environment

import (
	"log"
	"os"
	"os/exec"
	"strings"
)

// DetectAndSetKeyboardLocale attempts to determine the current keyboard layout
// and hints it to the webkit input layer for accelerator compatibility.
func DetectAndSetKeyboardLocale() {
	const colonSplitParts = 2 // Number of parts when splitting on colon

	// 0) Explicit override
	locale := os.Getenv("DUMBER_KEYBOARD_LOCALE")
	if locale == "" {
		locale = os.Getenv("DUMB_BROWSER_KEYBOARD_LOCALE") // legacy prefix
	}
	// 1) Environment
	if locale == "" {
		locale = os.Getenv("LC_ALL")
	}
	if locale == "" {
		locale = os.Getenv("LANG")
	}
	if locale == "" {
		locale = os.Getenv("LC_CTYPE")
	}
	// Trim variants
	if i := strings.IndexByte(locale, '.'); i > 0 {
		locale = locale[:i]
	}
	if i := strings.IndexByte(locale, '@'); i > 0 {
		locale = locale[:i]
	}
	if i := strings.IndexByte(locale, '_'); i > 0 {
		locale = locale[:i]
	}

	// 2) Try XKB env override
	if locale == "" {
		locale = os.Getenv("XKB_DEFAULT_LAYOUT")
	}

	// 3) Best-effort probe of setxkbmap/localectl (non-fatal)
	if locale == "" {
		if out, err := exec.Command("localectl", "status").Output(); err == nil {
			s := string(out)
			for _, line := range strings.Split(s, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(strings.ToLower(line), "x11 layout:") || strings.HasPrefix(strings.ToLower(line), "keyboard layout:") {
					parts := strings.SplitN(line, ":", colonSplitParts)
					if len(parts) == colonSplitParts {
						cand := strings.TrimSpace(parts[1])
						if cand != "" {
							locale = cand
						}
					}
					break
				}
			}
		}
	}
	if locale == "" {
		if out, err := exec.Command("setxkbmap", "-query").Output(); err == nil {
			s := string(out)
			for _, line := range strings.Split(s, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(strings.ToLower(line), "layout:") {
					parts := strings.SplitN(line, ":", colonSplitParts)
					if len(parts) == colonSplitParts {
						cand := strings.TrimSpace(parts[1])
						if cand != "" {
							locale = cand
						}
					}
					break
				}
			}
		}
	}

	if locale == "" {
		locale = "en"
	}
	// No layout-specific remaps; log for diagnostics only
	log.Printf("[locale] keyboard locale detected: %s", locale)
}
