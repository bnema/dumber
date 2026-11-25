package systemviews

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"bnema.dev/dumber/internal/application/dto"
)

func TestResolveHomeQuery(t *testing.T) {
	shortcuts := map[string]dto.HomeSearchShortcut{
		"gh":   {URL: "https://github.com/search?q=%s", Name: "GitHub"},
		"ghub": {URL: "https://ghub.example/?q=%s", Name: "Ghub"},
		"g":    {URL: "https://google.com/search?q=%s", Name: "Google"},
		"evil": {URL: "javascript:alert(1)//%s", Name: "Evil"},
	}
	items := []homeQueryItem{
		{Key: "s1", Name: "Grafana", Text: "grafana metrics", URL: "https://grafana.lan"},
		{Key: "s2", Name: "Gitea", Text: "gitea code github.com mirror", URL: "https://gitea.lan"},
		{Key: "s3", Name: "Bad", Text: "bad metrics", URL: "javascript:alert(1)"},
	}
	const ddg = "https://duckduckgo.com/?q=%s"
	base := homeQueryInput{EngineURL: ddg, EngineName: "DuckDuckGo", Shortcuts: shortcuts, Items: items}

	tests := []struct {
		name  string
		query string
		want  homeQueryResult
	}{
		{"empty", "   ", homeQueryResult{Mode: homeQueryIdle, EngineName: "DuckDuckGo", EngineURL: ddg}},
		{"filter", "Metrics", homeQueryResult{
			Mode: homeQueryFilter, EngineName: "DuckDuckGo", EngineURL: ddg,
			Matches: []string{"s1", "s3"}, Target: "https://grafana.lan",
			WebTarget: "https://duckduckgo.com/?q=Metrics",
		}},
		{"filter first match unsafe", "bad", homeQueryResult{
			Mode: homeQueryFilter, EngineName: "DuckDuckGo", EngineURL: ddg,
			Matches: []string{"s3"}, WebTarget: "https://duckduckgo.com/?q=bad",
		}},
		{"web", "hello world", homeQueryResult{
			Mode: homeQueryWeb, EngineName: "DuckDuckGo", EngineURL: ddg,
			Target: "https://duckduckgo.com/?q=hello+world", WebTarget: "https://duckduckgo.com/?q=hello+world",
		}},
		{"bang", "!gh dumber", homeQueryResult{
			Mode: homeQueryBang, EngineName: "GitHub", EngineURL: "https://github.com/search?q=%s",
			Target: "https://github.com/search?q=dumber", WebTarget: "https://duckduckgo.com/?q=%21gh+dumber",
			BangQuery: "gh",
		}},
		{"bang case-insensitive", "!GH x", homeQueryResult{
			Mode: homeQueryBang, EngineName: "GitHub", EngineURL: "https://github.com/search?q=%s",
			Target: "https://github.com/search?q=x", WebTarget: "https://duckduckgo.com/?q=%21GH+x",
			BangQuery: "GH",
		}},
		{"bare bang", "!gh", homeQueryResult{
			Mode: homeQueryBang, EngineName: "GitHub", EngineURL: "https://github.com/search?q=%s",
			WebTarget: "https://duckduckgo.com/?q=%21gh", BangQuery: "gh",
		}},
		{"hostile bang dropped", "!evil x", homeQueryResult{
			Mode: homeQueryBang, EngineName: "Evil",
			WebTarget: "https://duckduckgo.com/?q=%21evil+x", BangQuery: "evil",
		}},
		{"bang unknown", "!zz x", homeQueryResult{
			Mode: homeQueryBangUnknown, EngineName: "DuckDuckGo", EngineURL: ddg,
			Target: "https://duckduckgo.com/?q=%21zz+x", WebTarget: "https://duckduckgo.com/?q=%21zz+x",
			BangQuery: "zz",
		}},
		{"address", "example.com", homeQueryResult{
			Mode: homeQueryAddress, EngineName: "DuckDuckGo", EngineURL: ddg,
			Target: "https://example.com", WebTarget: "https://example.com",
		}},
		{"address matching a service", "github.com", homeQueryResult{
			Mode: homeQueryAddress, EngineName: "DuckDuckGo", EngineURL: ddg,
			Matches: []string{"s2"}, Target: "https://gitea.lan", WebTarget: "https://github.com",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := base
			in.Query = tt.query
			assert.Equal(t, tt.want, resolveHomeQuery(in))
		})
	}

	t.Run("bang partial candidates", func(t *testing.T) {
		in := base
		in.Shortcuts = map[string]dto.HomeSearchShortcut{
			"ghub": shortcuts["ghub"], "GHX": {URL: "https://x.example/?q=%s"}, "g": shortcuts["g"],
		}
		in.Query = "!gh"
		got := resolveHomeQuery(in)
		assert.Equal(t, homeQueryBangPartial, got.Mode)
		assert.Equal(t, "gh", got.BangQuery)
		assert.Equal(t, []string{"GHX", "ghub"}, got.Candidates)
		assert.Equal(t, "https://duckduckgo.com/?q=%21gh", got.Target)

		in.Query = "!gh x"
		got = resolveHomeQuery(in)
		assert.Equal(t, homeQueryBangUnknown, got.Mode, "a typed space ends the partial key")
		assert.Equal(t, []string{"GHX", "ghub"}, got.Candidates)
	})

	t.Run("empty engine falls back to default", func(t *testing.T) {
		got := resolveHomeQuery(homeQueryInput{Query: "hi"})
		assert.Equal(t, homeQueryWeb, got.Mode)
		assert.Equal(t, homeDefaultSearchEngine, got.EngineURL)
		assert.Equal(t, "duckduckgo.com", got.EngineName)
		assert.Equal(t, "https://duckduckgo.com/?q=hi", got.Target)
	})
}
