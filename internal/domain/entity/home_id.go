package entity

// IsValidHomeID reports whether id matches the home.toml id charset
// (lowercase letters, digits, '-' and '_', 1 to 64 characters).
func IsValidHomeID(id string) bool {
	return homeIDPattern.MatchString(id)
}
