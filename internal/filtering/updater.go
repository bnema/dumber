package filtering

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/logging"
)

const (
	updateTimeoutSeconds = 60 // HTTP timeout for update checks
	maxConcurrentUpdates = 4  // Maximum concurrent filter list downloads
)

// FilterUpdater manages differential filter list updates
type FilterUpdater struct {
	manager     *FilterManager
	httpClient  *http.Client
	diffEngine  *DiffEngine
	updateMutex sync.Mutex
}

// FilterUpdate represents an update to a filter list
type FilterUpdate struct {
	ListID   string
	URL      string
	Previous []byte
	Current  []byte
	Hash     string
	Version  string // Version extracted from content or HTTP headers
	Compiled *CompiledFilters
}

// FilterListInfo stores metadata about a filter list
type FilterListInfo struct {
	URL          string
	LastModified time.Time
	ETag         string
	ContentHash  string
	Size         int64
}

// DiffEngine handles differential updates between filter lists
type DiffEngine struct {
	// We'll implement a simple line-based diff for filter lists
}

// NewFilterUpdater creates a new filter updater
func NewFilterUpdater(manager *FilterManager) *FilterUpdater {
	return &FilterUpdater{
		manager: manager,
		httpClient: &http.Client{
			Timeout: updateTimeoutSeconds * time.Second,
		},
		diffEngine: &DiffEngine{},
	}
}

// CheckAndUpdate checks for filter list updates and applies them
func (fu *FilterUpdater) CheckAndUpdate(ctx context.Context) error {
	fu.updateMutex.Lock()
	defer fu.updateMutex.Unlock()

	// Skip if we checked within the last 24 hours
	lastCheck := fu.manager.store.GetLastCheckTime()
	if !lastCheck.IsZero() && time.Since(lastCheck) < 24*time.Hour {
		logging.Info(fmt.Sprintf("[filtering] Skipping update check, last check was %v ago", time.Since(lastCheck).Round(time.Minute)))
		return nil
	}

	startTime := time.Now()
	logging.Info("[filtering] Starting filter list update check...")

	sources := config.Get().ContentFiltering.FilterLists
	if len(sources) == 0 {
		logging.Warn("[filtering] No filter lists configured")
		return nil
	}

	// Check for updates concurrently
	updates := make(chan *FilterUpdate, len(sources))
	var wg sync.WaitGroup

	// Limit concurrent updates
	semaphore := make(chan struct{}, maxConcurrentUpdates)

	for _, url := range sources {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			update, err := fu.checkSingleUpdate(ctx, u)
			if err != nil {
				logging.Error(fmt.Sprintf("Failed to check update for %s: %v", u, err))
				return
			}

			if update != nil {
				updates <- update
			}
		}(url)
	}

	// Wait for all checks to complete
	go func() {
		wg.Wait()
		close(updates)
	}()

	// Apply updates as they come in
	updateCount := 0
	for update := range updates {
		if err := fu.applyUpdate(update); err != nil {
			logging.Error(fmt.Sprintf("Failed to apply update for %s: %v", update.URL, err))
			continue
		}
		updateCount++
	}

	elapsed := time.Since(startTime)
	if updateCount > 0 {
		logging.Info(fmt.Sprintf("[filtering] Update complete: applied %d filter list updates in %v", updateCount, elapsed.Round(time.Millisecond)))
	} else {
		logging.Info(fmt.Sprintf("[filtering] Update complete: all filters up to date (checked in %v)", elapsed.Round(time.Millisecond)))
	}

	return nil
}

// checkSingleUpdate checks if a single filter list needs updating
func (fu *FilterUpdater) checkSingleUpdate(ctx context.Context, url string) (*FilterUpdate, error) {
	// Check version using lightweight Range request
	if !fu.needsUpdate(ctx, url) {
		return nil, nil // No update needed
	}

	// Download the full content
	return fu.downloadUpdate(ctx, url, "", "")
}

// fetchRemoteVersion fetches version info using HEAD request (ETag or Last-Modified)
func (fu *FilterUpdater) fetchRemoteVersion(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Dumber Browser/1.0 Filter Updater")

	resp, err := fu.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HEAD request failed: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logging.Warn(fmt.Sprintf("warning: failed to close response body: %v", closeErr))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HEAD request returned %d", resp.StatusCode)
	}

	// Use ETag or Last-Modified as version identifier
	if etag := resp.Header.Get("ETag"); etag != "" {
		return etag, nil
	}
	if lastMod := resp.Header.Get("Last-Modified"); lastMod != "" {
		return lastMod, nil
	}

	return "", fmt.Errorf("no ETag or Last-Modified header")
}

