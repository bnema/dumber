// Package filtering provides filter management and compilation for content blocking.
package filtering

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/config"
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
	updater          *FilterUpdater
	updateInterval   time.Duration
	compileMutex     sync.RWMutex
	compiled         *CompiledFilters
	onFiltersReady   FilterReadyCallback
	// Database whitelist querier (optional, can be nil)
	whitelistDB WhitelistQuerier
	// Cached whitelist rules to avoid recomputing on every filter application
	cachedWhitelistRules []converter.WebKitRule
	cachedWhitelistHash  string
}

// FilterStore defines the interface for filter storage operations
type FilterStore interface {
	LoadCached() (*CompiledFilters, error)
	SaveCache(filters *CompiledFilters) error
	GetCacheInfo() (bool, time.Time, error)
	GetSourceVersion(url string) string
	SetSourceVersion(url string, version string) error
	GetLastCheckTime() time.Time
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

// WhitelistQuerier defines the interface for whitelist database operations
type WhitelistQuerier interface {
	GetAllWhitelistedDomains(ctx context.Context) ([]string, error)
	AddToWhitelist(ctx context.Context, domain string) error
	RemoveFromWhitelist(ctx context.Context, domain string) error
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
// whitelistDB can be nil if database whitelist is not needed
func NewFilterManager(store FilterStore, compiler FilterCompiler, whitelistDB WhitelistQuerier) *FilterManager {
	fm := &FilterManager{
		store:            store,
		compiler:         compiler,
		cosmeticInjector: cosmetic.NewCosmeticInjector(),
		updateInterval:   defaultUpdateHours * time.Hour, // Default update interval
		compiled:         &CompiledFilters{},
		onFiltersReady:   nil,
		whitelistDB:      whitelistDB,
	}
	// Initialize updater with reference to manager
	fm.updater = NewFilterUpdater(fm)
	return fm
}

// SetWhitelistDB sets the database whitelist querier
func (fm *FilterManager) SetWhitelistDB(db WhitelistQuerier) {
	fm.whitelistDB = db
}

// updateWhitelistCache rebuilds the cached whitelist rules from database
func (fm *FilterManager) updateWhitelistCache() {
	// Get domains from database whitelist
	var allDomains []string

	if fm.whitelistDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		dbDomains, err := fm.whitelistDB.GetAllWhitelistedDomains(ctx)
		if err != nil {
			logging.Warn(fmt.Sprintf("[filtering] Failed to get database whitelist: %v", err))
		} else {
			allDomains = dbDomains
			logging.Debug(fmt.Sprintf("[filtering] Loaded %d database whitelist domains", len(dbDomains)))
		}
	}

	// Create a hash of the whitelist to detect changes
	currentHash := strings.Join(allDomains, "|")

	// Only rebuild if whitelist changed
	if fm.cachedWhitelistHash == currentHash && fm.cachedWhitelistRules != nil {
		return
	}

	// Rebuild cache with pre-allocation
	rules := make([]converter.WebKitRule, 0, len(allDomains))
	for _, domain := range allDomains {
		// Create a regex pattern that matches all URLs containing this domain
		// Escape special regex characters in the domain
		escapedDomain := strings.ReplaceAll(domain, ".", "\\.")
		// Match any URL containing this domain (with protocol and optional path)
		urlPattern := ".*" + escapedDomain

		rules = append(rules, converter.WebKitRule{
			Trigger: converter.Trigger{
				URLFilter: urlPattern,
			},
			Action: converter.Action{
				Type: converter.ActionTypeIgnorePreviousRules,
			},
		})
		logging.Debug(fmt.Sprintf("[filtering] Created whitelist rule: %s → pattern: %s", domain, urlPattern))
	}

	fm.cachedWhitelistRules = rules
	fm.cachedWhitelistHash = currentHash
	logging.Info(fmt.Sprintf("[filtering] Whitelist cache updated with %d domains from database", len(allDomains)))
}

// AddToWhitelist adds a domain to the database whitelist and recompiles filters
func (fm *FilterManager) AddToWhitelist(ctx context.Context, domain string) error {
	if fm.whitelistDB == nil {
		return fmt.Errorf("database whitelist not available")
	}

	if err := fm.whitelistDB.AddToWhitelist(ctx, domain); err != nil {
		return fmt.Errorf("failed to add domain to whitelist: %w", err)
	}

	// Force whitelist cache refresh by clearing the hash
	fm.cachedWhitelistHash = ""

	// Re-apply filters to pick up the new whitelist entry
	fm.compileMutex.RLock()
	currentFilters := fm.compiled
	fm.compileMutex.RUnlock()

	if currentFilters != nil && len(currentFilters.NetworkRules) > 0 {
		// Create a copy of the filters to re-apply with updated whitelist
		filtersCopy := NewCompiledFilters()
		filtersCopy.NetworkRules = append(filtersCopy.NetworkRules, currentFilters.NetworkRules...)
		filtersCopy.CosmeticRules = currentFilters.CosmeticRules
		filtersCopy.GenericHiding = currentFilters.GenericHiding
		fm.applyFilters(filtersCopy)
	}

	logging.Info(fmt.Sprintf("[filtering] Added domain to whitelist: %s", domain))
	return nil
}

// RemoveFromWhitelist removes a domain from the database whitelist
func (fm *FilterManager) RemoveFromWhitelist(ctx context.Context, domain string) error {
	if fm.whitelistDB == nil {
		return fmt.Errorf("database whitelist not available")
	}

	if err := fm.whitelistDB.RemoveFromWhitelist(ctx, domain); err != nil {
		return fmt.Errorf("failed to remove domain from whitelist: %w", err)
	}

	// Force whitelist cache refresh
	fm.cachedWhitelistHash = ""

	logging.Info(fmt.Sprintf("[filtering] Removed domain from whitelist: %s", domain))
	return nil
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
	sources := config.Get().ContentFiltering.FilterLists
	if len(sources) == 0 {
		logging.Warn("[filtering] No filter lists configured")
		return
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
		}
	}

	// Only save/apply final result if we got actual content
	if hasContent {
		logging.Info(fmt.Sprintf("[filtering] Successfully compiled %d network rules from sources", len(merged.NetworkRules)))

		// Apply final merged filters once after all sources are processed
		fm.applyFilters(merged)

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

	// Add whitelist rules from config (ignore-previous-rules for whitelisted domains)
	// Update cache if whitelist changed, then use cached rules
	cfg := config.Get()

	// Only apply filters if content filtering is enabled
	if !cfg.ContentFiltering.Enabled {
		logging.Info("[filtering] Content filtering is DISABLED in config - no ad blocking active")
		return
	}
	// Re-enable database-driven whitelist injection (ignore-previous-rules) now that filtering apply is stabilized.
	fm.updateWhitelistCache()
	if len(fm.cachedWhitelistRules) > 0 {
		filters.NetworkRules = append(filters.NetworkRules, fm.cachedWhitelistRules...)
		logging.Info(fmt.Sprintf("[filtering] Applied %d whitelist rules (ignore-previous-rules)", len(fm.cachedWhitelistRules)))
	}

	// Add uBlock Origin style scriptlets to handle broken page loading

	// Instead of blocking completely, add exception rules for critical ad scripts that break page loading
	// This allows the page to load while neutralizing tracking
	antiBreakageRules := []converter.WebKitRule{
		{
			Trigger: converter.Trigger{
				URLFilter:    "googlesyndication.com",
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
				URLFilter:    "googletagservices.com/tag/js/gpt.js",
				ResourceType: []string{converter.ResourceTypeScript},
			},
			Action: converter.Action{
				Type: converter.ActionTypeBlock,
			},
		},
	}
	filters.NetworkRules = append(filters.NetworkRules, antiBreakageRules...)

	// Add internal scheme exceptions LAST to ensure they override all blocking rules
	// The dumb:// scheme is used for internal pages (homepage, blocked page, etc.)
	// and must never be blocked by content filtering
	internalSchemeRules := []converter.WebKitRule{
		{
			Trigger: converter.Trigger{
				URLFilter: "^dumb://",
			},
			Action: converter.Action{
				Type: converter.ActionTypeIgnorePreviousRules,
			},
		},
	}
	filters.NetworkRules = append(filters.NetworkRules, internalSchemeRules...)
	logging.Debug("[filtering] Added internal scheme exception for dumb://")

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

	// Inject cosmetic rules (domain-specific + generic hiding under empty key)
	cosmeticRulesWithGeneric := make(map[string][]string)
	for domain, selectors := range filters.CosmeticRules {
		cosmeticRulesWithGeneric[domain] = selectors
	}
	// Add generic hiding rules under empty string key (applies to all domains)
	if len(filters.GenericHiding) > 0 {
		cosmeticRulesWithGeneric[""] = filters.GenericHiding
		logging.Info(fmt.Sprintf("[filtering] Adding %d generic hiding rules to cosmetic injector", len(filters.GenericHiding)))
	}
	logging.Info(fmt.Sprintf("[filtering] Injecting cosmetic rules: %d domains + %d generic rules",
		len(filters.CosmeticRules), len(filters.GenericHiding)))
	if err := fm.cosmeticInjector.InjectRules(cosmeticRulesWithGeneric); err != nil {
		logging.Error(fmt.Sprintf("Failed to inject cosmetic rules: %v", err))
	}

	// Notify that filters are ready
	if fm.onFiltersReady != nil {
		go fm.onFiltersReady()
	}
}

func (fm *FilterManager) startUpdateLoop(ctx context.Context) {
	// Wait 2 minutes after startup before first version check
	// This ensures browser startup is not impacted
	startupDelay := 2 * time.Minute
	logging.Debug("[filtering] Waiting 2 minutes before first filter update check")

	select {
	case <-time.After(startupDelay):
		// Startup delay complete, proceed with first check
	case <-ctx.Done():
		return
	}

	// Perform initial version check
	logging.Info("[filtering] Performing initial filter version check")
	if fm.updater != nil {
		go fm.updater.CheckAndUpdate(ctx)
	}

	// Then check periodically (every 24 hours)
	ticker := time.NewTicker(fm.updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			logging.Info("[filtering] Periodic filter version check")
			if fm.updater != nil {
				go fm.updater.CheckAndUpdate(ctx)
			}
		}
	}
}

// updateFiltersInBackground updates filters without clearing existing ones
func (fm *FilterManager) updateFiltersInBackground(ctx context.Context) {
	logging.Info("[filtering] Starting background filter update")

	sources := config.Get().ContentFiltering.FilterLists
	if len(sources) == 0 {
		logging.Warn("[filtering] No filter lists configured")
		return
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
