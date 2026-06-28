package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite/sqlc"
	"github.com/bnema/dumber/internal/logging"
)

// ==================== Favorite Repository ====================

const favoriteTagHydrationBatchSize = 900

type favoriteRepo struct {
	queries *sqlc.Queries
}

// NewFavoriteRepository creates a new SQLite-backed favorite repository.
func NewFavoriteRepository(db *sql.DB) repository.FavoriteRepository {
	return &favoriteRepo{queries: sqlc.New(db)}
}

func (r *favoriteRepo) Save(ctx context.Context, fav *entity.Favorite) error {
	log := logging.FromContext(ctx)
	log.Debug().Str("url", fav.URL).Int64("id", int64(fav.ID)).Msg("saving favorite")

	if fav.ID > 0 {
		shortcutKey := sql.NullInt64{}
		if fav.ShortcutKey != nil {
			shortcutKey = sql.NullInt64{Int64: int64(*fav.ShortcutKey), Valid: true}
		}
		return r.queries.UpdateFavorite(ctx, sqlc.UpdateFavoriteParams{
			Title:       sql.NullString{String: fav.Title, Valid: fav.Title != ""},
			FaviconUrl:  sql.NullString{String: fav.FaviconURL, Valid: fav.FaviconURL != ""},
			ShortcutKey: shortcutKey,
			ID:          int64(fav.ID),
		})
	}

	row, err := r.queries.CreateFavorite(ctx, sqlc.CreateFavoriteParams{
		Url:        fav.URL,
		Title:      sql.NullString{String: fav.Title, Valid: fav.Title != ""},
		FaviconUrl: sql.NullString{String: fav.FaviconURL, Valid: fav.FaviconURL != ""},
	})
	if err != nil {
		return err
	}
	favoriteFromRow(favoriteRowFromCreate(row), fav)
	return nil
}

