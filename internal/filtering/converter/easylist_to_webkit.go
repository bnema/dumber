package converter

import (
	"encoding/json"
	"regexp"
	"strings"
)

const (
	cosmeticFilterParts = 2 // Number of parts when splitting cosmetic filter by ##
)

// FilterConverter converts EasyList filters to WebKit format
type FilterConverter struct {
	networkRules  []WebKitRule
	cosmeticRules map[string][]string // domain -> selectors
	genericHiding []string            // ##selector rules
}

// NewFilterConverter creates a new filter converter
func NewFilterConverter() *FilterConverter {
	return &FilterConverter{
		networkRules:  make([]WebKitRule, 0),
		cosmeticRules: make(map[string][]string),
		genericHiding: make([]string, 0),
	}
}

// isASCII checks if string contains only ASCII characters
func isASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

// ConvertEasyListLine converts a single EasyList line to WebKit format
func (fc *FilterConverter) ConvertEasyListLine(line string) error {
	line = strings.TrimSpace(line)

	// Skip comments and empty lines
	if line == "" || strings.HasPrefix(line, "!") {
		return nil
	}

	// Skip metadata lines like [Adblock Plus 2.0]
	if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
		return nil
	}

	// Skip lines with non-ASCII characters (WebKit doesn't support them in regex)
	if !isASCII(line) {
		return nil
	}

	// Cosmetic filters (element hiding)
	if strings.Contains(line, "##") {
		return fc.parseCosmeticFilter(line)
	}

	// Network filters
	return fc.parseNetworkFilter(line)
}

func (fc *FilterConverter) parseCosmeticFilter(line string) error {
	parts := strings.SplitN(line, "##", cosmeticFilterParts)
	if len(parts) != cosmeticFilterParts {
		return nil
	}

	domain := parts[0]
	selector := parts[1]

	// Handle :has(), :not(), :nth-child() pseudo-selectors
	selector = fc.convertAdvancedSelectors(selector)

	if domain == "" {
		// Generic hiding rule
		fc.genericHiding = append(fc.genericHiding, selector)
	} else {
		// Domain-specific rule
		domains := strings.Split(domain, ",")
		for _, d := range domains {
			d = strings.TrimSpace(d)
			if strings.HasPrefix(d, "~") {
				// Exception domain - handle separately
				continue
			}
			fc.cosmeticRules[d] = append(fc.cosmeticRules[d], selector)
		}
	}
	return nil
}

func (fc *FilterConverter) parseNetworkFilter(line string) error {
	// Handle exception rules (@@)
	isException := false
	if strings.HasPrefix(line, "@@") {
		isException = true
		line = line[2:] // Remove @@
	}

	// Remove options (everything after $)
	filterPart := line
	var options []string

	if idx := strings.Index(line, "$"); idx != -1 {
		filterPart = line[:idx]
		optStr := line[idx+1:]
		options = strings.Split(optStr, ",")
	}

	// Skip empty filter parts after processing
	if strings.TrimSpace(filterPart) == "" {
		return nil
	}

	// Convert to WebKit regex
	actionType := ActionTypeBlock
	if isException {
		actionType = ActionTypeIgnorePreviousRules
	}

	rule := WebKitRule{
		Trigger: Trigger{
			URLFilter: fc.convertToRegex(filterPart),
		},
		Action: Action{
			Type: actionType,
		},
	}

	// Parse options
	for _, opt := range options {
		opt = strings.TrimSpace(opt)
		switch {
		case opt == "third-party":
			rule.Trigger.LoadType = []string{LoadTypeThirdParty}
		case opt == "~third-party":
			rule.Trigger.LoadType = []string{LoadTypeFirstParty}
		case strings.HasPrefix(opt, "domain="):
			domains := strings.Split(opt[7:], "|")
			var ifDomains, unlessDomains []string
			for _, d := range domains {
				if strings.HasPrefix(d, "~") {
					unlessDomains = append(unlessDomains, d[1:])
				} else {
					ifDomains = append(ifDomains, d)
				}
			}
			if len(ifDomains) > 0 {
				rule.Trigger.IfDomain = ifDomains
			}
			if len(unlessDomains) > 0 {
				rule.Trigger.UnlessDomain = unlessDomains
			}
		case opt == "script":
			rule.Trigger.ResourceType = []string{ResourceTypeScript}
		case opt == "image":
			rule.Trigger.ResourceType = []string{ResourceTypeImage}
		case opt == "stylesheet":
			rule.Trigger.ResourceType = []string{ResourceTypeStyleSheet}
		case opt == "font":
			rule.Trigger.ResourceType = []string{ResourceTypeFont}
		case opt == "media":
			rule.Trigger.ResourceType = []string{ResourceTypeMedia}
		case opt == "document":
			rule.Trigger.ResourceType = []string{ResourceTypeDocument}
		case opt == "popup":
			rule.Trigger.ResourceType = []string{ResourceTypePopup}
		}
	}

	fc.networkRules = append(fc.networkRules, rule)
	return nil
}

func (fc *FilterConverter) convertToRegex(pattern string) string {
	// Handle anchors BEFORE escaping special characters
	hasStartAnchor := strings.HasPrefix(pattern, "||")
	hasEndAnchor := strings.HasSuffix(pattern, "|") && !strings.HasSuffix(pattern, "||")
	hasPipeStart := strings.HasPrefix(pattern, "|") && !strings.HasPrefix(pattern, "||")

	// Remove anchors from pattern
	if hasStartAnchor {
		pattern = pattern[2:]
	} else if hasPipeStart {
		pattern = pattern[1:]
	}

	if hasEndAnchor {
		pattern = pattern[:len(pattern)-1]
	}

	// Now escape special characters
	pattern = regexp.QuoteMeta(pattern)

	// Convert wildcards after escaping
	pattern = strings.ReplaceAll(pattern, `\*`, ".*")
	pattern = strings.ReplaceAll(pattern, `\^`, "[^a-zA-Z0-9_.%-]")

	// Apply anchors to regex
	if hasStartAnchor {
		// || means start of domain - match protocol and optional subdomain
		pattern = "^https?://([^/]*\\.)?" + pattern
	} else if hasPipeStart {
		// | means start of URL
		pattern = "^" + pattern
	}

	if hasEndAnchor {
		// | means end of URL
		pattern = pattern + "$"
	}

	return pattern
}

func (fc *FilterConverter) convertAdvancedSelectors(selector string) string {
	// For now, return as-is. Advanced selector conversion would be more complex
	// and would require parsing CSS selectors
	return selector
}

// GetNetworkRules returns compiled network blocking rules
func (fc *FilterConverter) GetNetworkRules() []WebKitRule {
	return fc.networkRules
}

// GetCosmeticRules returns cosmetic filtering rules organized by domain
func (fc *FilterConverter) GetCosmeticRules() map[string][]string {
	return fc.cosmeticRules
}

// GetGenericHiding returns generic cosmetic hiding rules
func (fc *FilterConverter) GetGenericHiding() []string {
	return fc.genericHiding
}

// ToJSON converts network rules to JSON format for WebKit
func (fc *FilterConverter) ToJSON() ([]byte, error) {
	return json.Marshal(fc.networkRules)
}

// Reset clears all rules from the converter
func (fc *FilterConverter) Reset() {
	fc.networkRules = make([]WebKitRule, 0)
	fc.cosmeticRules = make(map[string][]string)
	fc.genericHiding = make([]string, 0)
}
