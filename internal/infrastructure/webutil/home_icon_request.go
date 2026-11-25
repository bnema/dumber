package webutil

import (
	"errors"
	"net/url"
	"strings"

	"bnema.dev/dumber/internal/domain/entity"
)

// ErrHomeIconInvalidService reports a missing or malformed service id.
var ErrHomeIconInvalidService = errors.New("invalid home icon service id")

// ParseHomeIconRequest extracts and validates the service id from a
// dumb://api/home-icon?service=<id> request URL.
func ParseHomeIconRequest(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", ErrHomeIconInvalidService
	}
	id := strings.TrimSpace(parsed.Query().Get("service"))
	if !entity.IsValidHomeID(id) {
		return "", ErrHomeIconInvalidService
	}
	return id, nil
}
