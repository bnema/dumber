package updater

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestDownloadRedirects(t *testing.T) {
	for _, stage := range []string{"archive", "checksums"} {
		for _, target := range []string{
			"https://objects.githubusercontent.com/asset",
			"https://release-assets.githubusercontent.com/asset",
			"https://github.com/asset",
			"https://git.bnema.dev/asset",
			"https://evil.example/asset",
			"http://github.com/asset",
			"https://user@github.com/asset",
		} {
			t.Run(stage+"/"+target, func(t *testing.T) {
				const asset = "dumber_linux_x86_64.tar.gz"
				const content = "archive"
				checksum := fmt.Sprintf("%x  %s\n", sha256.Sum256([]byte(content)), asset)
				allowed := !strings.Contains(target, "evil.example") && !strings.HasPrefix(target, "http:") && !strings.Contains(target, "user@")
				followed := false
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/asset" {
						followed = true
						if stage == "checksums" {
							_, _ = fmt.Fprint(w, checksum)
						} else {
							_, _ = fmt.Fprint(w, content)
						}
						return
					}
					isChecksum := strings.HasSuffix(r.URL.Path, "checksums.txt")
					if (stage == "checksums") == isChecksum {
						http.Redirect(w, r, target, http.StatusFound)
					} else if isChecksum {
						_, _ = fmt.Fprint(w, checksum)
					} else {
						_, _ = fmt.Fprint(w, content)
					}
				}))
				defer srv.Close()
				local, err := url.Parse(srv.URL)
				if err != nil {
					t.Fatal(err)
				}
				d := NewGitHubDownloader()
				// Route every attempted hop locally; no public network is used.
				d.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
					clone := req.Clone(req.Context())
					clone.URL.Scheme, clone.URL.Host = local.Scheme, local.Host
					return srv.Client().Transport.RoundTrip(clone)
				})
				_, err = d.Download(context.Background(), githubDownloadBase+"/v1/"+asset, t.TempDir())
				if (err == nil) != allowed {
					t.Fatalf("allowed=%v, err=%v", allowed, err)
				}
				if followed != allowed {
					t.Fatalf("followed=%v, allowed=%v", followed, allowed)
				}
			})
		}
	}
}

func TestDownloadRedirectLimit(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://github.com/asset", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkDownloadRedirect(req, make([]*http.Request, 10)); err != nil {
		t.Fatal(err)
	}
	if err := checkDownloadRedirect(req, make([]*http.Request, 11)); err == nil {
		t.Fatal("expected redirect limit error")
	}
	if err := validateDownloadURL("https://objects.githubusercontent.com/releases/asset"); err == nil {
		t.Fatal("CDN must not be an initial URL")
	}
}

func TestChecksumsURLForEscapedTag(t *testing.T) {
	const archive = "https://git.bnema.dev/dumber/releases/download/release%2Fv1/dumber_linux_x86_64.tar.gz"
	u, err := url.Parse(archive)
	if err != nil {
		t.Fatal(err)
	}
	const want = "https://git.bnema.dev/dumber/releases/download/release%2Fv1/checksums.txt"
	if got := checksumsURLFor(u); got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestMetadataLimits(t *testing.T) {
	oversized := `{"tag_name":"v2.0.0","body":"` + strings.Repeat("x", maxReleaseJSONSize) + `"}`
	primary := releaseServer(t, http.StatusOK, oversized)
	fallback := releaseServer(t, http.StatusOK, `{"tag_name":"v3.0.0"}`)
	checker := newTestChecker(releaseSource{apiURL: primary.URL}, releaseSource{apiURL: fallback.URL, downloadBase: githubDownloadBase})
	if _, err := checker.fetchLatestRelease(context.Background(), checker.sources[0], "1.0.0"); err == nil {
		t.Fatal("expected oversized JSON error")
	}
	info, err := checker.CheckForUpdate(context.Background(), "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if info.LatestVersion != "3.0.0" {
		t.Fatalf("fallback version = %s", info.LatestVersion)
	}
	// Short valid lines ensure overflow is detected independently of Scanner's line limit.
	checksums := releaseServer(t, http.StatusOK, strings.Repeat("abc  asset\n", maxChecksumsSize/10+1))
	if _, err := NewGitHubDownloader().fetchChecksums(context.Background(), checksums.URL); err == nil {
		t.Fatal("expected oversized checksums error")
	}
}

func TestAPIRedirects(t *testing.T) {
	for _, target := range []string{"https://git.bnema.dev/next", "https://api.github.com/next", "https://github.com/next", "https://evil.invalid/next", "http://api.github.com/next", "https://user@api.github.com/next"} {
		t.Run(target, func(t *testing.T) {
			allowed := target == "https://git.bnema.dev/next" || target == "https://api.github.com/next"
			checker := NewGitHubChecker()
			var targetCalls, fallbackCalls int
			checker.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.String() {
				case ermiteAPIURL:
					resp := testResponse(http.StatusFound, "")
					resp.Header.Set("Location", target)
					return resp, nil
				case githubAPIURL:
					fallbackCalls++
					return testResponse(http.StatusOK, `{"tag_name":"v3.0.0"}`), nil
				default:
					targetCalls++
					return testResponse(http.StatusOK, `{"tag_name":"v2.0.0"}`), nil
				}
			})
			info, err := checker.CheckForUpdate(context.Background(), "1.0.0")
			if err != nil {
				t.Fatal(err)
			}
			if allowed {
				if targetCalls != 1 || fallbackCalls != 0 || info.LatestVersion != "2.0.0" {
					t.Fatalf("target=%d fallback=%d version=%s", targetCalls, fallbackCalls, info.LatestVersion)
				}
			} else if targetCalls != 0 || fallbackCalls != 1 || info.LatestVersion != "3.0.0" {
				t.Fatalf("rejected redirect: target=%d fallback=%d version=%s", targetCalls, fallbackCalls, info.LatestVersion)
			}
		})
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ermiteAPIURL, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkAPIRedirect(req, make([]*http.Request, 10)); err != nil {
		t.Fatal(err)
	}
	if err := checkAPIRedirect(req, make([]*http.Request, 11)); err == nil {
		t.Fatal("expected API redirect limit error")
	}
}

func TestReadMetadataExactLimit(t *testing.T) {
	for _, limit := range []int64{maxReleaseJSONSize, maxChecksumsSize} {
		input := strings.Repeat("x", int(limit))
		data, err := readMetadata(strings.NewReader(input), limit)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != input {
			t.Fatalf("read %d bytes, want %d", len(data), limit)
		}
	}
}
