package systemviews

import (
	"fmt"
	"html"
	"strings"

	"github.com/bnema/dumber/internal/domain/entity"
)

func favoritesHTML(favorites []*entity.Favorite, folders []*entity.Folder, tags []*entity.Tag) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>systemviews favorites</title>
</head>
<body>
  <main id="app" data-route="favorites">
    <h1>Favorites</h1>
    <p>%d favorites · %d folders · %d tags</p>
    <section>
      <h2>Favorites</h2>
      <ul>%s</ul>
    </section>
    <section>
      <h2>Folders</h2>
      <ul>%s</ul>
    </section>
    <section>
      <h2>Tags</h2>
      <ul>%s</ul>
    </section>
  </main>
</body>
</html>`, len(favorites), len(folders), len(tags), favoriteItemsHTML(favorites), folderItemsHTML(folders), tagItemsHTML(tags))
}

func favoriteItemsHTML(favorites []*entity.Favorite) string {
	if len(favorites) == 0 {
		return "<li>No favorites</li>"
	}

	var items strings.Builder
	for _, favorite := range favorites {
		if favorite == nil {
			continue
		}
		label := favorite.Title
		if label == "" {
			label = favorite.URL
		}
		_, _ = fmt.Fprintf(&items, `<li><a href="%s">%s</a></li>`, html.EscapeString(favorite.URL), html.EscapeString(label))
	}
	if items.Len() == 0 {
		return "<li>No favorites</li>"
	}
	return items.String()
}

func folderItemsHTML(folders []*entity.Folder) string {
	if len(folders) == 0 {
		return "<li>No folders</li>"
	}

	var items strings.Builder
	for _, folder := range folders {
		if folder == nil {
			continue
		}
		_, _ = fmt.Fprintf(&items, "<li>%s</li>", html.EscapeString(folder.Name))
	}
	if items.Len() == 0 {
		return "<li>No folders</li>"
	}
	return items.String()
}

func tagItemsHTML(tags []*entity.Tag) string {
	if len(tags) == 0 {
		return "<li>No tags</li>"
	}

	var items strings.Builder
	for _, tag := range tags {
		if tag == nil {
			continue
		}
		_, _ = fmt.Fprintf(&items, "<li>%s</li>", html.EscapeString(tag.Name))
	}
	if items.Len() == 0 {
		return "<li>No tags</li>"
	}
	return items.String()
}
