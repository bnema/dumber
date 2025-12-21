package usecase

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/logging"
)

// ManageFavoritesUseCase handles favorite, folder, and tag operations.
type ManageFavoritesUseCase struct {
	favoriteRepo repository.FavoriteRepository
	folderRepo   repository.FolderRepository
	tagRepo      repository.TagRepository
}

// NewManageFavoritesUseCase creates a new favorites management use case.
func NewManageFavoritesUseCase(
	favoriteRepo repository.FavoriteRepository,
	folderRepo repository.FolderRepository,
	tagRepo repository.TagRepository,
) *ManageFavoritesUseCase {
	return &ManageFavoritesUseCase{
		favoriteRepo: favoriteRepo,
		folderRepo:   folderRepo,
		tagRepo:      tagRepo,
	}
}

// AddFavoriteInput contains parameters for adding a favorite.
type AddFavoriteInput struct {
	URL        string
	Title      string
	FaviconURL string
	FolderID   *entity.FolderID
	Tags       []entity.TagID
}

// Add creates a new favorite.
func (uc *ManageFavoritesUseCase) Add(ctx context.Context, input AddFavoriteInput) (*entity.Favorite, error) {
	log := logging.FromContext(ctx)
	log.Debug().Str("url", input.URL).Str("title", input.Title).Msg("adding favorite")

	// Check if already favorited
	existing, err := uc.favoriteRepo.FindByURL(ctx, input.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing favorite: %w", err)
	}
	if existing != nil {
		log.Debug().Str("url", input.URL).Msg("URL already favorited")
		return existing, nil
	}

	fav := entity.NewFavorite(input.URL, input.Title)
	fav.FaviconURL = input.FaviconURL
	fav.FolderID = input.FolderID

	if err := uc.favoriteRepo.Save(ctx, fav); err != nil {
		return nil, fmt.Errorf("failed to save favorite: %w", err)
	}

	// Assign tags
	for _, tagID := range input.Tags {
		if err := uc.tagRepo.AssignToFavorite(ctx, tagID, fav.ID); err != nil {
			log.Warn().
				Int64("tag_id", int64(tagID)).
				Int64("favorite_id", int64(fav.ID)).
				Err(err).
				Msg("failed to assign tag")
		}
	}

	log.Info().Str("url", input.URL).Int64("id", int64(fav.ID)).Msg("favorite added")
	return fav, nil
}

// Remove deletes a favorite by ID.
func (uc *ManageFavoritesUseCase) Remove(ctx context.Context, id entity.FavoriteID) error {
	log := logging.FromContext(ctx)
	log.Debug().Int64("id", int64(id)).Msg("removing favorite")

	if err := uc.favoriteRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("failed to delete favorite: %w", err)
	}

	log.Info().Int64("id", int64(id)).Msg("favorite removed")
	return nil
}

// RemoveByURL deletes a favorite by its URL.
func (uc *ManageFavoritesUseCase) RemoveByURL(ctx context.Context, url string) error {
	log := logging.FromContext(ctx)
	log.Debug().Str("url", url).Msg("removing favorite by URL")

	fav, err := uc.favoriteRepo.FindByURL(ctx, url)
	if err != nil {
		return fmt.Errorf("failed to find favorite: %w", err)
	}
	if fav == nil {
		log.Debug().Str("url", url).Msg("favorite not found")
		return nil
	}

	return uc.Remove(ctx, fav.ID)
}

// Update modifies a favorite's metadata.
func (uc *ManageFavoritesUseCase) Update(ctx context.Context, fav *entity.Favorite) error {
	log := logging.FromContext(ctx)
	log.Debug().Int64("id", int64(fav.ID)).Msg("updating favorite")

	if err := uc.favoriteRepo.Save(ctx, fav); err != nil {
		return fmt.Errorf("failed to update favorite: %w", err)
	}

	log.Info().Int64("id", int64(fav.ID)).Msg("favorite updated")
	return nil
}

