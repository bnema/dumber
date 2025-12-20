package validation

import "regexp"

var hexColorRE = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func IsHexColor(value string) bool {
	return hexColorRE.MatchString(value)
}

func ValidatePaletteHex(
	prefix string,
	background string,
	surface string,
	surfaceVariant string,
	text string,
	muted string,
	accent string,
	border string,
) []string {
	var errs []string

	if !IsHexColor(background) {
		errs = append(errs, prefix+".background must be a hex color like #RRGGBB")
	}
	if !IsHexColor(surface) {
		errs = append(errs, prefix+".surface must be a hex color like #RRGGBB")
	}
	if !IsHexColor(surfaceVariant) {
		errs = append(errs, prefix+".surface_variant must be a hex color like #RRGGBB")
	}
	if !IsHexColor(text) {
		errs = append(errs, prefix+".text must be a hex color like #RRGGBB")
	}
	if !IsHexColor(muted) {
		errs = append(errs, prefix+".muted must be a hex color like #RRGGBB")
	}
	if !IsHexColor(accent) {
		errs = append(errs, prefix+".accent must be a hex color like #RRGGBB")
	}
	if !IsHexColor(border) {
		errs = append(errs, prefix+".border must be a hex color like #RRGGBB")
	}

	return errs
}
