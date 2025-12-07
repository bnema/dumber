package handlers

import (
	"fmt"
	"strconv"

	"github.com/bnema/dumber/internal/logging"
)

// FavoritesMessage contains fields for favorites operations.
type FavoritesMessage struct {
	Type       string `json:"type"`
	URL        string `json:"url"`
	Title      string `json:"title"`
	FaviconURL string `json:"faviconURL"`
	ID         int64  `json:"id"`
	FolderID   int64  `json:"folderId"`
	TagID      int64  `json:"tagId"`
	Shortcut   int    `json:"shortcut"` // 1-9
	Name       string `json:"name"`
	Icon       string `json:"icon"`
	Color      string `json:"color"`
	Position   int64  `json:"position"`
	RequestID  string `json:"requestId"`
}

// HandleGetFavorites sends all favorites to JavaScript.
func HandleGetFavorites(c *Context, msg FavoritesMessage) {
	if !c.IsWebViewReady() || c.BrowserService == nil {
		return
	}

	favorites, err := c.BrowserService.GetFavorites(c.Ctx())
	if err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to get favorites: %v", err))
		_ = c.InjectError("__dumber_favorites_error", "Failed to get favorites")
		return
	}

	logging.Debug(fmt.Sprintf("[handlers] Sending %d favorites to JavaScript", len(favorites)))
	_ = c.InjectJSONWithRequestID("__dumber_favorites", favorites, msg.RequestID)
}

// HandleToggleFavorite adds or removes a URL from favorites.
func HandleToggleFavorite(c *Context, msg FavoritesMessage) {
	if !c.IsWebViewReady() || c.BrowserService == nil {
		return
	}

	if msg.URL == "" {
		logging.Warn("[handlers] Cannot toggle favorite - URL is empty")
		return
	}

	added, err := c.BrowserService.ToggleFavorite(c.Ctx(), msg.URL, msg.Title, msg.FaviconURL)
	if err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to toggle favorite for %s: %v", msg.URL, err))
		_ = c.InjectError("__dumber_favorite_toggled_error", "Failed to toggle favorite")
		return
	}

	logging.Debug(fmt.Sprintf("[handlers] Favorite toggled for %s (added: %v)", msg.URL, added))
	_ = c.InjectJSON("__dumber_favorite_toggled", map[string]interface{}{
		"url":   msg.URL,
		"added": added,
	})
}

// HandleIsFavorite checks if a URL is favorited.
func HandleIsFavorite(c *Context, msg FavoritesMessage) {
	if !c.IsWebViewReady() || c.BrowserService == nil {
		return
	}

	if msg.URL == "" {
		return
	}

	isFavorite, err := c.BrowserService.IsFavorite(c.Ctx(), msg.URL)
	if err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to check if favorite for %s: %v", msg.URL, err))
		return
	}

	_ = c.InjectJSON("__dumber_is_favorite", map[string]interface{}{
		"url":        msg.URL,
		"isFavorite": isFavorite,
	})
}

// ═══════════════════════════════════════════════════════════════
// FOLDER HANDLERS
// ═══════════════════════════════════════════════════════════════

// HandleFolderList returns all favorite folders.
func HandleFolderList(c *Context, msg FavoritesMessage) {
	if !c.IsWebViewReady() || c.BrowserService == nil {
		return
	}

	folders, err := c.BrowserService.GetFolders(c.Ctx())
	if err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to get folders: %v", err))
		_ = c.InjectError("__dumber_folder_error", "Failed to load folders")
		return
	}

	_ = c.InjectJSONWithRequestID("__dumber_folders", folders, msg.RequestID)
}

// HandleFolderCreate creates a new folder.
func HandleFolderCreate(c *Context, msg FavoritesMessage) {
	if !c.IsWebViewReady() || c.BrowserService == nil {
		return
	}

	if msg.Name == "" {
		_ = c.InjectErrorWithRequestID("__dumber_error", "Folder name is required", msg.RequestID)
		return
	}

	folder, err := c.BrowserService.CreateFolder(c.Ctx(), msg.Name, msg.Icon)
	if err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to create folder: %v", err))
		_ = c.InjectErrorWithRequestID("__dumber_error", "Failed to create folder", msg.RequestID)
		return
	}

	_ = c.InjectJSONWithRequestID("__dumber_folders", folder, msg.RequestID)
}

