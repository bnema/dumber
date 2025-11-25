package api

import (
	"context"
	"fmt"
)

// Permissions represents an extension's permissions
type Permissions struct {
	Permissions []string `json:"permissions,omitempty"`
	Origins     []string `json:"origins,omitempty"`
}

// PermissionsAPIDispatcher handles permissions.* API calls
type PermissionsAPIDispatcher struct {
	// Extension's manifest permissions (static, granted at install time)
	manifestPermissions map[string]bool
	manifestOrigins     []string

	// Optional permissions (can be requested/removed at runtime)
	// Currently not implemented - all permissions are manifest-based
	optionalPermissions map[string]bool
	optionalOrigins     []string
}

// NewPermissionsAPIDispatcher creates a new permissions API dispatcher
func NewPermissionsAPIDispatcher(manifestPermissions []string, manifestOrigins []string) *PermissionsAPIDispatcher {
	// Convert permissions array to map for fast lookup
	permMap := make(map[string]bool, len(manifestPermissions))
	for _, perm := range manifestPermissions {
		permMap[perm] = true
	}

	return &PermissionsAPIDispatcher{
		manifestPermissions: permMap,
		manifestOrigins:     manifestOrigins,
		optionalPermissions: make(map[string]bool),
		optionalOrigins:     make([]string, 0),
	}
}

// Contains checks if the extension has the specified permissions
func (d *PermissionsAPIDispatcher) Contains(ctx context.Context, details map[string]interface{}) (*PermissionsResult, error) {
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
		if !d.hasPermission(permStr) {
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
			if !d.hasOrigin(originStr) {
				result.HasPermissions = false
				break
			}
		}
	}

	return result, nil
}

// GetAll returns all current permissions
func (d *PermissionsAPIDispatcher) GetAll(ctx context.Context) (*Permissions, error) {
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

// Request adds new permissions (stub - not implemented yet)
// In a full implementation, this would show a user prompt
func (d *PermissionsAPIDispatcher) Request(ctx context.Context, details map[string]interface{}) (*PermissionsResult, error) {
	// Extract requested permissions
	requestedPerms, _ := details["permissions"].([]interface{})
	requestedOrigins, _ := details["origins"].([]interface{})

	// For now, reject all permission requests
	// A full implementation would:
	// 1. Show a user prompt dialog
	// 2. If approved, add to optionalPermissions/optionalOrigins
	// 3. Fire permissions.onAdded event
	// 4. Return true

	// Check if already granted
	alreadyGranted := true
	for _, perm := range requestedPerms {
		permStr, ok := perm.(string)
		if !ok {
			continue
		}
		if !d.hasPermission(permStr) {
			alreadyGranted = false
			break
		}
	}

	if alreadyGranted {
		for _, origin := range requestedOrigins {
			originStr, ok := origin.(string)
			if !ok {
				continue
			}
			if !d.hasOrigin(originStr) {
				alreadyGranted = false
				break
			}
		}
	}

	if alreadyGranted {
		return &PermissionsResult{HasPermissions: true}, nil
	}

	// TODO: Show user prompt and handle approval
	// For now, deny all new permission requests
	return &PermissionsResult{HasPermissions: false}, fmt.Errorf("permissions.request(): user prompt not implemented")
}

// Remove removes permissions (stub - only works for optional permissions)
func (d *PermissionsAPIDispatcher) Remove(ctx context.Context, details map[string]interface{}) (*PermissionsResult, error) {
	// Extract permissions to remove
	permsToRemove, _ := details["permissions"].([]interface{})
	originsToRemove, _ := details["origins"].([]interface{})

	removed := false

	// Can only remove optional permissions, not manifest permissions
	for _, perm := range permsToRemove {
		permStr, ok := perm.(string)
		if !ok {
			continue
		}
		if d.optionalPermissions[permStr] {
			delete(d.optionalPermissions, permStr)
			removed = true
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
				removed = true
				break
			}
		}
	}

	// TODO: If removed, fire permissions.onRemoved event

	return &PermissionsResult{HasPermissions: removed}, nil
}

// Helper: Check if permission is granted (manifest or optional)
func (d *PermissionsAPIDispatcher) hasPermission(permission string) bool {
	return d.manifestPermissions[permission] || d.optionalPermissions[permission]
}

// Helper: Check if origin is granted (manifest or optional)
func (d *PermissionsAPIDispatcher) hasOrigin(origin string) bool {
	// Check manifest origins
	for _, o := range d.manifestOrigins {
		if matchesOriginPattern(origin, o) {
			return true
		}
	}
	// Check optional origins
	for _, o := range d.optionalOrigins {
		if matchesOriginPattern(origin, o) {
			return true
		}
	}
	return false
}

// Helper: Match origin against pattern (supports wildcards)
func matchesOriginPattern(origin, pattern string) bool {
	// Exact match
	if origin == pattern {
		return true
	}

	// Simple wildcard support (full implementation would handle *.example.com, etc.)
	if pattern == "<all_urls>" {
		return true
	}

	// TODO: Implement full WebExtension URL pattern matching
	return false
}

// PermissionsResult represents the result of contains/request/remove operations
type PermissionsResult struct {
	HasPermissions bool `json:"result"`
}
