package browser

import (
	"fmt"
	"strings"
	"sync"

	"github.com/bnema/dumber/internal/logging"
)

type TreeSnapshot struct {
	Label string
	Lines []string
}

type WorkspaceDiagnostics struct {
	mu        sync.Mutex
	enabled   bool
	snapshots []TreeSnapshot
}

func NewWorkspaceDiagnostics(enabled bool) *WorkspaceDiagnostics {
	return &WorkspaceDiagnostics{enabled: enabled}
}

func (d *WorkspaceDiagnostics) Enabled() bool {
	if d == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.enabled
}

func (d *WorkspaceDiagnostics) SetEnabled(enabled bool) {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.enabled = enabled
	d.mu.Unlock()
}

func (d *WorkspaceDiagnostics) Capture(label string, root *paneNode) TreeSnapshot {
	if d == nil {
		return TreeSnapshot{}
	}

	d.mu.Lock()
	enabled := d.enabled
	d.mu.Unlock()
	if !enabled {
		return TreeSnapshot{}
	}

	snapshot := TreeSnapshot{Label: label}
	visited := make(map[*paneNode]bool)
	buildTreeSnapshot(root, 0, "", visited, &snapshot.Lines)
	if len(snapshot.Lines) == 0 {
		snapshot.Lines = append(snapshot.Lines, "<empty workspace>")
	}

	d.mu.Lock()
	d.snapshots = append(d.snapshots, snapshot)
	d.mu.Unlock()

	logging.Info(fmt.Sprintf("[pane-close] snapshot %s\n%s", snapshot.Label, strings.Join(snapshot.Lines, "\n")))
	return snapshot
}

func (d *WorkspaceDiagnostics) Snapshots() []TreeSnapshot {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	out := make([]TreeSnapshot, len(d.snapshots))
	copy(out, d.snapshots)
	return out
}

func buildTreeSnapshot(node *paneNode, depth int, prefix string, visited map[*paneNode]bool, lines *[]string) {
	indent := strings.Repeat("  ", depth)
	if node == nil {
		*lines = append(*lines, fmt.Sprintf("%s%s<nil>", indent, prefix))
		return
	}

	if depth > 64 {
		*lines = append(*lines, fmt.Sprintf("%s%s<max depth reached>", indent, prefix))
		return
	}

	if visited[node] {
		*lines = append(*lines, fmt.Sprintf("%s%s<cycle node=%p>", indent, prefix, node))
		return
	}

	visited[node] = true
	*lines = append(*lines, fmt.Sprintf("%s%s%s", indent, prefix, describePaneNode(node)))

	if node.isStacked {
		for idx, child := range node.stackedPanes {
			childPrefix := fmt.Sprintf("[%d] ", idx)
			buildTreeSnapshot(child, depth+1, childPrefix, visited, lines)
		}
		return
	}

	if node.left != nil || node.right != nil {
		buildTreeSnapshot(node.left, depth+1, "L: ", visited, lines)
		buildTreeSnapshot(node.right, depth+1, "R: ", visited, lines)
	}
}

func describePaneNode(node *paneNode) string {
	if node == nil {
		return "<nil>"
	}

	parts := []string{fmt.Sprintf("node=%p", node)}

	if node.isLeaf {
		parts = append(parts, "leaf")
		if node.pane != nil && node.pane.ID() != "" {
			parts = append(parts, fmt.Sprintf("pane=%s", node.pane.ID()))
		}
		if node.isPopup {
			parts = append(parts, "popup")
		}
	} else if node.isStacked {
		parts = append(parts, fmt.Sprintf("stack size=%d active=%d", len(node.stackedPanes), node.activeStackIndex))
	} else {
		parts = append(parts, fmt.Sprintf("split orientation=%d", node.orientation))
	}

	if node.container != nil {
		parts = append(parts, fmt.Sprintf("widget=%p", node.container))
	}

	if node.parent != nil {
		parts = append(parts, fmt.Sprintf("parent=%p", node.parent))
	}

	if !node.isLeaf && !node.isStacked {
		if node.left != nil {
			parts = append(parts, fmt.Sprintf("left=%p", node.left))
		}
		if node.right != nil {
			parts = append(parts, fmt.Sprintf("right=%p", node.right))
		}
	}

	return strings.Join(parts, " ")
}
