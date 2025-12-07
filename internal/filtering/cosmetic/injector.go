package cosmetic

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/bnema/dumber/internal/logging"
)

//go:embed injector.js
var cosmeticScript string

// CosmeticInjector manages cosmetic filter injection
type CosmeticInjector struct {
	mu            sync.RWMutex
	domainRules   map[string][]string
	genericRules  []string
	scriptEnabled bool
}

// CosmeticRule represents a cosmetic filter rule for JavaScript injection
type CosmeticRule struct {
	Domain   string `json:"domain,omitempty"`
	Selector string `json:"selector"`
}

// NewCosmeticInjector creates a new cosmetic filter injector
func NewCosmeticInjector() *CosmeticInjector {
	return &CosmeticInjector{
		domainRules:   make(map[string][]string),
		genericRules:  make([]string, 0),
		scriptEnabled: true,
	}
}

// InjectRules stores cosmetic rules for domain-specific filtering
func (ci *CosmeticInjector) InjectRules(rules map[string][]string) error {
	if !ci.scriptEnabled {
		return fmt.Errorf("cosmetic filtering disabled")
	}

	ci.mu.Lock()
	defer ci.mu.Unlock()

	// Clear existing rules
	ci.domainRules = make(map[string][]string)
	ci.genericRules = make([]string, 0)

	// Process rules
	for domain, selectors := range rules {
		if domain == "" {
			// Generic rules (apply to all domains)
			ci.genericRules = append(ci.genericRules, selectors...)
		} else {
			// Domain-specific rules
			ci.domainRules[domain] = selectors
		}
	}

	logging.Debug(fmt.Sprintf("Injected %d domain-specific rules and %d generic rules",
		len(ci.domainRules), len(ci.genericRules)))

	return nil
}

// GetScriptForDomain returns the complete cosmetic filtering script for a domain
func (ci *CosmeticInjector) GetScriptForDomain(domain string) string {
	if !ci.scriptEnabled {
		return ""
	}

	ci.mu.RLock()
	defer ci.mu.RUnlock()

	// Build rules for this specific domain (including parent domains)
	var rules []CosmeticRule

	// Add generic rules
	for _, selector := range ci.genericRules {
		rules = append(rules, CosmeticRule{
			Selector: selector,
		})
	}

	// Add domain-specific rules for the domain and its parent domains
	addDomainRules := func(domainKey string) {
		if domainKey == "" {
			return
		}
		if domainSelectors, exists := ci.domainRules[domainKey]; exists {
			for _, selector := range domainSelectors {
				rules = append(rules, CosmeticRule{
					Domain:   domainKey,
					Selector: selector,
				})
			}
		}
	}

	addDomainRules(domain)

	// Walk up parent domains to include base-domain rules for subdomains
	parts := strings.Split(domain, ".")
	for len(parts) > 2 {
		parts = parts[1:]
		parentDomain := strings.Join(parts, ".")
		addDomainRules(parentDomain)
	}

	// Convert rules to JSON
	rulesJSON, err := json.Marshal(rules)
	if err != nil {
		logging.Error(fmt.Sprintf("Failed to marshal cosmetic rules: %v", err))
		return cosmeticScript
	}

	// Build the complete script with domain-specific rules
	initScript := fmt.Sprintf(`
// Initialize cosmetic filtering for domain: %s
(function() {
    if (typeof window.__dumber_cosmetic_init === 'function') {
        window.__dumber_cosmetic_init(%s);
        if (window.self === window.top) {
            console.log('[dumber] Cosmetic filtering initialized with %d rules');
        }
    } else {
        if (window.self === window.top) {
            console.warn('[dumber] Cosmetic filtering not available');
        }
    }
})();`, domain, string(rulesJSON), len(rules))

	return cosmeticScript + "\n" + initScript
}

