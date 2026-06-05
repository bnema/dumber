package usecase

import (
	"fmt"

	"github.com/bnema/dumber/internal/domain/entity"
)

// ExtractPaneToTabListUseCase moves a pane from a source tab list into a new
// tab in a target tab list. It is pure domain manipulation and has no
// knowledge of browser windows or UI objects.
type ExtractPaneToTabListUseCase struct {
	idGenerator IDGenerator
}

func NewExtractPaneToTabListUseCase(idGenerator IDGenerator) *ExtractPaneToTabListUseCase {
	return &ExtractPaneToTabListUseCase{idGenerator: idGenerator}
}

type ExtractPaneToTabListInput struct {
	SourceTabs   *entity.TabList
	SourceTabID  entity.TabID
	SourcePaneID entity.PaneID
	TargetTabs   *entity.TabList
}

type ExtractPaneToTabListOutput struct {
	NewTab          *entity.Tab
	MovedPaneNode   *entity.PaneNode
	SourceTabClosed bool
}

func (uc *ExtractPaneToTabListUseCase) Execute(input ExtractPaneToTabListInput) (*ExtractPaneToTabListOutput, error) {
	if err := validateExtractPaneToTabListInput(uc, input); err != nil {
		return nil, err
	}

	sourceTab, err := findSourceTab(input.SourceTabs, input.SourceTabID)
	if err != nil {
		return nil, err
	}

	movedPane, sourceNode, err := findSourcePane(sourceTab.Workspace, input.SourcePaneID)
	if err != nil {
		return nil, err
	}

	if err := detachPaneFromWorkspace(sourceTab.Workspace, sourceNode); err != nil {
		return nil, err
	}

	sourceTabClosed := closeSourceTabIfEmpty(input.SourceTabs, sourceTab)

	tabID := entity.TabID(uc.idGenerator())
	workspaceID := entity.WorkspaceID(uc.idGenerator())
	newTab := entity.NewTab(tabID, workspaceID, movedPane)
	input.TargetTabs.Add(newTab)

	return &ExtractPaneToTabListOutput{
		NewTab:          newTab,
		MovedPaneNode:   newTab.Workspace.Root,
		SourceTabClosed: sourceTabClosed,
	}, nil
}

func validateExtractPaneToTabListInput(uc *ExtractPaneToTabListUseCase, input ExtractPaneToTabListInput) error {
	if uc == nil {
		return fmt.Errorf("extract pane to tab list use case is nil")
	}
	if input.SourceTabs == nil {
		return fmt.Errorf("source tabs are required")
	}
	if input.TargetTabs == nil {
		return fmt.Errorf("target tabs are required")
	}
	if input.SourceTabID == "" {
		return fmt.Errorf("source tab id is required")
	}
	if input.SourcePaneID == "" {
		return fmt.Errorf("source pane id is required")
	}
	if uc.idGenerator == nil {
		return fmt.Errorf("id generator is required")
	}
	return nil
}
