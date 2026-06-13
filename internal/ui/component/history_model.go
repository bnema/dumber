// Package component provides UI components for the browser.
package component

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
)

// historyGroup represents a day-grouped section of history entries.
// Groups are ordered newest-first; entries within a group are newest-first.
type historyGroup struct {
	Label   string
	Entries []*entity.HistoryEntry
}

// dayKey identifies a unique calendar day for grouping.
type dayKey struct {
	year  int
	month time.Month
	day   int
}

const (
	dayLabelToday           = "Today"
	dayLabelYesterday       = "Yesterday"
	dayLabelOtherYearFormat = "January 2, 2006"
)

// groupHistoryByDay groups entries by calendar day, newest-first.
// Days are labeled: "Today", "Yesterday", "Monday, January 2" (current year),
// "January 2, 2006" (other years). Within each group entries remain in
// the order they arrived (assumed most-recent-first).
func groupHistoryByDay(entries []*entity.HistoryEntry) []historyGroup {
	if len(entries) == 0 {
		return nil
	}

	now := time.Now()
	local := now.Location()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, local)

	groupsMap := make(map[dayKey]*historyGroup)
	var keys []dayKey

	for _, entry := range entries {
		t := entry.LastVisited.In(local)
		key := dayKey{t.Year(), t.Month(), t.Day()}

		if _, ok := groupsMap[key]; !ok {
			label := dayLabelForKey(key, todayStart, now)
			groupsMap[key] = &historyGroup{Label: label}
			keys = append(keys, key)
		}
		groupsMap[key].Entries = append(groupsMap[key].Entries, entry)
	}

	// Sort keys newest-first
	sort.Slice(keys, func(i, j int) bool {
		ki, kj := keys[i], keys[j]
		ti := time.Date(ki.year, ki.month, ki.day, 0, 0, 0, 0, local)
		tj := time.Date(kj.year, kj.month, kj.day, 0, 0, 0, 0, local)
		return ti.After(tj)
	})

	groups := make([]historyGroup, len(keys))
	for i, k := range keys {
		groups[i] = *groupsMap[k]
	}
	return groups
}

func dayLabelForKey(key dayKey, todayStart time.Time, now time.Time) string {
	dayStart := time.Date(key.year, key.month, key.day, 0, 0, 0, 0, now.Location())
	switch {
	case dayStart.Equal(todayStart):
		return dayLabelToday
	case dayStart.Equal(todayStart.AddDate(0, 0, -1)):
		return dayLabelYesterday
	case key.year == now.Year():
		return dayStart.Format("Monday, January 2")
	default:
		return dayStart.Format(dayLabelOtherYearFormat)
	}
}

// readableURL strips the protocol prefix and optional www. for display.
func readableURL(rawURL string) string {
	u := rawURL
	for _, prefix := range []string{"https://", "http://", "ftp://"} {
		if strings.HasPrefix(u, prefix) {
			u = u[len(prefix):]
			break
		}
	}
	u = strings.TrimPrefix(u, "www.")
	// Remove trailing slash when there's no path beyond it
	if slashIdx := strings.IndexByte(u, '/'); slashIdx == -1 || slashIdx == len(u)-1 {
		u = strings.TrimSuffix(u, "/")
	}
	return u
}

// relativeTime returns a compact relative time label for a timestamp.
// Examples: "2m ago", "1h ago", "3d ago", "Jan 2".
func relativeTime(t time.Time) string {
	now := time.Now()
	d := now.Sub(t)

	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m < 2 {
			return "1m ago"
		}
		return strconv.Itoa(m) + "m ago"
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h < 2 {
			return "1h ago"
		}
		return strconv.Itoa(h) + "h ago"
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		if days < 2 {
			return "1d ago"
		}
		return strconv.Itoa(days) + "d ago"
	default:
		if t.Year() == now.Year() {
			return t.Format("Jan 2")
		}
		return t.Format("Jan 2, 2006")
	}
}

// historyDisplayRowKind identifies the type of row rendered in the sidebar.
type historyDisplayRowKind int

const (
	historyDisplayRowHeader historyDisplayRowKind = iota
	historyDisplayRowEntry
)

// historyDisplayRow is the explicit row model rendered by the history sidebar.
// It is the only source used for row lookup, selection, activation, and delete.
type historyDisplayRow struct {
	Kind       historyDisplayRowKind
	Label      string
	Entry      *entity.HistoryEntry
	GroupIndex int
}

