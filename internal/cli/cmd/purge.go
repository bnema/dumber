package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	purgeForce bool
	purgeAll   bool
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

Use --force to skip confirmation prompt.`,
	RunE: runPurge,
}

func init() {
	rootCmd.AddCommand(purgeCmd)
	purgeCmd.Flags().BoolVarP(&purgeForce, "force", "f", false, "skip confirmation prompt")
	purgeCmd.Flags().BoolVar(&purgeAll, "all", true, "remove all directories (default)")
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

	if len(existing) == 0 {
		fmt.Println("No dumber directories found.")
		return nil
	}

	// Show what will be deleted
	fmt.Println("The following directories will be removed:")
	fmt.Println()
	for _, d := range existing {
		fmt.Printf("  \u2022 %s (%s, %s)\n", d.path, d.desc, formatSize(d.size))
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

	if len(errors) > 0 {
		fmt.Println()
		fmt.Println("Errors:")
		for _, e := range errors {
			fmt.Printf("  \u2717 %s\n", e)
		}
		return fmt.Errorf("failed to remove some directories")
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
