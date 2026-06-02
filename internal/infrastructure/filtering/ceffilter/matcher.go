package ceffilter

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	domainurl "github.com/bnema/dumber/internal/domain/url"
)

// The initial CEF artifact format intentionally reuses the existing Safari
// Content Blocker JSON generated for WebKit: each file is a JSON array of
// {trigger, action} rules. The CEF backend interprets the network subset:
// block rules and ignore-previous-rules exceptions. It ignores unsupported
// cosmetic or extension-only behavior. This keeps the release/download pipeline
// shared while CEF gains request interception without browser extensions.

type ResourceType string

const (
	ResourceTypeDocument   ResourceType = "document"
	ResourceTypeImage      ResourceType = "image"
	ResourceTypeStyleSheet ResourceType = "style-sheet"
	ResourceTypeScript     ResourceType = "script"
	ResourceTypeFont       ResourceType = "font"
	ResourceTypeMedia      ResourceType = "media"
	ResourceTypeXHR        ResourceType = "xhr"
	ResourceTypeRaw        ResourceType = "raw"
)

type Request struct {
	URL              string
	ResourceType     ResourceType
	RequestInitiator string
	FrameURL         string
	IsNavigation     bool
}

type Matcher struct {
	rules   []rule
	skipped int
}

type ruleAction string

const (
	ruleActionBlock               ruleAction = "block"
	ruleActionIgnorePreviousRules ruleAction = "ignore-previous-rules"
)

type rule struct {
	action        ruleAction
	pattern       *regexp.Regexp
	resourceTypes map[ResourceType]struct{}
	loadTypes     map[string]struct{}
	ifDomains     []string
	unlessDomains []string
}

type rawRule struct {
	Trigger rawTrigger `json:"trigger"`
	Action  rawAction  `json:"action"`
}

type rawTrigger struct {
	URLFilter                string   `json:"url-filter"`
	URLFilterIsCaseSensitive bool     `json:"url-filter-is-case-sensitive"`
	ResourceType             []string `json:"resource-type"`
	LoadType                 []string `json:"load-type"`
	IfDomain                 []string `json:"if-domain"`
	UnlessDomain             []string `json:"unless-domain"`
}

type rawAction struct {
	Type string `json:"type"`
}

func NewMatcherFromFiles(paths []string) (*Matcher, error) {
	rules := make([]rule, 0)
	var skipped int
	for _, path := range paths {
		fileRules, fileSkipped, err := loadRulesFromFile(path)
		if err != nil {
			return nil, err
		}
		rules = append(rules, fileRules...)
		skipped += fileSkipped
	}
	if len(rules) == 0 {
		return nil, fmt.Errorf("no supported CEF filter rules loaded from %d file(s); skipped=%d", len(paths), skipped)
	}
	return &Matcher{rules: rules, skipped: skipped}, nil
}

func loadRulesFromFile(path string) ([]rule, int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, fmt.Errorf("read %s: %w", path, err)
	}
	var rawRules []rawRule
	if err := json.Unmarshal(data, &rawRules); err != nil {
		return nil, 0, fmt.Errorf("parse %s: %w", path, err)
	}

	rules := make([]rule, 0, len(rawRules))
	skipped := 0
	for _, raw := range rawRules {
		compiled, ok := compileRule(raw)
		if !ok {
			skipped++
			continue
		}
		rules = append(rules, compiled)
	}
	return rules, skipped, nil
}

func compileRule(raw rawRule) (rule, bool) {
	action := parseRuleAction(raw.Action.Type)
	if action == "" {
		return rule{}, false
	}
	patternText := strings.TrimSpace(raw.Trigger.URLFilter)
	if patternText == "" {
		return rule{}, false
	}
	if !raw.Trigger.URLFilterIsCaseSensitive {
		patternText = "(?i:" + patternText + ")"
	}
	pattern, err := regexp.Compile(patternText)
	if err != nil {
		return rule{}, false
	}
	return rule{
		action:        action,
		pattern:       pattern,
		resourceTypes: parseResourceTypes(raw.Trigger.ResourceType),
		loadTypes:     parseLoadTypes(raw.Trigger.LoadType),
		ifDomains:     normalizeDomainPatterns(raw.Trigger.IfDomain),
		unlessDomains: normalizeDomainPatterns(raw.Trigger.UnlessDomain),
	}, true
}

