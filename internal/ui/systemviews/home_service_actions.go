package systemviews

import (
	"context"
	"strings"
	"time"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/domain/entity"
)

// homeServiceActionsRegionPrefix + service id names the actions region of a
// tile. It is separate from the data slot so a refresh never wipes a form.
const homeServiceActionsRegionPrefix = "home-svc-actions:"

// Service action events. Data carries svService and svHomeAction; form
// inputs arrive as homeActionInputPrefix + input name.
const (
	homeActionSvcOpen    = "home.svc.open"
	homeActionSvcSubmit  = "home.svc.submit"
	homeActionSvcConfirm = "home.svc.confirm"
	homeActionSvcCancel  = "home.svc.cancel"

	homeActionInputPrefix = "input."
	homeActionToastTime   = 5 * time.Second
)

// homeActionStep is where a service's action UI stands.
type homeActionStep int

const (
	homeActionIdle homeActionStep = iota
	homeActionForm
	homeActionConfirm
	homeActionBusy
)

// homeActionState is the action UI state of one service.
type homeActionState struct {
	ActionID string
	Step     homeActionStep
	Inputs   map[string]string
}

// homeServiceActionsView is what the actions region renders.
type homeServiceActionsView struct {
	Service entity.HomeService
	State   homeActionState
}

func homeServiceActionsRegion(id string) string {
	return homeServiceActionsRegionPrefix + id
}

func homeServiceHasActions(service entity.HomeService) bool {
	return len(service.Actions) > 0
}

func homeServiceAction(event DOMAction) bool {
	switch event.Action {
	case homeActionSvcOpen, homeActionSvcSubmit, homeActionSvcConfirm, homeActionSvcCancel:
		return true
	default:
		return false
	}
}

func homeActionInputName(name string) string {
	return homeActionInputPrefix + name
}

// findHomeService returns the service with id in load.
func findHomeService(load dto.HomeLoadResponse, id string) (entity.HomeService, bool) {
	for _, group := range load.Layout.Groups {
		for i := range group.Services {
			service := &group.Services[i]
			if service.ID == id {
				return *service, true
			}
		}
	}
	return entity.HomeService{}, false
}

func findHomeAction(service entity.HomeService, id string) (entity.HomeAction, bool) {
	for i := range service.Actions {
		if service.Actions[i].ID == id {
			return service.Actions[i], true
		}
	}
	return entity.HomeAction{}, false
}

// handleHomeServiceAction moves an action through its form and confirm
// steps, then runs it in the background with the button busy.
func (a *App) handleHomeServiceAction(ctx context.Context, event DOMAction) error {
	a.lockState()
	defer a.unlockState()
	if a.closed || a.currentRoute != RouteHome {
		return nil
	}
	serviceID := event.Data["svService"]
	service, ok := findHomeService(a.homeData.Load, serviceID)
	if !ok {
		return nil
	}
	state := a.homeSvc.actions[serviceID]
	if state.Step == homeActionBusy {
		return nil // one request at a time per service
	}
	if event.Action == homeActionSvcCancel {
		return a.setHomeActionStateLocked(service, homeActionState{}, homeActionButtonFocus(service.ID, state.ActionID))
	}
	actionID := event.Data["svHomeAction"]
	if event.Action == homeActionSvcConfirm {
		actionID = state.ActionID
	}
	action, ok := findHomeAction(service, actionID)
	if !ok {
		return nil
	}

	next := homeActionState{ActionID: action.ID}
	switch event.Action {
	case homeActionSvcOpen:
		switch {
		case len(action.Inputs) > 0:
			next.Step = homeActionForm
		case action.Confirm != "":
			next.Step = homeActionConfirm
		default:
			next.Step = homeActionBusy
		}
	case homeActionSvcSubmit:
		next.Inputs = homeActionInputs(action, event.Data)
		next.Step = homeActionBusy
		if action.Confirm != "" {
			next.Step = homeActionConfirm
		}
	case homeActionSvcConfirm:
		if state.Step != homeActionConfirm || state.ActionID != action.ID {
			return nil
		}
		next.Inputs = state.Inputs
		next.Step = homeActionBusy
	}
	if err := a.setHomeActionStateLocked(service, next, homeActionStepFocus(service.ID, next)); err != nil {
		return err
	}
	if next.Step == homeActionBusy {
		a.runHomeActionLocked(ctx, service, action, next.Inputs)
	}
	return nil
}

func homeActionInputs(action entity.HomeAction, data map[string]string) map[string]string {
	if len(action.Inputs) == 0 {
		return nil
	}
	inputs := make(map[string]string, len(action.Inputs))
	for _, input := range action.Inputs {
		inputs[input.Name] = strings.TrimSpace(data[homeActionInputName(input.Name)])
	}
	return inputs
}

