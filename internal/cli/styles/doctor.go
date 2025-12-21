package styles

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	statusYes = "Yes"
	statusNo  = "No"
)

type DoctorRenderer struct {
	theme *Theme
}

func NewDoctorRenderer(theme *Theme) *DoctorRenderer {
	return &DoctorRenderer{theme: theme}
}

type DoctorReport struct {
	OverallOK bool
	Runtime   DoctorRuntimeReport
	Media     *DoctorMediaReport
}

type DoctorRuntimeReport struct {
	Prefix string
	OK     bool
	Checks []DoctorRuntimeCheck
}

type DoctorRuntimeCheck struct {
	Name            string
	PkgConfigName   string
	Installed       bool
	Version         string
	RequiredVersion string
	OK              bool
	Error           string
}

type DoctorMediaReport struct {
	GStreamerAvailable bool
	HWAccelAvailable   bool
	AV1HWAvailable     bool

	HasVAPlugin      bool
	HasVAAPIPlugin   bool
	HasNVCodecPlugin bool

	AV1Decoders  []string
	H264Decoders []string
	H265Decoders []string
	VP9Decoders  []string

	VAAPIAvailable bool
	VAAPIDriver    string
	VAAPIVersion   string

	Warnings []string
}

func (r *DoctorRenderer) Render(report DoctorReport) string {
	header := r.renderHeader(report.OverallOK)

	sections := []string{}
	if len(report.Runtime.Checks) > 0 || strings.TrimSpace(report.Runtime.Prefix) != "" {
		sections = append(sections, r.renderRuntime(report.Runtime))
	}
	if report.Media != nil {
		sections = append(sections, r.renderMedia(*report.Media))
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, "", strings.Join(sections, "\n\n"))
}

func (r *DoctorRenderer) renderHeader(ok bool) string {
	iconStyle := lipgloss.NewStyle().Foreground(r.theme.Accent)
	statusStyle := r.theme.SuccessStyle
	statusText := "OK"
	if !ok {
		statusStyle = r.theme.WarningStyle
		statusText = "Needs attention"
	}

	title := fmt.Sprintf("%s %s", iconStyle.Render(IconDoctor), r.theme.Title.Render("Doctor"))
	badge := r.theme.BadgeMuted.Render(statusStyle.Render(statusText))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, " ", badge)
}

func (r *DoctorRenderer) renderRuntime(rt DoctorRuntimeReport) string {
	lines := make([]string, 0, len(rt.Checks)+2)

	if strings.TrimSpace(rt.Prefix) != "" {
		lines = append(lines, fmt.Sprintf(
			"%s %s %s",
			r.theme.Subtle.Render("Prefix"),
			r.theme.Normal.Render(rt.Prefix),
			r.theme.Subtle.Render("(runtime override)"),
		))
	}

	for _, c := range rt.Checks {
		lines = append(lines, r.renderRuntimeCheck(c))
	}

	body := strings.Join(lines, "\n")
	return r.theme.Box.Render(r.theme.BoxHeader.Render(fmt.Sprintf("%s Runtime", r.theme.Highlight.Render(IconPackage))) + "\n" + body)
}

func (r *DoctorRenderer) renderRuntimeCheck(c DoctorRuntimeCheck) string {
	icon := IconCheck
	statusStyle := r.theme.SuccessStyle
	status := "OK"

	summary := ""
	if !c.Installed {
		icon = IconX
		statusStyle = r.theme.ErrorStyle
		status = "Missing"
		summary = c.Error
	} else if !c.OK {
		icon = IconWarning
		statusStyle = r.theme.WarningStyle
		status = "Too old"
		summary = fmt.Sprintf("have %s, need >= %s", c.Version, c.RequiredVersion)
	} else {
		summary = fmt.Sprintf("%s (>= %s)", c.Version, c.RequiredVersion)
	}

	name := r.theme.Normal.Render(c.Name)
	badge := r.theme.BadgeMuted.Render(statusStyle.Render(status))
	info := r.theme.Subtle.Render(summary)

	return fmt.Sprintf("%s %s %s\n  %s", statusStyle.Render(icon), name, badge, info)
}

