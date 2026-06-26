package usecase

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
)

type fakeLocalPathResolver struct {
	paths map[string]string
	err   error
}

func (r fakeLocalPathResolver) ResolveExistingPath(_ context.Context, input string) (string, bool, error) {
	if r.err != nil {
		return "", false, r.err
	}
	abs, ok := r.paths[input]
	return abs, ok, nil
}

func TestManageTabsCreateResolvesExistingLocalInitialURL(t *testing.T) {
	ctx := context.Background()
	tabs := entity.NewTabList()
	tabsFile := filepath.Join(string(filepath.Separator), "tmp", "tabs.html")
	uc := NewManageTabsUseCase(
		sequentialIDGenerator("tab", "workspace", "pane"),
		fakeLocalPathResolver{paths: map[string]string{"tabs.html": tabsFile}},
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
		fakeLocalPathResolver{paths: map[string]string{"provided.html": tabsFile}},
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
		fakeLocalPathResolver{paths: map[string]string{"split.html": paneFile}},
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

func TestManagePanesCreateStackResolvesExistingLocalInitialURL(t *testing.T) {
	ctx := context.Background()
	stackFile := filepath.Join(string(filepath.Separator), "tmp", "stack.html")
	uc := NewManagePanesUseCase(
		sequentialIDGenerator("new-pane"),
		fakeLocalPathResolver{paths: map[string]string{"stack.html": stackFile}},
	)
	active := entity.NewPane("active-pane")
	activeNode := &entity.PaneNode{ID: "active-node", Pane: active}
	workspace := &entity.Workspace{ID: "workspace", Root: activeNode, ActivePaneID: active.ID}

	out, err := uc.CreateStack(ctx, workspace, activeNode, "stack.html")
	if err != nil {
		t.Fatalf("CreateStack returned error: %v", err)
	}
	if got, want := out.NewPaneNode.Pane.URI, "file://"+stackFile; got != want {
		t.Fatalf("stack pane URI = %q, want %q", got, want)
	}
}

func TestManagePanesAddToStackResolvesExistingLocalInitialURL(t *testing.T) {
	ctx := context.Background()
	stackFile := filepath.Join(string(filepath.Separator), "tmp", "added.html")
	uc := NewManagePanesUseCase(
		sequentialIDGenerator("added-pane"),
		fakeLocalPathResolver{paths: map[string]string{"added.html": stackFile}},
	)
	active := entity.NewPane("active-pane")
	activeNode := &entity.PaneNode{ID: "active-node", Pane: active}
	stackNode := &entity.PaneNode{
		ID:               "stack",
		IsStacked:        true,
		ActiveStackIndex: 0,
		Children:         []*entity.PaneNode{activeNode},
	}
	activeNode.Parent = stackNode
	workspace := &entity.Workspace{ID: "workspace", Root: stackNode, ActivePaneID: active.ID}

	out, err := uc.AddToStack(ctx, workspace, stackNode, nil, "added.html")
	if err != nil {
		t.Fatalf("AddToStack returned error: %v", err)
	}
	if got, want := out.NewPaneNode.Pane.URI, "file://"+stackFile; got != want {
		t.Fatalf("added stack pane URI = %q, want %q", got, want)
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
