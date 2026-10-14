package ui

import (
	"slices"
	"strings"
	"unicode"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/input"
)

// legendGroupOrder is the fixed display order of legend groups.
var legendGroupOrder = []string{"SPLIT", "FOCUS", "SCROLL", "RESIZE", "SWITCH", "JUMP", "SELECT", "MANAGE", "MODES", "EXIT"}

// legendRow is one action of a legend group.
type legendRow struct {
	Action string
	Keys   []string // as configured; the view formats them for display
	Desc   string
}

// legendGroup is a titled run of related actions. A group is never split
// across columns.
type legendGroup struct {
	Name string
	Rows []legendRow
}

func legendActions(mode input.Mode, workspace entity.WorkspaceConfig, session entity.SessionConfig) map[string]entity.ActionBinding {
	switch mode {
	case input.ModePane:
		return workspace.PaneMode.Actions
	case input.ModeResize:
		return workspace.ResizeMode.Actions
	case input.ModeVim:
		return workspace.VimMode.Actions
	case input.ModeTab:
		return workspace.TabMode.Actions
	case input.ModeSession:
		return session.SessionMode.Actions
	case input.ModePrefix:
		return workspace.PrefixMode.Actions
	default:
		return nil
	}
}

// buildLegendGroups arranges the bound actions of mode in legendGroupOrder,
// each group sorted by action name. Actions without keys are left out.
func buildLegendGroups(mode input.Mode, actions map[string]entity.ActionBinding) []legendGroup {
	names := make(map[string][]string)
	for name, binding := range actions {
		if len(binding.Keys) == 0 {
			continue
		}
		group := modeLegendGroup(mode, name)
		names[group] = append(names[group], name)
	}
	var groups []legendGroup
	for _, groupName := range legendGroupOrder {
		members := names[groupName]
		if len(members) == 0 {
			continue
		}
		slices.Sort(members)
		group := legendGroup{Name: groupName}
		for _, name := range members {
			binding := actions[name]
			desc := binding.Desc
			if desc == "" {
				desc = strings.ReplaceAll(name, "-", " ")
			}
			group.Rows = append(group.Rows, legendRow{Action: name, Keys: binding.Keys, Desc: desc})
		}
		groups = append(groups, group)
	}
	return groups
}

// legendGroupHeights measures groups in rows: the title counts as one.
func legendGroupHeights(groups []legendGroup) []int {
	heights := make([]int, len(groups))
	for i, group := range groups {
		heights[i] = 1 + len(group.Rows)
	}
	return heights
}

func displayLegendKey(key string) string {
	switch strings.ToLower(key) {
	case "arrowright":
		return "→"
	case "arrowleft":
		return "←"
	case "arrowup":
		return "↑"
	case "arrowdown":
		return "↓"
	case "escape":
		return "esc"
	case "enter":
		return "↵"
	default:
		return key
	}
}

func legendSequenceMatches(pending string, keys []string) bool {
	sequence := strings.TrimLeftFunc(pending, unicode.IsDigit)
	if sequence == "" {
		return true
	}
	for _, key := range keys {
		if strings.HasPrefix(key, sequence) {
			return true
		}
	}
	return false
}

// vimLegendGroup returns the legend group of a Vim Mode action, or "" when the
// action belongs to a generic group.
func vimLegendGroup(name string) string {
	switch {
	case strings.HasPrefix(name, "hint-") || strings.HasPrefix(name, "yank-") || name == "visual":
		return "SELECT"
	case strings.Contains(name, "next") || strings.Contains(name, "prev") || name == "outline":
		return "JUMP"
	default:
		return ""
	}
}

func modeLegendGroup(mode input.Mode, name string) string {
	switch {
	case name == "confirm" || name == "cancel":
		return "EXIT"
	case strings.HasPrefix(name, "split-"):
		return "SPLIT"
	case strings.HasPrefix(name, "enter-"):
		return "MODES"
	case strings.HasPrefix(name, "focus-"):
		return "FOCUS"
	case strings.HasPrefix(name, "resize-"):
		return "RESIZE"
	case strings.HasPrefix(name, "vim-scroll-") || strings.HasPrefix(name, "half-page-"):
		return "SCROLL"
	case mode == input.ModeVim && vimLegendGroup(name) != "":
		return vimLegendGroup(name)
	case (mode == input.ModeTab || mode == input.ModePrefix) && (name == "next-tab" || name == "previous-tab"):
		return "SWITCH"
	default:
		return "MANAGE"
	}
}