func (r *DoctorRenderer) renderMedia(m DoctorMediaReport) string {
	lines := []string{}

	gstIcon := IconCheck
	gstStyle := r.theme.SuccessStyle
	gstText := statusYes
	if !m.GStreamerAvailable {
		gstIcon = IconX
		gstStyle = r.theme.ErrorStyle
		gstText = statusNo
	}
	lines = append(lines, fmt.Sprintf("%s %s %s", gstStyle.Render(gstIcon), r.theme.Subtle.Render("GStreamer"), gstStyle.Render(gstText)))

	hwIcon := IconWarning
	hwStyle := r.theme.WarningStyle
	hwText := statusNo
	if m.HWAccelAvailable {
		hwIcon = IconCheck
		hwStyle = r.theme.SuccessStyle
		hwText = statusYes
	}
	lines = append(lines, fmt.Sprintf("%s %s %s", hwStyle.Render(hwIcon), r.theme.Subtle.Render("HW decode"), hwStyle.Render(hwText)))

	av1Icon := IconWarning
	av1Style := r.theme.WarningStyle
	av1Text := statusNo
	if m.AV1HWAvailable {
		av1Icon = IconCheck
		av1Style = r.theme.SuccessStyle
		av1Text = statusYes
	}
	lines = append(lines, fmt.Sprintf(
		"%s %s %s %s",
		av1Style.Render(av1Icon),
		r.theme.Subtle.Render("AV1"),
		av1Style.Render(av1Text),
		r.theme.Subtle.Render("(preferred)"),
	))

	plugins := []string{
		fmt.Sprintf("%s VA (stateless): %s", r.theme.Subtle.Render("•"), pluginStatus(r.theme, m.HasVAPlugin, "recommended")),
		fmt.Sprintf("%s VAAPI (legacy): %s", r.theme.Subtle.Render("•"), pluginStatus(r.theme, m.HasVAAPIPlugin, "gstreamer-vaapi")),
		fmt.Sprintf("%s NVCodec: %s", r.theme.Subtle.Render("•"), pluginStatus(r.theme, m.HasNVCodecPlugin, "NVIDIA")),
	}

	lines = append(lines, "", r.theme.Subtle.Render("Plugins"), strings.Join(plugins, "\n"))

	if m.VAAPIAvailable {
		lines = append(lines, "", fmt.Sprintf("%s %s", r.theme.Subtle.Render("VA-API Driver"), r.theme.Normal.Render(m.VAAPIDriver)))
		if strings.TrimSpace(m.VAAPIVersion) != "" {
			lines = append(lines, fmt.Sprintf("%s %s", r.theme.Subtle.Render("VA-API Version"), r.theme.Normal.Render(m.VAAPIVersion)))
		}
	}

	if len(m.Warnings) > 0 {
		warnLines := make([]string, 0, len(m.Warnings))
		for _, w := range m.Warnings {
			warnLines = append(warnLines, fmt.Sprintf("%s %s", r.theme.WarningStyle.Render(IconWarning), r.theme.Normal.Render(w)))
		}
		lines = append(lines, "", r.theme.WarningStyle.Render("Warnings"), strings.Join(warnLines, "\n"))
	}

	body := strings.Join(lines, "\n")
	return r.theme.Box.Render(r.theme.BoxHeader.Render(fmt.Sprintf("%s Media", r.theme.Highlight.Render(IconVideo))) + "\n" + body)
}

func pluginStatus(theme *Theme, ok bool, hint string) string {
	if ok {
		if hint != "" {
			return theme.SuccessStyle.Render("Available") + " " + theme.Subtle.Render("("+hint+")")
		}
		return theme.SuccessStyle.Render("Available")
	}
	if hint != "" {
		return theme.WarningStyle.Render("Missing") + " " + theme.Subtle.Render("("+hint+")")
	}
	return theme.WarningStyle.Render("Missing")
}
