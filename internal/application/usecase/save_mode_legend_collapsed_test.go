package usecase

import (
	"context"
	"errors"
	"testing"

	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/stretchr/testify/require"
)

func TestSaveModeLegendCollapsedUseCase_PersistsValue(t *testing.T) {
	ctx := context.Background()
	for _, collapsed := range []bool{true, false} {
		saver := portmocks.NewMockModeLegendPreferencesSaver(t)
		saver.EXPECT().SaveModeLegendCollapsed(ctx, collapsed).Return(nil).Once()

		require.NoError(t, NewSaveModeLegendCollapsedUseCase(saver).Execute(ctx, collapsed))
	}
}

func TestSaveModeLegendCollapsedUseCase_ReturnsSaverError(t *testing.T) {
	ctx := context.Background()
	saver := portmocks.NewMockModeLegendPreferencesSaver(t)
	saver.EXPECT().SaveModeLegendCollapsed(ctx, true).Return(errors.New("disk full")).Once()

	require.ErrorContains(t, NewSaveModeLegendCollapsedUseCase(saver).Execute(ctx, true), "disk full")
}

func TestSaveModeLegendCollapsedUseCase_NilSaverReturnsError(t *testing.T) {
	require.Error(t, NewSaveModeLegendCollapsedUseCase(nil).Execute(context.Background(), true))
	var uc *SaveModeLegendCollapsedUseCase
	require.Error(t, uc.Execute(context.Background(), true))
}
