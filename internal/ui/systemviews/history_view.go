package systemviews

import (
	"fmt"
	"strings"

	"github.com/bnema/dumber/internal/domain/entity"
)

func historyHTML(entries []*entity.HistoryEntry) string {
	items := historyItemsHTML(entries)
	return sectionHTML("", "History", metaHTML(fmt.Sprintf("%d entries", countHistoryEntries(entries)))+listHTML(items, "No history entries"))
}

func historyItemsHTML(entries []*entity.HistoryEntry) string {
	var items strings.Builder
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		label := entry.Title
		if label == "" {
			label = entry.URL
		}
		items.WriteString(listRowHTML(linkHTML(entry.URL, label)))
	}

	return items.String()
}

func countHistoryEntries(entries []*entity.HistoryEntry) int {
	count := 0
	for _, entry := range entries {
		if entry != nil {
			count++
		}
	}
	return count
}
