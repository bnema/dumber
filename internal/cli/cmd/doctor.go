package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/cli/styles"
	"github.com/bnema/dumber/internal/infrastructure/deps"
	"github.com/bnema/dumber/internal/infrastructure/media"
)

var (
	doctorOnlyRuntime bool
	doctorOnlyMedia   bool
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check runtime requirements and diagnose issues",
	Long: `Doctor checks system prerequisites for running the GUI browser.

By default it runs both:
- Runtime checks (GTK4 + WebKitGTK 6.0)
- Media checks (GStreamer/VA-API)

Use flags to run only one category.

Examples:
  dumber doctor
  dumber doctor --runtime
  dumber doctor --media`,
	RunE: runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
	doctorCmd.Flags().BoolVar(&doctorOnlyRuntime, "runtime", false, "Only run runtime checks (GTK4/WebKitGTK)")
	doctorCmd.Flags().BoolVar(&doctorOnlyMedia, "media", false, "Only run media checks (GStreamer/VA-API)")
}
func runDoctor(cmd *cobra.Command, args []string) error {
	app := GetApp()
	if app == nil {
		return fmt.Errorf("app not initialized")
	}

	if doctorOnlyRuntime && doctorOnlyMedia {
		return fmt.Errorf("--runtime and --media are mutually exclusive")
	}

	runRuntime := true
	runMedia := true
	if doctorOnlyRuntime {
		runMedia = false
	}
	if doctorOnlyMedia {
		runRuntime = false
	}

	report := styles.DoctorReport{OverallOK: true}

	runtimeOK := true
	if runRuntime {
		probe := deps.NewPkgConfigProbe()
		runtimeUC := usecase.NewCheckRuntimeDependenciesUseCase(probe)
		runtimeOut, err := runtimeUC.Execute(app.Ctx(), usecase.CheckRuntimeDependenciesInput{
			Prefix: app.Config.Runtime.Prefix,
		})
		if err != nil {
			return err
		}

		runtimeOK = runtimeOut.OK
		report.Runtime = styles.DoctorRuntimeReport{
			Prefix: runtimeOut.Prefix,
			OK:     runtimeOut.OK,
			Checks: make([]styles.DoctorRuntimeCheck, 0, len(runtimeOut.Checks)),
		}

		for _, c := range runtimeOut.Checks {
			report.Runtime.Checks = append(report.Runtime.Checks, styles.DoctorRuntimeCheck{
				Name:            c.DisplayName,
				PkgConfigName:   c.PkgConfigName,
				Installed:       c.Installed,
				Version:         c.Version,
				RequiredVersion: c.RequiredVersion,
				OK:              c.MeetsRequirement,
				Error:           c.Error,
			})
		}
	}

	mediaOK := true
	if runMedia {
		adapter := media.New()
		mediaUC := usecase.NewRunMediaDiagnosticsUseCase(adapter)
		mediaOut, err := mediaUC.Execute(app.Ctx(), usecase.RunMediaDiagnosticsInput{})
		if err != nil {
			return err
		}

		mediaOK = mediaOut.GStreamerAvailable
		report.Media = &styles.DoctorMediaReport{
			GStreamerAvailable: mediaOut.GStreamerAvailable,
			HWAccelAvailable:   mediaOut.HWAccelAvailable,
			AV1HWAvailable:     mediaOut.AV1HWAvailable,
			HasVAPlugin:        mediaOut.HasVAPlugin,
			HasVAAPIPlugin:     mediaOut.HasVAAPIPlugin,
			HasNVCodecPlugin:   mediaOut.HasNVCodecPlugin,
			AV1Decoders:        mediaOut.AV1Decoders,
			H264Decoders:       mediaOut.H264Decoders,
			H265Decoders:       mediaOut.H265Decoders,
			VP9Decoders:        mediaOut.VP9Decoders,
			VAAPIAvailable:     mediaOut.VAAPIAvailable,
			VAAPIDriver:        mediaOut.VAAPIDriver,
			VAAPIVersion:       mediaOut.VAAPIVersion,
			Warnings:           mediaOut.Warnings,
		}
	}

	report.OverallOK = runtimeOK && mediaOK

	renderer := styles.NewDoctorRenderer(app.Theme)
	fmt.Println(renderer.Render(report))

	if runRuntime && !runtimeOK {
		return fmt.Errorf("runtime requirements not met")
	}
	if runMedia && !mediaOK {
		return fmt.Errorf("media requirements not met")
	}

	return nil
}
