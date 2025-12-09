package sqlite

import (
	"context"
	"database/sql"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite/sqlc"
	"github.com/bnema/dumber/internal/logging"
)

// ==================== Favorite Repository ====================

type favoriteRepo struct {
	queries *sqlc.Queries
}

// NewFavoriteRepository creates a new SQLite-backed favorite repository.
func NewFavoriteRepository(db *sql.DB) repository.FavoriteRepository {
	return &favoriteRepo{queries: sqlc.New(db)}
}

func (r *favoriteRepo) Save(ctx context.Context, fav *entity.Favorite) error {
	log := logging.FromContext(ctx)
	log.Debug().Str("url", fav.URL).Msg("saving favorite")

	var folderID sql.NullInt64
	if fav.FolderID != nil {
		folderID = sql.NullInt64{Int64: int64(*fav.FolderID), Valid: true}
	}

	row, err := r.queries.CreateFavorite(ctx, sqlc.CreateFavoriteParams{
		Url:        fav.URL,
		Title:      sql.NullString{String: fav.Title, Valid: fav.Title != ""},
		FaviconUrl: sql.NullString{String: fav.FaviconURL, Valid: fav.FaviconURL != ""},
		FolderID:   folderID,
	})
	if err != nil {
		return err
	}
	fav.ID = entity.FavoriteID(row.ID)
	return nil
}

func (r *favoriteRepo) FindByID(ctx context.Context, id entity.FavoriteID) (*entity.Favorite, error) {
	row, err := r.queries.GetFavoriteByID(ctx, int64(id))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return favoriteFromRow(row), nil
}

func (r *favoriteRepo) FindByURL(ctx context.Context, url string) (*entity.Favorite, error) {
	row, err := r.queries.GetFavoriteByURL(ctx, url)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return favoriteFromRow(row), nil
}

func (r *favoriteRepo) GetAll(ctx context.Context) ([]*entity.Favorite, error) {
	rows, err := r.queries.GetAllFavorites(ctx)
	if err != nil {
		return nil, err
	}
	return favoritesFromRows(rows), nil
}

func (r *favoriteRepo) GetByFolder(ctx context.Context, folderID *entity.FolderID) ([]*entity.Favorite, error) {
	var rows []sqlc.Favorite
	var err error

	if folderID == nil {
		rows, err = r.queries.GetFavoritesWithoutFolder(ctx)
	} else {
		rows, err = r.queries.GetFavoritesByFolder(ctx, sql.NullInt64{Int64: int64(*folderID), Valid: true})
	}
	if err != nil {
		return nil, err
	}
	return favoritesFromRows(rows), nil
}

func (r *favoriteRepo) GetByShortcut(ctx context.Context, key int) (*entity.Favorite, error) {
	row, err := r.queries.GetFavoriteByShortcut(ctx, sql.NullInt64{Int64: int64(key), Valid: true})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return favoriteFromRow(row), nil
}

func (r *favoriteRepo) UpdatePosition(ctx context.Context, id entity.FavoriteID, position int) error {
	return r.queries.UpdateFavoritePosition(ctx, sqlc.UpdateFavoritePositionParams{
		Position: int64(position),
		ID:       int64(id),
	})
}

func (r *favoriteRepo) SetFolder(ctx context.Context, id entity.FavoriteID, folderID *entity.FolderID) error {
	var fid sql.NullInt64
	if folderID != nil {
		fid = sql.NullInt64{Int64: int64(*folderID), Valid: true}
	}
	return r.queries.SetFavoriteFolder(ctx, sqlc.SetFavoriteFolderParams{
		FolderID: fid,
		ID:       int64(id),
	})
}

func (r *favoriteRepo) SetShortcut(ctx context.Context, id entity.FavoriteID, key *int) error {
	var k sql.NullInt64
	if key != nil {
		k = sql.NullInt64{Int64: int64(*key), Valid: true}
	}
	return r.queries.SetFavoriteShortcut(ctx, sqlc.SetFavoriteShortcutParams{
		ShortcutKey: k,
		ID:          int64(id),
	})
}

func (r *favoriteRepo) Delete(ctx context.Context, id entity.FavoriteID) error {
	return r.queries.DeleteFavorite(ctx, int64(id))
}

func favoriteFromRow(row sqlc.Favorite) *entity.Favorite {
	fav := &entity.Favorite{
		ID:         entity.FavoriteID(row.ID),
		URL:        row.Url,
		Title:      row.Title.String,
		FaviconURL: row.FaviconUrl.String,
		Position:   int(row.Position),
		CreatedAt:  row.CreatedAt.Time,
		UpdatedAt:  row.UpdatedAt.Time,
	}
	if row.FolderID.Valid {
		fid := entity.FolderID(row.FolderID.Int64)
		fav.FolderID = &fid
	}
	if row.ShortcutKey.Valid {
		key := int(row.ShortcutKey.Int64)
		fav.ShortcutKey = &key
	}
	return fav
}

func favoritesFromRows(rows []sqlc.Favorite) []*entity.Favorite {
	favorites := make([]*entity.Favorite, len(rows))
	for i, row := range rows {
		favorites[i] = favoriteFromRow(row)
	}
	return favorites
}

// ==================== Folder Repository ====================

type folderRepo struct {
	queries *sqlc.Queries
}

// NewFolderRepository creates a new SQLite-backed folder repository.
func NewFolderRepository(db *sql.DB) repository.FolderRepository {
	return &folderRepo{queries: sqlc.New(db)}
}

