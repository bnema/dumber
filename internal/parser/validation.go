package parser

import (
	"net"
	"net/url"
	"regexp"
	"strings"
)

var (
	// domainRegex matches valid domain names
	domainRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?$`)

	// ipRegex matches IPv4 addresses

	// urlSchemeRegex matches URL schemes
	urlSchemeRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*://`)

	// shortcutRegex matches search shortcuts (e.g., "g:", "gh:")
	shortcutRegex = regexp.MustCompile(`^([a-zA-Z0-9]+):\s*(.*)$`)

	// Common top-level domains for validation
	commonTLDs = map[string]bool{
		"com": true, "org": true, "net": true, "edu": true, "gov": true, "mil": true,
		"int": true, "co": true, "io": true, "ai": true, "ly": true, "me": true,
		"tv": true, "cc": true, "to": true, "app": true, "dev": true, "tech": true,
		"info": true, "biz": true, "name": true, "museum": true, "travel": true,
		"xyz": true, "fr": true, "de": true, "uk": true, "ca": true, "us": true,
		"au": true, "jp": true, "cn": true, "in": true, "ru": true, "br": true,
	}
)

// URLValidator provides URL validation utilities.
type URLValidator struct{}

// NewURLValidator creates a new URLValidator.
func NewURLValidator() *URLValidator {
	return &URLValidator{}
}

// IsValidURL checks if the input represents a valid URL.
func (v *URLValidator) IsValidURL(input string) bool {
	if input == "" {
		return false
	}

	// Check if it already has a scheme
	if urlSchemeRegex.MatchString(input) {
		return v.isWellFormedURL(input)
	}

	// Check if it looks like a domain or IP
	return v.isDomain(input) || v.isIPAddress(input)
}

// IsDirectURL checks if the input should be treated as a direct URL.
func (v *URLValidator) IsDirectURL(input string) bool {
	if input == "" {
		return false
	}

	input = strings.TrimSpace(input)

	// Has explicit scheme
	if urlSchemeRegex.MatchString(input) {
		return v.isWellFormedURL(input)
	}

	// Looks like a domain
	if v.isDomain(input) {
		return true
	}

	// Looks like an IP address
	if v.isIPAddress(input) {
		return true
	}

	// Contains domain-like patterns
	if v.looksLikeDomain(input) {
		return true
	}

	return false
}

// IsSearchShortcut checks if the input matches a search shortcut pattern.
func (v *URLValidator) IsSearchShortcut(input string) (bool, string, string) {
	input = strings.TrimSpace(input)

	// Don't match URLs with schemes (like https://example.com)
	if urlSchemeRegex.MatchString(input) {
		return false, "", ""
	}

	matches := shortcutRegex.FindStringSubmatch(input)
	if len(matches) != 3 {
		return false, "", ""
	}

	shortcutKey := strings.ToLower(matches[1])
	query := strings.TrimSpace(matches[2])

	return true, shortcutKey, query
}

// NormalizeURL normalizes a URL by adding scheme if missing.
func (v *URLValidator) NormalizeURL(input string) string {
	input = strings.TrimSpace(input)

	if input == "" {
		return input
	}

	// Already has a scheme
	if urlSchemeRegex.MatchString(input) {
		return input
	}

	// Add https:// for domains and IPs
	if v.isDomain(input) || v.isIPAddress(input) || v.looksLikeDomain(input) {
		return "https://" + input
	}

	return input
}

// ExtractDomain extracts the domain from a URL.
func (v *URLValidator) ExtractDomain(rawURL string) string {
	// Parse the URL
	parsedURL, err := url.Parse(v.NormalizeURL(rawURL))
	if err != nil {
		// Fallback: try to extract domain manually
		return v.extractDomainManual(rawURL)
	}

	// Extract hostname (without port) from Host
	host := parsedURL.Host
	if host == "" {
		return v.extractDomainManual(rawURL)
	}

	// Remove port from host if present
	hostname := host
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		// Check if this looks like a port (numeric)
		portPart := host[idx+1:]
		if isNumeric(portPart) {
			hostname = host[:idx]
		}
	}

	// Handle IPv6 addresses in brackets
	if strings.HasPrefix(hostname, "[") && strings.HasSuffix(hostname, "]") {
		hostname = hostname[1 : len(hostname)-1]
	}

	return hostname
}

// extractDomainManual manually extracts domain when URL parsing fails.
func (v *URLValidator) extractDomainManual(rawURL string) string {
	// Handle empty or invalid input
	if rawURL == "" {
		return ""
	}

	// Remove protocol
	domain := rawURL
	if strings.HasPrefix(domain, "http://") {
		domain = domain[7:]
	} else if strings.HasPrefix(domain, "https://") {
		domain = domain[8:]
	}

	// Handle IPv6 addresses in brackets
	if strings.HasPrefix(domain, "[") {
		if idx := strings.Index(domain, "]"); idx != -1 {
			return domain[1:idx] // Return IPv6 address without brackets
		}
	}

	// Remove path
	if idx := strings.Index(domain, "/"); idx != -1 {
		domain = domain[:idx]
	}

	// Remove query and fragment
	if idx := strings.Index(domain, "?"); idx != -1 {
		domain = domain[:idx]
	}
	if idx := strings.Index(domain, "#"); idx != -1 {
		domain = domain[:idx]
	}

	// Remove port - be careful with IPv6 addresses
	if !strings.Contains(domain, "::") { // Not IPv6
		if idx := strings.LastIndex(domain, ":"); idx != -1 {
			// Check if this is actually a port (numeric)
			portPart := domain[idx+1:]
			if isNumeric(portPart) {
				domain = domain[:idx]
			}
		}
	}

	return domain
}

