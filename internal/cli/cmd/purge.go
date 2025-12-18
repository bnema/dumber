package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/infrastructure/desktop"
)

var (
	purgeForce   bool
	purgeAll     bool
	purgeDesktop bool
)

var purgeCmd = &cobra.Command{
	Use:   "purge",
	Short: "Remove all dumber data and configuration",
	Long: `Remove all dumber XDG directories (config, data, state, cache).

This will delete:
  - ~/.config/dumber/     (configuration)
  - ~/.local/share/dumber/ (data, database)
  - ~/.local/state/dumber/ (state, logs)
  - ~/.cache/dumber/       (cache)

Use --force to skip confirmation prompt.
Use --desktop to also remove the desktop file from ~/.local/share/applications/`,
	RunE: runPurge,
}

func init() {
	rootCmd.AddCommand(purgeCmd)
	purgeCmd.Flags().BoolVarP(&purgeForce, "force", "f", false, "skip confirmation prompt")
	purgeCmd.Flags().BoolVar(&purgeAll, "all", true, "remove all directories (default)")
	purgeCmd.Flags().BoolVar(&purgeDesktop, "desktop", false, "also remove desktop file")
}

func runPurge(cmd *cobra.Command, args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	// Build list of directories to remove
	dirs := []struct {
		path string
		desc string
	}{
		{getXDGPath("XDG_CONFIG_HOME", home, ".config", "dumber"), "config"},
		{getXDGPath("XDG_DATA_HOME", home, ".local/share", "dumber"), "data"},
		{getXDGPath("XDG_STATE_HOME", home, ".local/state", "dumber"), "state"},
		{getXDGPath("XDG_CACHE_HOME", home, ".cache", "dumber"), "cache"},
	}

	// Check which directories exist
	var existing []struct {
		path string
		desc string
		size int64
	}

	for _, d := range dirs {
		if info, err := os.Stat(d.path); err == nil && info.IsDir() {
			size := getDirSize(d.path)
			existing = append(existing, struct {
				path string
				desc string
				size int64
			}{d.path, d.desc, size})
		}
	}

	// Check desktop integration status if --desktop flag is set
	var desktopStatus *struct {
		desktopInstalled bool
		iconInstalled    bool
		isDefault        bool
		desktopPath      string
		iconPath         string
	}
	if purgeDesktop {
		adapter := desktop.New()
		status, err := adapter.GetStatus(context.Background())
		if err == nil && (status.DesktopFileInstalled || status.IconInstalled) {
			desktopStatus = &struct {
				desktopInstalled bool
				iconInstalled    bool
				isDefault        bool
				desktopPath      string
				iconPath         string
			}{
				desktopInstalled: status.DesktopFileInstalled,
				iconInstalled:    status.IconInstalled,
				isDefault:        status.IsDefaultBrowser,
				desktopPath:      status.DesktopFilePath,
				iconPath:         status.IconFilePath,
			}
		}
	}

	// Check if there's anything to remove
	if len(existing) == 0 && desktopStatus == nil {
		fmt.Println("No dumber directories found.")
		return nil
	}

	// Show what will be deleted
	fmt.Println("The following will be removed:")
	fmt.Println()
	for _, d := range existing {
		fmt.Printf("  \u2022 %s (%s, %s)\n", d.path, d.desc, formatSize(d.size))
	}
	if desktopStatus != nil {
		if desktopStatus.desktopInstalled {
			fmt.Printf("  \u2022 %s (desktop file)\n", desktopStatus.desktopPath)
		}
		if desktopStatus.iconInstalled {
			fmt.Printf("  \u2022 %s (icon)\n", desktopStatus.iconPath)
		}
		if desktopStatus.isDefault {
			fmt.Println("    \u26a0 Dumber is currently the default browser")
		}
	}
	fmt.Println()

	// Confirm unless --force
	if !purgeForce {
		fmt.Print("Are you sure? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response != "y" && response != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Remove directories
	var errors []string
	for _, d := range existing {
		if err := os.RemoveAll(d.path); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", d.path, err))
		} else {
			fmt.Printf("\u2713 Removed %s\n", d.path)
		}
	}

	// Remove desktop integration
	if desktopStatus != nil {
		adapter := desktop.New()
		uc := usecase.NewRemoveDesktopUseCase(adapter)
		result, err := uc.Execute(context.Background())
		if err != nil {
			errors = append(errors, fmt.Sprintf("desktop integration: %v", err))
		} else {
			if result.WasDesktopInstalled {
				fmt.Printf("\u2713 Removed %s\n", result.RemovedDesktopPath)
			}
			if result.WasIconInstalled {
				fmt.Printf("\u2713 Removed %s\n", result.RemovedIconPath)
			}
			if result.WasDefault {
				fmt.Println("  \u26a0 Was default browser - please set a new default")
			}
		}
	}

	if len(errors) > 0 {
		fmt.Println()
		fmt.Println("Errors:")
		for _, e := range errors {
			fmt.Printf("  \u2717 %s\n", e)
		}
		return fmt.Errorf("failed to remove some items")
	}

	fmt.Println()
	fmt.Println("Purge complete.")
	return nil
}

// getXDGPath returns the XDG path for dumber.
func getXDGPath(envVar, home, defaultSuffix, appName string) string {
	base := os.Getenv(envVar)
	if base == "" {
		base = filepath.Join(home, defaultSuffix)
	}
	return filepath.Join(base, appName)
}

// getDirSize calculates the total size of a directory.
func getDirSize(path string) int64 {
	var size int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

// formatSize formats bytes as human-readable size.
func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
