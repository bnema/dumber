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
