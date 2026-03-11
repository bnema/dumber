package usecase_test

import (
	"errors"
	"testing"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	repomocks "github.com/bnema/dumber/internal/domain/repository/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestManageFavoritesUseCase_GetAllURLs_ReturnsURLSet(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	folderRepo := repomocks.NewMockFolderRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	favorites := []*entity.Favorite{
		{ID: 1, URL: "https://example.com"},
		{ID: 2, URL: "https://github.com"},
		{ID: 3, URL: "https://google.com"},
	}

	favoriteRepo.EXPECT().GetAll(mock.Anything).Return(favorites, nil)

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, folderRepo, tagRepo)

	urls, err := uc.GetAllURLs(ctx)
	require.NoError(t, err)
	require.NotNil(t, urls)

	assert.Len(t, urls, 3)
	_, hasExample := urls["https://example.com"]
	_, hasGithub := urls["https://github.com"]
	_, hasGoogle := urls["https://google.com"]
	assert.True(t, hasExample)
	assert.True(t, hasGithub)
	assert.True(t, hasGoogle)
}

func TestManageFavoritesUseCase_GetAllURLs_ReturnsEmptySetWhenNoFavorites(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	folderRepo := repomocks.NewMockFolderRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	favoriteRepo.EXPECT().GetAll(mock.Anything).Return([]*entity.Favorite{}, nil)

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, folderRepo, tagRepo)

	urls, err := uc.GetAllURLs(ctx)
	require.NoError(t, err)
	require.NotNil(t, urls)
	assert.Empty(t, urls)
}

func TestManageFavoritesUseCase_GetAllURLs_ReturnsErrorOnRepoFailure(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	folderRepo := repomocks.NewMockFolderRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	favoriteRepo.EXPECT().GetAll(mock.Anything).Return(nil, errors.New("db connection failed"))

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, folderRepo, tagRepo)

	urls, err := uc.GetAllURLs(ctx)
	require.Error(t, err)
	assert.Nil(t, urls)
	assert.Contains(t, err.Error(), "failed to get favorites")
}

func TestManageFavoritesUseCase_Toggle_AddsWhenNotFavorite(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	folderRepo := repomocks.NewMockFolderRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	// URL is not a favorite
	favoriteRepo.EXPECT().FindByURL(mock.Anything, "https://example.com").Return(nil, nil)
	favoriteRepo.EXPECT().Save(mock.Anything, mock.MatchedBy(func(f *entity.Favorite) bool {
		return f.URL == "https://example.com" && f.Title == "Example"
	})).Return(nil)

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, folderRepo, tagRepo)

	result, err := uc.Toggle(ctx, "https://example.com", "Example")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, result.Added)
	assert.Equal(t, "https://example.com", result.URL)
	assert.Equal(t, "Example", result.Title)
	assert.Equal(t, "Favorite added", result.Message)
}

func TestManageFavoritesUseCase_Toggle_RemovesWhenAlreadyFavorite(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	folderRepo := repomocks.NewMockFolderRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	existingFav := &entity.Favorite{
		ID:    42,
		URL:   "https://example.com",
		Title: "Example Site",
	}

	// URL is already a favorite
	favoriteRepo.EXPECT().FindByURL(mock.Anything, "https://example.com").Return(existingFav, nil)
	favoriteRepo.EXPECT().Delete(mock.Anything, entity.FavoriteID(42)).Return(nil)

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, folderRepo, tagRepo)

	result, err := uc.Toggle(ctx, "https://example.com", "Example")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.False(t, result.Added)
	assert.Equal(t, "https://example.com", result.URL)
	assert.Equal(t, "Example Site", result.Title) // Uses existing title
	assert.Equal(t, "Favorite removed", result.Message)
}

func TestManageFavoritesUseCase_Toggle_EmptyURLReturnsError(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	folderRepo := repomocks.NewMockFolderRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, folderRepo, tagRepo)

	result, err := uc.Toggle(ctx, "", "Title")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "URL cannot be empty")
}

func TestManageFavoritesUseCase_Toggle_FindByURLErrorReturnsError(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	folderRepo := repomocks.NewMockFolderRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	favoriteRepo.EXPECT().FindByURL(mock.Anything, "https://example.com").Return(nil, errors.New("db error"))

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, folderRepo, tagRepo)

	result, err := uc.Toggle(ctx, "https://example.com", "Example")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to check existing favorite")
}

func TestManageFavoritesUseCase_Toggle_SaveErrorReturnsError(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	folderRepo := repomocks.NewMockFolderRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	favoriteRepo.EXPECT().FindByURL(mock.Anything, "https://example.com").Return(nil, nil)
	favoriteRepo.EXPECT().Save(mock.Anything, mock.Anything).Return(errors.New("db error"))

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, folderRepo, tagRepo)

	result, err := uc.Toggle(ctx, "https://example.com", "Example")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to add favorite")
}

func TestManageFavoritesUseCase_Toggle_DeleteErrorReturnsError(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	folderRepo := repomocks.NewMockFolderRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	existingFav := &entity.Favorite{ID: 42, URL: "https://example.com", Title: "Example"}
	favoriteRepo.EXPECT().FindByURL(mock.Anything, "https://example.com").Return(existingFav, nil)
	favoriteRepo.EXPECT().Delete(mock.Anything, entity.FavoriteID(42)).Return(errors.New("db error"))

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, folderRepo, tagRepo)

	result, err := uc.Toggle(ctx, "https://example.com", "Example")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to remove favorite")
}

