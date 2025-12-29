package env

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

const (
	// kibibyte is 1024 bytes (/proc/meminfo reports in kB which is actually KiB)
	kibibyte = 1024
)

// HardwareSurveyor implements port.HardwareSurveyor for Linux systems.
// It is safe for concurrent use after creation.
type HardwareSurveyor struct {
	once   sync.Once
	cached port.HardwareInfo
}

// NewHardwareSurveyor creates a new hardware surveyor.
func NewHardwareSurveyor() *HardwareSurveyor {
	return &HardwareSurveyor{}
}

// Survey detects and returns hardware information.
// Results are cached after the first call. Safe for concurrent use.
func (s *HardwareSurveyor) Survey(ctx context.Context) port.HardwareInfo {
	s.once.Do(func() {
		s.cached = s.doSurvey(ctx)
	})
	return s.cached
}

// doSurvey performs the actual hardware detection.
func (s *HardwareSurveyor) doSurvey(ctx context.Context) port.HardwareInfo {
	log := logging.FromContext(ctx)

	info := port.HardwareInfo{
		CPUThreads: runtime.NumCPU(), // Go's NumCPU returns logical threads
	}

	// Detect physical CPU cores
	info.CPUCores = s.detectCPUCores(ctx)
	if info.CPUCores == 0 {
		info.CPUCores = info.CPUThreads // Fallback to threads if cores unknown
	}

	// Detect RAM
	info.TotalRAM, info.AvailableRAM = s.detectMemory(ctx)

	// Detect GPU
	info.GPUVendor, info.GPUName, info.VRAM = s.detectGPU(ctx)

	log.Debug().
		Int("cpu_cores", info.CPUCores).
		Int("cpu_threads", info.CPUThreads).
		Uint64("total_ram_mb", info.TotalRAM/(1024*1024)).
		Uint64("available_ram_mb", info.AvailableRAM/(1024*1024)).
		Str("gpu_vendor", string(info.GPUVendor)).
		Str("gpu_name", info.GPUName).
		Uint64("vram_mb", info.VRAM/(1024*1024)).
		Msg("hardware survey completed")

	return info
}

// detectCPUCores returns the number of physical CPU cores.
// Reads from /sys/devices/system/cpu/cpu*/topology/core_id to count unique cores.
func (s *HardwareSurveyor) detectCPUCores(ctx context.Context) int {
	log := logging.FromContext(ctx)

	// Method 1: Count unique core_ids across all CPUs
	coreIDs := make(map[string]struct{})
	cpuPath := "/sys/devices/system/cpu"

	entries, err := os.ReadDir(cpuPath)
	if err != nil {
		log.Debug().Err(err).Msg("cannot read /sys/devices/system/cpu")
		return 0
	}

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "cpu") {
			continue
		}
		// Skip non-numeric cpu entries (like cpufreq, cpuidle)
		cpuNum := strings.TrimPrefix(entry.Name(), "cpu")
		if _, err := strconv.Atoi(cpuNum); err != nil {
			continue
		}

		coreIDPath := filepath.Join(cpuPath, entry.Name(), "topology", "core_id")
		data, err := os.ReadFile(coreIDPath)
		if err != nil {
			continue
		}
		coreID := strings.TrimSpace(string(data))
		coreIDs[coreID] = struct{}{}
	}

	if len(coreIDs) > 0 {
		return len(coreIDs)
	}

	// Method 2: Parse /proc/cpuinfo for "cpu cores" field
	return s.detectCPUCoresFromProcCPUInfo(ctx)
}

// detectCPUCoresFromProcCPUInfo parses /proc/cpuinfo to get core count.
func (*HardwareSurveyor) detectCPUCoresFromProcCPUInfo(ctx context.Context) int {
	log := logging.FromContext(ctx)

	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		log.Debug().Err(err).Msg("cannot read /proc/cpuinfo")
		return 0
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu cores") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				cores, err := strconv.Atoi(strings.TrimSpace(parts[1]))
				if err == nil {
					return cores
				}
			}
		}
	}
	return 0
}

// detectMemory reads memory info from /proc/meminfo.
func (s *HardwareSurveyor) detectMemory(ctx context.Context) (total, available uint64) {
	log := logging.FromContext(ctx)

	file, err := os.Open("/proc/meminfo")
	if err != nil {
		log.Debug().Err(err).Msg("cannot read /proc/meminfo")
		return 0, 0
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			total = s.parseMemInfoLine(line)
		} else if strings.HasPrefix(line, "MemAvailable:") {
			available = s.parseMemInfoLine(line)
		}
		if total > 0 && available > 0 {
			break
		}
	}
	return total, available
}

// parseMemInfoLine extracts bytes from a /proc/meminfo line like "MemTotal: 12345 kB".
func (*HardwareSurveyor) parseMemInfoLine(line string) uint64 {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return 0
	}
	value, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return 0
	}
	// /proc/meminfo reports in kB (actually KiB)
	return value * kibibyte
}

