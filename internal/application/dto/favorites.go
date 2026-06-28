package dto

import "github.com/bnema/dumber/internal/domain/entity"

// FavoriteCreateInput contains fields needed to create a favorite from a UI port.
type FavoriteCreateInput struct {
	URL        string         `json:"url"`
	Title      string         `json:"title"`
	FaviconURL string         `json:"favicon_url"`
	Tags       []entity.TagID `json:"tags"`
}

// FavoriteUpdateInput contains editable favorite metadata.
type FavoriteUpdateInput struct {
	ID          entity.FavoriteID `json:"id"`
	Title       string            `json:"title"`
	FaviconURL  string            `json:"favicon_url"`
	ShortcutKey *int              `json:"shortcut_key"`
}
