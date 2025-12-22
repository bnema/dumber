package cmd

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/cli"
	"github.com/bnema/dumber/internal/cli/model"
	"github.com/bnema/dumber/internal/cli/styles"
	"github.com/bnema/dumber/internal/infrastructure/desktop"
	"github.com/bnema/dumber/internal/infrastructure/filesystem"
	xdgadapter "github.com/bnema/dumber/internal/infrastructure/xdg"
)

var purgeForce bool

var purgeCmd = &cobra.Command{
	Use:   "purge",
	Short: "Remove dumber data and configuration",
	Long: `Interactively select and remove dumber data directories and files.

This can remove:
  - Config directory
  - Data directory
  - State directory
  - Cache directory
  - Content filter caches
  - Desktop integration files

Use --force to remove everything without prompting.`,
	RunE: runPurge,
}

func init() {
	rootCmd.AddCommand(purgeCmd)
	purgeCmd.Flags().BoolVarP(&purgeForce, "force", "f", false, "remove all items without prompting")
}

func runPurge(_ *cobra.Command, _ []string) error {
	app := GetApp()
	if app == nil {
		return fmt.Errorf("app not initialized")
	}

	fsAdapter := filesystem.New()
	xdgAdapter := xdgadapter.New()
	desktopAdapter := desktop.New()
	purgeUC := usecase.NewPurgeDataUseCase(fsAdapter, xdgAdapter, desktopAdapter)

	if purgeForce {
		return runPurgeForce(app, purgeUC)
	}

	m := model.NewPurgeModel(app.Theme, purgeUC)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func runPurgeForce(app *cli.App, purgeUC *usecase.PurgeDataUseCase) error {
	out, err := purgeUC.PurgeAll(app.Ctx())
	if out != nil {
		for _, r := range out.Results {
			if r.Success {
				fmt.Printf("%s %s\n", app.Theme.SuccessStyle.Render(styles.IconCheck), r.Target.Path)
			} else {
				fmt.Printf("%s %s: %v\n", app.Theme.ErrorStyle.Render(styles.IconX), r.Target.Path, r.Error)
			}
		}
	}
	return err
}
