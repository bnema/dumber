package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/application/port/mocks"
	"bnema.dev/dumber/internal/domain/entity"
	"bnema.dev/dumber/internal/logging"
)

const homeTestSecret = "s3cr3t-token"

type homeWidgetFixture struct {
	uc     *HomeUseCase
	layout *entity.HomeLayout
	http   *mocks.MockHomeHTTPClient
	now    *time.Time
	logs   *bytes.Buffer
	ctx    context.Context
}

func newHomeWidgetFixture(t *testing.T) *homeWidgetFixture {
	t.Helper()
	layout := entity.HomeLayout{Groups: []entity.HomeGroup{{Name: "G", Services: []entity.HomeService{
		{
			ID: "media", Name: "Media",
			Widget: &entity.HomeServiceWidget{
				Type:    entity.HomeServiceWidgetAPI,
				URL:     "https://media.local/api?token={{env.TOKEN}}",
				Headers: map[string]string{"X-Token": "{{env.TOKEN}}"},
				Fields: []entity.HomeField{
					{Label: "Movies", Path: "movies", Format: entity.HomeFormatNumber},
					{Label: "Missing", Path: "nope"},
				},
			},
			Actions: []entity.HomeAction{
				{
					ID: "search", Label: "Search", Method: entity.HomeMethodPost,
					URL:     "https://media.local/search?q={{input.q}}&k={{env.TOKEN}}",
					Headers: map[string]string{"Authorization": "Bearer {{env.TOKEN}}"},
					Body:    `{"query":"{{input.q}}"}`,
					Inputs:  []entity.HomeActionInput{{Name: "q", Label: "Query"}},
				},
				{
					ID: "note", Label: "Note", Method: entity.HomeMethodPost, URL: "https://media.local/note",
					Body:   "text={{input.q}}",
					Inputs: []entity.HomeActionInput{{Name: "q", Label: "Query"}},
				},
			},
		},
		{ID: "plain", Name: "Plain"},
	}}}}
	layout.Normalize()
	require.Empty(t, layout.Validate())

	source := mocks.NewMockHomeConfigSource(t)
	source.EXPECT().Load(mock.Anything).RunAndReturn(func(context.Context) (port.HomeConfigSnapshot, error) {
		return port.HomeConfigSnapshot{Layout: layout, Exists: true}, nil
	}).Maybe()
	httpClient := mocks.NewMockHomeHTTPClient(t)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	logs := &bytes.Buffer{}
	f := &homeWidgetFixture{
		layout: &layout,
		http:   httpClient,
		now:    &now,
		logs:   logs,
		ctx:    logging.WithContext(context.Background(), zerolog.New(logs).Level(zerolog.DebugLevel)),
	}
	f.uc = NewHomeUseCase(HomeDeps{
		Source: source,
		HTTP:   httpClient,
		LookupEnv: func(name string) (string, bool) {
			if name == "TOKEN" {
				return homeTestSecret, true
			}
			return "", false
		},
		Now: func() time.Time { return *f.now },
	})
	return f
}

func (f *homeWidgetFixture) assertNoSecret(t *testing.T, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	assert.NotContains(t, string(data), homeTestSecret)
	assert.NotContains(t, f.logs.String(), homeTestSecret)
}

func okJSON(body string) port.HomeHTTPResponse {
	return port.HomeHTTPResponse{StatusCode: 200, Body: []byte(body)}
}

func TestFetchWidgetData_ExpandsEnvAndExtractsFields(t *testing.T) {
	f := newHomeWidgetFixture(t)
	f.http.EXPECT().Do(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, req port.HomeHTTPRequest) (port.HomeHTTPResponse, error) {
			assert.Equal(t, "GET", req.Method)
			assert.Equal(t, "https://media.local/api?token="+homeTestSecret, req.URL)
			assert.Equal(t, homeTestSecret, req.Headers["X-Token"])
			assert.Equal(t, 10*time.Second, req.Timeout)
			return okJSON(`{"movies":12345}`), nil
		}).Once()

	resp, err := f.uc.FetchWidgetData(f.ctx, "media")
	require.NoError(t, err)
	assert.Equal(t, dto.HomeStatusOK, resp.Status)
	require.Len(t, resp.Fields, 2)
	assert.Equal(t, "12,345", resp.Fields[0].Value)
	assert.InDelta(t, 12345.0, resp.Fields[0].Raw, 0)
	assert.Equal(t, dto.HomeWidgetValue{Label: "Missing"}, resp.Fields[1])
	assert.Equal(t, *f.now, resp.FetchedAt)
	assert.False(t, resp.Stale)
}

