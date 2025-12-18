package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bnema/dumber/internal/infrastructure/media"
)

var mediaCmd = &cobra.Command{
	Use:   "media",
	Short: "Show media playback diagnostics",
	Long: `Display media playback diagnostics including GStreamer plugins and VA-API status.

This command checks for hardware video acceleration support and reports:
- Available GStreamer decoder plugins (VA, VAAPI, NVCodec)
- Hardware video decoders (AV1, H.264, H.265, VP9)
- VA-API driver detection and version
- Warnings and recommendations

Use this to troubleshoot video playback issues (e.g., Twitch Error #4000).

Examples:
  dumber media     # Show full diagnostics`,
	RunE: runMedia,
}

func init() {
	rootCmd.AddCommand(mediaCmd)
}

func runMedia(cmd *cobra.Command, args []string) error {
	app := GetApp()
	if app == nil {
		return fmt.Errorf("app not initialized")
	}

	// Run diagnostics via adapter
	adapter := media.New()
	diag := adapter.RunDiagnostics(app.Ctx())

	theme := app.Theme

	// Header
	fmt.Println(theme.Highlight.Render("Media Diagnostics"))
	fmt.Println(theme.Subtle.Render(strings.Repeat("─", 40)))
	fmt.Println()

	// GStreamer Status
	gstStatus := "No"
	gstStyle := theme.ErrorStyle
	if diag.GStreamerAvailable {
		gstStatus = "Yes"
		gstStyle = theme.SuccessStyle
	}
	fmt.Printf("%s %s\n", theme.Subtle.Render("GStreamer Installed:"), gstStyle.Render(gstStatus))

	// Hardware Acceleration Status
	hwStatus := "No"
	hwStyle := theme.WarningStyle
	if diag.HWAccelAvailable {
		hwStatus = "Yes"
		hwStyle = theme.SuccessStyle
	}
	fmt.Printf("%s %s\n", theme.Subtle.Render("Hardware Acceleration:"), hwStyle.Render(hwStatus))

	// AV1 Status (preferred codec)
	av1Status := "No"
	av1Style := theme.WarningStyle
	if diag.AV1HWAvailable {
		av1Status = "Yes"
		av1Style = theme.SuccessStyle
	}
	fmt.Printf("%s %s %s\n", theme.Subtle.Render("AV1 Hardware Decoder:"), av1Style.Render(av1Status), theme.Subtle.Render("(preferred codec)"))

	// VA-API Status
	if diag.VAAPIAvailable {
		fmt.Printf("%s %s\n", theme.Subtle.Render("VA-API Driver:"), theme.Normal.Render(diag.VAAPIDriver))
		if diag.VAAPIVersion != "" {
			fmt.Printf("%s %s\n", theme.Subtle.Render("VA-API Version:"), theme.Normal.Render(diag.VAAPIVersion))
		}
	} else {
		fmt.Printf("%s %s\n", theme.Subtle.Render("VA-API:"), theme.WarningStyle.Render("Not detected"))
	}

	fmt.Println()

	// GStreamer Plugins
	fmt.Println(theme.Highlight.Render("GStreamer Plugins"))
	fmt.Println(theme.Subtle.Render(strings.Repeat("─", 40)))

	printPluginStatus := func(name string, available bool, note string) {
		status := "Not found"
		style := theme.WarningStyle
		if available {
			status = "Available"
			style = theme.SuccessStyle
		}
		if note != "" {
			fmt.Printf("  %s %s %s\n", theme.Subtle.Render(name+":"), style.Render(status), theme.Subtle.Render(note))
		} else {
			fmt.Printf("  %s %s\n", theme.Subtle.Render(name+":"), style.Render(status))
		}
	}

	printPluginStatus("VA (stateless)", diag.HasVAPlugin, "(modern, recommended)")
	printPluginStatus("VAAPI (legacy)", diag.HasVAAPIPlugin, "(gstreamer-vaapi)")
	printPluginStatus("NVCodec", diag.HasNVCodecPlugin, "(NVIDIA)")

	fmt.Println()

	// Hardware Decoders
	fmt.Println(theme.Highlight.Render("Hardware Decoders"))
	fmt.Println(theme.Subtle.Render(strings.Repeat("─", 40)))

	printDecoders := func(codec string, decoders []string) {
		if len(decoders) > 0 {
			fmt.Printf("  %s %s\n", theme.Subtle.Render(codec+":"), theme.SuccessStyle.Render(strings.Join(decoders, ", ")))
		} else {
			fmt.Printf("  %s %s\n", theme.Subtle.Render(codec+":"), theme.WarningStyle.Render("None"))
		}
	}

	printDecoders("AV1", diag.AV1Decoders)
	printDecoders("H.264", diag.H264Decoders)
	printDecoders("H.265", diag.H265Decoders)
	printDecoders("VP9", diag.VP9Decoders)

	// Warnings
	if len(diag.Warnings) > 0 {
		fmt.Println()
		fmt.Println(theme.WarningStyle.Render("Warnings"))
		fmt.Println(theme.Subtle.Render(strings.Repeat("─", 40)))
		for _, w := range diag.Warnings {
			fmt.Printf("  %s %s\n", theme.WarningStyle.Render("!"), theme.Normal.Render(w))
		}
	}

	return nil
}
