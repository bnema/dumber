package homehtml

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderTemplate_RendersAndSanitizes(t *testing.T) {
	t.Parallel()
	data := map[string]any{
		"name":  `<img src=x onerror=alert(1)>`,
		"tags":  []any{"a", "b"},
		"empty": "",
	}
	tmpl := `<div class="w" onclick="x()"><b>{{.name}}</b> {{join ", " .tags}} {{default "none" .empty}}<script>alert(1)</script></div>`
	out, err := RenderTemplate(tmpl, data)
	require.NoError(t, err)
	assert.Equal(t, `<div class="w"><b>&lt;img src=x onerror=alert(1)&gt;</b> a, b none</div>`, out)
}

func TestRenderTemplate_ParseErrorHasNoData(t *testing.T) {
	t.Parallel()
	_, err := RenderTemplate(`{{.secret`, map[string]any{"secret": "hunter2"})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "hunter2")
}

func TestRenderTemplate_OutputCap(t *testing.T) {
	t.Parallel()
	_, err := RenderTemplate(`{{range .}}xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx{{end}}`, make([]int, 10000))
	require.ErrorIs(t, err, ErrTemplateOutputTooLarge)
}

func TestRenderTemplate_Timeout(t *testing.T) {
	t.Parallel()
	// A slow data source stands in for a template that runs too long.
	slow := slowValue{delay: 200 * time.Millisecond}
	_, err := renderTemplate(context.Background(), `{{.Value}}`, slow, 20*time.Millisecond)
	require.ErrorIs(t, err, ErrTemplateTimeout)
}

type slowValue struct{ delay time.Duration }

func (s slowValue) Value() string {
	time.Sleep(s.delay)
	return strings.Repeat("x", 1)
}

func TestRenderTemplate_RejectsIntegerRange(t *testing.T) {
	t.Parallel()
	for _, tmpl := range []string{
		`{{range 2000000000}}x{{end}}`,
		`{{range $i := 2000000000}}x{{end}}`,
		`{{range (2000000000)}}x{{end}}`,
		`{{range len .data}}x{{end}}`,
		`{{$n := 2000000000}}{{range $n}}x{{end}}`,
		`{{if .data}}{{range 5}}x{{end}}{{end}}`,
		`{{define "t"}}{{range 9}}x{{end}}{{end}}ok`,
	} {
		start := time.Now()
		_, err := RenderTemplate(tmpl, map[string]any{"data": []any{1.0}})
		require.ErrorIs(t, err, ErrTemplateIntRange, tmpl)
		assert.Less(t, time.Since(start), 100*time.Millisecond, tmpl)
	}
}

func TestRenderTemplate_AllowsRangeOverData(t *testing.T) {
	t.Parallel()
	out, err := RenderTemplate(`{{range .items}}<i>{{.}}</i>{{end}}`, map[string]any{"items": []any{"a", "b"}})
	require.NoError(t, err)
	assert.Equal(t, "<i>a</i><i>b</i>", out)
}