func TestFetchWidgetData_CacheHitThenTTLExpiry(t *testing.T) {
	f := newHomeWidgetFixture(t)
	f.http.EXPECT().Do(mock.Anything, mock.Anything).Return(okJSON(`{"movies":1}`), nil).Once()

	_, err := f.uc.FetchWidgetData(f.ctx, "media")
	require.NoError(t, err)
	*f.now = f.now.Add(59 * time.Second)
	resp, err := f.uc.FetchWidgetData(f.ctx, "media")
	require.NoError(t, err)
	assert.Equal(t, "1", resp.Fields[0].Value)

	f.http.EXPECT().Do(mock.Anything, mock.Anything).Return(okJSON(`{"movies":2}`), nil).Once()
	*f.now = f.now.Add(2 * time.Second)
	resp, err = f.uc.FetchWidgetData(f.ctx, "media")
	require.NoError(t, err)
	assert.Equal(t, "2", resp.Fields[0].Value)
}

func TestFetchWidgetData_StaleOnError(t *testing.T) {
	f := newHomeWidgetFixture(t)
	f.http.EXPECT().Do(mock.Anything, mock.Anything).Return(okJSON(`{"movies":7}`), nil).Once()
	first, err := f.uc.FetchWidgetData(f.ctx, "media")
	require.NoError(t, err)

	*f.now = f.now.Add(time.Hour)
	f.http.EXPECT().Do(mock.Anything, mock.Anything).Return(port.HomeHTTPResponse{StatusCode: 503}, nil).Once()
	resp, err := f.uc.FetchWidgetData(f.ctx, "media")
	require.NoError(t, err)
	assert.Equal(t, dto.HomeStatusError, resp.Status)
	assert.True(t, resp.Stale)
	assert.Equal(t, "HTTP 503", resp.Error)
	assert.Equal(t, "7", resp.Fields[0].Value)
	assert.Equal(t, first.FetchedAt, resp.FetchedAt)
}

func TestFetchWidgetData_ErrorWithoutCacheHasNoSecrets(t *testing.T) {
	f := newHomeWidgetFixture(t)
	// A transport error that embeds the expanded URL, like *url.Error does.
	transportErr := &url.Error{Op: "Get", URL: "https://media.local/api?token=" + homeTestSecret, Err: errors.New("refused")}
	f.http.EXPECT().Do(mock.Anything, mock.Anything).Return(port.HomeHTTPResponse{}, transportErr).Once()

	resp, err := f.uc.FetchWidgetData(f.ctx, "media")
	require.NoError(t, err)
	assert.Equal(t, dto.HomeStatusError, resp.Status)
	assert.False(t, resp.Stale)
	assert.Equal(t, "request failed", resp.Error)
	assert.Empty(t, resp.Fields)
	f.assertNoSecret(t, resp)
}

func TestFetchWidgetData_MissingEnvNamesVariableOnly(t *testing.T) {
	f := newHomeWidgetFixture(t)
	f.layout.Groups[0].Services[0].Widget.URL = "https://media.local/{{env.NOPE}}"
	resp, err := f.uc.FetchWidgetData(f.ctx, "media")
	require.NoError(t, err)
	assert.Contains(t, resp.Error, "NOPE")
	f.assertNoSecret(t, resp)
}

func TestFetchWidgetData_InvalidatesOnConfigChange(t *testing.T) {
	f := newHomeWidgetFixture(t)
	f.http.EXPECT().Do(mock.Anything, mock.Anything).Return(okJSON(`{"movies":1}`), nil).Once()
	_, err := f.uc.FetchWidgetData(f.ctx, "media")
	require.NoError(t, err)

	f.layout.Groups[0].Services[0].Widget.Fields[0].Label = "Films"
	f.http.EXPECT().Do(mock.Anything, mock.Anything).Return(port.HomeHTTPResponse{StatusCode: 500}, nil).Once()
	resp, err := f.uc.FetchWidgetData(f.ctx, "media")
	require.NoError(t, err)
	// The old cache entry was dropped, so there is no stale data to show.
	assert.False(t, resp.Stale)
	assert.Empty(t, resp.Fields)
}

func TestFetchWidgetData_ListItems(t *testing.T) {
	f := newHomeWidgetFixture(t)
	w := f.layout.Groups[0].Services[0].Widget
	w.Display = entity.HomeDisplayList
	w.Items = "data.rows"
	w.Fields = []entity.HomeField{{Label: "Name", Path: "name", Format: entity.HomeFormatText}}
	f.http.EXPECT().Do(mock.Anything, mock.Anything).
		Return(okJSON(`{"data":{"rows":[{"name":"a"},{"name":"b"}]}}`), nil).Once()

	resp, err := f.uc.FetchWidgetData(f.ctx, "media")
	require.NoError(t, err)
	require.Len(t, resp.Items, 2)
	assert.Equal(t, "b", resp.Items[1].Fields[0].Value)
	assert.Empty(t, resp.Fields)
}

