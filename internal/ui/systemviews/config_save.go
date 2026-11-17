package systemviews

import (
	"context"
	"errors"
	"time"
)

// configRegionStatus is the save bar's status line. Validation errors are
// swapped into it so the form, and the user's unsaved edits, stay in place.
const configRegionStatus = "config-status"

// configSavedToastTimeout keeps the "saved" confirmation briefly on screen.
const configSavedToastTimeout = 3 * time.Second

// applyConfigActionLocked runs a config action and refreshes the page.
// Caller holds the state lock.
func (a *App) applyConfigActionLocked(ctx context.Context, event DOMAction) error {
	a.configError = ""
	var err error
	if event.Action == configActionSaveBatch {
		var wrote bool
		wrote, err = a.saveConfigBatch(ctx, event.Data)
		if err != nil && !wrote {
			// Nothing was persisted: keep the form as the user left it.
			if updater, ok := a.deps.DOM.(DOMRegionUpdater); ok {
				return updater.UpdateRegions(RegionUpdate{Regions: []DOMRegion{{
					Name: configRegionStatus,
					HTML: mustRenderComponent(ConfigStatus(configStatusFromError(err))),
				}}})
			}
		}
	} else {
		err = a.handleConfigAction(ctx, event)
	}
	if err != nil {
		a.configNotice = ""
		a.configError = err.Error()
	}

	notice := a.configNotice
	toaster, canToast := a.deps.DOM.(DOMToaster)
	if canToast && notice != "" {
		a.configNotice = ""
	}
	if mountErr := a.remountConfigLocked(ctx); mountErr != nil {
		return mountErr
	}
	if canToast && notice != "" && !toaster.ShowToast(ToastRequest{Message: notice, Timeout: configSavedToastTimeout}) {
		// No toast UI: show the notice in the page instead.
		a.configNotice = notice
		return a.remountConfigLocked(ctx)
	}
	return nil
}

func (a *App) remountConfigLocked(ctx context.Context) error {
	a.loadShellTheme(ctx)
	if err := a.loadConfigRoute(ctx); err != nil {
		a.renderRouteError(err)
		_ = a.mountRenderedHTML()
		return err
	}
	return a.mountRenderedHTML()
}

// configStatus is the save bar's status line content.
type configStatus struct {
	Error string
	// Field is the form field to focus, when the error belongs to one.
	Field string
}

func configStatusFromError(err error) configStatus {
	status := configStatus{Error: err.Error()}
	var fieldErr *configFieldError
	if errors.As(err, &fieldErr) {
		status.Field = fieldErr.Field
	}
	return status
}
