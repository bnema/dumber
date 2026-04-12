package systemviews

import (
	"fmt"
	"html"
	"strings"

	"github.com/bnema/dumber/internal/domain/entity"
)

func favoritesHTML(favorites []*entity.Favorite, folders []*entity.Folder, tags []*entity.Tag) string {
	favoritesSummary := fmt.Sprintf("%d favorites · %d folders · %d tags", len(favorites), len(folders), len(tags))
	foldersSummary := fmt.Sprintf("%d folders", len(folders))
	tagsSummary := fmt.Sprintf("%d tags", len(tags))
	sections := []string{
		sectionHTML("", "Favorites", metaHTML(favoritesSummary)+listHTML(favoriteItemsHTML(favorites), "No favorites")),
		sectionHTML("", "Folders", metaHTML(foldersSummary)+listHTML(folderItemsHTML(folders), "No folders")),
		sectionHTML("", "Tags", metaHTML(tagsSummary)+listHTML(tagItemsHTML(tags), "No tags")),
	}
	return strings.Join(sections, "")
}

func favoriteItemsHTML(favorites []*entity.Favorite) string {
	var items strings.Builder
	for _, favorite := range favorites {
		if favorite == nil {
			continue
		}
		label := favorite.Title
		if label == "" {
			label = favorite.URL
		}
		items.WriteString(listRowHTML(linkHTML(favorite.URL, label)))
	}
	return items.String()
}

func folderItemsHTML(folders []*entity.Folder) string {
	var items strings.Builder
	for _, folder := range folders {
		if folder == nil {
			continue
		}
		items.WriteString(listRowHTML(html.EscapeString(folder.Name)))
	}
	return items.String()
}

func tagItemsHTML(tags []*entity.Tag) string {
	var items strings.Builder
	for _, tag := range tags {
		if tag == nil {
			continue
		}
		items.WriteString(listRowHTML(html.EscapeString(tag.Name)))
	}
	return items.String()
}