// detectGPU detects GPU vendor, name, and VRAM from DRM subsystem.
// When multiple GPUs are present (e.g., iGPU + discrete), prefers the one with more VRAM.
func (s *HardwareSurveyor) detectGPU(ctx context.Context) (port.GPUVendor, string, uint64) {
	log := logging.FromContext(ctx)

	const drmBase = "/sys/class/drm"
	cards := []string{"card0", "card1", "card2", "card3"}

	var bestVendor port.GPUVendor
	var bestName string
	var bestVRAM uint64
	var bestCard string

	for _, card := range cards {
		cardPath := filepath.Join(drmBase, card)
		if _, err := os.Stat(cardPath); os.IsNotExist(err) {
			continue
		}

		vendor := s.detectGPUVendor(cardPath)
		if vendor == port.GPUVendorUnknown {
			continue
		}

		name := s.detectGPUName(ctx, cardPath)
		vram := s.detectVRAM(ctx, cardPath)

		log.Debug().
			Str("card", card).
			Str("vendor", string(vendor)).
			Str("name", name).
			Uint64("vram_mb", vram/(1024*1024)).
			Msg("found GPU")

		// Prefer GPU with more VRAM (discrete > integrated)
		if vram > bestVRAM {
			bestVendor = vendor
			bestName = name
			bestVRAM = vram
			bestCard = card
		}
	}

	if bestVendor != port.GPUVendorUnknown {
		log.Debug().
			Str("card", bestCard).
			Str("vendor", string(bestVendor)).
			Uint64("vram_mb", bestVRAM/(1024*1024)).
			Msg("selected GPU (highest VRAM)")
	}

	return bestVendor, bestName, bestVRAM
}

// detectGPUVendor reads the PCI vendor ID.
func (*HardwareSurveyor) detectGPUVendor(cardPath string) port.GPUVendor {
	vendorPath := filepath.Join(cardPath, "device", "vendor")
	data, err := os.ReadFile(vendorPath)
	if err != nil {
		return port.GPUVendorUnknown
	}

	vendorID := strings.TrimSpace(string(data))
	switch vendorID {
	case "0x1002":
		return port.GPUVendorAMD
	case "0x8086":
		return port.GPUVendorIntel
	case "0x10de":
		return port.GPUVendorNVIDIA
	default:
		return port.GPUVendorUnknown
	}
}

// detectGPUName reads the GPU product name from various sources.
func (*HardwareSurveyor) detectGPUName(ctx context.Context, cardPath string) string {
	log := logging.FromContext(ctx)

	// Try reading from device/label first (some drivers provide this)
	labelPath := filepath.Join(cardPath, "device", "label")
	if data, err := os.ReadFile(labelPath); err == nil {
		name := strings.TrimSpace(string(data))
		if name != "" {
			return name
		}
	}

	// Try reading product name from device subsystem (uevent)
	ueventPath := filepath.Join(cardPath, "device", "uevent")
	if file, err := os.Open(ueventPath); err == nil {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "PCI_SLOT_NAME=") {
				// This gives us the PCI address, useful for debugging
				log.Debug().Str("pci_slot", strings.TrimPrefix(line, "PCI_SLOT_NAME=")).Msg("GPU PCI slot")
			}
		}
		_ = file.Close()
	}

	// For AMD, try reading from /sys/kernel/debug/dri/*/name (requires root usually)
	// For now, return empty and rely on vendor detection
	return ""
}

// detectVRAM reads VRAM size from DRM memory info.
func (s *HardwareSurveyor) detectVRAM(ctx context.Context, cardPath string) uint64 {
	log := logging.FromContext(ctx)

	// Method 1: AMD uses mem_info_vram_total in device directory
	amdVRAMPath := filepath.Join(cardPath, "device", "mem_info_vram_total")
	if data, err := os.ReadFile(amdVRAMPath); err == nil {
		vram, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
		if err == nil && vram > 0 {
			log.Debug().Uint64("vram_bytes", vram).Msg("detected VRAM from AMD mem_info_vram_total")
			return vram
		}
	}

	// Method 2: Try reading from resource file (PCI BAR sizes)
	// The first non-zero memory region is usually VRAM
	if vram := s.detectVRAMFromPCIResource(ctx, cardPath); vram > 0 {
		return vram
	}

	// Method 3: For Intel integrated, check i915 specific paths
	// Intel iGPU uses system RAM, so VRAM is usually 0 or shared
	return 0
}

// detectVRAMFromPCIResource tries to detect VRAM from PCI BAR sizes.
func (*HardwareSurveyor) detectVRAMFromPCIResource(ctx context.Context, cardPath string) uint64 {
	log := logging.FromContext(ctx)

	resourcePath := filepath.Join(cardPath, "device", "resource")
	file, err := os.Open(resourcePath)
	if err != nil {
		return 0
	}
	defer func() { _ = file.Close() }()

	const minVRAMSize = 256 * 1024 * 1024 // 256MB - skip smaller I/O regions
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			start, err1 := strconv.ParseUint(strings.TrimPrefix(parts[0], "0x"), 16, 64)
			end, err2 := strconv.ParseUint(strings.TrimPrefix(parts[1], "0x"), 16, 64)
			if err1 == nil && err2 == nil && start > 0 && end > start {
				size := end - start + 1
				// VRAM is typically > 256MB, skip smaller regions (I/O, config)
				if size > minVRAMSize {
					log.Debug().Uint64("vram_bytes", size).Msg("detected VRAM from PCI resource")
					return size
				}
			}
		}
	}
	return 0
}

var _ port.HardwareSurveyor = (*HardwareSurveyor)(nil)
