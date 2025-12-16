package entity

import "time"

// FavoriteID uniquely identifies a favorite/bookmark.
type FavoriteID int64

// FolderID uniquely identifies a bookmark folder.
type FolderID int64

// TagID uniquely identifies a tag.
type TagID int64

// Favorite represents a bookmarked URL.
type Favorite struct {
	ID          FavoriteID `json:"id"`
	URL         string     `json:"url"`
	Title       string     `json:"title"`
	FaviconURL  string     `json:"favicon_url"`
	FolderID    *FolderID  `json:"folder_id"`    // nil = root level
	ShortcutKey *int       `json:"shortcut_key"` // 1-9 for quick access (Alt+1 through Alt+9)
	Position    int        `json:"position"`     // Order within folder
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

// InFolder returns true if this favorite is in a folder.
func (f *Favorite) InFolder() bool {
	return f.FolderID != nil
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

// Folder represents a container for organizing favorites.
type Folder struct {
	ID        FolderID  `json:"id"`
	Name      string    `json:"name"`
	Icon      string    `json:"icon"`      // Optional icon identifier
	ParentID  *FolderID `json:"parent_id"` // nil = root level
	Position  int       `json:"position"`  // Order within parent
	CreatedAt time.Time `json:"created_at"`
}

// NewFolder creates a new folder.
func NewFolder(name string) *Folder {
	return &Folder{
		Name:      name,
		CreatedAt: time.Now(),
	}
}

// IsRoot returns true if this folder is at root level.
func (f *Folder) IsRoot() bool {
	return f.ParentID == nil
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
		Name:      name,
		Color:     "#808080", // Default gray
		CreatedAt: time.Now(),
	}
}

// FavoriteTree represents a hierarchical view of folders and favorites.
type FavoriteTree struct {
	RootFolders    []*Folder
	RootFavorites  []*Favorite
	FolderMap      map[FolderID]*Folder     // Quick lookup
	ChildFolders   map[FolderID][]*Folder   // Children of each folder
	ChildFavorites map[FolderID][]*Favorite // Favorites in each folder
}

// NewFavoriteTree creates an empty favorite tree.
func NewFavoriteTree() *FavoriteTree {
	return &FavoriteTree{
		RootFolders:    make([]*Folder, 0),
		RootFavorites:  make([]*Favorite, 0),
		FolderMap:      make(map[FolderID]*Folder),
		ChildFolders:   make(map[FolderID][]*Folder),
		ChildFavorites: make(map[FolderID][]*Favorite),
	}
}
