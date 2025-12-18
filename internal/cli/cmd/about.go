package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bnema/dumber/internal/cli/styles"
)

var aboutCmd = &cobra.Command{
	Use:   "about",
	Short: "Show version and build information",
	Long:  `Display version, build info, repository URL, and contributors.`,
	RunE:  runAbout,
}

func init() {
	rootCmd.AddCommand(aboutCmd)
}

func runAbout(cmd *cobra.Command, args []string) error {
	app := GetApp()
	if app == nil {
		return fmt.Errorf("app not initialized")
	}

	renderer := styles.NewAboutRenderer(app.Theme)
	output := renderer.Render(app.BuildInfo)

	fmt.Println(output)
	return nil
}