func parseRuleAction(value string) ruleAction {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(ruleActionBlock):
		return ruleActionBlock
	case string(ruleActionIgnorePreviousRules):
		return ruleActionIgnorePreviousRules
	default:
		return ""
	}
}

func parseResourceTypes(values []string) map[ResourceType]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := make(map[ResourceType]struct{}, len(values))
	for _, value := range values {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "document", "svg-document":
			out[ResourceTypeDocument] = struct{}{}
		case "image":
			out[ResourceTypeImage] = struct{}{}
		case "style-sheet", "stylesheet":
			out[ResourceTypeStyleSheet] = struct{}{}
		case "script":
			out[ResourceTypeScript] = struct{}{}
		case "font":
			out[ResourceTypeFont] = struct{}{}
		case "media":
			out[ResourceTypeMedia] = struct{}{}
		case "xhr", "fetch":
			out[ResourceTypeXHR] = struct{}{}
		case "raw", "popup":
			out[ResourceTypeRaw] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseLoadTypes(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "first-party", "third-party":
			out[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeDomainPatterns(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		value = strings.TrimPrefix(value, "*")
		value = strings.TrimPrefix(value, ".")
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func (m *Matcher) RuleCount() int {
	if m == nil {
		return 0
	}
	return len(m.rules)
}

func (m *Matcher) SkippedRuleCount() int {
	if m == nil {
		return 0
	}
	return m.skipped
}

func (m *Matcher) ShouldBlock(req Request) bool {
	if m == nil || len(m.rules) == 0 || isBypassedURL(req.URL) {
		return false
	}
	requestURL := strings.TrimSpace(req.URL)
	if requestURL == "" {
		return false
	}
	reqDomain := canonicalDomain(requestURL)
	documentDomain := documentDomain(req)
	thirdParty := isThirdParty(reqDomain, documentDomain)

	blocked := false
	for _, rule := range m.rules {
		if !rule.matches(requestURL, req.ResourceType, documentDomain, thirdParty) {
			continue
		}
		switch rule.action {
		case ruleActionBlock:
			blocked = true
		case ruleActionIgnorePreviousRules:
			blocked = false
		}
	}
	return blocked
}

func (r rule) matches(requestURL string, resourceType ResourceType, documentDomain string, thirdParty bool) bool {
	if r.pattern == nil || !r.pattern.MatchString(requestURL) {
		return false
	}
	if len(r.resourceTypes) > 0 {
		if _, ok := r.resourceTypes[resourceType]; !ok {
			return false
		}
	}
	if len(r.loadTypes) > 0 {
		loadType := "first-party"
		if thirdParty {
			loadType = "third-party"
		}
		if _, ok := r.loadTypes[loadType]; !ok {
			return false
		}
	}
	if len(r.ifDomains) > 0 && !matchesAnyDomain(documentDomain, r.ifDomains) {
		return false
	}
	if len(r.unlessDomains) > 0 && matchesAnyDomain(documentDomain, r.unlessDomains) {
		return false
	}
	return true
}

func documentDomain(req Request) string {
	if req.IsNavigation {
		return canonicalDomain(req.URL)
	}
	if domain := canonicalDomain(req.RequestInitiator); domain != "" {
		return domain
	}
	if domain := canonicalDomain(req.FrameURL); domain != "" {
		return domain
	}
	return canonicalDomain(req.URL)
}

func canonicalDomain(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return domainurl.CanonicalDomain(parsed.Hostname())
}

func isThirdParty(requestDomain, documentDomain string) bool {
	if requestDomain == "" || documentDomain == "" {
		return false
	}
	return !sameSiteDomain(requestDomain, documentDomain)
}

func matchesAnyDomain(domain string, patterns []string) bool {
	for _, pattern := range patterns {
		if domainMatchesPattern(domain, pattern) {
			return true
		}
	}
	return false
}

func domainMatchesPattern(domain, pattern string) bool {
	domain = strings.ToLower(strings.TrimSpace(domain))
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if domain == "" || pattern == "" {
		return false
	}
	return domain == pattern || strings.HasSuffix(domain, "."+pattern)
}

func sameSiteDomain(a, b string) bool {
	return domainMatchesPattern(a, b) || domainMatchesPattern(b, a)
}

func isBypassedURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return true
	}
	scheme := strings.ToLower(parsed.Scheme)
	switch scheme {
	case "http", "https":
		return strings.EqualFold(parsed.Hostname(), "dumber.invalid")
	case "dumb", "about", "data", "blob", "file", "javascript":
		return true
	default:
		return true
	}
}
