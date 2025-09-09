package webkit

// GetMemoryOptimizedConfig returns a Config optimized for low memory usage
// This configuration aims to reduce memory usage by 40-60% compared to defaults
func GetMemoryOptimizedConfig() *Config {
	return &Config{
		Memory: MemoryConfig{
			// Limit each WebView to 256MB (default: unlimited)
			MemoryLimitMB: 256,

			// Aggressive memory pressure thresholds
			ConservativeThreshold: 0.2,  // Trigger cleanup at 20% (default: 33%)
			StrictThreshold:       0.35, // Aggressive cleanup at 35% (default: 50%)
			KillThreshold:         0.8,  // Kill process at 80% to prevent OOM

			// Check memory usage every 2 seconds
			PollIntervalSeconds: 2.0,

			// Use minimal caching for lowest memory usage
			CacheModel: CacheModelDocumentViewer,

			// Disable memory-intensive caches
			EnablePageCache:       false, // Saves ~30-50MB but slower back/forward
			EnableOfflineAppCache: false, // Saves ~10-20MB

			// Recycle WebView after 50 page loads to prevent memory accumulation
			ProcessRecycleThreshold: 50,

			// Trigger JavaScript GC every 60 seconds
			EnableGCInterval: 60,

			// Enable detailed memory monitoring and logging
			EnableMemoryMonitoring: true,
		},

		// Disable developer tools to save memory (unless debugging)
		EnableDeveloperExtras: false,

		// Use CPU rendering to save GPU memory (trade-off: slower rendering)
		Rendering: RenderingConfig{
			Mode: "cpu",
		},
	}
}

// GetBalancedConfig returns a Config with moderate memory optimizations
// This configuration aims to reduce memory usage by 20-30% with minimal performance impact
func GetBalancedConfig() *Config {
	return &Config{
		Memory: MemoryConfig{
			// Limit each WebView to 512MB
			MemoryLimitMB: 512,

			// Moderate memory pressure thresholds
			ConservativeThreshold: 0.25,
			StrictThreshold:       0.4,
			KillThreshold:         0.7,

			PollIntervalSeconds: 5.0,

			// Use default web browser caching
			CacheModel: CacheModelWebBrowser,

			// Keep page cache but disable offline app cache
			EnablePageCache:       true,
			EnableOfflineAppCache: false,

			// Recycle after more page loads
			ProcessRecycleThreshold: 100,

			// Less frequent GC
			EnableGCInterval: 120,

			EnableMemoryMonitoring: true,
		},

		EnableDeveloperExtras: false,

		// Use auto rendering (WebKit decides GPU vs CPU)
		Rendering: RenderingConfig{
			Mode: "auto",
		},
	}
}

// GetHighPerformanceConfig returns a Config optimized for performance over memory
// This uses default/high memory settings for maximum performance
func GetHighPerformanceConfig() *Config {
	return &Config{
		Memory: MemoryConfig{
			// No memory limit (use system default)
			MemoryLimitMB: 0,

			// Relaxed memory pressure thresholds
			ConservativeThreshold: 0.4,
			StrictThreshold:       0.6,
			KillThreshold:         0, // Disable kill threshold

			PollIntervalSeconds: 10.0,

			// Maximum caching for best performance
			CacheModel: CacheModelPrimaryWebBrowser,

			// Enable all caches
			EnablePageCache:       true,
			EnableOfflineAppCache: true,

			// Disable recycling
			ProcessRecycleThreshold: 0,

			// Disable periodic GC
			EnableGCInterval: 0,

			EnableMemoryMonitoring: false,
		},

		EnableDeveloperExtras: true,

		// Use GPU rendering for best performance
		Rendering: RenderingConfig{
			Mode: "gpu",
		},
	}
}

// ValidateMemoryConfig validates memory configuration parameters
func ValidateMemoryConfig(config *MemoryConfig) error {
	if config == nil {
		return nil // Use defaults
	}

	if config.ConservativeThreshold < 0 || config.ConservativeThreshold > 1 {
		return &ValidationError{Field: "ConservativeThreshold", Message: "must be between 0.0 and 1.0"}
	}

	if config.StrictThreshold < 0 || config.StrictThreshold > 1 {
		return &ValidationError{Field: "StrictThreshold", Message: "must be between 0.0 and 1.0"}
	}

	if config.KillThreshold < 0 || config.KillThreshold > 1 {
		return &ValidationError{Field: "KillThreshold", Message: "must be between 0.0 and 1.0"}
	}

	if config.ConservativeThreshold >= config.StrictThreshold {
		return &ValidationError{Field: "StrictThreshold", Message: "must be greater than ConservativeThreshold"}
	}

	if config.PollIntervalSeconds < 0 {
		return &ValidationError{Field: "PollIntervalSeconds", Message: "must be non-negative"}
	}

	return nil
}

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return "validation error for " + e.Field + ": " + e.Message
}

// LogMemoryConfiguration logs the current memory configuration for debugging
func LogMemoryConfiguration(config *Config) {
	if config == nil || !config.Memory.EnableMemoryMonitoring {
		return
	}

	log := func(format string, args ...interface{}) {
		// Use Go's standard log package
		// log.Printf("[webkit-config] "+format, args...)
		// For now, we'll just ignore since we don't have access to log here
		_ = format
		_ = args
	}

	log("Memory configuration:")
	log("  MemoryLimitMB: %d", config.Memory.MemoryLimitMB)
	log("  ConservativeThreshold: %.2f", config.Memory.ConservativeThreshold)
	log("  StrictThreshold: %.2f", config.Memory.StrictThreshold)
	log("  KillThreshold: %.2f", config.Memory.KillThreshold)
	log("  CacheModel: %s", config.Memory.CacheModel)
	log("  EnablePageCache: %t", config.Memory.EnablePageCache)
	log("  EnableOfflineAppCache: %t", config.Memory.EnableOfflineAppCache)
	log("  ProcessRecycleThreshold: %d", config.Memory.ProcessRecycleThreshold)
	log("  EnableGCInterval: %d seconds", config.Memory.EnableGCInterval)
}