// runHomeActionLocked calls HomeAction off App.mu, then clears the busy
// state and shows the result as a toast. Caller holds App.mu.
func (a *App) runHomeActionLocked(ctx context.Context, service entity.HomeService, action entity.HomeAction, inputs map[string]string) {
	home := a.deps.Home
	req := dto.HomeActionRequest{ServiceID: service.ID, ActionID: action.ID, Inputs: inputs}
	a.homeFillWG.Add(1)
	go func() {
		defer a.homeFillWG.Done()
		var resp dto.HomeActionResponse
		var err error
		if home == nil {
			resp = dto.HomeActionResponse{Status: dto.HomeStatusError, Message: "home service not configured"}
		} else if resp, err = home.HomeAction(ctx, req); err != nil {
			resp = dto.HomeActionResponse{Status: dto.HomeStatusError, Message: err.Error()}
		}

		a.lockState()
		defer a.unlockState()
		if a.closed {
			return
		}
		if a.homeSvc.actions[service.ID].ActionID == action.ID {
			current, ok := findHomeService(a.homeData.Load, service.ID)
			if !ok {
				delete(a.homeSvc.actions, service.ID)
			} else if err := a.setHomeActionStateLocked(
				current, homeActionState{}, homeActionButtonFocus(service.ID, action.ID),
			); err != nil {
				logActionMountError(ctx, err, nil)
			}
		}
		if toaster, ok := a.deps.DOM.(DOMToaster); ok {
			toaster.ShowToast(ToastRequest{Message: homeActionToastMessage(action, resp), Timeout: homeActionToastTime})
		}
	}()
}

func homeActionToastMessage(action entity.HomeAction, resp dto.HomeActionResponse) string {
	label := strings.TrimSpace(action.Label)
	if label == "" {
		label = action.ID
	}
	message := strings.TrimSpace(resp.Message)
	if resp.Status == dto.HomeStatusError {
		if message == "" {
			message = "failed"
		}
		return label + " failed: " + message
	}
	if message == "" {
		message = "done"
	}
	return label + ": " + message
}

// homeActionStepFocus is the element to focus once a step is shown: the
// first input of a form, or Confirm. A busy step has nothing to focus.
func homeActionStepFocus(serviceID string, state homeActionState) string {
	region := homeActionRegionSelector(serviceID)
	switch state.Step {
	case homeActionForm:
		return "sel:" + region + " form input"
	case homeActionConfirm:
		return "sel:" + region + ` [data-sv-action="` + homeActionSvcConfirm + `"]`
	default:
		return ""
	}
}

// homeActionButtonFocus focuses an action's button again after cancel or
// completion.
func homeActionButtonFocus(serviceID, actionID string) string {
	if actionID == "" {
		return ""
	}
	return "sel:" + homeActionRegionSelector(serviceID) + ` [data-sv-home-action="` + cssQuoted(actionID) + `"]`
}

func homeActionRegionSelector(serviceID string) string {
	return `[data-sv-region="` + cssQuoted(homeServiceActionsRegion(serviceID)) + `"]`
}

// setHomeActionStateLocked stores state and swaps the actions region when
// its markup changed, then focuses focus (a RegionUpdate focus key) when the
// swap dropped focus. Caller holds App.mu.
func (a *App) setHomeActionStateLocked(service entity.HomeService, state homeActionState, focus string) error {
	if a.homeSvc.actions == nil {
		a.homeSvc.actions = make(map[string]homeActionState)
	}
	if state.Step == homeActionIdle {
		delete(a.homeSvc.actions, service.ID)
	} else {
		a.homeSvc.actions[service.ID] = state
	}
	updater, ok := a.deps.DOM.(DOMRegionUpdater)
	if !ok {
		return nil
	}
	return updater.UpdateRegions(RegionUpdate{Regions: []DOMRegion{{
		Name: homeServiceActionsRegion(service.ID),
		HTML: mustRenderComponent(HomeServiceActions(homeServiceActionsView{Service: service, State: state})),
	}}, Focus: focus})
}

// homeActionRegionsLocked returns the actions regions that are not idle, to
// push back after the groups skeleton re-rendered them idle.
func (a *App) homeActionRegionsLocked() []DOMRegion {
	var regions []DOMRegion
	for id, state := range a.homeSvc.actions {
		service, ok := findHomeService(a.homeData.Load, id)
		if !ok || !homeServiceHasActions(service) {
			delete(a.homeSvc.actions, id)
			continue
		}
		if _, ok := findHomeAction(service, state.ActionID); !ok {
			delete(a.homeSvc.actions, id)
			continue
		}
		regions = append(regions, DOMRegion{
			Name: homeServiceActionsRegion(id),
			HTML: mustRenderComponent(HomeServiceActions(homeServiceActionsView{Service: service, State: state})),
		})
	}
	return regions
}

func homeActionLabel(action entity.HomeAction) string {
	if label := strings.TrimSpace(action.Label); label != "" {
		return label
	}
	return action.ID
}

func homeActionInputLabel(input entity.HomeActionInput) string {
	if label := strings.TrimSpace(input.Label); label != "" {
		return label
	}
	return input.Name
}
