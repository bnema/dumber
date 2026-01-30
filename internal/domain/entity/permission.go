package entity

// DomainTimestamp represents a Unix timestamp in seconds.
// This is a domain-owned type to avoid importing time in the domain layer.

// PermissionType represents the type of permission being requested.
type PermissionType string

const (
	// PermissionTypeMicrophone represents microphone access permission.
	PermissionTypeMicrophone PermissionType = "microphone"

	// PermissionTypeCamera represents camera access permission.
	PermissionTypeCamera PermissionType = "camera"

	// PermissionTypeDisplay represents screen sharing/display capture permission.
	PermissionTypeDisplay PermissionType = "display"

	// PermissionTypeDeviceInfo represents device enumeration permission.
	PermissionTypeDeviceInfo PermissionType = "device_info"

	// PermissionTypeClipboard represents clipboard access permission.
	PermissionTypeClipboard PermissionType = "clipboard"

	// PermissionTypeNotification represents notification permission.
	PermissionTypeNotification PermissionType = "notification"

	// PermissionTypeGeolocation represents geolocation permission.
	PermissionTypeGeolocation PermissionType = "geolocation"

	// PermissionTypePointerLock represents pointer lock permission.
	PermissionTypePointerLock PermissionType = "pointer_lock"

	// PermissionTypeMediaKeySystem represents DRM/media key system permission.
	PermissionTypeMediaKeySystem PermissionType = "media_key_system"

	// PermissionTypeWebsiteDataAccess represents 3rd party cookie/data access permission.
	PermissionTypeWebsiteDataAccess PermissionType = "website_data_access"
)

// PermissionDecision represents the user's decision for a permission.
type PermissionDecision string

const (
	// PermissionGranted means the permission was allowed.
	PermissionGranted PermissionDecision = "granted"

	// PermissionDenied means the permission was denied.
	PermissionDenied PermissionDecision = "denied"

	// PermissionPrompt means no decision has been made yet (default state).
	PermissionPrompt PermissionDecision = "prompt"
)

// PermissionRecord stores a permission decision for a specific origin and type.
type PermissionRecord struct {
	Origin    string             // The origin (domain) this permission applies to
	Type      PermissionType     // The type of permission
	Decision  PermissionDecision // The decision: granted, denied, or prompt
	UpdatedAt int64              // Unix timestamp in seconds when this record was last updated
}

// IsGranted returns true if the permission is granted.
func (p *PermissionRecord) IsGranted() bool {
	return p.Decision == PermissionGranted
}

// IsDenied returns true if the permission is denied.
func (p *PermissionRecord) IsDenied() bool {
	return p.Decision == PermissionDenied
}

// PermissionSet represents multiple permission decisions for a single request.
// This is used when handling combined permission requests (e.g., mic + camera).
type PermissionSet struct {
	Types    []PermissionType
	Decision PermissionDecision
}

// CanPersist returns true if this permission type can be persisted.
// Per W3C spec, display capture permissions cannot be persisted.
func CanPersist(permType PermissionType) bool {
	switch permType {
	case PermissionTypeDisplay:
		return false // W3C forbids persisting display capture permissions
	case PermissionTypeDeviceInfo, PermissionTypePointerLock:
		return false // Auto-allowed, no need to persist
	default:
		return true
	}
}

// IsAutoAllow returns true if this permission type should be auto-allowed.
func IsAutoAllow(permType PermissionType) bool {
	switch permType {
	case PermissionTypeDisplay:
		return true // Portal handles the UI
	case PermissionTypeDeviceInfo:
		return true // Low risk, just lists devices
	case PermissionTypePointerLock:
		return true // Low risk, default WebKit behavior
	default:
		return false
	}
}

// PermissionTypesToStrings converts permission types to strings for logging.
func PermissionTypesToStrings(types []PermissionType) []string {
	result := make([]string, len(types))
	for i, t := range types {
		result[i] = string(t)
	}
	return result
}