// UpdateRulesForDomain adds new rules for a specific domain
func (ci *CosmeticInjector) UpdateRulesForDomain(domain string, newSelectors []string) {
	if !ci.scriptEnabled {
		return
	}

	ci.mu.Lock()
	defer ci.mu.Unlock()

	if len(newSelectors) == 0 {
		return
	}

	if domain == "" {
		// Add to generic rules
		ci.genericRules = append(ci.genericRules, newSelectors...)
	} else {
		// Add to domain-specific rules
		if _, exists := ci.domainRules[domain]; !exists {
			ci.domainRules[domain] = make([]string, 0)
		}
		ci.domainRules[domain] = append(ci.domainRules[domain], newSelectors...)
	}

	logging.Debug(fmt.Sprintf("Added %d new cosmetic rules for domain: %s", len(newSelectors), domain))
}

// GetUpdateScript returns JavaScript to update rules dynamically
func (ci *CosmeticInjector) GetUpdateScript(newSelectors []string) string {
	if !ci.scriptEnabled || len(newSelectors) == 0 {
		return ""
	}

	ci.mu.RLock()
	defer ci.mu.RUnlock()

	selectorsJSON, err := json.Marshal(newSelectors)
	if err != nil {
		logging.Error(fmt.Sprintf("Failed to marshal update selectors: %v", err))
		return ""
	}

	return fmt.Sprintf(`
(function() {
    if (typeof window.__dumber_cosmetic_update === 'function') {
        window.__dumber_cosmetic_update(%s);
        if (window.self === window.top) {
            console.log('[dumber] Updated cosmetic rules with %d new selectors');
        }
    }
})();`, string(selectorsJSON), len(newSelectors))
}

// GetCleanupScript returns JavaScript to cleanup cosmetic filtering on navigation
func (ci *CosmeticInjector) GetCleanupScript() string {
	return `
(function() {
    if (typeof window.__dumber_cosmetic_cleanup === 'function') {
        window.__dumber_cosmetic_cleanup();
        if (window.self === window.top) {
            console.log('[dumber] Cosmetic filtering cleaned up');
        }
    }
})();`
}

// GetRulesForDomain returns all cosmetic rules that apply to a domain
func (ci *CosmeticInjector) GetRulesForDomain(domain string) []string {
	ci.mu.RLock()
	defer ci.mu.RUnlock()

	var rules []string

	// Add generic rules
	rules = append(rules, ci.genericRules...)

	// Add domain-specific rules
	if domainSelectors, exists := ci.domainRules[domain]; exists {
		rules = append(rules, domainSelectors...)
	}

	// Also check for subdomain matches
	domainParts := strings.Split(domain, ".")
	if len(domainParts) > 2 {
		// Try parent domain (e.g., "example.com" for "www.example.com")
		parentDomain := strings.Join(domainParts[1:], ".")
		if parentSelectors, exists := ci.domainRules[parentDomain]; exists {
			rules = append(rules, parentSelectors...)
		}
	}

	return rules
}

// Enable enables or disables cosmetic filtering
func (ci *CosmeticInjector) Enable(enabled bool) {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	ci.scriptEnabled = enabled
	if enabled {
		logging.Info("Cosmetic filtering enabled")
	} else {
		logging.Info("Cosmetic filtering disabled")
	}
}

// IsEnabled returns whether cosmetic filtering is enabled
func (ci *CosmeticInjector) IsEnabled() bool {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	return ci.scriptEnabled
}

// GetStats returns statistics about loaded cosmetic rules
func (ci *CosmeticInjector) GetStats() map[string]interface{} {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	domainCount := len(ci.domainRules)
	totalRules := len(ci.genericRules)

	for _, selectors := range ci.domainRules {
		totalRules += len(selectors)
	}

	return map[string]interface{}{
		"enabled":       ci.scriptEnabled,
		"generic_rules": len(ci.genericRules),
		"domain_count":  domainCount,
		"total_rules":   totalRules,
	}
}