func TestManageFavoritesUseCase_FilterForOmnibox_RanksPrefixBeforeContains(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	folderRepo := repomocks.NewMockFolderRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	favorites := []*entity.Favorite{
		{ID: 1, URL: "https://example.com/landing", Title: "GitHub docs mirror", Position: 0},
		{ID: 2, URL: "https://github.com/bnema/dumber", Title: "Project repo", Position: 1},
		{ID: 3, URL: "https://gitlab.com/team/project", Title: "GitLab", Position: 2},
	}
	favoriteRepo.EXPECT().GetAll(mock.Anything).Return(favorites, nil)

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, folderRepo, tagRepo)

	results, err := uc.FilterForOmnibox(ctx, "git")
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, "https://github.com/bnema/dumber", results[0].URL)
	assert.Equal(t, "https://gitlab.com/team/project", results[1].URL)
	assert.Equal(t, "https://example.com/landing", results[2].URL)
}

func TestManageFavoritesUseCase_FilterForOmnibox_EmptyQueryKeepsRepositoryOrder(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	folderRepo := repomocks.NewMockFolderRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	favorites := []*entity.Favorite{
		{ID: 1, URL: "https://first.example"},
		{ID: 2, URL: "https://second.example"},
	}
	favoriteRepo.EXPECT().GetAll(mock.Anything).Return(favorites, nil)

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, folderRepo, tagRepo)

	results, err := uc.FilterForOmnibox(ctx, "")
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "https://first.example", results[0].URL)
	assert.Equal(t, "https://second.example", results[1].URL)
}

func TestManageFavoritesUseCase_GetAllURLs_UsesCache(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	folderRepo := repomocks.NewMockFolderRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	favorites := []*entity.Favorite{
		{ID: 1, URL: "https://example.com"},
	}
	favoriteRepo.EXPECT().GetAll(mock.Anything).Return(favorites, nil).Once()

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, folderRepo, tagRepo)

	urls, err := uc.GetAllURLs(ctx)
	require.NoError(t, err)
	require.Contains(t, urls, "https://example.com")

	urls, err = uc.GetAllURLs(ctx)
	require.NoError(t, err)
	require.Contains(t, urls, "https://example.com")
}

func TestManageFavoritesUseCase_Toggle_InvalidatesCache(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	folderRepo := repomocks.NewMockFolderRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	initialFavorites := []*entity.Favorite{
		{ID: 1, URL: "https://cached.example"},
	}
	favoriteRepo.EXPECT().GetAll(mock.Anything).Return(initialFavorites, nil).Once()
	favoriteRepo.EXPECT().FindByURL(mock.Anything, "https://new.example").Return(nil, nil)
	favoriteRepo.EXPECT().Save(mock.Anything, mock.MatchedBy(func(f *entity.Favorite) bool {
		return f.URL == "https://new.example"
	})).Return(nil)
	updatedFavorites := []*entity.Favorite{
		{ID: 1, URL: "https://cached.example"},
		{ID: 2, URL: "https://new.example"},
	}
	favoriteRepo.EXPECT().GetAll(mock.Anything).Return(updatedFavorites, nil).Once()

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, folderRepo, tagRepo)

	urls, err := uc.GetAllURLs(ctx)
	require.NoError(t, err)
	require.Contains(t, urls, "https://cached.example")

	_, err = uc.Toggle(ctx, "https://new.example", "New")
	require.NoError(t, err)

	urls, err = uc.GetAllURLs(ctx)
	require.NoError(t, err)
	require.Contains(t, urls, "https://new.example")
}

func TestManageFavoritesUseCase_DeleteFolder_InvalidatesFavoritesCache(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	folderRepo := repomocks.NewMockFolderRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	folderID := entity.FolderID(9)
	initialFolderID := folderID
	cachedFavorites := []*entity.Favorite{
		{ID: 1, URL: "https://example.com", FolderID: &initialFolderID},
	}
	updatedFavorites := []*entity.Favorite{
		{ID: 1, URL: "https://example.com", FolderID: nil},
	}

	favoriteRepo.EXPECT().GetAll(mock.Anything).Return(cachedFavorites, nil).Once()
	folderRepo.EXPECT().FindByID(mock.Anything, folderID).Return(&entity.Folder{ID: folderID}, nil).Once()
	favoriteRepo.EXPECT().GetByFolder(mock.Anything, &folderID).Return(cachedFavorites, nil).Once()
	favoriteRepo.EXPECT().SetFolder(mock.Anything, entity.FavoriteID(1), (*entity.FolderID)(nil)).Return(nil).Once()
	folderRepo.EXPECT().GetChildren(mock.Anything, &folderID).Return([]*entity.Folder{}, nil).Once()
	folderRepo.EXPECT().Delete(mock.Anything, folderID).Return(nil).Once()
	favoriteRepo.EXPECT().GetAll(mock.Anything).Return(updatedFavorites, nil).Once()

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, folderRepo, tagRepo)

	favoritesBefore, err := uc.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, favoritesBefore, 1)
	require.NotNil(t, favoritesBefore[0].FolderID)

	err = uc.DeleteFolder(ctx, folderID)
	require.NoError(t, err)

	favoritesAfter, err := uc.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, favoritesAfter, 1)
	assert.Nil(t, favoritesAfter[0].FolderID)
}
