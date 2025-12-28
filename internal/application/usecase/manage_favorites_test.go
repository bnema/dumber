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