// isDomain checks if the input is a valid domain name.
func (v *URLValidator) isDomain(input string) bool {
	if !domainRegex.MatchString(input) {
		return false
	}

	// Check for at least one dot (to distinguish from single words)
	if !strings.Contains(input, ".") {
		return false
	}

	// Validate TLD
	parts := strings.Split(input, ".")
	if len(parts) < 2 {
		return false
	}

	tld := strings.ToLower(parts[len(parts)-1])

	// Check against common TLDs or validate length/format
	return commonTLDs[tld] || (len(tld) >= 2 && len(tld) <= 6)
}

// isIPAddress checks if the input is a valid IP address.
func (v *URLValidator) isIPAddress(input string) bool {
	// Try parsing as IP
	ip := net.ParseIP(input)
	return ip != nil
}

// looksLikeDomain checks if the input looks like a domain (more lenient).
func (v *URLValidator) looksLikeDomain(input string) bool {
	// Extract just the domain part if there's a path
	domainPart := input
	if slashIndex := strings.Index(input, "/"); slashIndex != -1 {
		domainPart = input[:slashIndex]
	}

	// Must contain at least one dot
	if !strings.Contains(domainPart, ".") {
		return false
	}

	// Must not contain spaces
	if strings.Contains(domainPart, " ") {
		return false
	}

	// Must not be too long
	if len(domainPart) > 253 {
		return false
	}

	// Check basic domain-like structure
	parts := strings.Split(domainPart, ".")
	if len(parts) < 2 {
		return false
	}

	// Each part should be reasonable
	for _, part := range parts {
		if len(part) == 0 || len(part) > 63 {
			return false
		}
		// Should start and end with alphanumeric
		if len(part) > 0 {
			if !isAlphaNumeric(part[0]) || !isAlphaNumeric(part[len(part)-1]) {
				return false
			}
		}
	}

	return true
}

// isWellFormedURL checks if a URL with scheme is well-formed.
func (v *URLValidator) isWellFormedURL(rawURL string) bool {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	// Must have a scheme and host
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return false
	}

	// Validate scheme
	validSchemes := map[string]bool{
		"http": true, "https": true, "ftp": true, "ftps": true,
		"file": true, "mailto": true, "tel": true, "sms": true,
	}

	return validSchemes[strings.ToLower(parsedURL.Scheme)]
}

// HasFileExtension checks if the URL path has a file extension.
func (v *URLValidator) HasFileExtension(rawURL string) bool {
	parsedURL, err := url.Parse(v.NormalizeURL(rawURL))
	if err != nil {
		return false
	}

	path := parsedURL.Path
	if path == "" || path == "/" {
		return false
	}

	// Check for file extension
	lastSlash := strings.LastIndex(path, "/")
	fileName := path
	if lastSlash != -1 && lastSlash < len(path)-1 {
		fileName = path[lastSlash+1:]
	}

	return strings.Contains(fileName, ".") && !strings.HasSuffix(fileName, ".")
}

// IsLocalhost checks if the URL is pointing to localhost.
func (v *URLValidator) IsLocalhost(rawURL string) bool {
	// Handle direct localhost strings (no protocol)
	rawURL = strings.ToLower(strings.TrimSpace(rawURL))
	if rawURL == "localhost" || rawURL == "127.0.0.1" || rawURL == "::1" {
		return true
	}

	domain := v.ExtractDomain(rawURL)
	domain = strings.ToLower(domain)

	// Remove port if present
	if idx := strings.LastIndex(domain, ":"); idx != -1 {
		portPart := domain[idx+1:]
		if isNumeric(portPart) {
			domain = domain[:idx]
		}
	}

	return domain == "localhost" ||
		domain == "127.0.0.1" ||
		domain == "::1" ||
		strings.HasSuffix(domain, ".local")
}

// SanitizeInput sanitizes user input for URL parsing.
func (v *URLValidator) SanitizeInput(input string) string {
	// Trim whitespace
	input = strings.TrimSpace(input)

	// Remove control characters
	result := make([]rune, 0, len(input))
	for _, r := range input {
		if r >= 32 && r < 127 || r > 127 { // Keep printable ASCII and non-ASCII
			result = append(result, r)
		}
	}

	return string(result)
}

// Helper functions

// isNumeric checks if a string contains only digits.
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isAlphaNumeric checks if a character is alphanumeric.
func isAlphaNumeric(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// GetURLType determines the type of URL (direct, search, etc.).
func (v *URLValidator) GetURLType(input string) InputType {
	input = v.SanitizeInput(input)

	// Check for direct URL first (this includes URLs with schemes)
	if v.IsDirectURL(input) {
		return InputTypeDirectURL
	}

	// Check for search shortcut (but exclude URL schemes)
	if isShortcut, _, _ := v.IsSearchShortcut(input); isShortcut {
		return InputTypeSearchShortcut
	}

	// Everything else is treated as search query initially
	// The parser will determine if it should be history search or web search
	return InputTypeHistorySearch
}
