package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bnema/dumber/assets"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/infrastructure/desktop"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Setup desktop integration",
	Long: `Setup dumber's integration with the desktop environment.

Subcommands:
  install  - Install desktop file to ~/.local/share/applications/
  default  - Set dumber as the default web browser`,
}

var setupInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install desktop file",
	Long: `Install the dumber.desktop file to the user's applications directory.

This registers dumber with the desktop environment, making it appear in
application menus and available as a browser choice.

Location: $XDG_DATA_HOME/applications/dumber.desktop
         (typically ~/.local/share/applications/dumber.desktop)

This command is idempotent - safe to run multiple times.`,
	RunE: runSetupInstall,
}

var setupDefaultCmd = &cobra.Command{
	Use:   "default",
	Short: "Set dumber as default browser",
	Long: `Set dumber as the default web browser using xdg-settings.

This makes dumber handle http:// and https:// links from other applications.

Requires the desktop file to be installed first (run 'dumber setup install').

This command is idempotent - safe to run multiple times.`,
	RunE: runSetupDefault,
}

func init() {
	rootCmd.AddCommand(setupCmd)
	setupCmd.AddCommand(setupInstallCmd)
	setupCmd.AddCommand(setupDefaultCmd)
}

func runSetupInstall(cmd *cobra.Command, args []string) error {
	app := GetApp()
	if app == nil {
		return fmt.Errorf("app not initialized")
	}

	theme := app.Theme
	adapter := desktop.New()
	uc := usecase.NewInstallDesktopUseCase(adapter)

	input := usecase.InstallDesktopInput{
		IconData: assets.LogoSVG,
	}

	result, err := uc.Execute(app.Ctx(), input)
	if err != nil {
		fmt.Printf("%s %s\n", theme.ErrorStyle.Render("\u2717"), err.Error())
		return err
	}

	// Desktop file status
	if result.WasDesktopExisting {
		fmt.Printf("%s Desktop file updated at %s\n",
			theme.SuccessStyle.Render("\u2713"),
			theme.Highlight.Render(result.DesktopPath))
	} else {
		fmt.Printf("%s Desktop file installed to %s\n",
			theme.SuccessStyle.Render("\u2713"),
			theme.Highlight.Render(result.DesktopPath))
	}

	// Icon status
	if result.IconPath != "" {
		if result.WasIconExisting {
			fmt.Printf("%s Icon updated at %s\n",
				theme.SuccessStyle.Render("\u2713"),
				theme.Highlight.Render(result.IconPath))
		} else {
			fmt.Printf("%s Icon installed to %s\n",
				theme.SuccessStyle.Render("\u2713"),
				theme.Highlight.Render(result.IconPath))
		}
	}

	fmt.Println()
	fmt.Println(theme.Subtle.Render("Run 'dumber setup default' to set as default browser"))

	return nil
}

func runSetupDefault(cmd *cobra.Command, args []string) error {
	app := GetApp()
	if app == nil {
		return fmt.Errorf("app not initialized")
	}

	theme := app.Theme
	adapter := desktop.New()
	uc := usecase.NewSetDefaultBrowserUseCase(adapter)

	result, err := uc.Execute(app.Ctx())
	if err != nil {
		errMsg := err.Error()
		fmt.Printf("%s %s\n", theme.ErrorStyle.Render("\u2717"), errMsg)
		if strings.Contains(errMsg, "desktop file not installed") {
			fmt.Println()
			fmt.Println(theme.Subtle.Render("Hint: Run 'dumber setup install' first"))
		}
		return err
	}

	if result.WasAlreadyDefault {
		fmt.Printf("%s Dumber is already the default browser\n",
			theme.SuccessStyle.Render("\u2713"))
	} else {
		fmt.Printf("%s Dumber is now the default browser\n",
			theme.SuccessStyle.Render("\u2713"))
	}

	return nil
}