func (r *favoriteRepo) FindByID(ctx context.Context, id entity.FavoriteID) (*entity.Favorite, error) {
	row, err := r.queries.GetFavoriteByID(ctx, int64(id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	fav := favoriteFromRow(favoriteRowFromGetByID(row), nil)
	return fav, r.hydrateTags(ctx, []*entity.Favorite{fav})
}

func (r *favoriteRepo) FindByURL(ctx context.Context, url string) (*entity.Favorite, error) {
	row, err := r.queries.GetFavoriteByURL(ctx, url)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	fav := favoriteFromRow(favoriteRowFromGetByURL(row), nil)
	return fav, r.hydrateTags(ctx, []*entity.Favorite{fav})
}

func (r *favoriteRepo) GetAll(ctx context.Context) ([]*entity.Favorite, error) {
	rows, err := r.queries.GetAllFavorites(ctx)
	if err != nil {
		return nil, err
	}
	favorites := favoritesFromAllRows(rows)
	return favorites, r.hydrateTags(ctx, favorites)
}

func (r *favoriteRepo) GetByTag(ctx context.Context, tagID entity.TagID) ([]*entity.Favorite, error) {
	rows, err := r.queries.GetFavoritesByTag(ctx, int64(tagID))
	if err != nil {
		return nil, err
	}
	favorites := favoritesFromTagRows(rows)
	return favorites, r.hydrateTags(ctx, favorites)
}

func (r *favoriteRepo) GetByShortcut(ctx context.Context, key int) (*entity.Favorite, error) {
	row, err := r.queries.GetFavoriteByShortcut(ctx, sql.NullInt64{Int64: int64(key), Valid: true})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	fav := favoriteFromRow(favoriteRowFromGetByShortcut(row), nil)
	return fav, r.hydrateTags(ctx, []*entity.Favorite{fav})
}

func (r *favoriteRepo) UpdatePosition(ctx context.Context, id entity.FavoriteID, position int) error {
	return r.queries.UpdateFavoritePosition(ctx, sqlc.UpdateFavoritePositionParams{
		Position: int64(position),
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

func (r *favoriteRepo) hydrateTags(ctx context.Context, favorites []*entity.Favorite) error {
	if len(favorites) == 0 {
		return nil
	}

	ids := make([]int64, 0, len(favorites))
	byID := make(map[entity.FavoriteID]*entity.Favorite, len(favorites))
	for _, fav := range favorites {
		if fav == nil {
			continue
		}
		fav.Tags = []entity.Tag{}
		ids = append(ids, int64(fav.ID))
		byID[fav.ID] = fav
	}
	if len(ids) == 0 {
		return nil
	}

	for start := 0; start < len(ids); start += favoriteTagHydrationBatchSize {
		end := start + favoriteTagHydrationBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		rows, err := r.queries.GetTagsForFavorites(ctx, ids[start:end])
		if err != nil {
			return err
		}
		for _, row := range rows {
			fav := byID[entity.FavoriteID(row.FavoriteID)]
			if fav == nil {
				continue
			}
			fav.Tags = append(fav.Tags, *tagFromRow(row.FavoriteTag))
		}
	}
	return nil
}

type favoriteRow struct {
	ID          int64
	URL         string
	Title       sql.NullString
	FaviconURL  sql.NullString
	ShortcutKey sql.NullInt64
	Position    int64
	CreatedAt   sql.NullTime
	UpdatedAt   sql.NullTime
}

func favoriteFromRow(row favoriteRow, target *entity.Favorite) *entity.Favorite {
	fav := target
	if fav == nil {
		fav = &entity.Favorite{}
	}
	fav.ID = entity.FavoriteID(row.ID)
	fav.URL = row.URL
	fav.Title = row.Title.String
	fav.FaviconURL = row.FaviconURL.String
	fav.Position = int(row.Position)
	fav.CreatedAt = row.CreatedAt.Time
	fav.UpdatedAt = row.UpdatedAt.Time
	fav.Tags = nil
	if row.ShortcutKey.Valid {
		key := int(row.ShortcutKey.Int64)
		fav.ShortcutKey = &key
	} else {
		fav.ShortcutKey = nil
	}
	return fav
}

func favoriteRowFromCreate(row sqlc.CreateFavoriteRow) favoriteRow {
	return favoriteRow{
		ID: row.ID, URL: row.Url, Title: row.Title, FaviconURL: row.FaviconUrl,
		ShortcutKey: row.ShortcutKey, Position: row.Position, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func favoriteRowFromGetByID(row sqlc.GetFavoriteByIDRow) favoriteRow {
	return favoriteRow{
		ID: row.ID, URL: row.Url, Title: row.Title, FaviconURL: row.FaviconUrl,
		ShortcutKey: row.ShortcutKey, Position: row.Position, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func favoriteRowFromGetByURL(row sqlc.GetFavoriteByURLRow) favoriteRow {
	return favoriteRow{
		ID: row.ID, URL: row.Url, Title: row.Title, FaviconURL: row.FaviconUrl,
		ShortcutKey: row.ShortcutKey, Position: row.Position, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func favoriteRowFromGetByShortcut(row sqlc.GetFavoriteByShortcutRow) favoriteRow {
	return favoriteRow{
		ID: row.ID, URL: row.Url, Title: row.Title, FaviconURL: row.FaviconUrl,
		ShortcutKey: row.ShortcutKey, Position: row.Position, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func favoritesFromAllRows(rows []sqlc.GetAllFavoritesRow) []*entity.Favorite {
	favorites := make([]*entity.Favorite, len(rows))
	for i := range rows {
		row := rows[i]
		favorites[i] = favoriteFromRow(favoriteRow{
			ID: row.ID, URL: row.Url, Title: row.Title, FaviconURL: row.FaviconUrl,
			ShortcutKey: row.ShortcutKey, Position: row.Position, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}, nil)
	}
	return favorites
}

func favoritesFromTagRows(rows []sqlc.GetFavoritesByTagRow) []*entity.Favorite {
	favorites := make([]*entity.Favorite, len(rows))
	for i := range rows {
		row := rows[i]
		favorites[i] = favoriteFromRow(favoriteRow{
			ID: row.ID, URL: row.Url, Title: row.Title, FaviconURL: row.FaviconUrl,
			ShortcutKey: row.ShortcutKey, Position: row.Position, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}, nil)
	}
	return favorites
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
	log.Debug().Str("name", tag.Name).Int64("id", int64(tag.ID)).Msg("saving tag")

	if tag.ID > 0 {
		return r.queries.UpdateTag(ctx, sqlc.UpdateTagParams{
			Name:  tag.Name,
			Color: tag.Color,
			ID:    int64(tag.ID),
		})
	}

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
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return tagFromRow(row), nil
}

func (r *tagRepo) FindByName(ctx context.Context, name string) (*entity.Tag, error) {
	row, err := r.queries.GetTagByName(ctx, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
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