// Move changes a favorite's folder.
func (uc *ManageFavoritesUseCase) Move(ctx context.Context, id entity.FavoriteID, folderID *entity.FolderID) error {
	log := logging.FromContext(ctx)
	log.Debug().
		Int64("favorite_id", int64(id)).
		Msg("moving favorite to folder")

	if err := uc.favoriteRepo.SetFolder(ctx, id, folderID); err != nil {
		return fmt.Errorf("failed to move favorite: %w", err)
	}

	log.Info().Int64("id", int64(id)).Msg("favorite moved")
	return nil
}

// SetShortcut assigns or removes a keyboard shortcut (1-9).
func (uc *ManageFavoritesUseCase) SetShortcut(ctx context.Context, id entity.FavoriteID, key *int) error {
	log := logging.FromContext(ctx)

	if key != nil && (*key < 1 || *key > 9) {
		return fmt.Errorf("shortcut key must be 1-9, got %d", *key)
	}

	log.Debug().
		Int64("id", int64(id)).
		Msg("setting favorite shortcut")

	if err := uc.favoriteRepo.SetShortcut(ctx, id, key); err != nil {
		return fmt.Errorf("failed to set shortcut: %w", err)
	}

	return nil
}

// GetByShortcut finds the favorite with the given shortcut key.
func (uc *ManageFavoritesUseCase) GetByShortcut(ctx context.Context, key int) (*entity.Favorite, error) {
	log := logging.FromContext(ctx)
	log.Debug().Int("key", key).Msg("getting favorite by shortcut")

	if key < 1 || key > 9 {
		return nil, fmt.Errorf("shortcut key must be 1-9, got %d", key)
	}

	return uc.favoriteRepo.GetByShortcut(ctx, key)
}

// GetByURL finds a favorite by its URL.
func (uc *ManageFavoritesUseCase) GetByURL(ctx context.Context, url string) (*entity.Favorite, error) {
	log := logging.FromContext(ctx)
	log.Debug().Str("url", url).Msg("getting favorite by URL")

	return uc.favoriteRepo.FindByURL(ctx, url)
}

// GetAll retrieves all favorites.
func (uc *ManageFavoritesUseCase) GetAll(ctx context.Context) ([]*entity.Favorite, error) {
	log := logging.FromContext(ctx)
	log.Debug().Msg("getting all favorites")

	favs, err := uc.favoriteRepo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get favorites: %w", err)
	}

	log.Debug().Int("count", len(favs)).Msg("retrieved favorites")
	return favs, nil
}

// IsFavorite checks if a URL is favorited.
func (uc *ManageFavoritesUseCase) IsFavorite(ctx context.Context, url string) (bool, error) {
	fav, err := uc.favoriteRepo.FindByURL(ctx, url)
	if err != nil {
		return false, fmt.Errorf("failed to check favorite: %w", err)
	}
	return fav != nil, nil
}

// GetTree builds a complete hierarchical view of folders and favorites.
func (uc *ManageFavoritesUseCase) GetTree(ctx context.Context) (*entity.FavoriteTree, error) {
	log := logging.FromContext(ctx)
	log.Debug().Msg("building favorite tree")

	tree := entity.NewFavoriteTree()

	// Get all folders
	folders, err := uc.folderRepo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get folders: %w", err)
	}

	// Build folder map and organize by parent
	for _, folder := range folders {
		tree.FolderMap[folder.ID] = folder
		if folder.ParentID == nil {
			tree.RootFolders = append(tree.RootFolders, folder)
		} else {
			tree.ChildFolders[*folder.ParentID] = append(tree.ChildFolders[*folder.ParentID], folder)
		}
	}

	// Get all favorites
	favorites, err := uc.favoriteRepo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get favorites: %w", err)
	}

	// Organize favorites by folder
	for _, fav := range favorites {
		if fav.FolderID == nil {
			tree.RootFavorites = append(tree.RootFavorites, fav)
		} else {
			tree.ChildFavorites[*fav.FolderID] = append(tree.ChildFavorites[*fav.FolderID], fav)
		}
	}

	log.Debug().
		Int("folders", len(folders)).
		Int("favorites", len(favorites)).
		Msg("favorite tree built")

	return tree, nil
}

