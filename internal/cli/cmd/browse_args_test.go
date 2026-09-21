package cmd

import (
	"io"
	"testing"

	"github.com/spf13/cobra"
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
		{"flag-like name", "", "", []string{"--instance", "--ephemeral"}, false},
		{"flag-like profile", "", "", []string{"--profile", "--ephemeral"}, false},
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

	_, ok = ParseBrowseLaunchArgs([]string{"--profile", "--ephemeral"})
	require.False(t, ok, "a flag-like profile must not be treated as a profile name")

	_, ok = ParseBrowseLaunchArgs([]string{"--instance", "--ephemeral"})
	require.False(t, ok, "a flag-like instance must not be treated as an instance name")
}

// browsePreRunEError exercises the real browse PreRunE through Cobra flag
// parsing, which is the fallback path main takes when early launch parsing
// rejects the arguments.
func browsePreRunEError(t *testing.T, args []string) error {
	t.Helper()
	cmd := &cobra.Command{
		Use:           "browse",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		PreRunE:       browseCmd.PreRunE,
		Run:           func(*cobra.Command, []string) {},
	}
	cmd.Flags().AddFlagSet(browseFlags())
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestBrowseCommandPreRunERejectsFlagLikeValues(t *testing.T) {
	require.Error(t, browsePreRunEError(t, []string{"--instance", "--ephemeral"}))
	require.Error(t, browsePreRunEError(t, []string{"--profile", "--ephemeral"}))
	require.Error(t, browsePreRunEError(t, []string{"--instance", ""}))
	require.Error(t, browsePreRunEError(t, []string{"--profile", ""}))

	require.NoError(t, browsePreRunEError(t, []string{"--instance", "work"}))
	require.NoError(t, browsePreRunEError(t, []string{"--instance", "private", "--ephemeral"}))
	require.NoError(t, browsePreRunEError(t, []string{"--profile", "work"}))
}
