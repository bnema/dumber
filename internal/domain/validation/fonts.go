package validation

import "strings"

func ValidateFontFamily(field string, value string) []string {
	value = strings.TrimSpace(value)
	var errs []string

	if value == "" {
		errs = append(errs, field+" cannot be empty")
		return errs
	}

	if strings.ContainsAny(value, "\r\n") {
		errs = append(errs, field+" must not contain newlines")
	}

	if len(value) > 200 {
		errs = append(errs, field+" is too long")
	}

	return errs
}
