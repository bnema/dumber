package api

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
)

// Permissions represents an extension's permissions
type Permissions struct {
	Permissions []string `json:"permissions,omitempty"`
	Origins     []string `json:"origins,omitempty"`
}

// PermissionChangeListener is called when permissions change
type PermissionChangeListener func(permissions Permissions)

// PermissionsAPIDispatcher handles permissions.* API calls
type PermissionsAPIDispatcher struct {
	mu sync.RWMutex

	// Extension's manifest permissions (static, granted at install time)
	manifestPermissions map[string]bool
	manifestOrigins     []string

	// Optional permissions that can be requested at runtime
	optionalDeclared    []string // From manifest optional_permissions
	optionalHostsDeclared []string // From manifest optional_host_permissions

	// Currently granted optional permissions
	optionalPermissions map[string]bool
	optionalOrigins     []string

	// Event listeners
	onAddedListeners   []PermissionChangeListener
	onRemovedListeners []PermissionChangeListener
}

// NewPermissionsAPIDispatcher creates a new permissions API dispatcher
func NewPermissionsAPIDispatcher(manifestPermissions []string, manifestOrigins []string) *PermissionsAPIDispatcher {
	// Convert permissions array to map for fast lookup
	permMap := make(map[string]bool, len(manifestPermissions))
	for _, perm := range manifestPermissions {
		permMap[perm] = true
	}

	return &PermissionsAPIDispatcher{
		manifestPermissions:   permMap,
		manifestOrigins:       manifestOrigins,
		optionalDeclared:      make([]string, 0),
		optionalHostsDeclared: make([]string, 0),
		optionalPermissions:   make(map[string]bool),
		optionalOrigins:       make([]string, 0),
		onAddedListeners:      make([]PermissionChangeListener, 0),
		onRemovedListeners:    make([]PermissionChangeListener, 0),
	}
}

// NewPermissionsAPIDispatcherWithOptional creates a dispatcher with optional permissions declared in manifest
func NewPermissionsAPIDispatcherWithOptional(manifestPermissions, manifestOrigins, optionalPerms, optionalHosts []string) *PermissionsAPIDispatcher {
	d := NewPermissionsAPIDispatcher(manifestPermissions, manifestOrigins)
	d.optionalDeclared = optionalPerms
	d.optionalHostsDeclared = optionalHosts
	return d
}

// Contains checks if the extension has the specified permissions
func (d *PermissionsAPIDispatcher) Contains(ctx context.Context, details map[string]interface{}) (*PermissionsResult, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Extract requested permissions
	requestedPerms, _ := details["permissions"].([]interface{})
	requestedOrigins, _ := details["origins"].([]interface{})

	result := &PermissionsResult{HasPermissions: true}

	// Check if all requested permissions are granted
	for _, perm := range requestedPerms {
		permStr, ok := perm.(string)
		if !ok {
			continue
		}
		if !d.hasPermissionLocked(permStr) {
			result.HasPermissions = false
			break
		}
	}

	// Check if all requested origins are granted
	if result.HasPermissions {
		for _, origin := range requestedOrigins {
			originStr, ok := origin.(string)
			if !ok {
				continue
			}
			if !d.hasOriginLocked(originStr) {
				result.HasPermissions = false
				break
			}
		}
	}

	return result, nil
}

// GetAll returns all current permissions
func (d *PermissionsAPIDispatcher) GetAll(ctx context.Context) (*Permissions, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Combine manifest and optional permissions
	allPerms := make([]string, 0, len(d.manifestPermissions)+len(d.optionalPermissions))
	for perm := range d.manifestPermissions {
		allPerms = append(allPerms, perm)
	}
	for perm := range d.optionalPermissions {
		allPerms = append(allPerms, perm)
	}

	// Combine manifest and optional origins
	allOrigins := make([]string, 0, len(d.manifestOrigins)+len(d.optionalOrigins))
	allOrigins = append(allOrigins, d.manifestOrigins...)
	allOrigins = append(allOrigins, d.optionalOrigins...)

	return &Permissions{
		Permissions: allPerms,
		Origins:     allOrigins,
	}, nil
}

