package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/domain/entity"
	"bnema.dev/dumber/internal/logging"
)

// homeMaxInputLen bounds one action input value.
const homeMaxInputLen = 4096

// errHomeActionRunning is returned while the same action is in flight.
var errHomeActionRunning = errors.New("action is already running")

// RunHomeAction runs a declared action of a service. Unknown services,
// actions or inputs are errors. HTTP failures are reported in the response.
func (uc *HomeUseCase) RunHomeAction(ctx context.Context, req dto.HomeActionRequest) (*dto.HomeActionResponse, error) {
	if uc.source == nil || uc.http == nil {
		return nil, errors.New("home actions not configured")
	}
	snap, err := uc.source.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("load home config: %w", err)
	}
	svc, ok := snap.Layout.FindService(req.ServiceID)
	if !ok {
		return nil, fmt.Errorf("unknown home service %q", req.ServiceID)
	}
	action, ok := svc.FindAction(req.ActionID)
	if !ok {
		return nil, fmt.Errorf("unknown action %q for home service %q", req.ActionID, req.ServiceID)
	}
	declared := action.InputNames()
	for name, value := range req.Inputs {
		if !containsString(declared, name) {
			return nil, fmt.Errorf("action %q has no input %q", action.ID, name)
		}
		if len(value) > homeMaxInputLen {
			return nil, fmt.Errorf("input %q is longer than %d bytes", name, homeMaxInputLen)
		}
	}

	key := req.ServiceID + "/" + action.ID
	uc.actionMu.Lock()
	if uc.actionRunning[key] {
		uc.actionMu.Unlock()
		return &dto.HomeActionResponse{Status: dto.HomeStatusError, Message: errHomeActionRunning.Error()}, nil
	}
	uc.actionRunning[key] = true
	uc.actionMu.Unlock()
	defer func() {
		uc.actionMu.Lock()
		delete(uc.actionRunning, key)
		uc.actionMu.Unlock()
	}()

	log := logging.FromContext(ctx).With().Str("home_service", req.ServiceID).Str("home_action", action.ID).Logger()
	fail := func(status int, msg string) *dto.HomeActionResponse {
		log.Debug().Int("http_status", status).Str("reason", msg).Msg("home action failed")
		return &dto.HomeActionResponse{Status: dto.HomeStatusError, HTTPStatus: status, Message: msg}
	}

	httpReq, err := uc.buildHomeActionRequest(action, req.Inputs)
	if err != nil {
		return fail(0, err.Error()), nil
	}
	resp, err := uc.http.Do(ctx, httpReq)
	if err != nil {
		return fail(0, safeHomeHTTPError(ctx, err)), nil
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fail(resp.StatusCode, fmt.Sprintf("HTTP %d", resp.StatusCode)), nil
	}
	log.Info().Int("http_status", resp.StatusCode).Msg("home action done")
	return &dto.HomeActionResponse{
		Status: dto.HomeStatusOK, HTTPStatus: resp.StatusCode, Message: fmt.Sprintf("HTTP %d", resp.StatusCode),
	}, nil
}

// buildHomeActionRequest expands env first, then inputs. Inputs are
// inserted last so that a user-typed `{{env.X}}` is never expanded. Inputs
// are URL-encoded in the url and JSON-string-escaped in the body. Headers
// only get env expansion.
func (uc *HomeUseCase) buildHomeActionRequest(a entity.HomeAction, inputs map[string]string) (port.HomeHTTPRequest, error) {
	base, err := uc.expandHomeRequest(string(a.Method), a.URL, a.Headers, nil)
	if err != nil {
		return base, err
	}
	for k, v := range base.Headers {
		if len(entity.HomeInputRefs(v)) > 0 {
			return base, fmt.Errorf("header %q must not reference inputs", k)
		}
	}
	declared := a.InputNames()
	if entity.HomeInputsInURLOrigin(a.URL) {
		return base, errors.New("url inputs must not be in the scheme, host or port")
	}
	envURL := base.URL
	base.URL, err = entity.ExpandHomeInputs(envURL, inputs, declared, url.QueryEscape)
	if err != nil {
		return base, err
	}
	// The typed inputs must never change where the request goes, since the
	// env-expanded secret headers follow it.
	template, err := entity.ExpandHomeInputs(envURL, nil, declared, func(string) string { return "" })
	if err != nil {
		return base, err
	}
	if !sameHomeOrigin(template, base.URL) {
		return base, errors.New("url inputs must not change the scheme, host or port")
	}
	if a.Body != "" {
		expandEnv := entity.ExpandHomeEnv
		if entity.HomeBodyIsJSON(a.Body) {
			expandEnv = entity.ExpandHomeJSONBodyEnv
		}
		body, envErr := expandEnv(a.Body, uc.lookupEnv)
		if envErr != nil {
			return base, envErr
		}
		// Validation only allows inputs inside JSON string literals; check
		// again so a body that bypassed it never gets raw user text.
		if len(entity.HomeInputRefs(a.Body)) > 0 {
			if msg := entity.CheckHomeInputBody(a.Body); msg != "" {
				return base, fmt.Errorf("body %s", msg)
			}
		}
		body, err = entity.ExpandHomeInputs(body, inputs, declared, jsonStringEscape)
		if err != nil {
			return base, err
		}
		base.Body = []byte(body)
	}
	return base, nil
}

// jsonStringEscape escapes s for use inside a JSON string literal.
func jsonStringEscape(s string) string {
	data, _ := json.Marshal(s)
	return string(data[1 : len(data)-1])
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// sameHomeOrigin reports whether two URLs share scheme, host and port.
func sameHomeOrigin(a, b string) bool {
	ua, errA := url.Parse(a)
	ub, errB := url.Parse(b)
	if errA != nil || errB != nil {
		return false
	}
	return strings.EqualFold(ua.Scheme, ub.Scheme) && strings.EqualFold(ua.Host, ub.Host)
}