// CreateFolder creates a new folder.
func (uc *ManageFavoritesUseCase) CreateFolder(ctx context.Context, name string, parentID *entity.FolderID) (*entity.Folder, error) {
	log := logging.FromContext(ctx)
	log.Debug().Str("name", name).Msg("creating folder")

	folder := entity.NewFolder(name)
	folder.ParentID = parentID

	if err := uc.folderRepo.Save(ctx, folder); err != nil {
		return nil, fmt.Errorf("failed to create folder: %w", err)
	}

	log.Info().Str("name", name).Int64("id", int64(folder.ID)).Msg("folder created")
	return folder, nil
}

// DeleteFolder removes a folder. Favorites in the folder are moved to root.
func (uc *ManageFavoritesUseCase) DeleteFolder(ctx context.Context, id entity.FolderID) error {
	log := logging.FromContext(ctx)
	log.Debug().Int64("id", int64(id)).Msg("deleting folder")

	// Move favorites from this folder to root (or parent)
	folder, err := uc.folderRepo.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to find folder: %w", err)
	}
	if folder == nil {
		log.Debug().Int64("id", int64(id)).Msg("folder not found")
		return nil
	}

	// Get favorites in this folder
	favorites, err := uc.favoriteRepo.GetByFolder(ctx, &id)
	if err != nil {
		return fmt.Errorf("failed to get folder favorites: %w", err)
	}

	// Move favorites to parent folder (or root)
	for _, fav := range favorites {
		if setErr := uc.favoriteRepo.SetFolder(ctx, fav.ID, folder.ParentID); setErr != nil {
			log.Warn().
				Int64("favorite_id", int64(fav.ID)).
				Err(setErr).
				Msg("failed to move favorite from deleted folder")
		}
	}

	// Get child folders
	children, err := uc.folderRepo.GetChildren(ctx, &id)
	if err != nil {
		return fmt.Errorf("failed to get child folders: %w", err)
	}

	// Recursively delete child folders
	for _, child := range children {
		if err := uc.DeleteFolder(ctx, child.ID); err != nil {
			log.Warn().
				Int64("child_id", int64(child.ID)).
				Err(err).
				Msg("failed to delete child folder")
		}
	}

	// Delete the folder itself
	if err := uc.folderRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("failed to delete folder: %w", err)
	}

	log.Info().Int64("id", int64(id)).Msg("folder deleted")
	return nil
}

// GetAllFolders retrieves all folders.
func (uc *ManageFavoritesUseCase) GetAllFolders(ctx context.Context) ([]*entity.Folder, error) {
	log := logging.FromContext(ctx)
	log.Debug().Msg("getting all folders")

	return uc.folderRepo.GetAll(ctx)
}

// AddTag creates a new tag.
func (uc *ManageFavoritesUseCase) AddTag(ctx context.Context, name, color string) (*entity.Tag, error) {
	log := logging.FromContext(ctx)
	log.Debug().Str("name", name).Str("color", color).Msg("creating tag")

	// Check if tag already exists
	existing, err := uc.tagRepo.FindByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing tag: %w", err)
	}
	if existing != nil {
		log.Debug().Str("name", name).Msg("tag already exists")
		return existing, nil
	}

	tag := entity.NewTag(name)
	if color != "" {
		tag.Color = color
	}

	if err := uc.tagRepo.Save(ctx, tag); err != nil {
		return nil, fmt.Errorf("failed to create tag: %w", err)
	}

	log.Info().Str("name", name).Int64("id", int64(tag.ID)).Msg("tag created")
	return tag, nil
}

