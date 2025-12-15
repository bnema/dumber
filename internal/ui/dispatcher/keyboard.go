package dispatcher

import (
	"context"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/coordinator"
	"github.com/bnema/dumber/internal/ui/input"
)

// KeyboardDispatcher routes keyboard actions to appropriate coordinators.
type KeyboardDispatcher struct {
	tabCoord *coordinator.TabCoordinator
	wsCoord  *coordinator.WorkspaceCoordinator
	navCoord *coordinator.NavigationCoordinator
	onQuit   func()
}

// NewKeyboardDispatcher creates a new KeyboardDispatcher.
func NewKeyboardDispatcher(
	ctx context.Context,
	tabCoord *coordinator.TabCoordinator,
	wsCoord *coordinator.WorkspaceCoordinator,
	navCoord *coordinator.NavigationCoordinator,
) *KeyboardDispatcher {
	log := logging.FromContext(ctx)
	log.Debug().Msg("creating keyboard dispatcher")

	return &KeyboardDispatcher{
		tabCoord: tabCoord,
		wsCoord:  wsCoord,
		navCoord: navCoord,
	}
}

// SetOnQuit sets the callback for quit action.
func (d *KeyboardDispatcher) SetOnQuit(fn func()) {
	d.onQuit = fn
}

// Dispatch routes a keyboard action to the appropriate coordinator.
func (d *KeyboardDispatcher) Dispatch(ctx context.Context, action input.Action) error {
	log := logging.FromContext(ctx)
	log.Debug().Str("action", string(action)).Msg("dispatching keyboard action")

	switch action {
	// Tab actions
	case input.ActionNewTab:
		_, err := d.tabCoord.Create(ctx, "about:blank")
		return err
	case input.ActionCloseTab:
		return d.tabCoord.Close(ctx)
	case input.ActionNextTab:
		return d.tabCoord.SwitchNext(ctx)
	case input.ActionPreviousTab:
		return d.tabCoord.SwitchPrev(ctx)

	// Pane actions
	case input.ActionSplitRight:
		return d.wsCoord.Split(ctx, usecase.SplitRight)
	case input.ActionSplitLeft:
		return d.wsCoord.Split(ctx, usecase.SplitLeft)
	case input.ActionSplitUp:
		return d.wsCoord.Split(ctx, usecase.SplitUp)
	case input.ActionSplitDown:
		return d.wsCoord.Split(ctx, usecase.SplitDown)
	case input.ActionClosePane:
		return d.wsCoord.ClosePane(ctx)
	case input.ActionStackPane:
		return d.wsCoord.StackPane(ctx)
	case input.ActionFocusRight:
		return d.wsCoord.FocusPane(ctx, usecase.NavRight)
	case input.ActionFocusLeft:
		return d.wsCoord.FocusPane(ctx, usecase.NavLeft)
	case input.ActionFocusUp:
		return d.wsCoord.FocusPane(ctx, usecase.NavUp)
	case input.ActionFocusDown:
		return d.wsCoord.FocusPane(ctx, usecase.NavDown)

	// Stack navigation
	case input.ActionStackNavUp:
		return d.wsCoord.NavigateStack(ctx, "up")
	case input.ActionStackNavDown:
		return d.wsCoord.NavigateStack(ctx, "down")

	// Navigation
	case input.ActionGoBack:
		return d.navCoord.GoBack(ctx)
	case input.ActionGoForward:
		return d.navCoord.GoForward(ctx)
	case input.ActionReload:
		return d.navCoord.Reload(ctx)
	case input.ActionHardReload:
		return d.navCoord.HardReload(ctx)

	// Zoom (stub - will be implemented later)
	case input.ActionZoomIn:
		log.Debug().Msg("zoom in action (not yet implemented)")
	case input.ActionZoomOut:
		log.Debug().Msg("zoom out action (not yet implemented)")
	case input.ActionZoomReset:
		log.Debug().Msg("zoom reset action (not yet implemented)")

	// UI
	case input.ActionOpenOmnibox:
		return d.navCoord.OpenOmnibox(ctx)
	case input.ActionOpenDevTools:
		return d.navCoord.OpenDevTools(ctx)
	case input.ActionToggleFullscreen:
		log.Debug().Msg("toggle fullscreen action (not yet implemented)")

	// Application
	case input.ActionQuit:
		if d.onQuit != nil {
			d.onQuit()
		}

	default:
		log.Warn().Str("action", string(action)).Msg("unhandled keyboard action")
	}

	return nil
}
