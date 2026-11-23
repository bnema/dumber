//go:build stress

package stress

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func fetch(t *testing.T, ps *pageServer, host, path string) (status int, location, body string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ps.url(hostIP, path), http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = strings.TrimPrefix(ps.url(host, ""), "http://")
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, resp.Header.Get("Location"), string(b)
}

func TestPageServer(t *testing.T) {
	ps := newPageServer(t)

	status, _, body := fetch(t, ps, hostIP, "/page")
	if status != http.StatusOK || !strings.Contains(body, "#2e8b57") {
		t.Fatalf("/page: status %d, body missing #2e8b57", status)
	}

	status, loc, _ := fetch(t, ps, hostIP, "/redirect")
	if want := ps.url(hostLocal, "/page"); status != http.StatusFound || loc != want {
		t.Fatalf("/redirect: status %d location %q, want 302 %q", status, loc, want)
	}

	for _, path := range []string{"/popup", "/iframe"} {
		_, _, body = fetch(t, ps, hostIP, path)
		if !strings.Contains(body, ps.url(hostLocal, "/page")) {
			t.Fatalf("%s from %s does not reference %s", path, hostIP, hostLocal)
		}
		_, _, body = fetch(t, ps, hostLocal, path)
		if !strings.Contains(body, ps.url(hostIP, "/page")) {
			t.Fatalf("%s from %s does not reference %s", path, hostLocal, hostIP)
		}
	}
}