func TestFetchWidgetData_InvalidJSON(t *testing.T) {
	f := newHomeWidgetFixture(t)
	f.http.EXPECT().Do(mock.Anything, mock.Anything).Return(okJSON(`<html>`), nil).Once()
	resp, err := f.uc.FetchWidgetData(f.ctx, "media")
	require.NoError(t, err)
	assert.Equal(t, "invalid JSON response", resp.Error)
}

func TestFetchWidgetData_UnknownOrNonAPIService(t *testing.T) {
	f := newHomeWidgetFixture(t)
	_, err := f.uc.FetchWidgetData(f.ctx, "nope")
	require.Error(t, err)
	_, err = f.uc.FetchWidgetData(f.ctx, "plain")
	require.Error(t, err)
}

func TestFetchWidgetData_ConcurrentCallsShareOneRequest(t *testing.T) {
	f := newHomeWidgetFixture(t)
	release := make(chan struct{})
	f.http.EXPECT().Do(mock.Anything, mock.Anything).RunAndReturn(
		func(context.Context, port.HomeHTTPRequest) (port.HomeHTTPResponse, error) {
			<-release
			return okJSON(`{"movies":3}`), nil
		}).Once()

	var wg sync.WaitGroup
	results := make([]*dto.HomeWidgetDataResponse, 5)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := f.uc.FetchWidgetData(f.ctx, "media")
			assert.NoError(t, err)
			results[i] = resp
		}()
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	for _, r := range results {
		require.NotNil(t, r)
		assert.Equal(t, "3", r.Fields[0].Value)
	}
}

func TestRunHomeAction_EncodesInputs(t *testing.T) {
	f := newHomeWidgetFixture(t)
	f.http.EXPECT().Do(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, req port.HomeHTTPRequest) (port.HomeHTTPResponse, error) {
			assert.Equal(t, "POST", req.Method)
			assert.Equal(t, "https://media.local/search?q=a+%26+b%3D%22c%22&k="+homeTestSecret, req.URL)
			assert.Equal(t, "Bearer "+homeTestSecret, req.Headers["Authorization"])
			assert.JSONEq(t, `{"query":"a & b=\"c\""}`, string(req.Body))
			return port.HomeHTTPResponse{StatusCode: 204}, nil
		}).Once()

	resp, err := f.uc.RunHomeAction(f.ctx, dto.HomeActionRequest{
		ServiceID: "media", ActionID: "search", Inputs: map[string]string{"q": `a & b="c"`},
	})
	require.NoError(t, err)
	assert.Equal(t, &dto.HomeActionResponse{Status: dto.HomeStatusOK, HTTPStatus: 204, Message: "HTTP 204"}, resp)
}

func TestRunHomeAction_RawBodyAndNoEnvExpansionOfInputs(t *testing.T) {
	f := newHomeWidgetFixture(t)
	f.http.EXPECT().Do(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, req port.HomeHTTPRequest) (port.HomeHTTPResponse, error) {
			assert.Equal(t, "text={{env.TOKEN}}", string(req.Body))
			return port.HomeHTTPResponse{StatusCode: 200}, nil
		}).Once()

	_, err := f.uc.RunHomeAction(f.ctx, dto.HomeActionRequest{
		ServiceID: "media", ActionID: "note", Inputs: map[string]string{"q": "{{env.TOKEN}}"},
	})
	require.NoError(t, err)
}

func TestRunHomeAction_Rejections(t *testing.T) {
	f := newHomeWidgetFixture(t)
	for _, req := range []dto.HomeActionRequest{
		{ServiceID: "nope", ActionID: "search"},
		{ServiceID: "media", ActionID: "nope"},
		{ServiceID: "media", ActionID: "search", Inputs: map[string]string{"other": "x"}},
		{ServiceID: "media", ActionID: "search", Inputs: map[string]string{"q": strings.Repeat("x", homeMaxInputLen+1)}},
	} {
		_, err := f.uc.RunHomeAction(f.ctx, req)
		require.Error(t, err, "%+v", req)
	}
}

func TestRunHomeAction_HTTPErrorHasNoSecrets(t *testing.T) {
	f := newHomeWidgetFixture(t)
	f.http.EXPECT().Do(mock.Anything, mock.Anything).Return(port.HomeHTTPResponse{StatusCode: 401}, nil).Once()
	resp, err := f.uc.RunHomeAction(f.ctx, dto.HomeActionRequest{ServiceID: "media", ActionID: "search"})
	require.NoError(t, err)
	assert.Equal(t, &dto.HomeActionResponse{Status: dto.HomeStatusError, HTTPStatus: 401, Message: "HTTP 401"}, resp)
	f.assertNoSecret(t, resp)
}

