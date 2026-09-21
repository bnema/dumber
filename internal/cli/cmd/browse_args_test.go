package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseBrowseArgs(t *testing.T) {
	for _, tt := range []struct {
		name, url, instance string
		args                []string
		ok                  bool
	}{
		{"normal", "", "", nil, true},
		{"named", "", "work", []string{"--instance", "work"}, true},
		{"url after", "https://example.com", "work", []string{"--instance", "work", "https://example.com"}, true},
		{"url before", "https://example.com", "work", []string{"https://example.com", "--instance", "work"}, true},
		{"missing name", "", "", []string{"--instance"}, false},
		{"empty equals", "", "", []string{"--instance="}, false},
		{"empty separate", "", "", []string{"--instance", ""}, false},
		{"help", "", "", []string{"--help"}, false},
		{"unknown", "", "", []string{"--unknown"}, false},
		{"extra", "", "", []string{"--instance", "work", "a", "b"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			url, instance, ok := ParseBrowseArgs(tt.args)
			require.Equal(t, tt.url, url)
			require.Equal(t, tt.instance, instance)
			require.Equal(t, tt.ok, ok)
		})
	}
	require.NotNil(t, browseCmd.Flags().Lookup("instance"))
	require.NotNil(t, browseCmd.Flags().Lookup("profile"))
	require.NotNil(t, browseCmd.Flags().Lookup("ephemeral"))
}

func TestParseBrowseLaunchArgsProfiles(t *testing.T) {
	parsed, ok := ParseBrowseLaunchArgs([]string{"--instance", "scratch", "https://example.com"})
	require.True(t, ok)
	require.Equal(t, BrowseArgs{URL: "https://example.com", Instance: "scratch"}, parsed)

	parsed, ok = ParseBrowseLaunchArgs([]string{"--instance", "work", "--profile", "work"})
	require.True(t, ok)
	require.Equal(t, BrowseArgs{Instance: "work", Profile: "work"}, parsed)

	parsed, ok = ParseBrowseLaunchArgs([]string{"--instance", "private", "--ephemeral"})
	require.True(t, ok)
	require.Equal(t, BrowseArgs{Instance: "private", Ephemeral: true}, parsed)

	_, ok = ParseBrowseLaunchArgs([]string{"--profile", "work", "--ephemeral"})
	require.False(t, ok)
}
