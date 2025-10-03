// workspace_tree_validator.go - Simplified validator stub to keep API surface.
package browser

import "fmt"

// TreeValidator previously performed extensive tree checks. The SafeWidget
// removal rendered the implementation obsolete, so we keep a lightweight shim
// until a new validator is written.
type TreeValidator struct {
	enabled bool
}

// NewTreeValidator returns a stub validator instance.
func NewTreeValidator(enabled bool, _ bool) *TreeValidator {
	return &TreeValidator{enabled: enabled}
}

// ValidateTree is a no-op that reports success unless validation is explicitly enabled.
func (tv *TreeValidator) ValidateTree(_ *paneNode, _ string) error {
	if tv == nil || !tv.enabled {
		return nil
	}
	return fmt.Errorf("tree validation not implemented")
}

// SetDebugMode preserved for compatibility.
func (tv *TreeValidator) SetDebugMode(_ bool) {}

// Enable turns validation on (no functional impact in the stub).
func (tv *TreeValidator) Enable() {
	if tv != nil {
		tv.enabled = true
	}
}

// Disable turns validation off.
func (tv *TreeValidator) Disable() {
	if tv != nil {
		tv.enabled = false
	}
}

// GetValidationStats mimics the old diagnostic output.
func (tv *TreeValidator) GetValidationStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled": tv != nil && tv.enabled,
	}
}

// Enabled reports current state (used by tests/instrumentation).
func (tv *TreeValidator) Enabled() bool {
	return tv != nil && tv.enabled
}
