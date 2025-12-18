// Package build provides domain entities for build information.
package build

// Info holds build-time information injected via ldflags.
type Info struct {
	Version   string
	Commit    string
	BuildDate string
	GoVersion string
}

// Contributors returns the list of project contributors.
func Contributors() []string {
	return []string{"bnema"}
}

// RepoURL returns the GitHub repository URL.
func RepoURL() string {
	return "https://github.com/bnema/dumber"
}
