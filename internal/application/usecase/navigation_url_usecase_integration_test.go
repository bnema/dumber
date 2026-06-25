package usecase

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
)

func TestManageTabsCreateResolvesExistingLocalInitialURL(t *testing.T) {
	ctx := context.Background()
	tabs := entity.NewTabList()
	tabsFile := filepath.Join(string(filepath.Separator), "tmp", "tabs.html")
	uc := NewManageTabsUseCase(
		sequentialIDGenerator("tab", "workspace", "pane"),
		WithManageTabsLocalPathResolver(fakeLocalPathResolver{paths: map[string]string{"tabs.html": tabsFile}}),
	)

	out, err := uc.Create(ctx, CreateTabInput{TabList: tabs, InitialURL: "tabs.html"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if got, want := out.Tab.Workspace.Root.Pane.URI, "file://"+tabsFile; got != want {
		t.Fatalf("created pane URI = %q, want %q", got, want)
	}
}

func TestManageTabsCreateWithPaneResolvesExistingLocalInitialURL(t *testing.T) {
	ctx := context.Background()
	tabs := entity.NewTabList()
	tabsFile := filepath.Join(string(filepath.Separator), "tmp", "provided.html")
	uc := NewManageTabsUseCase(
		sequentialIDGenerator("tab", "workspace"),
		WithManageTabsLocalPathResolver(fakeLocalPathResolver{paths: map[string]string{"provided.html": tabsFile}}),
	)
	pane := entity.NewPane("provided-pane")

	out, err := uc.CreateWithPane(ctx, CreateTabWithPaneInput{TabList: tabs, Pane: pane, InitialURL: "provided.html"})
	if err != nil {
		t.Fatalf("CreateWithPane returned error: %v", err)
	}
	if got, want := out.Tab.Workspace.Root.Pane.URI, "file://"+tabsFile; got != want {
		t.Fatalf("provided pane URI = %q, want %q", got, want)
	}
}

func TestManagePanesSplitResolvesExistingLocalInitialURL(t *testing.T) {
	ctx := context.Background()
	paneFile := filepath.Join(string(filepath.Separator), "tmp", "split.html")
	uc := NewManagePanesUseCase(
		sequentialIDGenerator("new-pane", "parent"),
		WithManagePanesLocalPathResolver(fakeLocalPathResolver{paths: map[string]string{"split.html": paneFile}}),
	)
	active := entity.NewPane("active-pane")
	activeNode := &entity.PaneNode{ID: "active-node", Pane: active}
	workspace := &entity.Workspace{ID: "workspace", Root: activeNode, ActivePaneID: active.ID}

	out, err := uc.Split(ctx, SplitPaneInput{
		Workspace:  workspace,
		TargetPane: activeNode,
		Direction:  SplitRight,
		InitialURL: "split.html",
	})
	if err != nil {
		t.Fatalf("Split returned error: %v", err)
	}
	if got, want := out.NewPaneNode.Pane.URI, "file://"+paneFile; got != want {
		t.Fatalf("split pane URI = %q, want %q", got, want)
	}
}

func sequentialIDGenerator(ids ...string) IDGenerator {
	idx := 0
	return func() string {
		if idx >= len(ids) {
			return "extra-id"
		}
		id := ids[idx]
		idx++
		return id
	}
}