func (r *folderRepo) Save(ctx context.Context, folder *entity.Folder) error {
	log := logging.FromContext(ctx)
	log.Debug().Str("name", folder.Name).Msg("saving folder")

	var parentID sql.NullInt64
	if folder.ParentID != nil {
		parentID = sql.NullInt64{Int64: int64(*folder.ParentID), Valid: true}
	}

	row, err := r.queries.CreateFolder(ctx, sqlc.CreateFolderParams{
		Name:     folder.Name,
		Icon:     sql.NullString{String: folder.Icon, Valid: folder.Icon != ""},
		ParentID: parentID,
	})
	if err != nil {
		return err
	}
	folder.ID = entity.FolderID(row.ID)
	return nil
}

func (r *folderRepo) FindByID(ctx context.Context, id entity.FolderID) (*entity.Folder, error) {
	row, err := r.queries.GetFolderByID(ctx, int64(id))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return folderFromRow(row), nil
}

func (r *folderRepo) GetAll(ctx context.Context) ([]*entity.Folder, error) {
	rows, err := r.queries.GetAllFolders(ctx)
	if err != nil {
		return nil, err
	}
	return foldersFromRows(rows), nil
}

func (r *folderRepo) GetChildren(ctx context.Context, parentID *entity.FolderID) ([]*entity.Folder, error) {
	var rows []sqlc.FavoriteFolder
	var err error

	if parentID == nil {
		rows, err = r.queries.GetRootFolders(ctx)
	} else {
		rows, err = r.queries.GetChildFolders(ctx, sql.NullInt64{Int64: int64(*parentID), Valid: true})
	}
	if err != nil {
		return nil, err
	}
	return foldersFromRows(rows), nil
}

func (r *folderRepo) UpdatePosition(ctx context.Context, id entity.FolderID, position int) error {
	return r.queries.UpdateFolderPosition(ctx, sqlc.UpdateFolderPositionParams{
		Position: int64(position),
		ID:       int64(id),
	})
}

func (r *folderRepo) Delete(ctx context.Context, id entity.FolderID) error {
	return r.queries.DeleteFolder(ctx, int64(id))
}

func folderFromRow(row sqlc.FavoriteFolder) *entity.Folder {
	folder := &entity.Folder{
		ID:        entity.FolderID(row.ID),
		Name:      row.Name,
		Icon:      row.Icon.String,
		Position:  int(row.Position),
		CreatedAt: row.CreatedAt.Time,
	}
	if row.ParentID.Valid {
		pid := entity.FolderID(row.ParentID.Int64)
		folder.ParentID = &pid
	}
	return folder
}

func foldersFromRows(rows []sqlc.FavoriteFolder) []*entity.Folder {
	folders := make([]*entity.Folder, len(rows))
	for i, row := range rows {
		folders[i] = folderFromRow(row)
	}
	return folders
}

// ==================== Tag Repository ====================

type tagRepo struct {
	queries *sqlc.Queries
}

// NewTagRepository creates a new SQLite-backed tag repository.
func NewTagRepository(db *sql.DB) repository.TagRepository {
	return &tagRepo{queries: sqlc.New(db)}
}

func (r *tagRepo) Save(ctx context.Context, tag *entity.Tag) error {
	log := logging.FromContext(ctx)
	log.Debug().Str("name", tag.Name).Msg("saving tag")

	row, err := r.queries.CreateTag(ctx, sqlc.CreateTagParams{
		Name:  tag.Name,
		Color: tag.Color,
	})
	if err != nil {
		return err
	}
	tag.ID = entity.TagID(row.ID)
	return nil
}

func (r *tagRepo) FindByID(ctx context.Context, id entity.TagID) (*entity.Tag, error) {
	row, err := r.queries.GetTagByID(ctx, int64(id))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return tagFromRow(row), nil
}

func (r *tagRepo) FindByName(ctx context.Context, name string) (*entity.Tag, error) {
	row, err := r.queries.GetTagByName(ctx, name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return tagFromRow(row), nil
}

func (r *tagRepo) GetAll(ctx context.Context) ([]*entity.Tag, error) {
	rows, err := r.queries.GetAllTags(ctx)
	if err != nil {
		return nil, err
	}
	return tagsFromRows(rows), nil
}

func (r *tagRepo) AssignToFavorite(ctx context.Context, tagID entity.TagID, favID entity.FavoriteID) error {
	return r.queries.AssignTagToFavorite(ctx, sqlc.AssignTagToFavoriteParams{
		FavoriteID: int64(favID),
		TagID:      int64(tagID),
	})
}

func (r *tagRepo) RemoveFromFavorite(ctx context.Context, tagID entity.TagID, favID entity.FavoriteID) error {
	return r.queries.RemoveTagFromFavorite(ctx, sqlc.RemoveTagFromFavoriteParams{
		FavoriteID: int64(favID),
		TagID:      int64(tagID),
	})
}

func (r *tagRepo) GetForFavorite(ctx context.Context, favID entity.FavoriteID) ([]*entity.Tag, error) {
	rows, err := r.queries.GetTagsForFavorite(ctx, int64(favID))
	if err != nil {
		return nil, err
	}
	return tagsFromRows(rows), nil
}

func (r *tagRepo) Delete(ctx context.Context, id entity.TagID) error {
	return r.queries.DeleteTag(ctx, int64(id))
}

func tagFromRow(row sqlc.FavoriteTag) *entity.Tag {
	return &entity.Tag{
		ID:        entity.TagID(row.ID),
		Name:      row.Name,
		Color:     row.Color,
		CreatedAt: row.CreatedAt.Time,
	}
}

func tagsFromRows(rows []sqlc.FavoriteTag) []*entity.Tag {
	tags := make([]*entity.Tag, len(rows))
	for i, row := range rows {
		tags[i] = tagFromRow(row)
	}
	return tags
}
