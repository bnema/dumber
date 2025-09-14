// Package filtering provides filter management and compilation for content blocking.
package filtering

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/filtering/converter"
	"github.com/bnema/dumber/internal/filtering/cosmetic"
	"github.com/bnema/dumber/internal/logging"
)

const (
	defaultUpdateHours   = 24   // Default update interval in hours
	httpTimeoutSeconds   = 30   // HTTP client timeout in seconds
	contextCheckInterval = 1000 // Check context cancellation every N lines
)

// FilterReadyCallback is called when filters are loaded/updated
type FilterReadyCallback func()

// FilterManager manages filter loading, compilation, and updates
type FilterManager struct {
	store            FilterStore
	compiler         FilterCompiler
	cosmeticInjector *cosmetic.CosmeticInjector
	updateInterval   time.Duration
	compileMutex     sync.RWMutex
	compiled         *CompiledFilters
	onFiltersReady   FilterReadyCallback
}

// FilterStore defines the interface for filter storage operations
type FilterStore interface {
	LoadCached() (*CompiledFilters, error)
	SaveCache(filters *CompiledFilters) error
	GetCacheInfo() (bool, time.Time, error)
}

// FilterCompiler defines the interface for filter compilation
type FilterCompiler interface {
	CompileFromSources(ctx context.Context, sources []string) (*CompiledFilters, error)
	CompileFromData(data []byte) (*CompiledFilters, error)
}

// CosmeticInjector defines the interface for cosmetic filter injection
type CosmeticInjector interface {
	InjectRules(rules map[string][]string) error
	GetScriptForDomain(domain string) string
}

// CompiledFilters represents the compiled filter data
type CompiledFilters struct {
	NetworkRules  []converter.WebKitRule `json:"network_rules"`
	CosmeticRules map[string][]string    `json:"cosmetic_rules"`
	GenericHiding []string               `json:"generic_hiding"`
	CompiledAt    time.Time              `json:"compiled_at"`
	Version       string                 `json:"version"`
	mu            sync.RWMutex
}

// NewFilterManager creates a new filter manager
func NewFilterManager(store FilterStore, compiler FilterCompiler) *FilterManager {
	return &FilterManager{
		store:            store,
		compiler:         compiler,
		cosmeticInjector: cosmetic.NewCosmeticInjector(),
		updateInterval:   defaultUpdateHours * time.Hour, // Default update interval
		compiled:         &CompiledFilters{},
		onFiltersReady:   nil,
	}
}

// SetFiltersReadyCallback sets a callback to be called when filters are loaded/updated
func (fm *FilterManager) SetFiltersReadyCallback(callback FilterReadyCallback) {
	fm.onFiltersReady = callback
}

// InitAsync initializes filter loading in background
func (fm *FilterManager) InitAsync(ctx context.Context) error {
	// Load pre-compiled filters in background
	go fm.loadCompiledFilters(ctx)

	// Start periodic update checker
	go fm.startUpdateLoop(ctx)

	// Note: compileIfNeeded removed to prevent race condition
	// Cache age checking is now handled in loadCompiledFilters

	return nil
}

func (fm *FilterManager) loadCompiledFilters(ctx context.Context) {
	// Try to load from cache first
	if cached, err := fm.store.LoadCached(); err == nil {
		logging.Info(fmt.Sprintf("[filtering] Successfully loaded cache with %d network rules, %d cosmetic domains",
			len(cached.NetworkRules), len(cached.CosmeticRules)))
		fm.applyFilters(cached)

		// Check if cache needs update
		if exists, lastModified, _ := fm.store.GetCacheInfo(); exists {
			if time.Since(lastModified) > fm.updateInterval {
				// Schedule background update without clearing current filters
				logging.Info("[filtering] Cache is outdated, scheduling background update")
				go fm.updateFiltersInBackground(ctx)
			}
		}
		return
	} else {
		logging.Error(fmt.Sprintf("[filtering] Failed to load cache: %v", err))
	}

	// Only compile if no cache exists
	logging.Info("[filtering] No cache found, compiling from sources")
	fm.compileFromSources(ctx)
}

