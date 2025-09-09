// Package contracts defines the WebKit memory optimization API contracts
// This file contains the expected function signatures and behavior contracts
package contracts

import (
	"time"
	"unsafe"
)

// WebKitMemoryAPI defines the contract for WebKit memory management functions
type WebKitMemoryAPI interface {
	// Memory Pressure Settings
	CreateMemoryPressureSettings(memoryLimitMB int, conservativeThreshold, strictThreshold, killThreshold, pollInterval float64) (unsafe.Pointer, error)
	ApplyMemoryPressureSettings(context unsafe.Pointer, settings unsafe.Pointer) error

	// Cache Model Management
	SetCacheModel(context unsafe.Pointer, model CacheModel) error
	GetCurrentCacheModel(context unsafe.Pointer) (CacheModel, error)

	// JavaScript Garbage Collection
	TriggerJavaScriptGC(context unsafe.Pointer) error

	// WebView Settings
	SetPageCacheEnabled(settings unsafe.Pointer, enabled bool) error
	SetOfflineAppCacheEnabled(settings unsafe.Pointer, enabled bool) error

	// Memory Monitoring
	GetProcessMemoryInfo(pid int) (ProcessMemoryInfo, error)
	ListWebKitProcesses() ([]ProcessMemoryInfo, error)
}

// CacheModel represents WebKit cache model options
type CacheModel int

const (
	CacheModelDocumentViewer CacheModel = iota
	CacheModelWebBrowser
	CacheModelPrimaryWebBrowser
)

// ProcessMemoryInfo represents process memory statistics
type ProcessMemoryInfo struct {
	PID         int    `json:"pid"`
	VmRSS       int64  `json:"vm_rss"`  // KB
	VmSize      int64  `json:"vm_size"` // KB
	VmPeak      int64  `json:"vm_peak"` // KB
	ProcessName string `json:"process_name"`
}

// MemoryConfigAPI defines the contract for memory configuration management
type MemoryConfigAPI interface {
	// Configuration Validation
	ValidateMemoryConfig(config MemoryConfig) error

	// Preset Management
	GetMemoryOptimizedPreset() MemoryConfig
	GetBalancedPreset() MemoryConfig
	GetHighPerformancePreset() MemoryConfig

	// Configuration Application
	ApplyMemoryConfig(webview unsafe.Pointer, config MemoryConfig) error
}

// MemoryConfig represents memory optimization configuration
type MemoryConfig struct {
	MemoryLimitMB           int     `json:"memory_limit_mb"`
	ConservativeThreshold   float64 `json:"conservative_threshold"`
	StrictThreshold         float64 `json:"strict_threshold"`
	KillThreshold           float64 `json:"kill_threshold"`
	PollIntervalSeconds     float64 `json:"poll_interval_seconds"`
	CacheModel              string  `json:"cache_model"`
	EnablePageCache         bool    `json:"enable_page_cache"`
	EnableOfflineAppCache   bool    `json:"enable_offline_app_cache"`
	ProcessRecycleThreshold int     `json:"process_recycle_threshold"`
	EnableGCInterval        int     `json:"enable_gc_interval"`
	EnableMemoryMonitoring  bool    `json:"enable_memory_monitoring"`
}

// MemoryStatsAPI defines the contract for memory statistics tracking
type MemoryStatsAPI interface {
	// Statistics Retrieval
	GetMemoryStats(webviewID uintptr) (MemoryStats, error)
	GetAllMemoryStats() (map[uintptr]MemoryStats, error)

	// Memory Management Operations
	TriggerMemoryCleanup(webviewID uintptr) error
	ShouldRecycleWebView(webviewID uintptr) (bool, error)

	// Monitoring Operations
	StartMemoryMonitoring(interval time.Duration) error
	StopMemoryMonitoring() error
	GetTotalMemoryUsage() (float64, error) // MB
}

// MemoryStats represents WebView memory statistics
type MemoryStats struct {
	PageLoadCount             int       `json:"page_load_count"`
	LastGCTime                time.Time `json:"last_gc_time"`
	HasMemoryPressureSettings bool      `json:"has_memory_pressure_settings"`
	WebViewID                 uintptr   `json:"webview_id"`
}

// WebViewLifecycleAPI defines the contract for WebView lifecycle management
type WebViewLifecycleAPI interface {
	// WebView Registration
	RegisterWebView(webviewID uintptr, config MemoryConfig) error
	UnregisterWebView(webviewID uintptr) error

	// Lifecycle Events
	OnPageLoad(webviewID uintptr, url string) error
	OnMemoryPressure(webviewID uintptr, pressureLevel int) error
	OnGarbageCollection(webviewID uintptr) error

	// Recycling Operations
	ShouldRecycleWebView(webviewID uintptr) bool
	RequestWebViewRecycling(webviewID uintptr) error
}

// Error Types - define expected error behaviors

// ErrMemoryLimitExceeded indicates memory usage exceeded configured limits
type ErrMemoryLimitExceeded struct {
	Limit  int64 // bytes
	Actual int64 // bytes
}

func (e ErrMemoryLimitExceeded) Error() string {
	return "memory limit exceeded"
}

// ErrInvalidMemoryConfig indicates invalid memory configuration parameters
type ErrInvalidMemoryConfig struct {
	Field   string
	Message string
}

func (e ErrInvalidMemoryConfig) Error() string {
	return "invalid memory configuration"
}

// ErrWebKitAPIUnavailable indicates WebKit memory APIs are not available
type ErrWebKitAPIUnavailable struct {
	Function string
	Reason   string
}

func (e ErrWebKitAPIUnavailable) Error() string {
	return "WebKit API unavailable"
}

// Behavioral Contracts - define expected behaviors as comments

/*
MEMORY_PRESSURE_CONTRACT:
- Conservative threshold (0.2) should trigger background cleanup
- Strict threshold (0.35) should trigger aggressive cleanup
- Kill threshold (0.8) should trigger process termination warning
- Poll interval determines monitoring frequency (2.0s recommended)
- Memory limit applies per-WebView instance, not global

CACHE_MODEL_CONTRACT:
- DocumentViewer: Minimal caching, ~40% less memory than WebBrowser
- WebBrowser: Default caching, balanced performance/memory
- PrimaryWebBrowser: Maximum caching, highest memory usage
- Changes take effect on next navigation

GC_CONTRACT:
- JavaScript GC triggers should complete within 2 seconds
- GC should not block UI thread or navigation
- Periodic GC intervals should be 30-120 seconds for effectiveness
- Manual GC triggers should be idempotent (safe to call repeatedly)

PROCESS_RECYCLING_CONTRACT:
- Page load counting should be accurate and monotonic
- Recycling recommendations should not force immediate action
- New WebView creation should reset all memory counters
- Recycling should preserve user session state until destruction

MONITORING_CONTRACT:
- Memory statistics should be updated within poll interval
- /proc parsing should handle process disappearance gracefully
- Memory reporting should be accurate within 5% of system tools
- Monitoring overhead should be <1% of total browser memory usage
*/
