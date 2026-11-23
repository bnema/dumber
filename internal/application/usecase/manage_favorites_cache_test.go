package usecase

import (
	"context"
	"testing"
	"time"

	"bnema.dev/dumber/internal/domain/entity"
	repomocks "bnema.dev/dumber/internal/domain/repository/mocks"
	"bnema.dev/dumber/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManageFavoritesUseCase_GetAllDoesNotOverwriteFreshCache(t *testing.T) {
	logger := logging.NewFromConfigValues("debug", "console")
	ctx := logging.WithContext(context.Background(), logger)

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	fetchedFavorites := []*entity.Favorite{
		{ID: 1, URL: "https://stale.example"},
	}
	refreshedFavorites := []*entity.Favorite{
		{ID: 2, URL: "https://fresh.example"},
	}

	uc := NewManageFavoritesUseCase(favoriteRepo, tagRepo)
	favoriteRepo.EXPECT().GetAll(ctx).Run(func(_ context.Context) {
		uc.cacheMu.Lock()
		uc.favoritesCache = refreshedFavorites
		uc.favoriteURLsCache = buildFavoriteURLSet(refreshedFavorites)
		uc.cacheTime = time.Now()
		uc.cacheMu.Unlock()
	}).Return(fetchedFavorites, nil).Once()

	favorites, err := uc.GetAll(ctx)
	require.NoError(t, err)
	assert.Equal(t, refreshedFavorites, favorites)

	uc.cacheMu.Lock()
	assert.Equal(t, refreshedFavorites, uc.favoritesCache)
	assert.Contains(t, uc.favoriteURLsCache, "https://fresh.example")
	assert.NotContains(t, uc.favoriteURLsCache, "https://stale.example")
	uc.cacheMu.Unlock()
}
