package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/domain/entity"
	"bnema.dev/dumber/internal/logging"
)

// homeWidgetCacheEntry is the last good response of an api widget.
type homeWidgetCacheEntry struct {
	hash [sha256.Size]byte
	resp dto.HomeWidgetDataResponse
}

// homeWidgetFlight lets concurrent fetches of the same service share one
// HTTP request.
type homeWidgetFlight struct {
	done chan struct{}
	resp *dto.HomeWidgetDataResponse
}

// FetchWidgetData returns the data of the api widget of serviceID. Fetch
// failures are reported in the response (status error), with the last good
// data marked stale when available. The error return is for unknown
// services or non-api widgets.
func (uc *HomeUseCase) FetchWidgetData(ctx context.Context, serviceID string) (*dto.HomeWidgetDataResponse, error) {
	if uc.source == nil || uc.http == nil {
		return nil, errors.New("home widgets not configured")
	}
	snap, err := uc.source.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("load home config: %w", err)
	}
	svc, ok := snap.Layout.FindService(serviceID)
	if !ok {
		return nil, fmt.Errorf("unknown home service %q", serviceID)
	}
	if svc.Widget == nil || svc.Widget.Type != entity.HomeServiceWidgetAPI {
		return nil, fmt.Errorf("home service %q has no api widget", serviceID)
	}
	widget := *svc.Widget
	hash := hashHomeWidget(widget)

	uc.widgetMu.Lock()
	entry, cached := uc.widgetCache[serviceID]
	if cached && entry.hash != hash {
		delete(uc.widgetCache, serviceID)
		cached = false
	}
	if cached && uc.now().Sub(entry.resp.FetchedAt) < widget.RefreshDuration() {
		uc.widgetMu.Unlock()
		resp := entry.resp
		return &resp, nil
	}
	if f, running := uc.widgetInflight[serviceID]; running {
		uc.widgetMu.Unlock()
		select {
		case <-f.done:
			resp := *f.resp
			return &resp, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	f := &homeWidgetFlight{done: make(chan struct{})}
	uc.widgetInflight[serviceID] = f
	uc.widgetMu.Unlock()

	resp, allowStale := uc.fetchWidget(ctx, serviceID, widget)

	uc.widgetMu.Lock()
	if resp.Status == dto.HomeStatusOK {
		uc.widgetCache[serviceID] = homeWidgetCacheEntry{hash: hash, resp: resp}
	} else if last, ok := uc.widgetCache[serviceID]; ok && allowStale && last.hash == hash {
		stale := last.resp
		stale.Status = dto.HomeStatusError
		stale.Error = resp.Error
		stale.Stale = true
		resp = stale
	}
	delete(uc.widgetInflight, serviceID)
	f.resp = &resp
	close(f.done)
	uc.widgetMu.Unlock()

	out := resp
	return &out, nil
}

// fetchWidget performs the HTTP request. Errors in the result are safe to
// show: they never include header values or the expanded URL. allowStale is
// false for template errors, which must not fall back to older data.
func (uc *HomeUseCase) fetchWidget(
	ctx context.Context, serviceID string, w entity.HomeServiceWidget,
) (resp dto.HomeWidgetDataResponse, allowStale bool) {
	log := logging.FromContext(ctx).With().Str("home_service", serviceID).Logger()
	fail := func(msg string) (dto.HomeWidgetDataResponse, bool) {
		log.Debug().Str("reason", msg).Msg("home widget fetch failed")
		return dto.HomeWidgetDataResponse{
			ServiceID: serviceID, Status: dto.HomeStatusError, Error: msg, FetchedAt: uc.now(),
		}, true
	}

	req, err := uc.expandHomeRequest(http.MethodGet, w.URL, w.Headers, nil)
	if err != nil {
		return fail(err.Error())
	}
	req.Timeout = w.TimeoutDuration()

	httpResp, err := uc.http.Do(ctx, req)
	if err != nil {
		return fail(safeHomeHTTPError(ctx, err))
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode > 299 {
		return fail(fmt.Sprintf("HTTP %d", httpResp.StatusCode))
	}
	var doc any
	if err := json.Unmarshal(httpResp.Body, &doc); err != nil {
		return fail("invalid JSON response")
	}

	resp = dto.HomeWidgetDataResponse{ServiceID: serviceID, Status: dto.HomeStatusOK, FetchedAt: uc.now()}
	if w.Template != "" {
		resp.Fields = extractHomeValues(doc, w.Fields)
		html, err := uc.renderHomeTemplate(w.Template, doc, resp.Fields)
		if err != nil {
			failed, _ := fail(err.Error())
			return failed, false
		}
		resp.HTML = html
		return resp, true
	}
	if items, ok := homeWidgetItems(doc, w); ok {
		resp.Items = make([]dto.HomeWidgetItem, 0, len(items))
		for _, item := range items {
			resp.Items = append(resp.Items, dto.HomeWidgetItem{Fields: extractHomeValues(item, w.Fields)})
		}
		resp.Fields = []dto.HomeWidgetValue{}
		return resp, true
	}
	resp.Fields = extractHomeValues(doc, w.Fields)
	return resp, true
}

// renderHomeTemplate renders tmpl with {"data": doc, "fields": label→value}.
func (uc *HomeUseCase) renderHomeTemplate(tmpl string, doc any, fields []dto.HomeWidgetValue) (string, error) {
	if uc.templates == nil {
		return "", errors.New("template error: rendering is not available")
	}
	byLabel := make(map[string]string, len(fields))
	for _, f := range fields {
		byLabel[f.Label] = f.Value
	}
	html, err := uc.templates.Render(tmpl, map[string]any{"data": doc, "fields": byLabel})
	if err != nil {
		return "", fmt.Errorf("template error: %s", err.Error())
	}
	return html, nil
}

// homeWidgetItems returns the array a list widget renders, if any.
func homeWidgetItems(doc any, w entity.HomeServiceWidget) ([]any, bool) {
	if w.Display != entity.HomeDisplayList {
		return nil, false
	}
	target := doc
	if w.Items != "" {
		v, ok := entity.ExtractHomeField(doc, w.Items)
		if !ok {
			return nil, false
		}
		target = v
	}
	items, ok := target.([]any)
	return items, ok
}

func extractHomeValues(doc any, fields []entity.HomeField) []dto.HomeWidgetValue {
	out := make([]dto.HomeWidgetValue, 0, len(fields))
	for _, f := range fields {
		v := dto.HomeWidgetValue{Label: f.Label}
		if raw, ok := entity.ExtractHomeField(doc, f.Path); ok {
			v.Raw = raw
			v.Value = entity.FormatHomeField(raw, f.Format)
		}
		out = append(out, v)
	}
	return out
}

// expandHomeRequest applies env expansion to the url and headers. Errors
// only name missing variables.
func (uc *HomeUseCase) expandHomeRequest(
	method, rawURL string, headers map[string]string, body []byte,
) (port.HomeHTTPRequest, error) {
	expandedURL, err := entity.ExpandHomeEnv(rawURL, uc.lookupEnv)
	if err != nil {
		return port.HomeHTTPRequest{}, err
	}
	var outHeaders map[string]string
	if len(headers) > 0 {
		outHeaders = make(map[string]string, len(headers))
		for k, v := range headers {
			ev, err := entity.ExpandHomeEnv(v, uc.lookupEnv)
			if err != nil {
				return port.HomeHTTPRequest{}, err
			}
			outHeaders[k] = ev
		}
	}
	return port.HomeHTTPRequest{Method: method, URL: expandedURL, Headers: outHeaders, Body: body}, nil
}

// safeHomeHTTPError maps transport errors to messages without the URL,
// which may carry tokens.
func safeHomeHTTPError(ctx context.Context, err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded) || isTimeout(err):
		return "request timed out"
	case errors.Is(err, context.Canceled) || ctx.Err() != nil:
		return "request canceled"
	default:
		return "request failed"
	}
}

func isTimeout(err error) bool {
	var t interface{ Timeout() bool }
	return errors.As(err, &t) && t.Timeout()
}

func hashHomeWidget(w entity.HomeServiceWidget) [sha256.Size]byte {
	// json.Marshal sorts map keys, so equal widgets hash equally.
	data, _ := json.Marshal(w)
	return sha256.Sum256(data)
}
