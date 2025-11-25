// Package validation provides manifest validation for WebExtensions.
// It validates both Manifest V2 and V3 formats and returns detailed errors.
package validation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ValidationError represents a manifest validation error.
type ValidationError struct {
	Field   string // JSON field path (e.g., "content_scripts[0].matches")
	Message string
	Severe  bool // If true, extension cannot load
}

func (e ValidationError) Error() string {
	severity := "warning"
	if e.Severe {
		severity = "error"
	}
	return fmt.Sprintf("[%s] %s: %s", severity, e.Field, e.Message)
}

// ValidationResult contains all validation errors and warnings.
type ValidationResult struct {
	Errors   []ValidationError
	Warnings []ValidationError
}

// HasErrors returns true if there are severe errors.
func (r *ValidationResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// Error returns a combined error message or nil if no errors.
func (r *ValidationResult) Error() error {
	if !r.HasErrors() {
		return nil
	}
	var msgs []string
	for _, e := range r.Errors {
		msgs = append(msgs, e.Error())
	}
	return fmt.Errorf("manifest validation failed:\n  %s", strings.Join(msgs, "\n  "))
}

// AllIssues returns all errors and warnings combined.
func (r *ValidationResult) AllIssues() []ValidationError {
	return append(r.Errors, r.Warnings...)
}

func (r *ValidationResult) addError(field, message string) {
	r.Errors = append(r.Errors, ValidationError{Field: field, Message: message, Severe: true})
}

func (r *ValidationResult) addWarning(field, message string) {
	r.Warnings = append(r.Warnings, ValidationError{Field: field, Message: message, Severe: false})
}

// RawManifest is used for initial JSON parsing before validation.
type RawManifest map[string]interface{}

// ValidateManifestFile reads and validates a manifest.json file.
func ValidateManifestFile(path string) (*ValidationResult, RawManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read manifest: %w", err)
	}

	var raw RawManifest
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, fmt.Errorf("parse manifest JSON: %w", err)
	}

	extDir := filepath.Dir(path)
	result := ValidateManifest(raw, extDir)
	return result, raw, nil
}

// ValidateManifest validates a parsed manifest.
func ValidateManifest(m RawManifest, extDir string) *ValidationResult {
	r := &ValidationResult{}

	// Required fields
	version := validateManifestVersion(r, m)
	validateName(r, m)
	validateVersion(r, m)

	// Version-specific validation
	if version == 2 {
		validateMV2(r, m, extDir)
	} else if version == 3 {
		validateMV3(r, m, extDir)
	}

	// Common validation
	validateBackground(r, m, extDir, version)
	validateContentScripts(r, m, extDir)
	validatePermissions(r, m, version)
	validateIcons(r, m, extDir)
	validateBrowserAction(r, m, version)
	validateOptionsUI(r, m, extDir)
	validateWebAccessibleResources(r, m, version, extDir)

	return r
}

func validateManifestVersion(r *ValidationResult, m RawManifest) int {
	v, ok := m["manifest_version"]
	if !ok {
		r.addError("manifest_version", "required field is missing")
		return 0
	}

	// JSON numbers are float64
	vf, ok := v.(float64)
	if !ok {
		r.addError("manifest_version", fmt.Sprintf("must be a number, got %T", v))
		return 0
	}

	vi := int(vf)
	if vi != 2 && vi != 3 {
		r.addError("manifest_version", fmt.Sprintf("must be 2 or 3, got %d", vi))
		return 0
	}

	return vi
}

func validateName(r *ValidationResult, m RawManifest) {
	name, ok := m["name"]
	if !ok {
		r.addError("name", "required field is missing")
		return
	}

	nameStr, ok := name.(string)
	if !ok {
		r.addError("name", fmt.Sprintf("must be a string, got %T", name))
		return
	}

	if nameStr == "" {
		r.addError("name", "cannot be empty")
		return
	}

	if len(nameStr) > 45 {
		r.addWarning("name", "exceeds recommended 45 character limit")
	}

	// Check for i18n message reference
	if strings.HasPrefix(nameStr, "__MSG_") && strings.HasSuffix(nameStr, "__") {
		// Valid i18n reference - will be resolved later
		return
	}
}

