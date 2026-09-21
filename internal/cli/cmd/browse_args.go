package cmd

import (
	"io"

	"github.com/spf13/pflag"
)

type BrowseArgs struct {
	URL       string
	Instance  string
	Profile   string
	Ephemeral bool
}

func browseFlags() *pflag.FlagSet {
	flags := pflag.NewFlagSet("browse", pflag.ContinueOnError)
	flags.String("instance", "", "Open a dedicated window in the named logical instance")
	flags.String("profile", "", "Use an isolated persistent CEF profile")
	flags.Bool("ephemeral", false, "Use a fresh temporary CEF profile for this process")
	return flags
}

// ParseBrowseLaunchArgs recognizes GUI launches. Help and invalid input are left to Cobra.
func ParseBrowseLaunchArgs(args []string) (BrowseArgs, bool) {
	var result BrowseArgs
	flags := browseFlags()
	flags.SetOutput(io.Discard)
	if err := flags.Parse(args); err != nil || flags.NArg() > 1 {
		return result, false
	}
	result.Instance, _ = flags.GetString("instance")
	result.Profile, _ = flags.GetString("profile")
	result.Ephemeral, _ = flags.GetBool("ephemeral")
	if flags.Changed("instance") && result.Instance == "" {
		return BrowseArgs{}, false
	}
	if flags.Changed("profile") && result.Profile == "" {
		return BrowseArgs{}, false
	}
	if result.Profile != "" && result.Ephemeral {
		return BrowseArgs{}, false
	}
	if flags.NArg() == 1 {
		result.URL = flags.Arg(0)
	}
	return result, true
}

// ParseBrowseArgs is the compatibility projection used by existing early launch parsing.
func ParseBrowseArgs(args []string) (url, instance string, ok bool) {
	parsed, ok := ParseBrowseLaunchArgs(args)
	return parsed.URL, parsed.Instance, ok
}