// HandleFolderUpdate updates an existing folder.
func HandleFolderUpdate(c *Context, msg FavoritesMessage) {
	if !c.IsWebViewReady() || c.BrowserService == nil {
		return
	}

	if msg.ID == 0 {
		_ = c.InjectError("__dumber_folder_error", "Folder ID is required")
		return
	}

	if err := c.BrowserService.UpdateFolder(c.Ctx(), msg.ID, msg.Name, msg.Icon); err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to update folder %d: %v", msg.ID, err))
		_ = c.InjectError("__dumber_folder_error", "Failed to update folder")
		return
	}

	_ = c.InjectJSON("__dumber_folder_updated", map[string]int64{"id": msg.ID})
}

// HandleFolderDelete deletes a folder.
func HandleFolderDelete(c *Context, msg FavoritesMessage) {
	if !c.IsWebViewReady() || c.BrowserService == nil {
		return
	}

	if msg.ID == 0 {
		_ = c.InjectError("__dumber_folder_error", "Folder ID is required")
		return
	}

	if err := c.BrowserService.DeleteFolder(c.Ctx(), msg.ID); err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to delete folder %d: %v", msg.ID, err))
		_ = c.InjectError("__dumber_folder_error", "Failed to delete folder")
		return
	}

	_ = c.InjectJSON("__dumber_folder_deleted", map[string]int64{"id": msg.ID})
}

// ═══════════════════════════════════════════════════════════════
// TAG HANDLERS
// ═══════════════════════════════════════════════════════════════

// HandleTagList returns all tags.
func HandleTagList(c *Context, msg FavoritesMessage) {
	if !c.IsWebViewReady() || c.BrowserService == nil {
		return
	}

	tags, err := c.BrowserService.GetTags(c.Ctx())
	if err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to get tags: %v", err))
		_ = c.InjectError("__dumber_tag_error", "Failed to load tags")
		return
	}

	_ = c.InjectJSONWithRequestID("__dumber_tags", tags, msg.RequestID)
}

// HandleTagCreate creates a new tag.
func HandleTagCreate(c *Context, msg FavoritesMessage) {
	if !c.IsWebViewReady() || c.BrowserService == nil {
		return
	}

	if msg.Name == "" {
		_ = c.InjectErrorWithRequestID("__dumber_error", "Tag name is required", msg.RequestID)
		return
	}

	color := msg.Color
	if color == "" {
		color = "#6b7280" // default gray
	}

	tag, err := c.BrowserService.CreateTag(c.Ctx(), msg.Name, color)
	if err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to create tag: %v", err))
		_ = c.InjectErrorWithRequestID("__dumber_error", "Failed to create tag", msg.RequestID)
		return
	}

	_ = c.InjectJSONWithRequestID("__dumber_tags", tag, msg.RequestID)
}

// HandleTagUpdate updates an existing tag.
func HandleTagUpdate(c *Context, msg FavoritesMessage) {
	if !c.IsWebViewReady() || c.BrowserService == nil {
		return
	}

	if msg.ID == 0 {
		_ = c.InjectError("__dumber_tag_error", "Tag ID is required")
		return
	}

	if err := c.BrowserService.UpdateTag(c.Ctx(), msg.ID, msg.Name, msg.Color); err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to update tag %d: %v", msg.ID, err))
		_ = c.InjectError("__dumber_tag_error", "Failed to update tag")
		return
	}

	_ = c.InjectJSON("__dumber_tag_updated", map[string]int64{"id": msg.ID})
}

// HandleTagDelete deletes a tag.
func HandleTagDelete(c *Context, msg FavoritesMessage) {
	if !c.IsWebViewReady() || c.BrowserService == nil {
		return
	}

	if msg.ID == 0 {
		_ = c.InjectError("__dumber_tag_error", "Tag ID is required")
		return
	}

	if err := c.BrowserService.DeleteTag(c.Ctx(), msg.ID); err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to delete tag %d: %v", msg.ID, err))
		_ = c.InjectError("__dumber_tag_error", "Failed to delete tag")
		return
	}

	_ = c.InjectJSON("__dumber_tag_deleted", map[string]int64{"id": msg.ID})
}