// needsUpdate determines if a filter list needs updating based on version comparison
func (fu *FilterUpdater) needsUpdate(ctx context.Context, url string) bool {
	// Extract short name for logging
	shortName := url[strings.LastIndex(url, "/")+1:]

	remoteVersion, err := fu.fetchRemoteVersion(ctx, url)
	if err != nil {
		logging.Info(fmt.Sprintf("[filtering] %s: fetch failed, will update (%v)", shortName, err))
		return true
	}

	storedVersion := fu.manager.store.GetSourceVersion(url)
	if storedVersion == "" {
		logging.Info(fmt.Sprintf("[filtering] %s: no stored version, will update", shortName))
		return true
	}

	needsUpdate := remoteVersion != storedVersion
	if needsUpdate {
		logging.Info(fmt.Sprintf("[filtering] %s: new version %s (was %s)", shortName, remoteVersion, storedVersion))
	} else {
		logging.Info(fmt.Sprintf("[filtering] %s: up to date (%s)", shortName, storedVersion))
	}

	return needsUpdate
}

// downloadUpdate downloads the updated filter list and creates a diff
func (fu *FilterUpdater) downloadUpdate(ctx context.Context, url, lastModified, etag string) (*FilterUpdate, error) {
	// Download current content
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create GET request: %w", err)
	}

	req.Header.Set("User-Agent", "Dumber Browser/1.0 Filter Updater")

	resp, err := fu.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logging.Warn(fmt.Sprintf("warning: failed to close response body: %v", err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET request returned %d", resp.StatusCode)
	}

	// Read content (handle ContentLength=-1 when server uses chunked encoding)
	initialCap := resp.ContentLength
	if initialCap < 0 {
		initialCap = 64 * 1024 // 64KB initial capacity
	}
	currentContent := make([]byte, 0, initialCap)
	buffer := make([]byte, 8192)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		n, err := resp.Body.Read(buffer)
		if n > 0 {
			currentContent = append(currentContent, buffer[:n]...)
		}
		if err != nil {
			break
		}
	}

	// Calculate content hash
	hash := fmt.Sprintf("%x", sha256.Sum256(currentContent))

	// Fetch version with HEAD request (same method as fetchRemoteVersion)
	// This ensures consistency - HEAD may return different ETag than GET
	version, _ := fu.fetchRemoteVersion(ctx, url)

	// Create update object
	update := &FilterUpdate{
		ListID:  fu.getListID(url),
		URL:     url,
		Current: currentContent,
		Hash:    hash,
		Version: version,
	}

	// Get previous version for diffing (if available)
	previous := fu.getPreviousContent(update.ListID)
	if previous != nil {
		update.Previous = previous

		// Check if content actually changed
		if fu.contentEqual(previous, currentContent) {
			logging.Debug(fmt.Sprintf("No content changes in %s", url))
			return nil, nil
		}
	}

	// Compile the differential update
	compiled, err := fu.compileDiff(update)
	if err != nil {
		return nil, fmt.Errorf("failed to compile diff: %w", err)
	}

	update.Compiled = compiled
	return update, nil
}

// compileDiff compiles only the changed parts of a filter list
func (fu *FilterUpdater) compileDiff(update *FilterUpdate) (*CompiledFilters, error) {
	if update.Previous == nil {
		// No previous version, compile everything
		return fu.compileComplete(update.Current)
	}

	// Find differences between old and new content
	added, removed := fu.diffEngine.Diff(update.Previous, update.Current)

	compiled := NewCompiledFilters()

	// Process added lines
	for _, line := range added {
		rule, err := fu.parseFilterLine(line)
		if err != nil {
			logging.Debug(fmt.Sprintf("Failed to parse added line: %s, error: %v", line, err))
			continue
		}
		if rule != nil {
			fu.addRuleToCompiled(compiled, rule)
		}
	}

	// Process removed lines (mark for removal)
	for _, line := range removed {
		rule, err := fu.parseFilterLine(line)
		if err != nil {
			logging.Debug(fmt.Sprintf("Failed to parse removed line: %s, error: %v", line, err))
			continue
		}
		if rule != nil {
			fu.removeRuleFromCompiled(compiled, rule)
		}
	}

	compiled.Version = fmt.Sprintf("diff-update-%d", time.Now().Unix())
	compiled.CompiledAt = time.Now()

	logging.Info(fmt.Sprintf("Compiled differential update: %d rules added/removed for %s",
		len(added)+len(removed), update.URL))

	return compiled, nil
}

// compileComplete compiles a complete filter list
func (fu *FilterUpdater) compileComplete(content []byte) (*CompiledFilters, error) {
	// Use existing manager's compiler functionality
	if fu.manager.compiler != nil {
		return fu.manager.compiler.CompileFromData(content)
	}

	// Fallback: manual compilation
	return fu.compileManually(content)
}

// compileManually manually compiles filter content when no compiler is available
func (fu *FilterUpdater) compileManually(content []byte) (*CompiledFilters, error) {
	compiled := NewCompiledFilters()

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		rule, err := fu.parseFilterLine(strings.TrimSpace(line))
		if err != nil {
			continue // Skip invalid lines
		}
		if rule != nil {
			fu.addRuleToCompiled(compiled, rule)
		}
	}

	compiled.Version = fmt.Sprintf("manual-compile-%d", time.Now().Unix())
	compiled.CompiledAt = time.Now()

	return compiled, nil
}

