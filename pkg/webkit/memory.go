//go:build webkit_cgo

package webkit

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// ProcessMemoryInfo contains memory information for a WebKit process
type ProcessMemoryInfo struct {
	PID         int    `json:"pid"`
	VmRSS       int64  `json:"vm_rss"`  // Resident Set Size in KB
	VmSize      int64  `json:"vm_size"` // Virtual Memory Size in KB
	VmPeak      int64  `json:"vm_peak"` // Peak Virtual Memory in KB
	ProcessName string `json:"process_name"`
}

// WebViewMemoryManager handles memory monitoring and lifecycle management for WebViews
type WebViewMemoryManager struct {
	views              map[uintptr]*WebView
	enableMonitoring   bool
	recycleThreshold   int
	monitoringInterval time.Duration
	stopMonitoring     chan struct{}
	lastLoggedMemory   int64
}

// NewWebViewMemoryManager creates a new memory manager
func NewWebViewMemoryManager(enableMonitoring bool, recycleThreshold int, monitoringInterval time.Duration) *WebViewMemoryManager {
	return &WebViewMemoryManager{
		views:              make(map[uintptr]*WebView),
		enableMonitoring:   enableMonitoring,
		recycleThreshold:   recycleThreshold,
		monitoringInterval: monitoringInterval,
		stopMonitoring:     make(chan struct{}),
	}
}

// RegisterWebView registers a WebView for monitoring
func (mgr *WebViewMemoryManager) RegisterWebView(view *WebView) {
	if mgr == nil || view == nil {
		return
	}
	mgr.views[view.id] = view

	if mgr.enableMonitoring && len(mgr.views) == 1 {
		// Start monitoring when first view is registered
		go mgr.startMemoryMonitoring()
	}
}

// UnregisterWebView removes a WebView from monitoring
func (mgr *WebViewMemoryManager) UnregisterWebView(viewID uintptr) {
	if mgr == nil {
		return
	}
	delete(mgr.views, viewID)

	if len(mgr.views) == 0 {
		// Stop monitoring when no views left
		close(mgr.stopMonitoring)
		mgr.stopMonitoring = make(chan struct{})
	}
}

// startMemoryMonitoring starts the background memory monitoring routine
func (mgr *WebViewMemoryManager) startMemoryMonitoring() {
	if mgr == nil || mgr.monitoringInterval <= 0 {
		return
	}

	ticker := time.NewTicker(mgr.monitoringInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			mgr.checkMemoryUsage()
		case <-mgr.stopMonitoring:
			return
		}
	}
}

// checkMemoryUsage monitors memory usage of WebKit processes
func (mgr *WebViewMemoryManager) checkMemoryUsage() {
	if mgr == nil {
		return
	}

	processes, err := mgr.getWebKitProcesses()
	if err != nil {
		if mgr.enableMonitoring {
			log.Printf("[webkit] Error monitoring processes: %v", err)
		}
		return
	}

	totalMemory := int64(0)
	for _, proc := range processes {
		totalMemory += proc.VmRSS
	}

	// Only log if memory usage changed by more than 10MB (10240 KB)
	const memoryChangeThreshold = 10240 // 10MB in KB
	if mgr.enableMonitoring {
		memoryDiff := totalMemory - mgr.lastLoggedMemory
		if memoryDiff < 0 {
			memoryDiff = -memoryDiff
		}

		if mgr.lastLoggedMemory == 0 || memoryDiff >= memoryChangeThreshold {
			log.Printf("[webkit] Memory usage: %d processes, total %.1f MB RSS",
				len(processes), float64(totalMemory)/1024.0)
			mgr.lastLoggedMemory = totalMemory
		}
	}

	// Check for recycling needs
	for _, view := range mgr.views {
		if view.memStats != nil && mgr.recycleThreshold > 0 {
			if view.memStats.pageLoadCount >= mgr.recycleThreshold {
				if mgr.enableMonitoring {
					log.Printf("[webkit] WebView %d needs recycling (%d pages loaded)",
						view.id, view.memStats.pageLoadCount)
				}
			}
		}
	}
}

