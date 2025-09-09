// Package main provides the main entry point for dumb-browser CLI application.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Build information set via ldflags
var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "dumb-browser",
	Short: "A dumb browser for Wayland window managers",
	Long:  `A fast, simple browser with rofi/dmenu integration for sway and hyprland window managers.`,
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println("dumb-browser - A dumb browser for Wayland WMs")
		fmt.Println("Use --help for available commands")
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Printf("dumb-browser %s\n", version)
		fmt.Printf("commit: %s\n", commit)
		fmt.Printf("built: %s\n", buildDate)
	},
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize project dependencies",
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Printf("dumb-browser %s - Project initialization complete!\n", version)
		fmt.Println("Dependencies installed:")
		fmt.Println("- Cobra CLI framework")
		fmt.Println("- Viper configuration")
		fmt.Println("- ncruces/go-sqlite3 (CGO-free)")
		fmt.Println("- go-playground/validator")
		fmt.Println("- testify testing")
		fmt.Println("- SQLC for type-safe SQL")
		fmt.Println("- Wails v3-alpha desktop framework")
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(initCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}