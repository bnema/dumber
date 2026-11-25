package entity

import (
	"strings"
	"testing"
)

func TestIsBlockedEmbedURL(t *testing.T) {
	tests := []struct {
		raw     string
		blocked bool
	}{
		{"https://grafana.local/d/x", false},
		{"http://192.168.1.10:3000/", false},
		{"http://[::1]:8080/", false},
		{"https://bücher.example/", false},
		{"http://dumber.invalid/", true},
		{"http://DUMBER.invalid./", true},
		{"http://x.dumber.invalid/", true},
		{"http://ｄｕｍｂｅｒ.invalid/", true},
		{"http://dumber。invalid/", true},
		{"http://dumber．invalid/", true},
		{"http://ｘ.ＤＵＭＢＥＲ｡ｉｎｖａｌｉｄ/", true},
		{"http://dumber%2Einvalid/", true},
		{"http://%64umber.invalid/", true},
		{"http:///path", true},
	}
	for _, tt := range tests {
		if got := IsBlockedEmbedURL(tt.raw); got != tt.blocked {
			t.Errorf("IsBlockedEmbedURL(%q) = %v, want %v", tt.raw, got, tt.blocked)
		}
	}
}

func TestHomeValidateEmbedUnicodeHost(t *testing.T) {
	for _, raw := range []string{"http://ｄｕｍｂｅｒ.invalid/", "http://dumber。invalid/"} {
		l := validHomeLayout()
		l.Normalize()
		l.Groups[0].Services[1].Widget.URL = raw
		errs := l.Validate()
		if len(errs) != 1 || errs[0].Path != "groups[0].services[1].widget.url" {
			t.Errorf("%s: errors = %v", raw, errs)
		}
	}
}

func TestExpandHomeJSONBodyEnv(t *testing.T) {
	env := map[string]string{"TOK": `a"b\c`, "N": "42", "BAD": `1, "admin": true`}
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }

	out, err := ExpandHomeJSONBodyEnv(`{"t":"{{env.TOK}}","n":{{env.N}},"q":"{{input.q}}"}`, lookup)
	if err != nil || out != `{"t":"a\"b\\c","n":42,"q":"{{input.q}}"}` {
		t.Fatalf("out=%s err=%v", out, err)
	}
	_, err = ExpandHomeJSONBodyEnv(`{"n":{{env.BAD}}}`, lookup)
	if err == nil || strings.Contains(err.Error(), "admin") {
		t.Fatalf("injected raw value: err = %v", err)
	}
	if _, err = ExpandHomeJSONBodyEnv(`{"n":{{env.MISSING}}}`, lookup); err == nil {
		t.Fatal("missing variable accepted")
	}
}
