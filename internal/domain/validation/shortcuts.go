package validation

import (
	"net/url"
	"regexp"
	"strings"
)

var shortcutKeyRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,19}$`)

func ValidateShortcutKey(value string) []string {
	var errs []string
	value = strings.TrimSpace(value)
	if value == "" {
		errs = append(errs, "shortcut key cannot be empty")
		return errs
	}
	if !shortcutKeyRE.MatchString(value) {
		errs = append(errs, "shortcut key must start with a letter and be 1-20 alphanumeric characters")
	}
	return errs
}

func ValidateShortcutURL(value string) []string {
	var errs []string
	value = strings.TrimSpace(value)
	if value == "" {
		errs = append(errs, "shortcut url cannot be empty")
		return errs
	}
	if !strings.Contains(value, "%s") {
		errs = append(errs, "shortcut url must contain %s placeholder for the search query")
		return errs
	}

	candidate := strings.ReplaceAll(value, "%s", "query")
	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		errs = append(errs, "shortcut url must be a valid absolute URL")
	}

	return errs
}

func ValidateShortcutDescription(value string) []string {
	var errs []string
	value = strings.TrimSpace(value)
	if strings.ContainsAny(value, "\r\n") {
		errs = append(errs, "shortcut description must not contain newlines")
	}
	if len(value) > 200 {
		errs = append(errs, "shortcut description is too long")
	}
	return errs
}
