package gpu

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bnema/dumber/internal/logging"
)

// GPUVendor represents GPU vendor types
type GPUVendor string

const (
	GPUVendorAMD     GPUVendor = "amd"
	GPUVendorNVIDIA  GPUVendor = "nvidia"
	GPUVendorIntel   GPUVendor = "intel"
	GPUVendorUnknown GPUVendor = "unknown"
)

// GPUInfo contains information about the detected GPU
type GPUInfo struct {
	Vendor GPUVendor
	Name   string
	Driver string // radeonsi, nouveau, i965, etc.
}

// String returns a string representation of the GPU info
func (g GPUInfo) String() string {
	if g.Name != "" {
		return string(g.Vendor) + " (" + g.Name + ")"
	}
	return string(g.Vendor)
}

// DetectGPU attempts to detect the GPU vendor and driver using multiple methods
func DetectGPU() GPUInfo {
	logging.Debug("Starting GPU detection")
	
	// Try methods in order of reliability
	methods := []func() GPUInfo{
		detectViaGLXInfo,
		detectViaLSPCI,
		detectViaSysFS,
		detectViaNVIDIAProc,
	}

	for _, method := range methods {
		if info := method(); info.Vendor != GPUVendorUnknown {
			logging.Info("GPU detected: " + info.String() + " (driver: " + info.GetVAAPIDriverName() + ")")
			return info
		}
	}

	logging.Warn("No GPU detected using any method")
	return GPUInfo{Vendor: GPUVendorUnknown}
}

// detectViaGLXInfo uses glxinfo to detect GPU vendor and driver
func detectViaGLXInfo() GPUInfo {
	cmd := exec.Command("glxinfo")
	output, err := cmd.Output()
	if err != nil {
		return GPUInfo{Vendor: GPUVendorUnknown}
	}

	lines := strings.Split(string(output), "\n")
	var vendor, renderer string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "OpenGL vendor string:") {
			vendor = strings.TrimSpace(strings.TrimPrefix(line, "OpenGL vendor string:"))
		} else if strings.HasPrefix(line, "OpenGL renderer string:") {
			renderer = strings.TrimSpace(strings.TrimPrefix(line, "OpenGL renderer string:"))
		}
	}

	info := GPUInfo{Name: renderer}

	// Parse vendor
	vendor = strings.ToLower(vendor)
	if strings.Contains(vendor, "amd") || strings.Contains(vendor, "ati") {
		info.Vendor = GPUVendorAMD
		// Extract driver from renderer (e.g., "radeonsi" from "AMD Radeon RX 9070 XT (radeonsi, ...)")
		if strings.Contains(renderer, "radeonsi") {
			info.Driver = "radeonsi"
		}
	} else if strings.Contains(vendor, "nvidia") {
		info.Vendor = GPUVendorNVIDIA
		info.Driver = "nvidia"
	} else if strings.Contains(vendor, "intel") {
		info.Vendor = GPUVendorIntel
		// Intel can use iHD or i965 drivers
		if strings.Contains(renderer, "iHD") {
			info.Driver = "iHD"
		} else {
			info.Driver = "i965"
		}
	}

	return info
}

// detectViaLSPCI uses lspci to detect GPU vendor
func detectViaLSPCI() GPUInfo {
	cmd := exec.Command("lspci")
	output, err := cmd.Output()
	if err != nil {
		return GPUInfo{Vendor: GPUVendorUnknown}
	}

	lines := strings.Split(string(output), "\n")
	vgaPattern := regexp.MustCompile(`(?i)VGA|3D|Display`)

	for _, line := range lines {
		if vgaPattern.MatchString(line) {
			line = strings.ToLower(line)
			if strings.Contains(line, "amd") || strings.Contains(line, "ati") {
				return GPUInfo{
					Vendor: GPUVendorAMD,
					Name:   extractDeviceName(line),
					Driver: "radeonsi", // Default for modern AMD GPUs
				}
			} else if strings.Contains(line, "nvidia") {
				return GPUInfo{
					Vendor: GPUVendorNVIDIA,
					Name:   extractDeviceName(line),
					Driver: "nvidia",
				}
			} else if strings.Contains(line, "intel") {
				return GPUInfo{
					Vendor: GPUVendorIntel,
					Name:   extractDeviceName(line),
					Driver: "iHD", // Default to newer Intel driver
				}
			}
		}
	}

	return GPUInfo{Vendor: GPUVendorUnknown}
}

// detectViaSysFS checks sysfs for GPU vendor IDs
func detectViaSysFS() GPUInfo {
	pattern := "/sys/class/drm/card*/device/vendor"
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return GPUInfo{Vendor: GPUVendorUnknown}
	}

	for _, vendorFile := range matches {
		data, err := os.ReadFile(vendorFile)
		if err != nil {
			continue
		}

		vendorID := strings.TrimSpace(string(data))
		switch vendorID {
		case "0x1002": // AMD
			return GPUInfo{
				Vendor: GPUVendorAMD,
				Driver: "radeonsi",
			}
		case "0x10de": // NVIDIA
			return GPUInfo{
				Vendor: GPUVendorNVIDIA,
				Driver: "nvidia",
			}
		case "0x8086": // Intel
			return GPUInfo{
				Vendor: GPUVendorIntel,
				Driver: "iHD",
			}
		}
	}

	return GPUInfo{Vendor: GPUVendorUnknown}
}

// detectViaNVIDIAProc checks for NVIDIA driver via /proc
func detectViaNVIDIAProc() GPUInfo {
	if _, err := os.Stat("/proc/driver/nvidia/version"); err == nil {
		return GPUInfo{
			Vendor: GPUVendorNVIDIA,
			Driver: "nvidia",
		}
	}
	return GPUInfo{Vendor: GPUVendorUnknown}
}

// extractDeviceName extracts a readable device name from lspci output
func extractDeviceName(line string) string {
	// Extract text after the colon but before any brackets
	parts := strings.Split(line, ":")
	if len(parts) < 2 {
		return ""
	}

	name := strings.TrimSpace(parts[len(parts)-1])
	
	// Remove revision info in parentheses
	if idx := strings.Index(name, " (rev "); idx != -1 {
		name = name[:idx]
	}

	return name
}

// GetVAAPIDriverName returns the appropriate VA-API driver name for the GPU vendor
func (g GPUInfo) GetVAAPIDriverName() string {
	if g.Driver != "" {
		return g.Driver
	}

	switch g.Vendor {
	case GPUVendorAMD:
		return "radeonsi"
	case GPUVendorNVIDIA:
		return "vdpau" // NVIDIA uses VDPAU backend for VA-API
	case GPUVendorIntel:
		return "iHD" // Modern Intel GPUs
	default:
		return ""
	}
}

// SupportsVAAPI returns true if the GPU vendor supports VA-API
func (g GPUInfo) SupportsVAAPI() bool {
	return g.Vendor == GPUVendorAMD || g.Vendor == GPUVendorIntel || g.Vendor == GPUVendorNVIDIA
}