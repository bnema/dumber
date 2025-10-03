// workspace_tree_rebalancer.go - Simplified no-op tree rebalancer placeholder.
//
// The original TreeRebalancer relied on SafeWidget abstractions. Until a
// pointer-based implementation is restored, we keep a lightweight stub so the
// rest of the workspace code can continue to depend on the API without
// carrying dead logic.
package browser

// TreeRebalancer is currently a no-op shim kept for API compatibility.
type TreeRebalancer struct {
	enabled bool
}

// RebalanceOperation describes an attempted rebalance step (unused in shim).
type RebalanceOperation struct {
	Type        RebalanceType
	Node        *paneNode
	Description string
}

// RebalanceType enumerates rebalancer operations (kept for completeness).
type RebalanceType int

const (
	RebalanceRotateLeft RebalanceType = iota
	RebalanceRotateRight
	RebalancePromote
	RebalanceRestructure
)

// TreeMetrics represents the structural metrics of the workspace tree.
type TreeMetrics struct{}

// NewTreeRebalancer returns a disabled stub implementation.
func NewTreeRebalancer(_ *WorkspaceManager, _ *TreeValidator) *TreeRebalancer {
	return &TreeRebalancer{}
}

// SetMaxImbalance is preserved for API compatibility.
func (tr *TreeRebalancer) SetMaxImbalance(_ int) {}

// Enable toggles the stub on (no functional impact).
func (tr *TreeRebalancer) Enable() { tr.enabled = true }

// Disable toggles the stub off.
func (tr *TreeRebalancer) Disable() { tr.enabled = false }

// RebalanceAfterClose is a no-op in the stub implementation.
func (tr *TreeRebalancer) RebalanceAfterClose(_ *paneNode, _ *paneNode) error {
	return nil
}

// CalculateTreeMetrics returns empty metrics in the stub.
func (tr *TreeRebalancer) CalculateTreeMetrics(_ *paneNode) TreeMetrics {
	return TreeMetrics{}
}

// GetRebalancingStats exposes minimal instrumentation for diagnostics.
func (tr *TreeRebalancer) GetRebalancingStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled": tr != nil && tr.enabled,
	}
}
