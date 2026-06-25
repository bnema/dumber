package usecase

import (
	"context"
	stdurl "net/url"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
	domainurl "github.com/bnema/dumber/internal/domain/url"
)

// NavigationURLNormalizer normalizes user navigation input while keeping
// filesystem probing behind an application port.
type NavigationURLNormalizer struct {
	localPaths port.LocalPathResolver
}

// NewNavigationURLNormalizer creates a navigation URL normalizer.
func NewNavigationURLNormalizer(localPaths port.LocalPathResolver) *NavigationURLNormalizer {
	return &NavigationURLNormalizer{localPaths: localPaths}
}

// Normalize converts existing local paths to file:// URLs, then falls back to
// the pure domain URL normalizer for URL-like decisions.
func (n *NavigationURLNormalizer) Normalize(ctx context.Context, input string) string {
	if input == "" {
		return ""
	}
	if n != nil && n.localPaths != nil && shouldProbeLocalPath(input) {
		if absPath, ok, err := n.localPaths.ResolveExistingPath(ctx, input); err == nil && ok {
			return (&stdurl.URL{Scheme: "file", Path: absPath}).String()
		}
	}
	return domainurl.Normalize(input)
}

func (n *NavigationURLNormalizer) normalize(ctx context.Context, input string) string {
	if n == nil {
		return domainurl.Normalize(input)
	}
	return n.Normalize(ctx, input)
}

func shouldProbeLocalPath(input string) bool {
	if input == "" {
		return false
	}
	if strings.Contains(input, "://") || strings.HasPrefix(input, "about:") || domainurl.IsExternalScheme(input) {
		return false
	}
	return true
}