func (fm *FilterManager) compileIfNeeded(ctx context.Context) {
	// Check if cache exists and is recent
	exists, lastModified, err := fm.store.GetCacheInfo()
	if err != nil {
		fm.compileFromSources(ctx)
		return
	}

	// If cache is older than update interval, recompile
	if !exists || time.Since(lastModified) > fm.updateInterval {
		fm.compileFromSources(ctx)
	}
}

func (fm *FilterManager) compileFromSources(ctx context.Context) {
	sources := []string{
		"https://easylist.to/easylist/easylist.txt",
		"https://easylist.to/easylist/easyprivacy.txt",
		"https://easylist-downloads.adblockplus.org/liste_fr.txt", // For French sites like lesnumeriques.com
	}

	var wg sync.WaitGroup
	results := make(chan *CompiledFilters, len(sources))

	for _, url := range sources {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			if filters, err := fm.downloadAndCompile(ctx, u); err == nil {
				results <- filters
			} else {
				logging.Error(fmt.Sprintf("[filtering] Failed to download/compile %s: %v", u, err))
			}
		}(url)
	}

	// Wait for all compilations
	go func() {
		wg.Wait()
		close(results)
	}()

	// Merge results as they come in
	merged := NewCompiledFilters()
	hasContent := false

	for compiled := range results {
		if compiled != nil && len(compiled.NetworkRules) > 0 {
			hasContent = true
			merged.Merge(compiled)
			// Apply incrementally for faster initial blocking
			logging.Info(fmt.Sprintf("[filtering] Applying incremental filters: %d network rules", len(merged.NetworkRules)))
			fm.applyFilters(merged)
		}
	}

	// Only save/apply final result if we got actual content
	if hasContent {
		logging.Info(fmt.Sprintf("[filtering] Successfully compiled %d network rules from sources", len(merged.NetworkRules)))

		// Cache the final result
		if err := fm.store.SaveCache(merged); err != nil {
			logging.Error(fmt.Sprintf("Failed to save filter cache: %v", err))
		}
	} else {
		logging.Warn("[filtering] No network rules were successfully compiled from any source - keeping existing filters if any")
	}
}

func (fm *FilterManager) downloadAndCompile(ctx context.Context, url string) (*CompiledFilters, error) {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: httpTimeoutSeconds * time.Second,
	}

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", url, err)
	}

	// Set User-Agent to identify as browser
	req.Header.Set("User-Agent", "Dumber Browser/1.0")

	// Download filter list
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download %s: %w", url, ErrNetworkError)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logging.Error(fmt.Sprintf("Failed to close response body for %s: %v", url, closeErr))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d downloading %s", resp.StatusCode, url)
	}

	// Create filter converter
	filterConverter := converter.NewFilterConverter()
	compiled := NewCompiledFilters()

	// Parse line by line for memory efficiency
	scanner := bufio.NewScanner(resp.Body)
	lineCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineCount++

		// Check context cancellation periodically
		if lineCount%contextCheckInterval == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}

		// Convert each line
		if err := filterConverter.ConvertEasyListLine(line); err != nil {
			logging.Debug(fmt.Sprintf("Failed to convert filter line: %s, error: %v", line, err))
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading filter list %s: %w", url, err)
	}

	// Get compiled rules from converter
	compiled.NetworkRules = filterConverter.GetNetworkRules()
	compiled.CosmeticRules = filterConverter.GetCosmeticRules()
	compiled.GenericHiding = filterConverter.GetGenericHiding()
	compiled.CompiledAt = time.Now()
	compiled.Version = fmt.Sprintf("downloaded-%d", time.Now().Unix())

	// Log parsing results
	logging.Info(fmt.Sprintf("[filtering] Parsed %s: %d lines, %d network rules, %d cosmetic domains, %d generic hiding rules",
		url, lineCount, len(compiled.NetworkRules), len(compiled.CosmeticRules), len(compiled.GenericHiding)))

	return compiled, nil
}