// Request adds new permissions
// For declared optional permissions, this grants them automatically
// (In a full browser implementation, this would show a user prompt)
func (d *PermissionsAPIDispatcher) Request(ctx context.Context, details map[string]interface{}) (*PermissionsResult, error) {
	// Extract requested permissions
	requestedPerms, _ := details["permissions"].([]interface{})
	requestedOrigins, _ := details["origins"].([]interface{})

	d.mu.Lock()
	defer d.mu.Unlock()

	// Check if already granted
	allGranted := true
	permsToGrant := make([]string, 0)
	originsToGrant := make([]string, 0)

	for _, perm := range requestedPerms {
		permStr, ok := perm.(string)
		if !ok {
			continue
		}
		if !d.hasPermissionLocked(permStr) {
			// Check if it's a declared optional permission
			if d.isOptionalPermissionDeclared(permStr) {
				permsToGrant = append(permsToGrant, permStr)
			} else {
				allGranted = false
			}
		}
	}

	for _, origin := range requestedOrigins {
		originStr, ok := origin.(string)
		if !ok {
			continue
		}
		if !d.hasOriginLocked(originStr) {
			// Check if it matches a declared optional host pattern
			if d.isOptionalOriginDeclared(originStr) {
				originsToGrant = append(originsToGrant, originStr)
			} else {
				allGranted = false
			}
		}
	}

	if !allGranted {
		return &PermissionsResult{HasPermissions: false}, nil
	}

	// Grant the optional permissions
	addedPerms := make([]string, 0)
	addedOrigins := make([]string, 0)

	for _, perm := range permsToGrant {
		if !d.optionalPermissions[perm] {
			d.optionalPermissions[perm] = true
			addedPerms = append(addedPerms, perm)
		}
	}

	for _, origin := range originsToGrant {
		if !containsString(d.optionalOrigins, origin) {
			d.optionalOrigins = append(d.optionalOrigins, origin)
			addedOrigins = append(addedOrigins, origin)
		}
	}

	// Fire onAdded event if any permissions were added
	if len(addedPerms) > 0 || len(addedOrigins) > 0 {
		added := Permissions{
			Permissions: addedPerms,
			Origins:     addedOrigins,
		}
		for _, listener := range d.onAddedListeners {
			go listener(added)
		}
	}

	return &PermissionsResult{HasPermissions: true}, nil
}

// isOptionalPermissionDeclared checks if a permission is declared in optional_permissions
func (d *PermissionsAPIDispatcher) isOptionalPermissionDeclared(perm string) bool {
	for _, opt := range d.optionalDeclared {
		if opt == perm {
			return true
		}
	}
	return false
}

// isOptionalOriginDeclared checks if an origin matches a declared optional host pattern
func (d *PermissionsAPIDispatcher) isOptionalOriginDeclared(origin string) bool {
	for _, pattern := range d.optionalHostsDeclared {
		if MatchURLPattern(pattern, origin) {
			return true
		}
	}
	return false
}

// containsString checks if a slice contains a string
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// Remove removes permissions (only works for optional permissions)
func (d *PermissionsAPIDispatcher) Remove(ctx context.Context, details map[string]interface{}) (*PermissionsResult, error) {
	// Extract permissions to remove
	permsToRemove, _ := details["permissions"].([]interface{})
	originsToRemove, _ := details["origins"].([]interface{})

	d.mu.Lock()
	defer d.mu.Unlock()

	removedPerms := make([]string, 0)
	removedOrigins := make([]string, 0)

	// Can only remove optional permissions, not manifest permissions
	for _, perm := range permsToRemove {
		permStr, ok := perm.(string)
		if !ok {
			continue
		}
		if d.optionalPermissions[permStr] {
			delete(d.optionalPermissions, permStr)
			removedPerms = append(removedPerms, permStr)
		}
	}

	// Remove optional origins
	for _, origin := range originsToRemove {
		originStr, ok := origin.(string)
		if !ok {
			continue
		}
		// Remove from optional origins slice
		for i, o := range d.optionalOrigins {
			if o == originStr {
				d.optionalOrigins = append(d.optionalOrigins[:i], d.optionalOrigins[i+1:]...)
				removedOrigins = append(removedOrigins, originStr)
				break
			}
		}
	}

	// Fire onRemoved event if any permissions were removed
	if len(removedPerms) > 0 || len(removedOrigins) > 0 {
		removed := Permissions{
			Permissions: removedPerms,
			Origins:     removedOrigins,
		}
		for _, listener := range d.onRemovedListeners {
			go listener(removed)
		}
	}

	return &PermissionsResult{HasPermissions: len(removedPerms) > 0 || len(removedOrigins) > 0}, nil
}

// hasPermissionLocked checks if permission is granted (must hold lock)
func (d *PermissionsAPIDispatcher) hasPermissionLocked(permission string) bool {
	return d.manifestPermissions[permission] || d.optionalPermissions[permission]
}

