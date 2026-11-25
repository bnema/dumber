package homehtml

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
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
	synctest.Test(t, func(t *testing.T) {
		// A slow data source stands in for a template that runs too long.
		// Both the sleep and the timeout run on the bubble's fake clock.
		slow := slowValue{delay: time.Hour}
		_, err := renderTemplate(context.Background(), `{{.Value}}`, slow, time.Second)
		require.ErrorIs(t, err, ErrTemplateTimeout)
		// The abandoned render goroutine finishes once its sleep ends;
		// advance the fake clock so the bubble can exit.
		time.Sleep(slow.delay)
		synctest.Wait()
	})
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

func TestRenderTemplate_CapsArrays(t *testing.T) {
	t.Parallel()
	big := make([]any, 600)
	for i := range big {
		big[i] = map[string]any{"tags": make([]any, 600)}
	}
	out, err := RenderTemplate(`{{len .data.items}} {{len (index .data.items 0).tags}}`,
		map[string]any{"data": map[string]any{"items": big}})
	require.NoError(t, err)
	assert.Equal(t, "500 500", out)
	assert.Len(t, big, 600, "caller data is not modified")
}

func TestRenderTemplate_RangeDepth(t *testing.T) {
	t.Parallel()
	data := map[string]any{"a": []any{[]any{[]any{"x"}}}}
	out, err := RenderTemplate(`{{range .a}}{{range .}}ok{{end}}{{end}}`, data)
	require.NoError(t, err)
	assert.Equal(t, "ok", out)
	_, err = RenderTemplate(`{{range .a}}{{range .}}{{range .}}x{{end}}{{end}}{{end}}`, data)
	require.ErrorIs(t, err, ErrTemplateRangeDepth)
	_, err = RenderTemplate(`{{range .a}}{{if .}}{{range .}}{{with .}}{{range .}}x{{end}}{{end}}{{end}}{{end}}{{end}}`, data)
	require.ErrorIs(t, err, ErrTemplateRangeDepth)
}

func TestParseTemplateCachesAndBounds(t *testing.T) {
	first, err := parseTemplate("<p>{{.data.a}}</p>")
	if err != nil {
		t.Fatal(err)
	}
	again, err := parseTemplate("<p>{{.data.a}}</p>")
	if err != nil {
		t.Fatal(err)
	}
	if first != again {
		t.Fatal("same source must reuse the parsed template")
	}
	for i := range maxCachedTemplates + 5 {
		if _, err := parseTemplate(fmt.Sprintf("<i>%d</i>", i)); err != nil {
			t.Fatal(err)
		}
	}
	templateCache.Lock()
	n, order := len(templateCache.byHash), len(templateCache.order)
	templateCache.Unlock()
	if n != maxCachedTemplates || order != maxCachedTemplates {
		t.Fatalf("cache size = %d/%d, want %d", n, order, maxCachedTemplates)
	}
	if _, err := parseTemplate("{{range 5}}x{{end}}"); err == nil {
		t.Fatal("rejected templates are never cached as valid")
	}
}

func TestCachedTemplateRendersConcurrently(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			out, err := RenderTemplate("<b>{{.n}}</b>", map[string]any{"n": i})
			if err != nil || out != fmt.Sprintf("<b>%d</b>", i) {
				t.Errorf("render %d = %q, %v", i, out, err)
			}
		})
	}
	wg.Wait()
}

func TestRenderTemplate_RejectsSubTemplates(t *testing.T) {
	t.Parallel()
	probes := []string{
		`{{define "inner"}}{{range .}}{{range .}}x{{end}}{{end}}{{end}}{{range .}}{{range .}}{{template "inner" .}}{{end}}{{end}}`,
		`{{define "r"}}{{range .}}{{template "r" .}}{{end}}{{end}}{{template "r" .}}`,
		`{{block "b" .}}x{{end}}`,
		`{{if .}}{{template "missing" .}}{{end}}`,
	}
	for _, p := range probes {
		_, err := RenderTemplate(p, []any{[]any{1}})
		require.ErrorIs(t, err, ErrTemplateSubTemplate, p)
	}
}

func TestCapTemplateArrays_CapsObjectKeys(t *testing.T) {
	t.Parallel()
	obj := map[string]any{}
	for i := range MaxTemplateArrayItems + 10 {
		obj[fmt.Sprintf("k%04d", i)] = i
	}
	got := capTemplateArrays(map[string]any{"o": obj}, MaxTemplateArrayItems).(map[string]any)["o"].(map[string]any)
	require.Len(t, got, MaxTemplateArrayItems)
	require.Contains(t, got, "k0000")
	require.Contains(t, got, fmt.Sprintf("k%04d", MaxTemplateArrayItems-1))
	require.NotContains(t, got, fmt.Sprintf("k%04d", MaxTemplateArrayItems))
}
