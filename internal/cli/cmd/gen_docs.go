package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"

	xdgadapter "github.com/bnema/dumber/internal/infrastructure/xdg"
)

const dirPerm = 0o755

var (
	genDocsOutputDir string
	genDocsFormat    string
)

var genDocsCmd = &cobra.Command{
	Use:   "gen-docs",
	Short: "Generate documentation from CLI commands",
	Long: `Generate documentation (man pages or markdown) from CLI command definitions.

The documentation is auto-generated from the command structure, including:
- Command names and aliases
- Short and long descriptions
- Flags and their descriptions
- Usage examples

Supported formats:
  man       Unix manual pages (groff format)
  markdown  Markdown files (for websites/wikis)

By default, man pages are installed to ~/.local/share/man/man1/ so they
are immediately available via 'man dumber'. You may need to run 'mandb'
to update the man page index.

Examples:
  dumber gen-docs                           # Install man pages to ~/.local/share/man/man1/
  dumber gen-docs --format markdown         # Generate markdown docs
  dumber gen-docs --output ./man            # Generate to local directory`,
	RunE: runGenDocs,
}

func init() {
	rootCmd.AddCommand(genDocsCmd)
	genDocsCmd.Flags().StringVarP(&genDocsOutputDir, "output", "o", "", "Output directory for generated docs")
	genDocsCmd.Flags().StringVarP(&genDocsFormat, "format", "f", "man", "Output format: man, markdown")
}

func runGenDocs(_ *cobra.Command, _ []string) error {
	// Resolve output directory
	outputDir := genDocsOutputDir
	if outputDir == "" {
		switch genDocsFormat {
		case "man":
			xdg := xdgadapter.New()
			manDir, err := xdg.ManDir()
			if err != nil {
				return fmt.Errorf("resolve man directory: %w", err)
			}
			outputDir = manDir
		case "markdown":
			outputDir = "./docs"
		}
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, dirPerm); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	switch genDocsFormat {
	case "man":
		return generateManPages(outputDir)
	case "markdown":
		return generateMarkdown(outputDir)
	default:
		return fmt.Errorf("unsupported format %q (use: man, markdown)", genDocsFormat)
	}
}

func generateManPages(outputDir string) error {
	header := &doc.GenManHeader{
		Title:   "DUMBER",
		Section: "1",
		Source:  "dumber " + buildInfo.Version,
		Manual:  "Dumber Manual",
		Date:    func() *time.Time { t := time.Now(); return &t }(),
	}

	// Disable auto-generation timestamp in the footer for reproducible builds
	rootCmd.DisableAutoGenTag = true

	if err := doc.GenManTree(rootCmd, header, outputDir); err != nil {
		return fmt.Errorf("generate man pages: %w", err)
	}

	fmt.Printf("Installed man pages to %s\n", outputDir)
	fmt.Println("Run 'mandb' if 'man dumber' doesn't work immediately.")

	// List generated files
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return nil // Non-fatal
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".1" {
			fmt.Printf("  - %s\n", e.Name())
		}
	}

	return nil
}

func generateMarkdown(outputDir string) error {
	// Disable auto-generation timestamp for reproducible builds
	rootCmd.DisableAutoGenTag = true

	if err := doc.GenMarkdownTree(rootCmd, outputDir); err != nil {
		return fmt.Errorf("generate markdown docs: %w", err)
	}

	fmt.Printf("Generated markdown docs in %s\n", outputDir)

	// List generated files
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return nil // Non-fatal
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".md" {
			fmt.Printf("  - %s\n", e.Name())
		}
	}

	return nil
}