func validateVersion(r *ValidationResult, m RawManifest) {
	v, ok := m["version"]
	if !ok {
		r.addError("version", "required field is missing")
		return
	}

	vStr, ok := v.(string)
	if !ok {
		r.addError("version", fmt.Sprintf("must be a string, got %T", v))
		return
	}

	if vStr == "" {
		r.addError("version", "cannot be empty")
		return
	}

	// Validate version format (1-4 dot-separated integers)
	versionRe := regexp.MustCompile(`^\d+(\.\d+){0,3}$`)
	if !versionRe.MatchString(vStr) {
		r.addWarning("version", fmt.Sprintf("'%s' does not follow recommended format (e.g., '1.0.0')", vStr))
	}
}

func validateMV2(r *ValidationResult, m RawManifest, extDir string) {
	// MV2-specific validations
	if csp, ok := m["content_security_policy"]; ok {
		if _, ok := csp.(string); !ok {
			r.addError("content_security_policy", "must be a string in MV2")
		}
	}
}

func validateMV3(r *ValidationResult, m RawManifest, extDir string) {
	// MV3-specific validations
	if csp, ok := m["content_security_policy"]; ok {
		if _, ok := csp.(map[string]interface{}); !ok {
			r.addError("content_security_policy", "must be an object in MV3")
		}
	}

	// MV3 requires host_permissions instead of host patterns in permissions
	if perms, ok := m["permissions"].([]interface{}); ok {
		for i, p := range perms {
			if pStr, ok := p.(string); ok {
				if isHostPattern(pStr) {
					r.addWarning(fmt.Sprintf("permissions[%d]", i),
						fmt.Sprintf("host pattern '%s' should be in host_permissions for MV3", pStr))
				}
			}
		}
	}

	// MV3 background must use service_worker
	if bg, ok := m["background"].(map[string]interface{}); ok {
		if _, hasScripts := bg["scripts"]; hasScripts {
			r.addError("background.scripts", "MV3 does not support background.scripts, use service_worker instead")
		}
		if _, hasPage := bg["page"]; hasPage {
			r.addError("background.page", "MV3 does not support background.page, use service_worker instead")
		}
	}
}

func validateBackground(r *ValidationResult, m RawManifest, extDir string, version int) {
	bg, ok := m["background"]
	if !ok {
		return // Background is optional
	}

	bgMap, ok := bg.(map[string]interface{})
	if !ok {
		r.addError("background", fmt.Sprintf("must be an object, got %T", bg))
		return
	}

	// Check for scripts
	if scripts, ok := bgMap["scripts"].([]interface{}); ok {
		if len(scripts) == 0 {
			r.addWarning("background.scripts", "empty scripts array")
		}
		for i, s := range scripts {
			sStr, ok := s.(string)
			if !ok {
				r.addError(fmt.Sprintf("background.scripts[%d]", i), "must be a string")
				continue
			}
			if !fileExists(extDir, sStr) {
				r.addError(fmt.Sprintf("background.scripts[%d]", i),
					fmt.Sprintf("file not found: %s", sStr))
			}
		}
	}

	// Check for page
	if page, ok := bgMap["page"].(string); ok {
		if !fileExists(extDir, page) {
			r.addError("background.page", fmt.Sprintf("file not found: %s", page))
		}
	}

	// Check for service_worker (MV3)
	if sw, ok := bgMap["service_worker"].(string); ok {
		if version == 2 {
			r.addWarning("background.service_worker", "service_worker is MV3 only, will be ignored in MV2")
		}
		if !fileExists(extDir, sw) {
			r.addError("background.service_worker", fmt.Sprintf("file not found: %s", sw))
		}
	}

	// Mutual exclusivity
	hasScripts := bgMap["scripts"] != nil
	hasPage := bgMap["page"] != nil
	hasSW := bgMap["service_worker"] != nil

	if hasScripts && hasPage {
		r.addError("background", "cannot have both scripts and page")
	}
	if (hasScripts || hasPage) && hasSW {
		r.addError("background", "cannot mix scripts/page with service_worker")
	}
}