func (fm *FilterManager) applyFilters(filters *CompiledFilters) {
	fm.compileMutex.Lock()
	defer fm.compileMutex.Unlock()

	// Add uBlock Origin style scriptlets to handle broken page loading

	// Instead of blocking completely, add exception rules for critical ad scripts that break page loading
	// This allows the page to load while neutralizing tracking
	antiBreakageRules := []converter.WebKitRule{
		{
			Trigger: converter.Trigger{
				URLFilter: "googlesyndication.com",
				ResourceType: []string{converter.ResourceTypeScript},
			},
			Action: converter.Action{
				Type: converter.ActionTypeBlock,
			},
		},
		{
			Trigger: converter.Trigger{
				URLFilter: "doubleclick.net",
			},
			Action: converter.Action{
				Type: converter.ActionTypeBlock,
			},
		},
		{
			Trigger: converter.Trigger{
				URLFilter: "googletagservices.com/tag/js/gpt.js",
				ResourceType: []string{converter.ResourceTypeScript},
			},
			Action: converter.Action{
				Type: converter.ActionTypeBlock,
			},
		},
	}
	filters.NetworkRules = append(filters.NetworkRules, antiBreakageRules...)

	// Add generic cosmetic rules for common ad loading indicators and infinite loaders
	antiLoaderSelectors := []string{
		// Generic ad loader patterns (work across all sites)
		".ad-loading", ".ad-placeholder", ".loading-ad", ".ad-spinner",
		"[class*='loading'][class*='ad']", "[id*='loading'][id*='ad']",
		".spinner[class*='ad']", "[data-loading*='ad']",
		// Generic infinite loader patterns
		".infinite-loader", ".loading-infinite", "[data-loading='infinite']",
		".loader[style*='infinite']", ".spinner[data-infinite='true']",
		// Generic advertising loader patterns (multi-language)
		".advertisement-loading", ".ads-loading", ".publicity-loading",
	}

	// Add generic hiding rules for loader prevention
	filters.GenericHiding = append(filters.GenericHiding, antiLoaderSelectors...)

	logging.Info(fmt.Sprintf("[filtering] Applying filters: %d network rules (including test rule), %d cosmetic domains, %d generic hiding rules",
		len(filters.NetworkRules), len(filters.CosmeticRules), len(filters.GenericHiding)))

	fm.compiled = filters

	// Inject cosmetic rules
	if err := fm.cosmeticInjector.InjectRules(filters.CosmeticRules); err != nil {
		logging.Error(fmt.Sprintf("Failed to inject cosmetic rules: %v", err))
	}

	// Notify that filters are ready
	if fm.onFiltersReady != nil {
		go fm.onFiltersReady()
	}
}

func (fm *FilterManager) startUpdateLoop(ctx context.Context) {
	ticker := time.NewTicker(fm.updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Check if cache needs update and schedule background update
			if exists, lastModified, _ := fm.store.GetCacheInfo(); exists {
				if time.Since(lastModified) > fm.updateInterval {
					logging.Info("[filtering] Periodic update triggered")
					go fm.updateFiltersInBackground(ctx)
				}
			}
		}
	}
}

// updateFiltersInBackground updates filters without clearing existing ones
func (fm *FilterManager) updateFiltersInBackground(ctx context.Context) {
	logging.Info("[filtering] Starting background filter update")

	// Compile new filters
	sources := []string{
		"https://easylist.to/easylist/easylist.txt",
		"https://easylist.to/easylist/easyprivacy.txt",
		"https://easylist-downloads.adblockplus.org/liste_fr.txt", // For French sites like lesnumeriques.com
	}

	var wg sync.WaitGroup
	results := make(chan *CompiledFilters, len(sources))

	for _, url := range sources {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			if filters, err := fm.downloadAndCompile(ctx, u); err == nil {
				results <- filters
			} else {
				logging.Warn(fmt.Sprintf("[filtering] Failed to update from %s: %v", u, err))
			}
		}(url)
	}

	// Wait for all compilations
	go func() {
		wg.Wait()
		close(results)
	}()

	// Merge results
	merged := NewCompiledFilters()
	hasContent := false

	for compiled := range results {
		if compiled != nil && len(compiled.NetworkRules) > 0 {
			hasContent = true
			merged.Merge(compiled)
		}
	}

	// Only apply and save if we got actual content
	if hasContent {
		logging.Info(fmt.Sprintf("[filtering] Background update succeeded with %d network rules", len(merged.NetworkRules)))
		fm.applyFilters(merged)

		if err := fm.store.SaveCache(merged); err != nil {
			logging.Error(fmt.Sprintf("Failed to save updated filter cache: %v", err))
		}
	} else {
		logging.Warn("[filtering] Background update produced no filters, keeping existing filters")
	}
}

