package systemviews

import (
	"fmt"
	"strings"

	"github.com/bnema/dumber/internal/domain/entity"
)

const defaultTagColor = "#808080"

type favoritesRenderData struct {
	Favorites      []*entity.Favorite
	Tags           []*entity.Tag
	TagFilter      *entity.TagID
	UntaggedFilter bool
	Notice         string
	Error          string
}

func favoritesHTML(data favoritesRenderData) string {
	return mustRenderComponent(FavoritesView(data))
}

func favoritesDocumentTitle(data favoritesRenderData) string {
	count := len(data.Favorites)
	if data.TagFilter != nil || data.UntaggedFilter {
		if count == 1 {
			return "Favorites — 1 filtered bookmark"
		}
		return fmt.Sprintf("Favorites — %d filtered bookmarks", count)
	}
	if count == 1 {
		return "Favorites — 1 bookmark"
	}
	return fmt.Sprintf("Favorites — %d bookmarks", count)
}

func favoritesSummary(data favoritesRenderData) string {
	return fmt.Sprintf("%d favorites · %d tags", len(data.Favorites), len(data.Tags))
}

func tagsSummary(data favoritesRenderData) string {
	return fmt.Sprintf("%d tags", len(data.Tags))
}

func favoriteFilterClass(active bool) string {
	classes := "sv-button sv-button-secondary"
	if active {
		classes += " sv-button-active"
	}
	return classes
}

func favoriteUntaggedFilterActive(data favoritesRenderData) bool {
	return data.UntaggedFilter
}

func favoriteAllFilterActive(data favoritesRenderData) bool {
	return data.TagFilter == nil && !data.UntaggedFilter
}

func favoriteTagFilterActive(data favoritesRenderData, tagID entity.TagID) bool {
	return data.TagFilter != nil && *data.TagFilter == tagID
}

func favoriteItemLabel(favorite *entity.Favorite) string {
	if favorite == nil {
		return ""
	}
	label := strings.TrimSpace(favorite.Title)
	if label == "" {
		label = favorite.URL
	}
	return label
}

func favoriteMetaText(favorite *entity.Favorite) string {
	if favorite == nil || favorite.ShortcutKey == nil {
		return ""
	}
	return fmt.Sprintf("Shortcut %d", *favorite.ShortcutKey)
}

func favoriteTagButtonAction(favorite *entity.Favorite, tag *entity.Tag) string {
	if favorite != nil && tag != nil && favorite.HasTag(tag.ID) {
		return tagActionRemove
	}
	return tagActionAssign
}

func favoriteTagButtonLabel(favorite *entity.Favorite, tag *entity.Tag) string {
	if tag == nil {
		return ""
	}
	if favorite != nil && favorite.HasTag(tag.ID) {
		return "✓ " + tag.Name
	}
	return "+ " + tag.Name
}

func favoriteTagButtonClass(favorite *entity.Favorite, tag *entity.Tag) string {
	classes := "sv-button sv-button-secondary sv-tag-button"
	if favorite != nil && tag != nil && favorite.HasTag(tag.ID) {
		classes += " sv-button-active sv-tag-button-active"
	}
	return classes
}

func safeTagColor(raw string) string {
	value := strings.TrimSpace(raw)
	if len(value) != 4 && len(value) != 7 {
		return defaultTagColor
	}
	if value[0] != '#' {
		return defaultTagColor
	}
	for _, ch := range value[1:] {
		if (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F') {
			continue
		}
		return defaultTagColor
	}
	return value
}

func shortcutSelected(selected *int, value int) bool {
	return selected != nil && *selected == value
}

func filterFavorites(favorites []*entity.Favorite, tagID *entity.TagID, untagged bool) []*entity.Favorite {
	if tagID == nil && !untagged {
		return favorites
	}
	out := make([]*entity.Favorite, 0, len(favorites))
	for _, favorite := range favorites {
		if favorite == nil {
			continue
		}
		if untagged {
			if len(favorite.Tags) != 0 {
				continue
			}
		} else if tagID != nil && !favorite.HasTag(*tagID) {
			continue
		}
		out = append(out, favorite)
	}
	return out
}
