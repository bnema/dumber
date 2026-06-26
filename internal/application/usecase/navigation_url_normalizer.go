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

// BuildNavigationURL resolves local paths before applying bang shortcuts and search fallback.
func (n *NavigationURLNormalizer) BuildNavigationURL(
	ctx context.Context,
	input string,
	shortcutURLs map[string]string,
	defaultSearch string,
) string {
	return BuildNavigationURL(ctx, input, n.normalize, shortcutURLs, defaultSearch)
}

// BuildNavigationURL resolves a user navigation string using the shared CLI/UI policy.
func BuildNavigationURL(
	ctx context.Context,
	input string,
	normalize func(context.Context, string) string,
	shortcutURLs map[string]string,
	defaultSearch string,
) string {
	if _, _, found := domainurl.ParseBangShortcut(input); !found && normalize != nil {
		normalized := normalize(ctx, input)
		if normalized != input {
			return normalized
		}
	}
	return domainurl.BuildSearchURL(input, shortcutURLs, defaultSearch)
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
	if hasPreservedNavigationScheme(input) {
		return false
	}
	return true
}

func hasPreservedNavigationScheme(input string) bool {
	parsed, err := stdurl.Parse(input)
	if err != nil || parsed.Scheme == "" {
		return false
	}

	scheme := strings.ToLower(parsed.Scheme)
	lowerInput := strings.ToLower(input)

	switch scheme {
	case "http", "https", "dumb", "file", "vscode", "vscode-insiders", "spotify", "steam":
		return strings.HasPrefix(lowerInput, scheme+"://") && domainurl.Normalize(input) == input
	case "blob":
		return strings.HasPrefix(lowerInput, "blob:") && strings.Contains(lowerInput, "://") &&
			domainurl.Normalize(input) == input
	case "about", "data", "javascript", "mailto":
		return strings.HasPrefix(lowerInput, scheme+":") && domainurl.Normalize(input) == input
	default:
		return false
	}
}