// parseFilterLine parses a single filter line (simplified implementation)
func (fu *FilterUpdater) parseFilterLine(line string) (interface{}, error) {
	line = strings.TrimSpace(line)

	// Skip empty lines and comments
	if line == "" || strings.HasPrefix(line, "!") {
		return nil, nil
	}

	// This is a simplified parser - in reality, you'd use the converter package
	return map[string]string{
		"type": "filter",
		"line": line,
	}, nil
}

// addRuleToCompiled adds a rule to compiled filters
func (fu *FilterUpdater) addRuleToCompiled(compiled *CompiledFilters, rule interface{}) {
	// Simplified implementation - in reality, this would properly categorize rules
	ruleMap, ok := rule.(map[string]string)
	if !ok {
		return
	}

	line := ruleMap["line"]
	if strings.Contains(line, "##") {
		// Cosmetic filter
		parts := strings.SplitN(line, "##", 2)
		if len(parts) == 2 {
			domain := parts[0]
			selector := parts[1]

			if domain == "" {
				compiled.GenericHiding = append(compiled.GenericHiding, selector)
			} else {
				if compiled.CosmeticRules == nil {
					compiled.CosmeticRules = make(map[string][]string)
				}
				compiled.CosmeticRules[domain] = append(compiled.CosmeticRules[domain], selector)
			}
		}
	}
	// Network filters would be handled here too
}

// removeRuleFromCompiled marks a rule for removal
func (fu *FilterUpdater) removeRuleFromCompiled(compiled *CompiledFilters, rule interface{}) {
	// Implementation would mark rules for removal
	// For now, we'll just log the removal
	ruleMap, ok := rule.(map[string]string)
	if ok {
		logging.Debug(fmt.Sprintf("Marking rule for removal: %s", ruleMap["line"]))
	}
}

// applyUpdate applies a compiled filter update to the manager
func (fu *FilterUpdater) applyUpdate(update *FilterUpdate) error {
	if update.Compiled == nil {
		return fmt.Errorf("no compiled filters in update")
	}

	// Apply the update to the filter manager
	fu.manager.compileMutex.Lock()
	defer fu.manager.compileMutex.Unlock()

	// Merge the update with existing filters
	if fu.manager.compiled == nil {
		fu.manager.compiled = update.Compiled
	} else {
		fu.manager.compiled.Merge(update.Compiled)
	}

	// Update cosmetic rules
	if err := fu.manager.cosmeticInjector.InjectRules(update.Compiled.CosmeticRules); err != nil {
		return fmt.Errorf("failed to inject cosmetic rules: %w", err)
	}

	// Store the updated content for future diffs
	fu.storePreviousContent(update.ListID, update.Current)

	// Store the version for future update checks
	if update.Version != "" {
		if err := fu.manager.store.SetSourceVersion(update.URL, update.Version); err != nil {
			logging.Warn(fmt.Sprintf("Failed to store version for %s: %v", update.URL, err))
		}
	}

	logging.Info(fmt.Sprintf("Applied filter update for %s", update.URL))
	return nil
}

// getListID generates a unique ID for a filter list URL
func (fu *FilterUpdater) getListID(url string) string {
	hash := sha256.Sum256([]byte(url))
	return fmt.Sprintf("%x", hash)[:16]
}

// getPreviousContent retrieves previously stored filter list content
func (fu *FilterUpdater) getPreviousContent(listID string) []byte {
	// This would typically read from persistent storage
	// For now, return nil (no previous content)
	return nil
}

// storePreviousContent stores filter list content for future diff operations
func (fu *FilterUpdater) storePreviousContent(listID string, content []byte) {
	// This would typically write to persistent storage
	// For now, we'll just log the action
	logging.Debug(fmt.Sprintf("Storing %d bytes for list %s", len(content), listID))
}

// contentEqual checks if two byte slices contain the same content
func (fu *FilterUpdater) contentEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}

	hashA := sha256.Sum256(a)
	hashB := sha256.Sum256(b)

	return hashA == hashB
}

// Diff finds added and removed lines between two versions
func (de *DiffEngine) Diff(previous, current []byte) (added []string, removed []string) {
	prevLines := strings.Split(string(previous), "\n")
	currLines := strings.Split(string(current), "\n")

	// Create sets for efficient lookup
	prevSet := make(map[string]bool)
	currSet := make(map[string]bool)

	for _, line := range prevLines {
		line = strings.TrimSpace(line)
		if line != "" {
			prevSet[line] = true
		}
	}

	for _, line := range currLines {
		line = strings.TrimSpace(line)
		if line != "" {
			currSet[line] = true
		}
	}

	// Find added lines (in current but not in previous)
	for line := range currSet {
		if !prevSet[line] {
			added = append(added, line)
		}
	}

	// Find removed lines (in previous but not in current)
	for line := range prevSet {
		if !currSet[line] {
			removed = append(removed, line)
		}
	}

	return added, removed
}
