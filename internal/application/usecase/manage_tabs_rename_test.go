package usecase

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManageTabsUseCase_Rename(t *testing.T) {
	ctx := context.Background()
	uc := NewManageTabsUseCase(sequentialIDGenerator(), fakeLocalPathResolver{})

	tabs := entity.NewTabList()
	tabs.Add(entity.NewTab("tab", "workspace", &entity.Pane{ID: "pane"}))

	require.NoError(t, uc.Rename(ctx, tabs, "tab", "Research"))
	assert.Equal(t, "Research", tabs.Find("tab").Title())

	require.Error(t, uc.Rename(ctx, tabs, "missing", "Other"))
	require.Error(t, uc.Rename(ctx, nil, "tab", "Other"))
	assert.Equal(t, "Research", tabs.Find("tab").Title())
}