// getWebKitProcesses returns memory info for all WebKit-related processes
func (mgr *WebViewMemoryManager) getWebKitProcesses() ([]ProcessMemoryInfo, error) {
	var processes []ProcessMemoryInfo

	// Find all WebKit processes by scanning /proc
	procDir, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("failed to read /proc: %w", err)
	}

	for _, entry := range procDir {
		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue // Not a PID directory
		}

		// Check if this is a WebKit process
		cmdlinePath := fmt.Sprintf("/proc/%d/cmdline", pid)
		cmdlineBytes, err := os.ReadFile(cmdlinePath)
		if err != nil {
			continue // Process might have exited
		}

		cmdline := string(cmdlineBytes)
		if !strings.Contains(cmdline, "WebKit") && !strings.Contains(cmdline, "dumber") {
			continue // Not a WebKit or dumber process
		}

		// Get memory information
		statusPath := fmt.Sprintf("/proc/%d/status", pid)
		statusData, err := os.ReadFile(statusPath)
		if err != nil {
			continue
		}

		memInfo := mgr.parseMemoryStatus(string(statusData), pid)
		if memInfo != nil {
			// Extract process name from cmdline
			parts := strings.Split(cmdline, "\x00")
			if len(parts) > 0 {
				memInfo.ProcessName = parts[0]
			}
			processes = append(processes, *memInfo)
		}
	}

	return processes, nil
}

// parseMemoryStatus parses /proc/[pid]/status for memory information
func (mgr *WebViewMemoryManager) parseMemoryStatus(status string, pid int) *ProcessMemoryInfo {
	lines := strings.Split(status, "\n")
	info := &ProcessMemoryInfo{PID: pid}

	for _, line := range lines {
		if strings.HasPrefix(line, "VmRSS:") {
			if val := mgr.extractMemoryValue(line); val > 0 {
				info.VmRSS = val
			}
		} else if strings.HasPrefix(line, "VmSize:") {
			if val := mgr.extractMemoryValue(line); val > 0 {
				info.VmSize = val
			}
		} else if strings.HasPrefix(line, "VmPeak:") {
			if val := mgr.extractMemoryValue(line); val > 0 {
				info.VmPeak = val
			}
		}
	}

	// Only return if we found at least VmRSS
	if info.VmRSS > 0 {
		return info
	}

	return nil
}

// extractMemoryValue extracts memory value in KB from status lines like "VmRSS: 12345 kB"
func (mgr *WebViewMemoryManager) extractMemoryValue(line string) int64 {
	parts := strings.Fields(line)
	if len(parts) >= 2 {
		val, err := strconv.ParseInt(parts[1], 10, 64)
		if err == nil {
			return val
		}
	}
	return 0
}

// GetTotalMemoryUsage returns total memory usage of all WebKit processes in MB
func (mgr *WebViewMemoryManager) GetTotalMemoryUsage() (float64, error) {
	if mgr == nil {
		return 0, fmt.Errorf("memory manager is nil")
	}

	processes, err := mgr.getWebKitProcesses()
	if err != nil {
		return 0, err
	}

	totalKB := int64(0)
	for _, proc := range processes {
		totalKB += proc.VmRSS
	}

	return float64(totalKB) / 1024.0, nil
}

// GetDetailedMemoryInfo returns detailed memory information for all WebKit processes
func (mgr *WebViewMemoryManager) GetDetailedMemoryInfo() ([]ProcessMemoryInfo, error) {
	if mgr == nil {
		return nil, fmt.Errorf("memory manager is nil")
	}

	return mgr.getWebKitProcesses()
}

// ShouldRecycleWebView checks if a WebView should be recycled based on memory pressure
func (mgr *WebViewMemoryManager) ShouldRecycleWebView(view *WebView) bool {
	if mgr == nil || view == nil || view.memStats == nil {
		return false
	}

	if mgr.recycleThreshold > 0 && view.memStats.pageLoadCount >= mgr.recycleThreshold {
		return true
	}

	return false
}

// Global memory manager instance
var globalMemoryManager *WebViewMemoryManager

// InitializeGlobalMemoryManager initializes the global memory manager
func InitializeGlobalMemoryManager(enableMonitoring bool, recycleThreshold int, monitoringInterval time.Duration) {
	if monitoringInterval == 0 {
		monitoringInterval = 30 * time.Second // Default 30 seconds
	}
	globalMemoryManager = NewWebViewMemoryManager(enableMonitoring, recycleThreshold, monitoringInterval)
}

// GetGlobalMemoryManager returns the global memory manager
func GetGlobalMemoryManager() *WebViewMemoryManager {
	return globalMemoryManager
}
