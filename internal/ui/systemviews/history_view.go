package systemviews

import (
	"fmt"
	"html"
	"strings"

	"github.com/bnema/dumber/internal/domain/entity"
)

func historyHTML(entries []*entity.HistoryEntry) string {
	var items strings.Builder
	if len(entries) == 0 {
		items.WriteString("<li>No history entries</li>")
	} else {
		for _, entry := range entries {
			if entry == nil {
				continue
			}
			label := entry.Title
			if label == "" {
				label = entry.URL
			}
			_, _ = fmt.Fprintf(&items, `<li><a href="%s">%s</a></li>`, html.EscapeString(entry.URL), html.EscapeString(label))
		}
	}

	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>systemviews history</title>
</head>
<body>
  <main id="app" data-route="history">
    <h1>History</h1>
    <ul>%s</ul>
  </main>
</body>
</html>`, items.String())
}
