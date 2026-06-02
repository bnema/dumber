package ceffilter

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeRules(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rules.json")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

func TestMatcherBlocksMatchingNetworkRule(t *testing.T) {
	path := writeRules(t, `[
		{"trigger":{"url-filter":"ads\\.example\\.com","resource-type":["image"]},"action":{"type":"block"}}
	]`)
	matcher, err := NewMatcherFromFiles([]string{path})
	require.NoError(t, err)

	blocked := matcher.ShouldBlock(Request{URL: "https://ads.example.com/banner.png", ResourceType: ResourceTypeImage})
	allowed := matcher.ShouldBlock(Request{URL: "https://ads.example.com/app.js", ResourceType: ResourceTypeScript})

	assert.True(t, blocked)
	assert.False(t, allowed)
}

func TestMatcherReportsPartiallySkippedRules(t *testing.T) {
	path := writeRules(t, `[
		{"trigger":{"url-filter":"ads"},"action":{"type":"block"}},
		{"trigger":{"url-filter":"ads"},"action":{"type":"make-https"}},
		{"trigger":{"url-filter":"("},"action":{"type":"block"}}
	]`)
	matcher, err := NewMatcherFromFiles([]string{path})
	require.NoError(t, err)

	assert.Equal(t, 1, matcher.RuleCount())
	assert.Equal(t, 2, matcher.SkippedRuleCount())
	assert.True(t, matcher.ShouldBlock(Request{URL: "https://ads.test/banner"}))
}

func TestMatcherBypassesInternalAndNonHTTPURLs(t *testing.T) {
	path := writeRules(t, `[
		{"trigger":{"url-filter":".*"},"action":{"type":"block"}}
	]`)
	matcher, err := NewMatcherFromFiles([]string{path})
	require.NoError(t, err)

	assert.False(t, matcher.ShouldBlock(Request{URL: "dumb://history", ResourceType: ResourceTypeDocument}))
	assert.False(t, matcher.ShouldBlock(Request{URL: "https://dumber.invalid/history", ResourceType: ResourceTypeDocument}))
	assert.False(t, matcher.ShouldBlock(Request{URL: "data:text/plain,hello", ResourceType: ResourceTypeRaw}))
}

func TestMatcherHonorsDomainAndLoadTypeConstraints(t *testing.T) {
	path := writeRules(t, `[
		{"trigger":{"url-filter":"tracker","load-type":["third-party"],"if-domain":["news.example"]},"action":{"type":"block"}}
	]`)
	matcher, err := NewMatcherFromFiles([]string{path})
	require.NoError(t, err)

	assert.True(t, matcher.ShouldBlock(Request{
		URL:              "https://tracker.test/pixel",
		ResourceType:     ResourceTypeImage,
		RequestInitiator: "https://news.example/article",
	}))
	assert.False(t, matcher.ShouldBlock(Request{
		URL:              "https://tracker.test/pixel",
		ResourceType:     ResourceTypeImage,
		RequestInitiator: "https://blog.example/article",
	}))
	assert.False(t, matcher.ShouldBlock(Request{
		URL:              "https://cdn.news.example/tracker.js",
		ResourceType:     ResourceTypeScript,
		RequestInitiator: "https://news.example/article",
	}))
}

func TestMatcherHonorsUnlessDomain(t *testing.T) {
	path := writeRules(t, `[
		{"trigger":{"url-filter":"ads","unless-domain":["allowed.example"]},"action":{"type":"block"}}
	]`)
	matcher, err := NewMatcherFromFiles([]string{path})
	require.NoError(t, err)

	assert.False(t, matcher.ShouldBlock(Request{URL: "https://ads.test/banner", RequestInitiator: "https://allowed.example"}))
	assert.True(t, matcher.ShouldBlock(Request{URL: "https://ads.test/banner", RequestInitiator: "https://blocked.example"}))
}

func TestMatcherHonorsIgnorePreviousRulesExceptions(t *testing.T) {
	path := writeRules(t, `[
		{"trigger":{"url-filter":"ads"},"action":{"type":"block"}},
		{"trigger":{"url-filter":"ads","if-domain":["allowed.example"]},"action":{"type":"ignore-previous-rules"}}
	]`)
	matcher, err := NewMatcherFromFiles([]string{path})
	require.NoError(t, err)

	assert.False(t, matcher.ShouldBlock(Request{URL: "https://ads.test/banner", RequestInitiator: "https://allowed.example"}))
	assert.True(t, matcher.ShouldBlock(Request{URL: "https://ads.test/banner", RequestInitiator: "https://blocked.example"}))
}

func TestMatcherAppliesLaterBlockAfterIgnorePreviousRules(t *testing.T) {
	path := writeRules(t, `[
		{"trigger":{"url-filter":"ads"},"action":{"type":"block"}},
		{"trigger":{"url-filter":"ads"},"action":{"type":"ignore-previous-rules"}},
		{"trigger":{"url-filter":"critical-ads"},"action":{"type":"block"}}
	]`)
	matcher, err := NewMatcherFromFiles([]string{path})
	require.NoError(t, err)

	assert.False(t, matcher.ShouldBlock(Request{URL: "https://example.test/ads.js"}))
	assert.True(t, matcher.ShouldBlock(Request{URL: "https://example.test/critical-ads.js"}))
}

func TestMatcherDomainConstraintsAreOneWay(t *testing.T) {
	path := writeRules(t, `[
		{"trigger":{"url-filter":"ads","if-domain":["sub.example.com"]},"action":{"type":"block"}}
	]`)
	matcher, err := NewMatcherFromFiles([]string{path})
	require.NoError(t, err)

	assert.False(t, matcher.ShouldBlock(Request{URL: "https://ads.test/banner", RequestInitiator: "https://example.com"}))
	assert.True(t, matcher.ShouldBlock(Request{URL: "https://ads.test/banner", RequestInitiator: "https://sub.example.com"}))
	assert.True(t, matcher.ShouldBlock(Request{URL: "https://ads.test/banner", RequestInitiator: "https://deep.sub.example.com"}))
}

func TestMatcherUnlessDomainConstraintsAreOneWay(t *testing.T) {
	path := writeRules(t, `[
		{"trigger":{"url-filter":"ads","unless-domain":["sub.example.com"]},"action":{"type":"block"}}
	]`)
	matcher, err := NewMatcherFromFiles([]string{path})
	require.NoError(t, err)

	assert.True(t, matcher.ShouldBlock(Request{URL: "https://ads.test/banner", RequestInitiator: "https://example.com"}))
	assert.False(t, matcher.ShouldBlock(Request{URL: "https://ads.test/banner", RequestInitiator: "https://sub.example.com"}))
	assert.False(t, matcher.ShouldBlock(Request{URL: "https://ads.test/banner", RequestInitiator: "https://deep.sub.example.com"}))
}

func TestBackendActivatesMatcherFromFiles(t *testing.T) {
	path := writeRules(t, `[
		{"trigger":{"url-filter":"ads"},"action":{"type":"block"}}
	]`)
	backend := NewBackend()

	require.False(t, backend.HasActive())
	require.NoError(t, backend.ActivateFiles(context.Background(), []string{path}))
	require.True(t, backend.HasActive())
	assert.True(t, backend.ShouldBlock(Request{URL: "https://example.test/ads.js"}))

	require.NoError(t, backend.Clear(context.Background()))
	assert.False(t, backend.HasActive())
	assert.False(t, backend.ShouldBlock(Request{URL: "https://example.test/ads.js"}))
}