func validateContentScripts(r *ValidationResult, m RawManifest, extDir string) {
	cs, ok := m["content_scripts"]
	if !ok {
		return
	}

	csList, ok := cs.([]interface{})
	if !ok {
		r.addError("content_scripts", fmt.Sprintf("must be an array, got %T", cs))
		return
	}

	for i, entry := range csList {
		prefix := fmt.Sprintf("content_scripts[%d]", i)
		entryMap, ok := entry.(map[string]interface{})
		if !ok {
			r.addError(prefix, "must be an object")
			continue
		}

		// matches is required
		matches, ok := entryMap["matches"]
		if !ok {
			r.addError(prefix+".matches", "required field is missing")
		} else {
			matchesList, ok := matches.([]interface{})
			if !ok {
				r.addError(prefix+".matches", "must be an array")
			} else if len(matchesList) == 0 {
				r.addError(prefix+".matches", "cannot be empty")
			} else {
				for j, match := range matchesList {
					if mStr, ok := match.(string); ok {
						if err := validateMatchPattern(mStr); err != nil {
							r.addWarning(fmt.Sprintf("%s.matches[%d]", prefix, j), err.Error())
						}
					}
				}
			}
		}

		// Validate js files
		if js, ok := entryMap["js"].([]interface{}); ok {
			for j, f := range js {
				if fStr, ok := f.(string); ok {
					if !fileExists(extDir, fStr) {
						r.addError(fmt.Sprintf("%s.js[%d]", prefix, j),
							fmt.Sprintf("file not found: %s", fStr))
					}
				}
			}
		}

		// Validate css files
		if css, ok := entryMap["css"].([]interface{}); ok {
			for j, f := range css {
				if fStr, ok := f.(string); ok {
					if !fileExists(extDir, fStr) {
						r.addError(fmt.Sprintf("%s.css[%d]", prefix, j),
							fmt.Sprintf("file not found: %s", fStr))
					}
				}
			}
		}

		// run_at validation
		if runAt, ok := entryMap["run_at"].(string); ok {
			validRunAt := map[string]bool{
				"document_start": true,
				"document_end":   true,
				"document_idle":  true,
			}
			if !validRunAt[runAt] {
				r.addError(prefix+".run_at",
					fmt.Sprintf("invalid value '%s', must be document_start, document_end, or document_idle", runAt))
			}
		}
	}
}

func validatePermissions(r *ValidationResult, m RawManifest, version int) {
	validatePermissionArray(r, m, "permissions", version)
	validatePermissionArray(r, m, "optional_permissions", version)

	if version == 3 {
		validatePermissionArray(r, m, "host_permissions", version)
	}
}

func validatePermissionArray(r *ValidationResult, m RawManifest, field string, version int) {
	perms, ok := m[field]
	if !ok {
		return
	}

	permsList, ok := perms.([]interface{})
	if !ok {
		r.addError(field, fmt.Sprintf("must be an array, got %T", perms))
		return
	}

	knownAPIs := map[string]bool{
		"activeTab": true, "alarms": true, "bookmarks": true, "browserSettings": true,
		"browsingData": true, "clipboardRead": true, "clipboardWrite": true,
		"contextMenus": true, "contextualIdentities": true, "cookies": true,
		"devtools": true, "dns": true, "downloads": true, "find": true,
		"geolocation": true, "history": true, "identity": true, "idle": true,
		"management": true, "menus": true, "nativeMessaging": true, "notifications": true,
		"pageCapture": true, "privacy": true, "proxy": true, "scripting": true,
		"search": true, "sessions": true, "storage": true, "tabHide": true,
		"tabs": true, "theme": true, "topSites": true, "unlimitedStorage": true,
		"webNavigation": true, "webRequest": true, "webRequestBlocking": true,
		"webRequestFilterResponse": true,
	}

	for i, p := range permsList {
		pStr, ok := p.(string)
		if !ok {
			r.addError(fmt.Sprintf("%s[%d]", field, i), "must be a string")
			continue
		}

		// Check if it's a host pattern
		if isHostPattern(pStr) {
			if field == "permissions" && version == 3 {
				r.addWarning(fmt.Sprintf("%s[%d]", field, i),
					"host patterns should be in host_permissions for MV3")
			}
			continue
		}

		// Check if it's a known API permission
		if !knownAPIs[pStr] && !strings.HasPrefix(pStr, "unlimitedStorage") {
			r.addWarning(fmt.Sprintf("%s[%d]", field, i),
				fmt.Sprintf("unknown permission '%s'", pStr))
		}
	}
}