// GetNetworkFilters returns the compiled network filters for WebKit
func (fm *FilterManager) GetNetworkFilters() ([]byte, error) {
	fm.compileMutex.RLock()
	defer fm.compileMutex.RUnlock()

	if fm.compiled == nil {
		return nil, ErrFiltersNotReady
	}

	json, err := fm.compiled.ToJSON()
	if err != nil {
		return nil, err
	}

	return json, nil
}

// GetCosmeticRulesForDomain returns cosmetic rules for a specific domain
func (fm *FilterManager) GetCosmeticRulesForDomain(domain string) []string {
	fm.compileMutex.RLock()
	defer fm.compileMutex.RUnlock()

	if fm.compiled == nil {
		return nil
	}

	return fm.compiled.GetRulesForDomain(domain)
}

// GetCosmeticScript returns the cosmetic filtering script
func (fm *FilterManager) GetCosmeticScript() string {
	return fm.cosmeticInjector.GetScriptForDomain("")
}

// GetCosmeticScriptForDomain returns the cosmetic filtering script for a specific domain
func (fm *FilterManager) GetCosmeticScriptForDomain(domain string) string {
	return fm.cosmeticInjector.GetScriptForDomain(domain)
}

// UpdateCosmeticRules adds new cosmetic rules for a domain
func (fm *FilterManager) UpdateCosmeticRules(domain string, selectors []string) {
	fm.cosmeticInjector.UpdateRulesForDomain(domain, selectors)
}

// GetCosmeticUpdateScript returns JavaScript to update cosmetic rules dynamically
func (fm *FilterManager) GetCosmeticUpdateScript(selectors []string) string {
	return fm.cosmeticInjector.GetUpdateScript(selectors)
}

// GetCosmeticCleanupScript returns JavaScript to cleanup cosmetic filtering
func (fm *FilterManager) GetCosmeticCleanupScript() string {
	return fm.cosmeticInjector.GetCleanupScript()
}

// NewCompiledFilters creates a new CompiledFilters instance
func NewCompiledFilters() *CompiledFilters {
	return &CompiledFilters{
		NetworkRules:  make([]converter.WebKitRule, 0),
		CosmeticRules: make(map[string][]string),
		GenericHiding: make([]string, 0),
		CompiledAt:    time.Now(),
	}
}

// Merge combines two CompiledFilters instances
func (cf *CompiledFilters) Merge(other *CompiledFilters) {
	cf.mu.Lock()
	defer cf.mu.Unlock()

	if other == nil {
		return
	}

	other.mu.RLock()
	defer other.mu.RUnlock()

	// Merge network rules
	cf.NetworkRules = append(cf.NetworkRules, other.NetworkRules...)

	// Merge cosmetic rules
	for domain, rules := range other.CosmeticRules {
		cf.CosmeticRules[domain] = append(cf.CosmeticRules[domain], rules...)
	}

	// Merge generic hiding rules
	cf.GenericHiding = append(cf.GenericHiding, other.GenericHiding...)
}

// GetRulesForDomain returns cosmetic rules for a specific domain
func (cf *CompiledFilters) GetRulesForDomain(domain string) []string {
	cf.mu.RLock()
	defer cf.mu.RUnlock()

	rules := make([]string, 0)

	// Add generic hiding rules
	rules = append(rules, cf.GenericHiding...)

	// Add domain-specific rules
	if domainRules, exists := cf.CosmeticRules[domain]; exists {
		rules = append(rules, domainRules...)
	}

	return rules
}

// ToJSON converts network rules to JSON format
func (cf *CompiledFilters) ToJSON() ([]byte, error) {
	cf.mu.RLock()
	defer cf.mu.RUnlock()

	return json.Marshal(cf.NetworkRules)
}
