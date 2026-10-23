package updater

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"testing"
)

// Exercise Download without weakening its production host validation: only the
// test transport routes the allowed public URL to a local server.
func TestDownload_SourceAndTag(t *testing.T) {
	for _, base := range []string{ermiteDownloadBase, githubDownloadBase} {
		t.Run(base, func(t *testing.T) {
			const asset = "dumber_linux_x86_64.tar.gz"
			const content = "test archive content"
			archiveURL := base + "/v2.3.4/" + asset
			parsed, err := url.Parse(archiveURL)
			if err != nil {
				t.Fatal(err)
			}
			checksumPath := parsed.Path[:len(parsed.Path)-len(asset)] + "checksums.txt"
			var paths []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				switch r.URL.Path {
				case checksumPath:
					_, _ = fmt.Fprintf(w, "%x  %s\n", sha256.Sum256([]byte(content)), asset)
				case parsed.Path:
					_, _ = fmt.Fprint(w, content)
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			local, err := url.Parse(srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			d := NewGitHubDownloader()
			d.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host != parsed.Host {
					t.Errorf("unexpected source: %s", req.URL.Host)
				}
				clone := req.Clone(req.Context())
				clone.URL.Scheme, clone.URL.Host = local.Scheme, local.Host
				return srv.Client().Transport.RoundTrip(clone)
			})
			archive, err := d.Download(context.Background(), archiveURL, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(archive)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != content {
				t.Fatalf("archive = %q", got)
			}
			if want := []string{checksumPath, parsed.Path}; !reflect.DeepEqual(paths, want) {
				t.Fatalf("requested paths = %v, want %v", paths, want)
			}
		})
	}
}
