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

// keyboardNavModel provides pure functions for keyboard navigation over
// day-grouped history entries. It has no GTK dependencies and can be tested
// directly. A "linear index" refers to the ListBox row position: group
// headers occupy one row, followed by each entry row.
type keyboardNavModel struct {
	groups []historyGroup
}

// newKeyboardNavModel creates a keyboardNavModel over the given groups.
func newKeyboardNavModel(groups []historyGroup) keyboardNavModel {
	return keyboardNavModel{groups: groups}
}

// totalRows returns the number of linear rows (one per group header +
// one per entry).
func (m keyboardNavModel) totalRows() int {
	n := 0
	for _, g := range m.groups {
		n++ // header
		n += len(g.Entries)
	}
	return n
}

// isSelectable returns true when the linear index corresponds to an entry
// row (not a group header).
func (m keyboardNavModel) isSelectable(index int) bool {
	if index < 0 {
		return false
	}
	linear := 0
	for _, g := range m.groups {
		if index == linear {
			return false // header
		}
		linear++ // skip header
		if index < linear+len(g.Entries) {
			return true
		}
		linear += len(g.Entries)
	}
	return false
}

// firstSelectableIndex returns the linear index of the first entry row,
// or -1 when there are no entries.
func (m keyboardNavModel) firstSelectableIndex() int {
	for i := 0; i < m.totalRows(); i++ {
		if m.isSelectable(i) {
			return i
		}
	}
	return -1
}

// lastSelectableIndex returns the linear index of the last entry row,
// or -1 when there are no entries.
func (m keyboardNavModel) lastSelectableIndex() int {
	for i := m.totalRows() - 1; i >= 0; i-- {
		if m.isSelectable(i) {
			return i
		}
	}
	return -1
}

// nextSelectableIndex returns the next selectable index in direction dir
// (-1 or +1), or -1 when there is no further selectable row in that
// direction. Skips group headers.
func (m keyboardNavModel) nextSelectableIndex(from, dir int) int {
	if dir != -1 && dir != +1 {
		return -1
	}
	i := from + dir
	for i >= 0 && i < m.totalRows() {
		if m.isSelectable(i) {
			return i
		}
		i += dir
	}
	return -1
}

// groupIndexAt returns the index of the group containing the given linear row.
func (m keyboardNavModel) groupIndexAt(index int) int {
	linear := 0
	for gi, g := range m.groups {
		if index == linear {
			return gi // header
		}
		linear++ // skip header
		if index < linear+len(g.Entries) {
			return gi
		}
		linear += len(g.Entries)
	}
	return -1
}

// cumulativeOffsetAtGroup returns the linear index of the group header row
// for the group at gi.
func (m keyboardNavModel) cumulativeOffsetAtGroup(gi int) int {
	if gi < 0 || gi >= len(m.groups) {
		return -1
	}
	offset := 0
	for i := 0; i < gi; i++ {
		offset += 1 + len(m.groups[i].Entries)
	}
	return offset
}

// firstEntryOfGroup returns the linear index of the first entry in the
// group at gi, or -1 if the group has no entries.
func (m keyboardNavModel) firstEntryOfGroup(gi int) int {
	if gi < 0 || gi >= len(m.groups) {
		return -1
	}
	offset := m.cumulativeOffsetAtGroup(gi)
	firstEntry := offset + 1
	if firstEntry < m.totalRows() && m.isSelectable(firstEntry) {
		return firstEntry
	}
	return -1
}

// previousDayBoundary returns the linear index of the first entry in the
// day group that precedes the row at fromIndex. Returns -1 when there is
// no earlier day group.
func (m keyboardNavModel) previousDayBoundary(from int) int {
	gi := m.groupIndexAt(from)
	if gi <= 0 {
		return -1
	}
	return m.firstEntryOfGroup(gi - 1)
}

// nextDayBoundary returns the linear index of the first entry in the day
// group that follows the row at fromIndex. Returns -1 when there is no
// later day group.
func (m keyboardNavModel) nextDayBoundary(from int) int {
	gi := m.groupIndexAt(from)
	if gi < 0 || gi >= len(m.groups)-1 {
		return -1
	}
	return m.firstEntryOfGroup(gi + 1)
}

// entryAt returns the history entry at the given linear index, or nil
// when the index is out of range or points at a group header.
func (m keyboardNavModel) entryAt(index int) *entity.HistoryEntry {
	if index < 0 {
		return nil
	}
	linear := 0
	for _, g := range m.groups {
		if index == linear {
			return nil // header
		}
		linear++
		if index < linear+len(g.Entries) {
			return g.Entries[index-linear]
		}
		linear += len(g.Entries)
	}
	return nil
}

// entryURLAt returns the URL of the entry at the given linear index, or
// "" for header rows or out-of-range indices.
func (m keyboardNavModel) entryURLAt(index int) string {
	e := m.entryAt(index)
	if e == nil {
		return ""
	}
	return e.URL
}

// entryCount returns the total number of history entries (selectable rows).
func (m keyboardNavModel) entryCount() int {
	n := 0
	for _, g := range m.groups {
		n += len(g.Entries)
	}
	return n
}