// hasOriginLocked checks if origin is granted (must hold lock)
func (d *PermissionsAPIDispatcher) hasOriginLocked(origin string) bool {
	// Check manifest origins
	for _, pattern := range d.manifestOrigins {
		if MatchURLPattern(pattern, origin) {
			return true
		}
	}
	// Check optional origins
	for _, pattern := range d.optionalOrigins {
		if MatchURLPattern(pattern, origin) {
			return true
		}
	}
	return false
}

// OnAdded registers a listener for the onAdded event
func (d *PermissionsAPIDispatcher) OnAdded(listener PermissionChangeListener) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onAddedListeners = append(d.onAddedListeners, listener)
}

// OnRemoved registers a listener for the onRemoved event
func (d *PermissionsAPIDispatcher) OnRemoved(listener PermissionChangeListener) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onRemovedListeners = append(d.onRemovedListeners, listener)
}

// PermissionsResult represents the result of contains/request/remove operations
type PermissionsResult struct {
	HasPermissions bool `json:"result"`
}

// MatchURLPattern matches a URL against a WebExtension match pattern
// Pattern format: <scheme>://<host>/<path>
// - scheme: *, http, https, ws, wss, ftp, file, etc.
// - host: *, *.domain.com, or exact host
// - path: path with * wildcards
func MatchURLPattern(pattern, urlStr string) bool {
	// Special case: <all_urls> matches all supported schemes
	if pattern == "<all_urls>" {
		return isValidScheme(urlStr)
	}

	// Exact match
	if pattern == urlStr {
		return true
	}

	// Parse the pattern
	patternScheme, patternHost, patternPath, err := parseMatchPattern(pattern)
	if err != nil {
		return false
	}

	// Parse the URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	// Match scheme
	if !matchScheme(patternScheme, parsedURL.Scheme) {
		return false
	}

	// Match host
	if !matchHost(patternHost, parsedURL.Host) {
		return false
	}

	// Match path
	if !matchPath(patternPath, parsedURL.Path) {
		return false
	}

	return true
}

// parseMatchPattern parses a WebExtension match pattern into components
func parseMatchPattern(pattern string) (scheme, host, path string, err error) {
	// Find scheme separator
	schemeEnd := strings.Index(pattern, "://")
	if schemeEnd == -1 {
		return "", "", "", fmt.Errorf("invalid pattern: no scheme separator")
	}

	scheme = pattern[:schemeEnd]
	rest := pattern[schemeEnd+3:]

	// Find path separator
	pathStart := strings.Index(rest, "/")
	if pathStart == -1 {
		return "", "", "", fmt.Errorf("invalid pattern: no path")
	}

	host = rest[:pathStart]
	path = rest[pathStart:]

	return scheme, host, path, nil
}

// matchScheme checks if URL scheme matches pattern scheme
func matchScheme(patternScheme, urlScheme string) bool {
	switch patternScheme {
	case "*":
		// * matches http, https, ws, wss
		switch urlScheme {
		case "http", "https", "ws", "wss":
			return true
		}
		return false
	default:
		return patternScheme == urlScheme
	}
}

// matchHost checks if URL host matches pattern host
func matchHost(patternHost, urlHost string) bool {
	// Remove port from URL host if present
	host := urlHost
	if colonIdx := strings.LastIndex(urlHost, ":"); colonIdx != -1 {
		// Check if it's actually a port (not part of IPv6)
		if !strings.Contains(urlHost[colonIdx:], "]") {
			host = urlHost[:colonIdx]
		}
	}

	switch {
	case patternHost == "*":
		// * matches any host
		return true
	case strings.HasPrefix(patternHost, "*."):
		// *.domain.com matches domain.com and any subdomain
		baseDomain := patternHost[2:]
		return host == baseDomain || strings.HasSuffix(host, "."+baseDomain)
	default:
		// Exact match
		return patternHost == host
	}
}

// matchPath checks if URL path matches pattern path
func matchPath(patternPath, urlPath string) bool {
	// Empty URL path should be treated as "/"
	if urlPath == "" {
		urlPath = "/"
	}

	// Convert pattern to regex
	// Escape special regex chars except *
	escaped := regexp.QuoteMeta(patternPath)
	// Replace escaped * with .*
	regexStr := strings.ReplaceAll(escaped, `\*`, ".*")
	// Anchor the pattern
	regexStr = "^" + regexStr + "$"

	regex, err := regexp.Compile(regexStr)
	if err != nil {
		return false
	}

	return regex.MatchString(urlPath)
}

// isValidScheme checks if URL has a supported scheme
func isValidScheme(urlStr string) bool {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	switch parsedURL.Scheme {
	case "http", "https", "ws", "wss", "ftp", "file", "data":
		return true
	}
	return false
}
