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

// SupportsAV1Hardware returns true if GPU has hardware AV1 decoding support
func (g GPUInfo) SupportsAV1Hardware() bool {
	switch g.Vendor {
	case GPUVendorAMD:
		return g.supportsAMDAV1()
	case GPUVendorNVIDIA:
		return g.supportsNVIDIAAV1()
	case GPUVendorIntel:
		return g.supportsIntelAV1()
	default:
		return false
	}
}

// supportsAMDAV1 checks for AMD AV1 hardware decode support based on official AMD specifications
// RDNA2+ architectures support AV1, with RDNA3+ adding encode support and RDNA4 bringing full B-frame support
func (g GPUInfo) supportsAMDAV1() bool {
	name := strings.ToLower(g.Name)

	// RDNA 2 architecture (RX 6000 series) - AV1 decode only
	// Source: Official AMD RDNA2 specifications - hardware-accelerated AV1 decoding
	rdna2Models := []string{
		// Desktop RX 6000 series (all RDNA2 models support AV1 decode)
		"rx 6400", "rx 6450", "rx 6500", "rx 6550", "rx 6600", "rx 6650",
		"rx 6700", "rx 6700 xt", "rx 6750", "rx 6750 xt",
		"rx 6800", "rx 6800 xt", "rx 6900", "rx 6900 xt", "rx 6950 xt",
	}

	// RDNA 3 architecture (RX 7000 series) - AV1 encode/decode with VCN 4.0
	// Source: Official AMD specifications - first RDNA with dedicated media engine and AV1 encode
	rdna3Models := []string{
		"rx 7600", "rx 7600 xt", "rx 7700", "rx 7700 xt",
		"rx 7800", "rx 7800 xt", "rx 7900", "rx 7900 xt", "rx 7900 gre", "rx 7900 xtx",
	}

	// RDNA 4 architecture (RX 9000 series) - Full AV1 support with B-frame encoding
	// Source: Official AMD RX 9070 series announcement - enhanced AV1 with B-frame support
	rdna4Models := []string{
		"rx 9070", "rx 9070 xt", // Officially announced models
		// Future RDNA4 models expected to follow similar naming
		"rx 9060", "rx 9080", "rx 9090",
	}

	// Check against all supported models
	allModels := append(rdna2Models, rdna3Models...)
	allModels = append(allModels, rdna4Models...)

	for _, model := range allModels {
		if strings.Contains(name, model) {
			return true
		}
	}

	// APUs with RDNA2/3 integrated graphics
	// Source: AMD APU specifications with RDNA2+ iGPUs
	apuModels := []string{
		// RDNA2-based APUs
		"ryzen 6000", "radeon 680m", "radeon 660m",
		// RDNA3-based APUs
		"ryzen 7000", "radeon 780m", "radeon 760m",
		// Future RDNA3+ APUs
		"ryzen 8000", "ryzen 9000", "radeon 880m",
	}

	for _, apu := range apuModels {
		if strings.Contains(name, apu) {
			return true
		}
	}

	return false
}

// supportsNVIDIAAV1 checks for NVIDIA AV1 hardware decode/encode support based on official NVIDIA specifications
func (g GPUInfo) supportsNVIDIAAV1() bool {
	name := strings.ToLower(g.Name)

	// RTX 30 series (Ampere) - AV1 decode only, up to 8K HDR
	// Source: Official NVIDIA RTX 30 series specifications - first NVIDIA GPUs with AV1 decode
	rtx30Series := []string{
		"rtx 3060", "rtx 3060 ti", "rtx 3070", "rtx 3070 ti",
		"rtx 3080", "rtx 3080 ti", "rtx 3090", "rtx 3090 ti",
		"rtx 3050", // Budget model with AV1 decode
	}

	// RTX 40 series (Ada Lovelace) - Full AV1 encode/decode with 8th gen NVENC
	// Source: Official NVIDIA RTX 40 series specifications - real-time AV1 encoding support
	rtx40Series := []string{
		"rtx 4060", "rtx 4060 ti", "rtx 4070", "rtx 4070 ti", "rtx 4070 super",
		"rtx 4080", "rtx 4080 super", "rtx 4090",
		"rtx 4050", // Mobile/budget variants
	}

	// Mobile RTX 40 series (Ada Lovelace) - Full AV1 support
	mobileRtx40 := []string{
		"rtx 4050 mobile", "rtx 4060 mobile", "rtx 4070 mobile", "rtx 4080 mobile", "rtx 4090 mobile",
	}

	// Professional Ada Lovelace cards
	professionalAda := []string{
		"rtx a4000", "rtx a4500", "rtx a5000", "rtx a6000", // Ada Lovelace professional
		"l4", "l40", "l40s", // Data center Ada Lovelace cards
	}

	// Check all series with AV1 support (decode-only for RTX 30, encode+decode for RTX 40+)
	allCards := append(rtx30Series, rtx40Series...)
	allCards = append(allCards, mobileRtx40...)
	allCards = append(allCards, professionalAda...)

	for _, card := range allCards {
		if strings.Contains(name, card) {
			return true
		}
	}

	// Architecture-based detection
	if strings.Contains(name, "ampere") || strings.Contains(name, "ada lovelace") {
		return true
	}

	return false
}