func validateIcons(r *ValidationResult, m RawManifest, extDir string) {
	icons, ok := m["icons"]
	if !ok {
		return
	}

	iconsMap, ok := icons.(map[string]interface{})
	if !ok {
		r.addError("icons", fmt.Sprintf("must be an object, got %T", icons))
		return
	}

	for size, path := range iconsMap {
		pathStr, ok := path.(string)
		if !ok {
			r.addError(fmt.Sprintf("icons.%s", size), "must be a string path")
			continue
		}

		if !fileExists(extDir, pathStr) {
			r.addWarning(fmt.Sprintf("icons.%s", size),
				fmt.Sprintf("file not found: %s", pathStr))
		}
	}
}

func validateBrowserAction(r *ValidationResult, m RawManifest, version int) {
	// MV2 uses browser_action, MV3 uses action
	field := "browser_action"
	if version == 3 {
		field = "action"
	}

	// Check for wrong field usage
	if version == 3 {
		if _, ok := m["browser_action"]; ok {
			r.addWarning("browser_action", "MV3 uses 'action' instead of 'browser_action'")
		}
	}

	action, ok := m[field]
	if !ok {
		return
	}

	_, ok = action.(map[string]interface{})
	if !ok {
		r.addError(field, fmt.Sprintf("must be an object, got %T", action))
	}
}

func validateOptionsUI(r *ValidationResult, m RawManifest, extDir string) {
	opts, ok := m["options_ui"]
	if !ok {
		return
	}

	optsMap, ok := opts.(map[string]interface{})
	if !ok {
		r.addError("options_ui", fmt.Sprintf("must be an object, got %T", opts))
		return
	}

	page, ok := optsMap["page"].(string)
	if !ok {
		r.addError("options_ui.page", "required field is missing or not a string")
		return
	}

	if !fileExists(extDir, page) {
		r.addError("options_ui.page", fmt.Sprintf("file not found: %s", page))
	}
}

func validateWebAccessibleResources(r *ValidationResult, m RawManifest, version int, extDir string) {
	war, ok := m["web_accessible_resources"]
	if !ok {
		return
	}

	warList, ok := war.([]interface{})
	if !ok {
		r.addError("web_accessible_resources", fmt.Sprintf("must be an array, got %T", war))
		return
	}

	if version == 2 {
		// MV2: array of strings (glob patterns)
		for i, item := range warList {
			if _, ok := item.(string); !ok {
				r.addError(fmt.Sprintf("web_accessible_resources[%d]", i),
					"must be a string in MV2")
			}
		}
	} else {
		// MV3: array of objects with resources and matches
		for i, item := range warList {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				r.addError(fmt.Sprintf("web_accessible_resources[%d]", i),
					"must be an object in MV3")
				continue
			}

			if _, ok := itemMap["resources"]; !ok {
				r.addError(fmt.Sprintf("web_accessible_resources[%d].resources", i),
					"required field is missing")
			}

			// Must have matches or extension_ids
			hasMatches := itemMap["matches"] != nil
			hasExtIDs := itemMap["extension_ids"] != nil
			if !hasMatches && !hasExtIDs {
				r.addError(fmt.Sprintf("web_accessible_resources[%d]", i),
					"must have either 'matches' or 'extension_ids'")
			}
		}
	}
}

// Helper functions

func isHostPattern(s string) bool {
	// Host patterns start with scheme or *
	if strings.HasPrefix(s, "*://") ||
		strings.HasPrefix(s, "http://") ||
		strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "file://") ||
		s == "<all_urls>" {
		return true
	}
	return false
}

func validateMatchPattern(pattern string) error {
	if pattern == "<all_urls>" {
		return nil
	}

	// Basic match pattern validation
	// Format: <scheme>://<host>/<path>
	parts := strings.SplitN(pattern, "://", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid match pattern: missing ://")
	}

	scheme := parts[0]
	validSchemes := map[string]bool{
		"*": true, "http": true, "https": true, "file": true,
		"ftp": true, "ws": true, "wss": true, "data": true,
	}
	if !validSchemes[scheme] {
		return fmt.Errorf("invalid scheme '%s'", scheme)
	}

	return nil
}

func fileExists(extDir, path string) bool {
	fullPath := filepath.Join(extDir, filepath.Clean(path))
	_, err := os.Stat(fullPath)
	return err == nil
}
