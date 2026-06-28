package entity

import (
	"strings"
	"time"
)

// FavoriteID uniquely identifies a favorite/bookmark.
type FavoriteID int64

// TagID uniquely identifies a tag.
type TagID int64

// Favorite represents a bookmarked URL.
type Favorite struct {
	ID          FavoriteID `json:"id"`
	URL         string     `json:"url"`
	Title       string     `json:"title"`
	FaviconURL  string     `json:"favicon_url"`
	ShortcutKey *int       `json:"shortcut_key"` // 1-9 for quick access (Alt+1 through Alt+9)
	Position    int        `json:"position"`     // Order within tag or default list
	Tags        []Tag      `json:"tags,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// NewFavorite creates a new favorite for a URL.
func NewFavorite(url, title string) *Favorite {
	now := time.Now()
	return &Favorite{
		URL:       url,
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// HasShortcut returns true if this favorite has a keyboard shortcut.
func (f *Favorite) HasShortcut() bool {
	return f.ShortcutKey != nil && *f.ShortcutKey >= 1 && *f.ShortcutKey <= 9
}

// HasTag returns true if this favorite has the given tag.
func (f *Favorite) HasTag(tagID TagID) bool {
	for _, t := range f.Tags {
		if t.ID == tagID {
			return true
		}
	}
	return false
}

// Tag represents a label that can be applied to favorites.
type Tag struct {
	ID        TagID     `json:"id"`
	Name      string    `json:"name"`
	Color     string    `json:"color"` // Hex color code (e.g., "#FF5733")
	CreatedAt time.Time `json:"created_at"`
}

// NewTag creates a new tag with default color.
func NewTag(name string) *Tag {
	return &Tag{
		Name:      strings.TrimSpace(name),
		Color:     "#808080", // Default gray
		CreatedAt: time.Now(),
	}
}