func TestRunHomeAction_OneInFlightPerAction(t *testing.T) {
	f := newHomeWidgetFixture(t)
	started := make(chan struct{})
	release := make(chan struct{})
	f.http.EXPECT().Do(mock.Anything, mock.Anything).RunAndReturn(
		func(context.Context, port.HomeHTTPRequest) (port.HomeHTTPResponse, error) {
			close(started)
			<-release
			return port.HomeHTTPResponse{StatusCode: 200}, nil
		}).Once()

	done := make(chan *dto.HomeActionResponse)
	go func() {
		resp, _ := f.uc.RunHomeAction(f.ctx, dto.HomeActionRequest{ServiceID: "media", ActionID: "search"})
		done <- resp
	}()
	<-started
	busy, err := f.uc.RunHomeAction(f.ctx, dto.HomeActionRequest{ServiceID: "media", ActionID: "search"})
	require.NoError(t, err)
	assert.Equal(t, dto.HomeStatusError, busy.Status)
	assert.Equal(t, errHomeActionRunning.Error(), busy.Message)
	close(release)
	assert.Equal(t, dto.HomeStatusOK, (<-done).Status)
}

func TestRunHomeAction_UndeclaredInputInTemplateRejectedAtCall(t *testing.T) {
	f := newHomeWidgetFixture(t)
	f.layout.Groups[0].Services[0].Actions[0].URL = "https://media.local/{{input.zzz}}"
	resp, err := f.uc.RunHomeAction(f.ctx, dto.HomeActionRequest{ServiceID: "media", ActionID: "search"})
	require.NoError(t, err)
	assert.Equal(t, dto.HomeStatusError, resp.Status)
	assert.Contains(t, resp.Message, "zzz")
}

type fakeHomeRenderer struct {
	calls int
	data  any
	err   error
}

func (r *fakeHomeRenderer) Render(tmpl string, data any) (string, error) {
	r.calls++
	r.data = data
	if r.err != nil {
		return "", r.err
	}
	return "<p>" + tmpl + "</p>", nil
}

func TestFetchWidgetData_TemplateRenderedAndCached(t *testing.T) {
	f := newHomeWidgetFixture(t)
	renderer := &fakeHomeRenderer{}
	f.uc.templates = renderer
	f.layout.Groups[0].Services[0].Widget.Template = "T"
	f.http.EXPECT().Do(mock.Anything, mock.Anything).Return(okJSON(`{"movies":5}`), nil).Once()

	resp, err := f.uc.FetchWidgetData(f.ctx, "media")
	require.NoError(t, err)
	assert.Equal(t, dto.HomeStatusOK, resp.Status)
	assert.Equal(t, "<p>T</p>", resp.HTML)
	assert.Equal(t, map[string]any{
		"data":   map[string]any{"movies": 5.0},
		"fields": map[string]string{"Movies": "5", "Missing": ""},
	}, renderer.data)

	cached, err := f.uc.FetchWidgetData(f.ctx, "media")
	require.NoError(t, err)
	assert.Equal(t, "<p>T</p>", cached.HTML)
	assert.Equal(t, 1, renderer.calls)
}

func TestFetchWidgetData_TemplateErrorHasNoStaleData(t *testing.T) {
	f := newHomeWidgetFixture(t)
	renderer := &fakeHomeRenderer{}
	f.uc.templates = renderer
	f.layout.Groups[0].Services[0].Widget.Template = "T"
	f.http.EXPECT().Do(mock.Anything, mock.Anything).Return(okJSON(`{"movies":5}`), nil).Twice()
	_, err := f.uc.FetchWidgetData(f.ctx, "media")
	require.NoError(t, err)

	*f.now = f.now.Add(time.Hour)
	renderer.err = errors.New(`template: w:1: unexpected "}"`)
	resp, err := f.uc.FetchWidgetData(f.ctx, "media")
	require.NoError(t, err)
	assert.Equal(t, dto.HomeStatusError, resp.Status)
	assert.Equal(t, `template error: template: w:1: unexpected "}"`, resp.Error)
	assert.False(t, resp.Stale)
	assert.Empty(t, resp.HTML)
	assert.Empty(t, resp.Fields)
}

func TestFetchWidgetData_TemplateWithoutRenderer(t *testing.T) {
	f := newHomeWidgetFixture(t)
	f.layout.Groups[0].Services[0].Widget.Template = "T"
	f.http.EXPECT().Do(mock.Anything, mock.Anything).Return(okJSON(`{}`), nil).Once()
	resp, err := f.uc.FetchWidgetData(f.ctx, "media")
	require.NoError(t, err)
	assert.Equal(t, dto.HomeStatusError, resp.Status)
	assert.Contains(t, resp.Error, "template error")
}
