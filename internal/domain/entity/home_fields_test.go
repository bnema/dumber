package entity

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestExtractHomeField(t *testing.T) {
	var doc any
	raw := `{"data":{"items":[{"count":3},{"count":5}]},"MovieCount":42,"a.b":{"c":"dot"},"list":[[1,2]]}`
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	var arr any
	if err := json.Unmarshal([]byte(`[{"name":"x"},{"name":"y"},{"name":"z"}]`), &arr); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		doc    any
		path   string
		want   any
		wantOK bool
	}{
		{doc, "data.items[0].count", 3.0, true},
		{doc, "data.items[1].count", 5.0, true},
		{doc, "MovieCount", 42.0, true},
		{doc, `"a.b".c`, "dot", true},
		{doc, "list[0][1]", 2.0, true},
		{arr, "[2].name", "z", true},
		{doc, "data.items[2].count", nil, false},
		{doc, "data.missing", nil, false},
		{doc, "MovieCount.x", nil, false},
		{doc, "data..items", nil, false},
		{doc, "data.", nil, false},
		{doc, "data.items[x]", nil, false},
		{doc, "data.items[-1]", nil, false},
		{doc, "", nil, false},
		{arr, "name", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, ok := ExtractHomeField(tt.doc, tt.path)
			if ok != tt.wantOK || (ok && got != tt.want) {
				t.Fatalf("ExtractHomeField(%q) = %v, %v; want %v, %v", tt.path, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestFormatHomeField(t *testing.T) {
	tests := []struct {
		v      any
		format HomeFieldFormat
		want   string
	}{
		{"hello", HomeFormatText, "hello"},
		{1234567.0, HomeFormatText, "1234567"},
		{nil, HomeFormatText, ""},
		{true, "", "true"},
		{1234567.891, HomeFormatNumber, "1,234,567.89"},
		{-1234.5, HomeFormatNumber, "-1,234.5"},
		{999.0, HomeFormatNumber, "999"},
		{"2500", HomeFormatNumber, "2,500"},
		{"n/a", HomeFormatNumber, "n/a"},
		{0.42, HomeFormatPercent, "42%"},
		{1.0, HomeFormatPercent, "100%"},
		{87.456, HomeFormatPercent, "87.46%"},
		{512.0, HomeFormatBytes, "512 B"},
		{1536.0, HomeFormatBytes, "1.5 KiB"},
		{5.0 * 1024 * 1024, HomeFormatBytes, "5 MiB"},
		{3.25 * 1024 * 1024 * 1024, HomeFormatBytes, "3.3 GiB"},
		{3720.0, HomeFormatDuration, "1h 2m"},
		{45.0, HomeFormatDuration, "45s"},
		{90061.0, HomeFormatDuration, "1d 1h"},
		{0.0, HomeFormatDuration, "0s"},
	}
	for _, tt := range tests {
		if got := FormatHomeField(tt.v, tt.format); got != tt.want {
			t.Errorf("FormatHomeField(%v, %q) = %q, want %q", tt.v, tt.format, got, tt.want)
		}
	}
}

func TestExpandHomeEnv(t *testing.T) {
	env := map[string]string{"TOKEN": "s3cret", "HOST": "nas.lan"}
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }

	got, err := ExpandHomeEnv("Bearer {{env.TOKEN}} @ {{ env.HOST }} {{other}}", lookup)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Bearer s3cret @ nas.lan {{other}}" {
		t.Fatalf("got %q", got)
	}

	_, err = ExpandHomeEnv("{{env.TOKEN}}:{{env.MISSING}}", lookup)
	if err == nil {
		t.Fatal("expected error for unknown var")
	}
	if !strings.Contains(err.Error(), "MISSING") || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("error = %q: must name MISSING and not leak values", err)
	}
}

func TestExpandHomeInputs(t *testing.T) {
	got, err := ExpandHomeInputs(
		"/search?q={{input.query}}&p={{ input.page }}",
		map[string]string{"query": "a b&c"},
		[]string{"query", "page"},
		url.QueryEscape,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/search?q=a+b%26c&p=" {
		t.Fatalf("got %q", got)
	}

	_, err = ExpandHomeInputs("{{input.evil}}", map[string]string{"evil": "x"}, []string{"query"}, nil)
	if err == nil || !strings.Contains(err.Error(), "evil") {
		t.Fatalf("err = %v, want undeclared input error", err)
	}
}
