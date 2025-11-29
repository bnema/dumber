package environment

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/bnema/dumber/internal/logging"
)

// DetectAndSetKeyboardLocale attempts to determine the current keyboard layout
// and hints it to the webkit input layer for accelerator compatibility.
func DetectAndSetKeyboardLocale() {
	const colonSplitParts = 2 // Number of parts when splitting on colon

	var locale string

	// 0) Explicit override
	locale = os.Getenv("DUMBER_KEYBOARD_LOCALE")
	if locale == "" {
		locale = os.Getenv("DUMB_BROWSER_KEYBOARD_LOCALE") // legacy prefix
	}

	// 1) XKB env override (actual keyboard layout)
	if locale == "" {
		locale = os.Getenv("XKB_DEFAULT_LAYOUT")
	}

	// 2) Best-effort probe of localectl/setxkbmap (actual keyboard layout)
	// Check these BEFORE LANG since LANG is system language, not keyboard layout
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
							logging.Debug(fmt.Sprintf("[locale] keyboard layout from localectl: %s", locale))
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
							logging.Debug(fmt.Sprintf("[locale] keyboard layout from setxkbmap: %s", locale))
						}
					}
					break
				}
			}
		}
	}

	// 3) Fallback to system locale (language, not keyboard layout)
	if locale == "" {
		locale = os.Getenv("LC_ALL")
	}
	if locale == "" {
		locale = os.Getenv("LANG")
	}
	if locale == "" {
		locale = os.Getenv("LC_CTYPE")
	}
	// Trim variants from locale
	if locale != "" {
		if i := strings.IndexByte(locale, '.'); i > 0 {
			locale = locale[:i]
		}
		if i := strings.IndexByte(locale, '@'); i > 0 {
			locale = locale[:i]
		}
		if i := strings.IndexByte(locale, '_'); i > 0 {
			locale = locale[:i]
		}
		logging.Debug(fmt.Sprintf("[locale] keyboard layout from system locale: %s", locale))
	}

	if locale == "" {
		locale = "en"
		logging.Debug(fmt.Sprintf("[locale] keyboard layout defaulted to: %s", locale))
	}

	logging.Info(fmt.Sprintf("[locale] keyboard locale detected: %s", locale))
}