// supportsIntelAV1 checks for Intel AV1 hardware decode/encode support based on official Intel specifications
// Intel Arc GPUs are the first to offer full hardware AV1 acceleration (encode + decode)
func (g GPUInfo) supportsIntelAV1() bool {
	name := strings.ToLower(g.Name)

	// Intel Arc A-Series discrete GPUs (Alchemist, Xe-HPG architecture)
	// Source: Official Intel Arc specifications - industry-first full AV1 hardware acceleration
	arcAlchemist := []string{
		"arc a310", "arc a350", "arc a380", // Entry-level
		"arc a580", "arc a750", "arc a770", // Performance models
	}

	// Intel Arc B-Series discrete GPUs (Battlemage, Xe2 architecture)
	// Source: Official Intel Arc B-Series launch (December 2024) - B580 and B570 with enhanced AV1
	arcBattlemage := []string{
		"arc b570", "arc b580", // Officially announced December 2024
		// Future Battlemage models expected
		"arc b750", "arc b770",
	}

	// Intel Xe integrated graphics with AV1 support (12th gen+)
	// Note: Integrated graphics may have limited AV1 encode capabilities compared to discrete
	xeIntegrated := []string{
		"xe graphics", "iris xe", "uhd graphics xe",
		// Specific integrated GPU models
		"xe graphics g7", "iris xe graphics", "uhd graphics xe g4",
	}

	// CPU generation indicators for AV1-capable integrated graphics
	// Source: Intel specifications - AV1 support starting with Alder Lake (12th gen)
	cpuGenerations := []string{
		// Architecture codenames
		"alder lake", "raptor lake", "meteor lake", "lunar lake", "arrow lake",
		// Consumer generation naming
		"12th gen", "13th gen", "14th gen", "15th gen",
		// Core series with AV1 support
		"core ultra", // New branding for newer processors
	}

	// Check discrete Arc GPUs (full AV1 encode/decode support)
	allArcModels := append(arcAlchemist, arcBattlemage...)
	for _, gpu := range allArcModels {
		if strings.Contains(name, gpu) {
			return true
		}
	}

	// Check integrated Xe graphics with generation validation
	for _, xe := range xeIntegrated {
		if strings.Contains(name, xe) {
			// Verify it's from a generation that supports AV1 (12th gen+)
			for _, gen := range cpuGenerations {
				if strings.Contains(name, gen) {
					return true
				}
			}
			// If we find Xe but can't determine generation, assume newer (likely supports AV1)
			return true
		}
	}

	// Check by CPU generation alone (may have Xe integrated graphics)
	for _, gen := range cpuGenerations {
		if strings.Contains(name, gen) {
			return true
		}
	}

	return false
}

// GetAV1HardwareCapabilities returns detailed AV1 capabilities for the GPU
func (g GPUInfo) GetAV1HardwareCapabilities() map[string]bool {
	capabilities := map[string]bool{
		"decode": false,
		"encode": false,
		"10bit":  false, // 10-bit AV1 support
		"hdr":    false, // HDR support
	}

	if !g.SupportsAV1Hardware() {
		return capabilities
	}

	name := strings.ToLower(g.Name)

	switch g.Vendor {
	case GPUVendorAMD:
		// All RDNA2+ GPUs support AV1 decode, RDNA3+ adds encode
		capabilities["decode"] = true
		capabilities["10bit"] = true

		// RDNA3 (RX 7000) has VCN 4.0 with AV1 encode support
		// RDNA4 (RX 9000) adds B-frame encoding for better compression
		if strings.Contains(name, "rx 7") || strings.Contains(name, "rx 9") {
			capabilities["encode"] = true
			capabilities["hdr"] = true
		}

	case GPUVendorNVIDIA:
		// RTX 30 series: AV1 decode only (8K HDR capable)
		if strings.Contains(name, "rtx 3") {
			capabilities["decode"] = true
			capabilities["10bit"] = true
			capabilities["hdr"] = true
		}
		// RTX 40 series: Full AV1 encode/decode with 8th gen NVENC
		if strings.Contains(name, "rtx 4") {
			capabilities["decode"] = true
			capabilities["encode"] = true
			capabilities["10bit"] = true
			capabilities["hdr"] = true
		}

	case GPUVendorIntel:
		// Intel Arc discrete GPUs: Full AV1 encode/decode (industry first)
		// Xe integrated: AV1 decode, limited encode capabilities
		capabilities["decode"] = true
		capabilities["10bit"] = true

		// Arc discrete GPUs have full AV1 encode support
		if strings.Contains(name, "arc a") || strings.Contains(name, "arc b") {
			capabilities["encode"] = true
			capabilities["hdr"] = true
		}
		// Xe integrated may have basic encode on newer generations
		if strings.Contains(name, "xe graphics") || strings.Contains(name, "iris xe") {
			// Conservative: assume decode-only for integrated unless proven otherwise
			capabilities["hdr"] = true
		}
	}

	return capabilities
}
