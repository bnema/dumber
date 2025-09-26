package browser

import (
	"fmt"
	"sync"
	"time"
)

// PaneType defines the different types of panes in the workspace
type PaneType int

const (
	PaneTypeRegular PaneType = iota
	PaneTypeStacked
	PaneTypePopup
	PaneTypeOAuthPopup
)

func (pt PaneType) String() string {
	switch pt {
	case PaneTypeRegular:
		return "regular"
	case PaneTypeStacked:
		return "stacked"
	case PaneTypePopup:
		return "popup"
	case PaneTypeOAuthPopup:
		return "oauth-popup"
	default:
		return "unknown"
	}
}

// PaneState tracks the lifecycle state of a pane
type PaneState int

const (
	PaneStateInitializing PaneState = iota
	PaneStateActive
	PaneStateInactive
	PaneStateClosing
	PaneStateClosed
)

func (ps PaneState) String() string {
	switch ps {
	case PaneStateInitializing:
		return "initializing"
	case PaneStateActive:
		return "active"
	case PaneStateInactive:
		return "inactive"
	case PaneStateClosing:
		return "closing"
	case PaneStateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// PaneMetadata contains unified metadata for all pane types
type PaneMetadata struct {
	ID           string        `json:"id"`
	Type         PaneType      `json:"type"`
	State        PaneState     `json:"state"`
	CreatedAt    time.Time     `json:"created_at"`
	LastActiveAt time.Time     `json:"last_active_at"`
	CloseReason  string        `json:"close_reason,omitempty"`

	// Navigation metadata
	URL          string        `json:"url,omitempty"`
	Title        string        `json:"title,omitempty"`

	// Stack-specific metadata
	StackIndex   int           `json:"stack_index,omitempty"`
	StackSize    int           `json:"stack_size,omitempty"`

	// Parent-child relationships
	ParentID     string        `json:"parent_id,omitempty"`
	ChildIDs     []string      `json:"child_ids,omitempty"`

	// Thread safety
	mu           sync.RWMutex
}

// NewPaneMetadata creates new metadata for a pane
func NewPaneMetadata(id string, paneType PaneType) *PaneMetadata {
	return &PaneMetadata{
		ID:           id,
		Type:         paneType,
		State:        PaneStateInitializing,
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
		ChildIDs:     make([]string, 0),
	}
}

// GetState returns the current state thread-safely
func (pm *PaneMetadata) GetState() PaneState {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.State
}

// SetState updates the state thread-safely
func (pm *PaneMetadata) SetState(state PaneState) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.State = state
	if state == PaneStateActive {
		pm.LastActiveAt = time.Now()
	}
}

// GetURL returns the current URL thread-safely
func (pm *PaneMetadata) GetURL() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.URL
}

// SetURL updates the URL thread-safely
func (pm *PaneMetadata) SetURL(url string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.URL = url
}

// GetTitle returns the current title thread-safely
func (pm *PaneMetadata) GetTitle() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.Title
}

// SetTitle updates the title thread-safely
func (pm *PaneMetadata) SetTitle(title string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.Title = title
}

// AddChild adds a child pane ID
func (pm *PaneMetadata) AddChild(childID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.ChildIDs = append(pm.ChildIDs, childID)
}

// RemoveChild removes a child pane ID
func (pm *PaneMetadata) RemoveChild(childID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for i, id := range pm.ChildIDs {
		if id == childID {
			pm.ChildIDs = append(pm.ChildIDs[:i], pm.ChildIDs[i+1:]...)
			break
		}
	}
}

// GetChildIDs returns a copy of child IDs
func (pm *PaneMetadata) GetChildIDs() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	result := make([]string, len(pm.ChildIDs))
	copy(result, pm.ChildIDs)
	return result
}

// IsValidForOperation checks if the pane is in a valid state for operations
func (pm *PaneMetadata) IsValidForOperation() bool {
	state := pm.GetState()
	return state == PaneStateActive || state == PaneStateInactive
}