// HandleTagAssign assigns a tag to a favorite.
func HandleTagAssign(c *Context, msg FavoritesMessage) {
	if !c.IsWebViewReady() || c.BrowserService == nil {
		return
	}

	if msg.ID == 0 || msg.TagID == 0 {
		_ = c.InjectError("__dumber_tag_error", "Favorite ID and Tag ID are required")
		return
	}

	if err := c.BrowserService.AssignTag(c.Ctx(), msg.ID, msg.TagID); err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to assign tag %d to favorite %d: %v", msg.TagID, msg.ID, err))
		_ = c.InjectError("__dumber_tag_error", "Failed to assign tag")
		return
	}

	_ = c.InjectJSON("__dumber_tag_assigned", map[string]int64{
		"favoriteId": msg.ID,
		"tagId":      msg.TagID,
	})
}

// HandleTagRemove removes a tag from a favorite.
func HandleTagRemove(c *Context, msg FavoritesMessage) {
	if !c.IsWebViewReady() || c.BrowserService == nil {
		return
	}

	if msg.ID == 0 || msg.TagID == 0 {
		_ = c.InjectError("__dumber_tag_error", "Favorite ID and Tag ID are required")
		return
	}

	if err := c.BrowserService.RemoveTag(c.Ctx(), msg.ID, msg.TagID); err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to remove tag %d from favorite %d: %v", msg.TagID, msg.ID, err))
		_ = c.InjectError("__dumber_tag_error", "Failed to remove tag")
		return
	}

	_ = c.InjectJSON("__dumber_tag_removed", map[string]int64{
		"favoriteId": msg.ID,
		"tagId":      msg.TagID,
	})
}

// ═══════════════════════════════════════════════════════════════
// FAVORITE SHORTCUT & FOLDER HANDLERS
// ═══════════════════════════════════════════════════════════════

// HandleFavoriteSetShortcut sets a keyboard shortcut (1-9) for a favorite.
func HandleFavoriteSetShortcut(c *Context, msg FavoritesMessage) {
	if !c.IsWebViewReady() || c.BrowserService == nil {
		return
	}

	if msg.ID == 0 {
		_ = c.InjectError("__dumber_favorite_error", "Favorite ID is required")
		return
	}

	if msg.Shortcut < 0 || msg.Shortcut > 9 {
		_ = c.InjectError("__dumber_favorite_error", "Shortcut must be 1-9 or 0 to clear")
		return
	}

	if err := c.BrowserService.SetFavoriteShortcut(c.Ctx(), msg.ID, msg.Shortcut); err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to set shortcut for favorite %d: %v", msg.ID, err))
		_ = c.InjectError("__dumber_favorite_error", "Failed to set shortcut")
		return
	}

	_ = c.InjectJSON("__dumber_favorite_shortcut_set", map[string]interface{}{
		"id":       msg.ID,
		"shortcut": msg.Shortcut,
	})
}

// HandleFavoriteGetByShortcut gets a favorite by its shortcut key.
func HandleFavoriteGetByShortcut(c *Context, msg FavoritesMessage) {
	if !c.IsWebViewReady() || c.BrowserService == nil {
		return
	}

	if msg.Shortcut < 1 || msg.Shortcut > 9 {
		_ = c.InjectJSON("__dumber_favorite_by_shortcut", nil)
		return
	}

	favorite, err := c.BrowserService.GetFavoriteByShortcut(c.Ctx(), msg.Shortcut)
	if err != nil {
		// Not found is not an error - just return null
		_ = c.InjectJSON("__dumber_favorite_by_shortcut", nil)
		return
	}

	_ = c.InjectJSON("__dumber_favorite_by_shortcut", favorite)
}

// HandleFavoriteSetFolder moves a favorite to a folder.
func HandleFavoriteSetFolder(c *Context, msg FavoritesMessage) {
	if !c.IsWebViewReady() || c.BrowserService == nil {
		return
	}

	if msg.ID == 0 {
		_ = c.InjectError("__dumber_favorite_error", "Favorite ID is required")
		return
	}

	// FolderID of 0 means remove from folder
	var folderID *int64
	if msg.FolderID > 0 {
		folderID = &msg.FolderID
	}

	if err := c.BrowserService.SetFavoriteFolder(c.Ctx(), msg.ID, folderID); err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to set folder for favorite %d: %v", msg.ID, err))
		_ = c.InjectError("__dumber_favorite_error", "Failed to move favorite")
		return
	}

	_ = c.InjectJSON("__dumber_favorite_folder_set", map[string]interface{}{
		"id":       msg.ID,
		"folderId": msg.FolderID,
	})
}

// ParseFavoriteID parses a string ID to int64.
func ParseFavoriteID(idStr string) (int64, error) {
	return strconv.ParseInt(idStr, 10, 64)
}
