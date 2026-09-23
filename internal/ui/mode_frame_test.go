package ui

import (
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/dumber/internal/ui/layout/mocks"
	"github.com/stretchr/testify/require"
)

func TestModeFrameTargets(t *testing.T) {
	f := newVimModeTestFixture(t)
	tests := []struct {
		mode input.Mode
		want layout.Widget
	}{
		{input.ModeVim, f.pv1A.Widget()},
		{input.ModePane, f.pv1A.Widget()},
		{input.ModeResize, f.pv1A.Widget()},
		{input.ModeTab, f.app.workspaceViews["tab-1"].Widget()},
		{input.ModeSession, f.app.workspaceViews["tab-1"].Widget()},
		{input.ModeNormal, nil},
	}
	for _, tt := range tests {
		t.Run(tt.mode.String(), func(t *testing.T) {
			require.Equal(t, tt.want, f.app.modeFrameTarget(f.bw1, tt.mode))
		})
	}
	f.bw1.tabs.ActiveTabID = "tab-missing"
	require.Nil(t, f.app.modeFrameTarget(f.bw1, input.ModePane))
}

func TestModeFrameSetTargetRemovesPreviousBorder(t *testing.T) {
	first := mocks.NewMockWidget(t)
	second := mocks.NewMockWidget(t)
	f := &modeFrame{}
	first.EXPECT().AddCssClass("pane-mode-active").Once()
	f.setTarget(first, "pane-mode-active")
	first.EXPECT().RemoveCssClass("pane-mode-active").Once()
	second.EXPECT().AddCssClass("resize-mode-active").Once()
	f.setTarget(second, "resize-mode-active")
	second.EXPECT().RemoveCssClass("resize-mode-active").Once()
	f.setTarget(nil, "")
}

func TestModeFrameVimPulseAlternatesOnTarget(t *testing.T) {
	target := mocks.NewMockWidget(t)
	f := &modeFrame{mode: input.ModeVim, target: target, frameClass: "vim-mode-active"}
	for _, pulse := range []struct {
		fast  bool
		class string
		cycle string
	}{
		{false, "vim-mode-pulse", "vim-mode-pulse-cycle-a"},
		{true, "vim-mode-pulse-fast", "vim-mode-pulse-cycle-b"},
	} {
		for _, class := range []string{"vim-mode-pulse", "vim-mode-pulse-fast", "vim-mode-pulse-cycle-a", "vim-mode-pulse-cycle-b"} {
			target.EXPECT().RemoveCssClass(class).Once()
		}
		target.EXPECT().AddCssClass(pulse.class).Once()
		target.EXPECT().AddCssClass(pulse.cycle).Once()
		f.pulse(pulse.fast)
	}
	target.EXPECT().RemoveCssClass("vim-mode-active").Once()
	for _, class := range []string{"vim-mode-pulse", "vim-mode-pulse-fast", "vim-mode-pulse-cycle-a", "vim-mode-pulse-cycle-b"} {
		target.EXPECT().RemoveCssClass(class).Once()
	}
	f.setTarget(nil, "")
}

func TestModeLegendActionsUseActiveConfiguration(t *testing.T) {
	workspace := entity.WorkspaceConfig{
		PaneMode:   entity.PaneModeConfig{Actions: map[string]entity.ActionBinding{"split-right": {Keys: []string{"r"}}}},
		ResizeMode: entity.ResizeModeConfig{Actions: map[string]entity.ActionBinding{"resize-increase": {Keys: []string{"+"}}}},
		VimMode:    entity.VimModeConfig{Actions: map[string]entity.ActionBinding{"focus-input": {Keys: []string{"gi"}}}},
		TabMode:    entity.TabModeConfig{Actions: map[string]entity.ActionBinding{"next-tab": {Keys: []string{"l"}}}},
	}
	session := entity.SessionConfig{SessionMode: entity.SessionModeConfig{Actions: map[string]entity.ActionBinding{
		"session-manager": {Keys: []string{"s"}},
	}}}
	for _, tc := range []struct {
		mode    input.Mode
		class   string
		actions map[string]entity.ActionBinding
	}{
		{input.ModePane, "pane-mode-active", workspace.PaneMode.Actions},
		{input.ModeResize, "resize-mode-active", workspace.ResizeMode.Actions},
		{input.ModeVim, "vim-mode-active", workspace.VimMode.Actions},
		{input.ModeTab, "tab-mode-active", workspace.TabMode.Actions},
		{input.ModeSession, "session-mode-active", session.SessionMode.Actions},
	} {
		t.Run(tc.mode.String(), func(t *testing.T) {
			require.Equal(t, tc.actions, legendActions(tc.mode, workspace, session))
			require.Equal(t, tc.class, modeFrameClass(tc.mode))
		})
	}
	require.Nil(t, legendActions(input.ModeNormal, workspace, session))
	require.Empty(t, modeFrameClass(input.ModeNormal))
	require.Equal(t, "→", displayLegendKey("arrowright"))
	require.Equal(t, "SPLIT", modeLegendGroup(input.ModePane, "split-right"))
	require.Equal(t, "JUMP", modeLegendGroup(input.ModeVim, "heading-next"))
}

func TestModeFramePendingSequenceFiltersBindings(t *testing.T) {
	require.True(t, legendSequenceMatches("3]", []string{"]]"}))
	require.False(t, legendSequenceMatches("3]", []string{"gi"}))
	require.True(t, legendSequenceMatches("3", []string{"gi"}))
	require.True(t, legendSequenceMatches("", []string{"gi"}))
}

func TestModeFrameTargetDoesNotDependOnGlobalFocusedWindow(t *testing.T) {
	f := newVimModeTestFixture(t)
	f.app.lastFocusedWindowID = f.bw2.id
	require.Equal(t, f.pv1A.Widget(), f.app.modeFrameTarget(f.bw1, input.ModeVim))
	require.Equal(t, f.pv2A.Widget(), f.app.modeFrameTarget(f.bw2, input.ModeVim))
}
