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
	purgeFilters bool
)

const dumberAppName = "dumber"

type purgeDir struct {
	path string
	desc string
}

type purgeDirWithSize struct {
	path string
	desc string
	size int64
}

type purgeDesktopStatus struct {
	desktopInstalled bool
	iconInstalled    bool
	isDefault        bool
	desktopPath      string
	iconPath         string
}

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
Use --desktop to also remove the desktop file from ~/.local/share/applications/
Use --filters to only remove content filter cache (for re-downloading filters)`,
	RunE: runPurge,
}

func init() {
	rootCmd.AddCommand(purgeCmd)
	purgeCmd.Flags().BoolVarP(&purgeForce, "force", "f", false, "skip confirmation prompt")
	purgeCmd.Flags().BoolVar(&purgeAll, "all", true, "remove all directories (default)")
	purgeCmd.Flags().BoolVar(&purgeDesktop, "desktop", false, "also remove desktop file")
	purgeCmd.Flags().BoolVar(&purgeFilters, "filters", false, "only remove content filter cache")
}

func runPurge(_ *cobra.Command, _ []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	// Handle --filters flag (only purge filter cache)
	if purgeFilters {
		return runPurgeFilters(home)
	}

	dirs := purgeDirList(home)
	existing := collectExistingDirs(dirs)

	var desktopStatus *purgeDesktopStatus
	// Check desktop integration status if --desktop flag is set
	if purgeDesktop {
		desktopStatus = getDesktopStatus()
	}

	// Check if there's anything to remove
	if len(existing) == 0 && desktopStatus == nil {
		fmt.Println("No dumber directories found.")
		return nil
	}

	describePurgeTargets(existing, desktopStatus)

	// Confirm unless --force
	if !confirmPurge() {
		fmt.Println("Aborted.")
		return nil
	}

	// Remove directories
	errors := removeDirs(existing)

	// Remove desktop integration
	if desktopStatus != nil {
		errors = removeDesktopIntegration(errors)
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

func purgeDirList(home string) []purgeDir {
	return []purgeDir{
		{getXDGPath("XDG_CONFIG_HOME", home, ".config"), "config"},
		{getXDGPath("XDG_DATA_HOME", home, ".local/share"), "data"},
		{getXDGPath("XDG_STATE_HOME", home, ".local/state"), "state"},
		{getXDGPath("XDG_CACHE_HOME", home, ".cache"), "cache"},
	}
}

func collectExistingDirs(dirs []purgeDir) []purgeDirWithSize {
	var existing []purgeDirWithSize
	for _, d := range dirs {
		if info, err := os.Stat(d.path); err == nil && info.IsDir() {
			existing = append(existing, purgeDirWithSize{
				path: d.path,
				desc: d.desc,
				size: getDirSize(d.path),
			})
		}
	}
	return existing
}

func getDesktopStatus() *purgeDesktopStatus {
	adapter := desktop.New()
	status, err := adapter.GetStatus(context.Background())
	if err != nil || (!status.DesktopFileInstalled && !status.IconInstalled) {
		return nil
	}
	return &purgeDesktopStatus{
		desktopInstalled: status.DesktopFileInstalled,
		iconInstalled:    status.IconInstalled,
		isDefault:        status.IsDefaultBrowser,
		desktopPath:      status.DesktopFilePath,
		iconPath:         status.IconFilePath,
	}
}

func describePurgeTargets(existing []purgeDirWithSize, desktopStatus *purgeDesktopStatus) {
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
}

func confirmPurge() bool {
	if purgeForce {
		return true
	}
	fmt.Print("Are you sure? [y/N]: ")
	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes"
}

func removeDirs(existing []purgeDirWithSize) []string {
	var errors []string
	for _, d := range existing {
		if err := os.RemoveAll(d.path); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", d.path, err))
		} else {
			fmt.Printf("\u2713 Removed %s\n", d.path)
		}
	}
	return errors
}

func removeDesktopIntegration(errors []string) []string {
	adapter := desktop.New()
	uc := usecase.NewRemoveDesktopUseCase(adapter)
	result, err := uc.Execute(context.Background())
	if err != nil {
		return append(errors, fmt.Sprintf("desktop integration: %v", err))
	}
	if result.WasDesktopInstalled {
		fmt.Printf("\u2713 Removed %s\n", result.RemovedDesktopPath)
	}
	if result.WasIconInstalled {
		fmt.Printf("\u2713 Removed %s\n", result.RemovedIconPath)
	}
	if result.WasDefault {
		fmt.Println("  \u26a0 Was default browser - please set a new default")
	}
	return errors
}

// getXDGPath returns the XDG path for dumber.
func getXDGPath(envVar, home, defaultSuffix string) string {
	base := os.Getenv(envVar)
	if base == "" {
		base = filepath.Join(home, defaultSuffix)
	}
	return filepath.Join(base, dumberAppName)
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

// runPurgeFilters removes only the content filter cache.
func runPurgeFilters(home string) error {
	dataDir := getXDGPath("XDG_DATA_HOME", home, ".local/share")
	filterDirs := []struct {
		path string
		desc string
	}{
		{filepath.Join(dataDir, "filters", "json"), "filter JSON cache"},
		{filepath.Join(dataDir, "filters", "store"), "compiled filters"},
	}

	// Check which directories exist
	var existing []struct {
		path string
		desc string
		size int64
	}

	for _, d := range filterDirs {
		if info, err := os.Stat(d.path); err == nil && info.IsDir() {
			size := getDirSize(d.path)
			existing = append(existing, struct {
				path string
				desc string
				size int64
			}{d.path, d.desc, size})
		}
	}

	if len(existing) == 0 {
		fmt.Println("No filter cache found.")
		return nil
	}

	// Show what will be deleted
	fmt.Println("The following filter cache will be removed:")
	fmt.Println()
	for _, d := range existing {
		fmt.Printf("  • %s (%s, %s)\n", d.path, d.desc, formatSize(d.size))
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
			fmt.Printf("✓ Removed %s\n", d.path)
		}
	}

	if len(errors) > 0 {
		fmt.Println()
		fmt.Println("Errors:")
		for _, e := range errors {
			fmt.Printf("  ✗ %s\n", e)
		}
		return fmt.Errorf("failed to remove some items")
	}

	fmt.Println()
	fmt.Println("Filter cache purged. Filters will be re-downloaded on next browser start.")
	return nil
}