func buildHistoryDisplayRows(groups []historyGroup) []historyDisplayRow {
	if len(groups) == 0 {
		return nil
	}
	rows := make([]historyDisplayRow, 0)
	for gi, group := range groups {
		rows = append(rows, historyDisplayRow{Kind: historyDisplayRowHeader, Label: group.Label, GroupIndex: gi})
		for _, entry := range group.Entries {
			rows = append(rows, historyDisplayRow{Kind: historyDisplayRowEntry, Entry: entry, GroupIndex: gi})
		}
	}
	return rows
}

// keyboardNavModel provides pure functions for keyboard navigation over the
// explicit display rows. It has no GTK dependencies and can be tested directly.
type keyboardNavModel struct {
	rows []historyDisplayRow
}

// newKeyboardNavModel creates a keyboardNavModel over day-grouped history.
func newKeyboardNavModel(groups []historyGroup) keyboardNavModel {
	return newKeyboardNavModelFromRows(buildHistoryDisplayRows(groups))
}

func newKeyboardNavModelFromRows(rows []historyDisplayRow) keyboardNavModel {
	return keyboardNavModel{rows: rows}
}

// totalRows returns the number of explicit display rows.
func (m keyboardNavModel) totalRows() int { return len(m.rows) }

// isSelectable returns true when the display row corresponds to an entry.
func (m keyboardNavModel) isSelectable(index int) bool {
	return index >= 0 && index < len(m.rows) && m.rows[index].Kind == historyDisplayRowEntry && m.rows[index].Entry != nil
}

func (m keyboardNavModel) firstSelectableIndex() int {
	for i := 0; i < m.totalRows(); i++ {
		if m.isSelectable(i) {
			return i
		}
	}
	return -1
}

func (m keyboardNavModel) lastSelectableIndex() int {
	for i := m.totalRows() - 1; i >= 0; i-- {
		if m.isSelectable(i) {
			return i
		}
	}
	return -1
}

func (m keyboardNavModel) nextSelectableIndex(from, dir int) int {
	if dir != -1 && dir != +1 {
		return -1
	}
	for i := from + dir; i >= 0 && i < m.totalRows(); i += dir {
		if m.isSelectable(i) {
			return i
		}
	}
	return -1
}

func (m keyboardNavModel) groupIndexAt(index int) int {
	if index < 0 || index >= len(m.rows) {
		return -1
	}
	return m.rows[index].GroupIndex
}

func (m keyboardNavModel) maxGroupIndex() int {
	max := -1
	for _, row := range m.rows {
		if row.GroupIndex > max {
			max = row.GroupIndex
		}
	}
	return max
}

func (m keyboardNavModel) cumulativeOffsetAtGroup(gi int) int {
	if gi < 0 {
		return -1
	}
	for i, row := range m.rows {
		if row.GroupIndex == gi && row.Kind == historyDisplayRowHeader {
			return i
		}
	}
	return -1
}

func (m keyboardNavModel) firstEntryOfGroup(gi int) int {
	for i, row := range m.rows {
		if row.GroupIndex == gi && row.Kind == historyDisplayRowEntry && row.Entry != nil {
			return i
		}
	}
	return -1
}

func (m keyboardNavModel) previousDayBoundary(from int) int {
	gi := m.groupIndexAt(from)
	if gi <= 0 {
		return -1
	}
	return m.firstEntryOfGroup(gi - 1)
}

func (m keyboardNavModel) nextDayBoundary(from int) int {
	gi := m.groupIndexAt(from)
	if gi < 0 || gi >= m.maxGroupIndex() {
		return -1
	}
	return m.firstEntryOfGroup(gi + 1)
}

func (m keyboardNavModel) entryAt(index int) *entity.HistoryEntry {
	if !m.isSelectable(index) {
		return nil
	}
	return m.rows[index].Entry
}

func (m keyboardNavModel) entryURLAt(index int) string {
	e := m.entryAt(index)
	if e == nil {
		return ""
	}
	return e.URL
}

func (m keyboardNavModel) entryCount() int {
	n := 0
	for i := range m.rows {
		if m.isSelectable(i) {
			n++
		}
	}
	return n
}