// CanClose checks if the pane can be closed
func (pm *PaneMetadata) CanClose() bool {
	state := pm.GetState()
	return state != PaneStateClosing && state != PaneStateClosed
}

// SetCloseReason sets the reason for closing
func (pm *PaneMetadata) SetCloseReason(reason string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.CloseReason = reason
}

// String returns a string representation of the metadata
func (pm *PaneMetadata) String() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return fmt.Sprintf("PaneMetadata{ID:%s, Type:%s, State:%s, URL:%s}",
		pm.ID, pm.Type, pm.State, pm.URL)
}

// PaneCloseRequest represents a request to close a pane
type PaneCloseRequest struct {
	PaneID      string
	Reason      string
	Force       bool
	Callback    func(error)
	RequestedAt time.Time
}

// NewPaneCloseRequest creates a new close request
func NewPaneCloseRequest(paneID, reason string, force bool) *PaneCloseRequest {
	return &PaneCloseRequest{
		PaneID:      paneID,
		Reason:      reason,
		Force:       force,
		RequestedAt: time.Now(),
	}
}

// TypedPaneNode extends paneNode with type information and metadata
type TypedPaneNode struct {
	*paneNode // Embed existing paneNode

	Metadata    *PaneMetadata
	CloseHandler func(*TypedPaneNode) error
}

// NewTypedPaneNode creates a new typed pane node
func NewTypedPaneNode(node *paneNode, paneType PaneType) *TypedPaneNode {
	metadata := NewPaneMetadata(generatePaneID(node), paneType)

	return &TypedPaneNode{
		paneNode: node,
		Metadata: metadata,
	}
}

// generatePaneID generates a unique ID for a pane based on its properties
func generatePaneID(node *paneNode) string {
	if node == nil {
		return fmt.Sprintf("pane_%d", time.Now().UnixNano())
	}

	if node.pane != nil && node.pane.webView != nil {
		return node.pane.webView.ID()
	}

	return fmt.Sprintf("pane_%p_%d", node, time.Now().UnixNano())
}

// GetPaneType returns the type of this pane
func (tpn *TypedPaneNode) GetPaneType() PaneType {
	if tpn.Metadata == nil {
		return PaneTypeRegular
	}
	return tpn.Metadata.Type
}

// IsStacked returns true if this is a stacked pane
func (tpn *TypedPaneNode) IsStacked() bool {
	return tpn.GetPaneType() == PaneTypeStacked
}

// IsPopup returns true if this is a popup pane
func (tpn *TypedPaneNode) IsPopup() bool {
	paneType := tpn.GetPaneType()
	return paneType == PaneTypePopup || paneType == PaneTypeOAuthPopup
}

// CanFocus returns true if this pane can receive focus
func (tpn *TypedPaneNode) CanFocus() bool {
	if tpn.Metadata == nil {
		return false
	}
	return tpn.Metadata.IsValidForOperation()
}

// Close closes the pane using its type-specific close handler
func (tpn *TypedPaneNode) Close(reason string) error {
	if tpn.Metadata == nil {
		return fmt.Errorf("cannot close pane: no metadata")
	}

	if !tpn.Metadata.CanClose() {
		return fmt.Errorf("pane %s cannot be closed in state %s",
			tpn.Metadata.ID, tpn.Metadata.State)
	}

	tpn.Metadata.SetState(PaneStateClosing)
	tpn.Metadata.SetCloseReason(reason)

	if tpn.CloseHandler != nil {
		return tpn.CloseHandler(tpn)
	}

	// Fallback to default close behavior
	return tpn.defaultClose()
}

// defaultClose provides default close behavior for panes without custom handlers
func (tpn *TypedPaneNode) defaultClose() error {
	if tpn.paneNode == nil {
		return fmt.Errorf("cannot close: nil paneNode")
	}

	// Mark as closed
	if tpn.Metadata != nil {
		tpn.Metadata.SetState(PaneStateClosed)
	}

	return nil
}