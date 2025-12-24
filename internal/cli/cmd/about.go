package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/cli"
	"github.com/bnema/dumber/internal/cli/styles"
	"github.com/bnema/dumber/internal/infrastructure/updater"
	"github.com/bnema/dumber/internal/logging"
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

func runAbout(_ *cobra.Command, _ []string) error {
	app := GetApp()
	if app == nil {
		return fmt.Errorf("app not initialized")
	}

	renderer := styles.NewAboutRenderer(app.Theme)
	output := renderer.Render(app.BuildInfo)

	fmt.Println(output)

	// Check for updates
	if err := checkForUpdates(app); err != nil {
		// Log but don't fail - update check is optional
		ctx := logging.WithContext(context.Background(), logging.NewFromConfigValues("warn", "text"))
		log := logging.FromContext(ctx)
		log.Debug().Err(err).Msg("update check failed")
	}

	return nil
}

func checkForUpdates(app *cli.App) error {
	ctx := context.Background()

	// Create update checker
	checker := updater.NewGitHubChecker()
	applier, err := updater.NewApplierFromXDG()
	if err != nil {
		return fmt.Errorf("failed to create applier: %w", err)
	}

	checkUC := usecase.NewCheckUpdateUseCase(checker, applier, app.BuildInfo)

	result, err := checkUC.Execute(ctx, usecase.CheckUpdateInput{})
	if err != nil {
		return err
	}

	if result.UpdateAvailable {
		fmt.Println()
		fmt.Printf("  Update available: %s -> %s\n", result.CurrentVersion, result.LatestVersion)
		fmt.Printf("     %s\n", result.ReleaseURL)
	}

	return nil
}
