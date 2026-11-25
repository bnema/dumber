package homehtml_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/application/usecase"
	"bnema.dev/dumber/internal/domain/entity"
	"bnema.dev/dumber/internal/infrastructure/homehtml"
)

type staticSource struct{ layout entity.HomeLayout }

func (s staticSource) Load(context.Context) (port.HomeConfigSnapshot, error) {
	return port.HomeConfigSnapshot{Layout: s.layout, Exists: true}, nil
}
func (staticSource) Path() string                { return "/c/home.toml" }
func (staticSource) EnsureExists() (bool, error) { return false, nil }

type staticHTTP struct{ body string }

func (h staticHTTP) Do(context.Context, port.HomeHTTPRequest) (port.HomeHTTPResponse, error) {
	return port.HomeHTTPResponse{StatusCode: 200, Body: []byte(h.body)}, nil
}

func homeWithTemplate(t *testing.T, tmpl string) *usecase.HomeUseCase {
	t.Helper()
	layout := entity.HomeLayout{Groups: []entity.HomeGroup{{Name: "G", Services: []entity.HomeService{{
		ID: "media", Name: "Media",
		Widget: &entity.HomeServiceWidget{
			Type: entity.HomeServiceWidgetAPI, URL: "https://media.local/api",
			Fields:   []entity.HomeField{{Label: "Movies", Path: "count", Format: entity.HomeFormatNumber}},
			Template: tmpl,
		},
	}}}}}
	layout.Normalize()
	require.Empty(t, layout.Validate())
	return usecase.NewHomeUseCase(usecase.HomeDeps{
		Source:    staticSource{layout: layout},
		HTTP:      staticHTTP{body: `{"count":1234,"name":"<script>alert(1)</script>"}`},
		Templates: homehtml.Renderer{},
	})
}

func TestRenderer_FetchWidgetDataRendersSanitizedHTML(t *testing.T) {
	uc := homeWithTemplate(t,
		`<p onclick="x()"><strong>{{ index .fields "Movies" }}</strong> {{ .data.name }}</p><script>bad()</script>`)
	resp, err := uc.FetchWidgetData(context.Background(), "media")
	require.NoError(t, err)
	assert.Equal(t, dto.HomeStatusOK, resp.Status)
	assert.Contains(t, resp.HTML, "<strong>1,234</strong>")
	assert.Contains(t, resp.HTML, "&lt;script&gt;")
	assert.NotContains(t, resp.HTML, "onclick")
	assert.NotContains(t, resp.HTML, "bad()")

	data, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "<script>")
}

func TestRenderer_ParseErrorIsReported(t *testing.T) {
	uc := homeWithTemplate(t, `{{ if }`)
	resp, err := uc.FetchWidgetData(context.Background(), "media")
	require.NoError(t, err)
	assert.Equal(t, dto.HomeStatusError, resp.Status)
	assert.Contains(t, resp.Error, "template error: ")
	assert.Empty(t, resp.HTML)
}
