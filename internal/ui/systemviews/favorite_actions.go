package systemviews

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
)

const (
	favoriteActionCreate       = "favorite.create"
	favoriteActionUpdate       = "favorite.update"
	favoriteActionDelete       = "favorite.delete"
	favoriteActionFilterFolder = "favorite.filterFolder"
	favoriteActionFilterTag    = "favorite.filterTag"
	favoriteActionClearFilters = "favorite.clearFilters"
	folderActionCreate         = "folder.create"
	folderActionUpdate         = "folder.update"
	folderActionDelete         = "folder.delete"
	tagActionCreate            = "tag.create"
	tagActionUpdate            = "tag.update"
	tagActionDelete            = "tag.delete"
	tagActionAssign            = "tag.assign"
	tagActionRemove            = "tag.remove"
)

func (a *App) handleFavoriteAction(ctx context.Context, event DOMAction) error {
	if a.deps.Favorites == nil {
		return fmt.Errorf("favorites service not configured")
	}
	data := event.Data
	switch event.Action {
	case favoriteActionCreate:
		folderID, err := optionalFolderID(data["folder_id"])
		if err != nil {
			return err
		}
		favorite, err := a.deps.Favorites.CreateFavorite(ctx, port.FavoriteCreateInput{
			URL:      strings.TrimSpace(data["url"]),
			Title:    strings.TrimSpace(data["title"]),
			FolderID: folderID,
		})
		if err != nil {
			return err
		}
		a.favoritesNotice = "Added favorite " + favoriteItemLabel(favorite)
	case favoriteActionUpdate:
		id, err := parsePositiveInt64(data["id"], "favorite id")
		if err != nil {
			return err
		}
		folderID, err := optionalFolderID(data["folder_id"])
		if err != nil {
			return err
		}
		shortcut, err := optionalShortcut(data["shortcut_key"])
		if err != nil {
			return err
		}
		favorite, err := a.deps.Favorites.UpdateFavorite(ctx, port.FavoriteUpdateInput{
			ID:          entity.FavoriteID(id),
			Title:       strings.TrimSpace(data["title"]),
			FaviconURL:  strings.TrimSpace(data["favicon_url"]),
			FolderID:    folderID,
			ShortcutKey: shortcut,
		})
		if err != nil {
			return err
		}
		a.favoritesNotice = "Saved favorite " + favoriteItemLabel(favorite)
	case favoriteActionDelete:
		id, err := parsePositiveInt64(data["id"], "favorite id")
		if err != nil {
			return err
		}
		if err := a.deps.Favorites.DeleteFavorite(ctx, id); err != nil {
			return err
		}
		a.favoritesNotice = "Deleted favorite"
	case favoriteActionFilterFolder:
		folderID, err := filterFolderID(data["folder_id"])
		if err != nil {
			return err
		}
		a.favoriteFolderFilter = folderID
		a.favoriteTagFilter = nil
		a.favoritesNotice = ""
	case favoriteActionFilterTag:
		tagID, err := parsePositiveInt64(data["tag_id"], "tag id")
		if err != nil {
			return err
		}
		id := entity.TagID(tagID)
		a.favoriteTagFilter = &id
		a.favoriteFolderFilter = nil
		a.favoritesNotice = ""
	case favoriteActionClearFilters:
		a.favoriteFolderFilter = nil
		a.favoriteTagFilter = nil
		a.favoritesNotice = ""
	case folderActionCreate:
		name := strings.TrimSpace(data["name"])
		if name == "" {
			return fmt.Errorf("folder name is required")
		}
		folder, err := a.deps.Favorites.CreateFolder(ctx, name, nil)
		if err != nil {
			return err
		}
		a.favoritesNotice = "Created folder " + folderDisplayName(folder)
	case folderActionUpdate:
		id, err := parsePositiveInt64(data["id"], "folder id")
		if err != nil {
			return err
		}
		name := strings.TrimSpace(data["name"])
		if name == "" {
			return fmt.Errorf("folder name is required")
		}
		if err := a.deps.Favorites.UpdateFolder(ctx, id, name, strings.TrimSpace(data["icon"])); err != nil {
			return err
		}
		a.favoritesNotice = "Saved folder " + name
	case folderActionDelete:
		id, err := parsePositiveInt64(data["id"], "folder id")
		if err != nil {
			return err
		}
		if err := a.deps.Favorites.DeleteFolder(ctx, id); err != nil {
			return err
		}
		a.favoriteFolderFilter = nil
		a.favoritesNotice = "Deleted folder"
	case tagActionCreate:
		name := strings.TrimSpace(data["name"])
		if name == "" {
			return fmt.Errorf("tag name is required")
		}
		tag, err := a.deps.Favorites.CreateTag(ctx, name, strings.TrimSpace(data["color"]))
		if err != nil {
			return err
		}
		a.favoritesNotice = "Created tag " + tag.Name
	case tagActionUpdate:
		id, err := parsePositiveInt64(data["id"], "tag id")
		if err != nil {
			return err
		}
		name := strings.TrimSpace(data["name"])
		if name == "" {
			return fmt.Errorf("tag name is required")
		}
		if err := a.deps.Favorites.UpdateTag(ctx, id, name, strings.TrimSpace(data["color"])); err != nil {
			return err
		}
		a.favoritesNotice = "Saved tag " + name
	case tagActionDelete:
		id, err := parsePositiveInt64(data["id"], "tag id")
		if err != nil {
			return err
		}
		if err := a.deps.Favorites.DeleteTag(ctx, id); err != nil {
			return err
		}
		a.favoriteTagFilter = nil
		a.favoritesNotice = "Deleted tag"
	case tagActionAssign, tagActionRemove:
		favoriteID, err := parsePositiveInt64(data["favoriteId"], "favorite id")
		if err != nil {
			return err
		}
		tagID, err := parsePositiveInt64(data["tagId"], "tag id")
		if err != nil {
			return err
		}
		if event.Action == tagActionAssign {
			if err := a.deps.Favorites.AssignTag(ctx, favoriteID, tagID); err != nil {
				return err
			}
			a.favoritesNotice = "Assigned tag"
		} else {
			if err := a.deps.Favorites.RemoveTag(ctx, favoriteID, tagID); err != nil {
				return err
			}
			a.favoritesNotice = "Removed tag"
		}
	}
	return nil
}

func parsePositiveInt64(raw, label string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid %s", label)
	}
	return id, nil
}

func optionalFolderID(raw string) (*entity.FolderID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "root" {
		return nil, nil
	}
	id, err := parsePositiveInt64(raw, "folder id")
	if err != nil {
		return nil, err
	}
	folderID := entity.FolderID(id)
	return &folderID, nil
}

func filterFolderID(raw string) (*entity.FolderID, error) {
	if strings.TrimSpace(raw) == "root" {
		root := entity.FolderID(0)
		return &root, nil
	}
	return optionalFolderID(raw)
}

func optionalShortcut(raw string) (*int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > 9 {
		return nil, fmt.Errorf("shortcut key must be 1-9")
	}
	return &value, nil
}