// DeleteTag removes a tag.
func (uc *ManageFavoritesUseCase) DeleteTag(ctx context.Context, id entity.TagID) error {
	log := logging.FromContext(ctx)
	log.Debug().Int64("id", int64(id)).Msg("deleting tag")

	if err := uc.tagRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("failed to delete tag: %w", err)
	}

	log.Info().Int64("id", int64(id)).Msg("tag deleted")
	return nil
}

// GetAllTags retrieves all tags.
func (uc *ManageFavoritesUseCase) GetAllTags(ctx context.Context) ([]*entity.Tag, error) {
	log := logging.FromContext(ctx)
	log.Debug().Msg("getting all tags")

	return uc.tagRepo.GetAll(ctx)
}

// TagFavorite assigns a tag to a favorite.
func (uc *ManageFavoritesUseCase) TagFavorite(ctx context.Context, favID entity.FavoriteID, tagID entity.TagID) error {
	log := logging.FromContext(ctx)
	log.Debug().
		Int64("favorite_id", int64(favID)).
		Int64("tag_id", int64(tagID)).
		Msg("tagging favorite")

	if err := uc.tagRepo.AssignToFavorite(ctx, tagID, favID); err != nil {
		return fmt.Errorf("failed to tag favorite: %w", err)
	}

	return nil
}

// UntagFavorite removes a tag from a favorite.
func (uc *ManageFavoritesUseCase) UntagFavorite(ctx context.Context, favID entity.FavoriteID, tagID entity.TagID) error {
	log := logging.FromContext(ctx)
	log.Debug().
		Int64("favorite_id", int64(favID)).
		Int64("tag_id", int64(tagID)).
		Msg("untagging favorite")

	if err := uc.tagRepo.RemoveFromFavorite(ctx, tagID, favID); err != nil {
		return fmt.Errorf("failed to untag favorite: %w", err)
	}

	return nil
}

// GetTagsForFavorite retrieves all tags for a favorite.
func (uc *ManageFavoritesUseCase) GetTagsForFavorite(ctx context.Context, favID entity.FavoriteID) ([]*entity.Tag, error) {
	log := logging.FromContext(ctx)
	log.Debug().Int64("favorite_id", int64(favID)).Msg("getting tags for favorite")

	return uc.tagRepo.GetForFavorite(ctx, favID)
}

// UpdateFolder updates a folder's name and icon.
func (uc *ManageFavoritesUseCase) UpdateFolder(ctx context.Context, id entity.FolderID, name, icon string) error {
	log := logging.FromContext(ctx)
	log.Debug().Int64("id", int64(id)).Str("name", name).Msg("updating folder")

	folder, err := uc.folderRepo.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to find folder: %w", err)
	}
	if folder == nil {
		return fmt.Errorf("folder %d not found", id)
	}

	if name != "" {
		folder.Name = name
	}
	folder.Icon = icon

	if err := uc.folderRepo.Save(ctx, folder); err != nil {
		return fmt.Errorf("failed to update folder: %w", err)
	}

	log.Info().Int64("id", int64(id)).Msg("folder updated")
	return nil
}

// UpdateTag updates a tag's name and color.
func (uc *ManageFavoritesUseCase) UpdateTag(ctx context.Context, id entity.TagID, name, color string) error {
	log := logging.FromContext(ctx)
	log.Debug().Int64("id", int64(id)).Str("name", name).Str("color", color).Msg("updating tag")

	tag, err := uc.tagRepo.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to find tag: %w", err)
	}
	if tag == nil {
		return fmt.Errorf("tag %d not found", id)
	}

	if name != "" {
		tag.Name = name
	}
	if color != "" {
		tag.Color = color
	}

	if err := uc.tagRepo.Save(ctx, tag); err != nil {
		return fmt.Errorf("failed to update tag: %w", err)
	}

	log.Info().Int64("id", int64(id)).Msg("tag updated")
	return nil
}
