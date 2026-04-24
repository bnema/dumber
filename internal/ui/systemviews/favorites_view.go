package systemviews

import (
	"fmt"
	"strings"

	"github.com/bnema/dumber/internal/domain/entity"
)

type favoritesRenderData struct {
	Favorites    []*entity.Favorite
	Folders      []*entity.Folder
	Tags         []*entity.Tag
	FolderFilter *entity.FolderID
	TagFilter    *entity.TagID
	Notice       string
	Error        string
}

func favoritesHTML(data favoritesRenderData) string {
	return mustRenderComponent(FavoritesView(data))
}

func favoritesSummary(data favoritesRenderData) string {
	return fmt.Sprintf("%d favorites · %d folders · %d tags", len(data.Favorites), len(data.Folders), len(data.Tags))
}

func foldersSummary(data favoritesRenderData) string {
	return fmt.Sprintf("%d folders", len(data.Folders))
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

func favoriteFolderFilterActive(data favoritesRenderData, folderID entity.FolderID) bool {
	return data.FolderFilter != nil && *data.FolderFilter == folderID
}

func favoriteAllFilterActive(data favoritesRenderData) bool {
	return data.FolderFilter == nil && data.TagFilter == nil
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

func favoriteMetaText(favorite *entity.Favorite, folders []*entity.Folder) string {
	if favorite == nil {
		return ""
	}
	parts := []string{}
	if favorite.ShortcutKey != nil {
		parts = append(parts, fmt.Sprintf("Shortcut %d", *favorite.ShortcutKey))
	}
	if name := folderNameForID(folders, favorite.FolderID); name != "" {
		parts = append(parts, "Folder "+name)
	}
	return strings.Join(parts, " · ")
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
	classes := "sv-button sv-button-secondary"
	if favorite != nil && tag != nil && favorite.HasTag(tag.ID) {
		classes += " sv-button-active"
	}
	return classes
}

func folderDisplayName(folder *entity.Folder) string {
	if folder == nil {
		return ""
	}
	if strings.TrimSpace(folder.Icon) != "" {
		return folder.Icon + " " + folder.Name
	}
	return folder.Name
}

func folderNameForID(folders []*entity.Folder, id *entity.FolderID) string {
	if id == nil {
		return "Unfiled"
	}
	for _, folder := range folders {
		if folder != nil && folder.ID == *id {
			return folderDisplayName(folder)
		}
	}
	return "Unknown"
}

func folderSelected(selected *entity.FolderID, folderID entity.FolderID) bool {
	return selected != nil && *selected == folderID
}

func shortcutSelected(selected *int, value int) bool {
	return selected != nil && *selected == value
}

func filterFavorites(favorites []*entity.Favorite, folderID *entity.FolderID, tagID *entity.TagID) []*entity.Favorite {
	if folderID == nil && tagID == nil {
		return favorites
	}
	out := make([]*entity.Favorite, 0, len(favorites))
	for _, favorite := range favorites {
		if favorite == nil {
			continue
		}
		if folderID != nil {
			if *folderID == 0 {
				if favorite.FolderID != nil {
					continue
				}
			} else if favorite.FolderID == nil || *favorite.FolderID != *folderID {
				continue
			}
		}
		if tagID != nil && !favorite.HasTag(*tagID) {
			continue
		}
		out = append(out, favorite)
	}
	return out
}
