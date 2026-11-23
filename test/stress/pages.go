//go:build stress

package stress

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

const (
	hostIP    = "127.0.0.1"
	hostLocal = "localhost"
)

// pageServer serves local multi-site pages on 127.0.0.1 and localhost.
type pageServer struct {
	srv  *httptest.Server
	port string
}

// newPageServer starts a page server on 127.0.0.1:0, closed on test cleanup.
func newPageServer(t *testing.T) *pageServer {
	t.Helper()
	ps := &pageServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/page", ps.handlePage)
	mux.HandleFunc("/redirect", ps.handleRedirect)
	mux.HandleFunc("/popup", ps.handlePopup)
	mux.HandleFunc("/iframe", ps.handleIframe)
	mux.HandleFunc("/alloc", ps.handleAlloc)

	ln, err := net.Listen("tcp", net.JoinHostPort(hostIP, "0"))
	if err != nil {
		t.Fatalf("listen page server: %v", err)
	}
	ps.srv = &httptest.Server{Listener: ln, Config: &http.Server{Handler: mux}}
	ps.srv.Start()
	ps.port = strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	t.Cleanup(ps.srv.Close)
	return ps
}

// url builds http://<host>:<port><path>.
func (ps *pageServer) url(host, path string) string {
	return "http://" + net.JoinHostPort(host, ps.port) + path
}

// otherHost returns the host the request did not target.
func (ps *pageServer) otherHost(r *http.Request) string {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host == hostIP {
		return hostLocal
	}
	return hostIP
}

func writeHTML(w http.ResponseWriter, title, head, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html><head><meta charset="utf-8"><title>%s</title>
<style>html,body{margin:0;width:100%%;height:100%%;background:#2e8b57;color:#fff;font-family:sans-serif}</style>
%s</head><body>%s</body></html>
`, title, head, body)
}

func (ps *pageServer) handlePage(w http.ResponseWriter, r *http.Request) {
	o := ps.otherHost(r)
	body := fmt.Sprintf(`<h1>stress page</h1><ul>
<li><a id="page" href="%s">page</a></li>
<li><a id="redirect" href="%s">redirect</a></li>
<li><a id="popup" href="%s">popup</a></li>
<li><a id="iframe" href="%s">iframe</a></li>
<li><a id="alloc" href="%s">alloc</a></li>
</ul>`, ps.url(o, "/page"), ps.url(o, "/redirect"), ps.url(o, "/popup"),
		ps.url(o, "/iframe"), ps.url(o, "/alloc"))
	writeHTML(w, "page", "", body)
}

func (ps *pageServer) handleRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, ps.url(ps.otherHost(r), "/page"), http.StatusFound)
}

func (ps *pageServer) handlePopup(w http.ResponseWriter, r *http.Request) {
	target := strconv.Quote(ps.url(ps.otherHost(r), "/page"))
	head := fmt.Sprintf(`<script>window.addEventListener("load",function(){window.open(%s,"_blank");});</script>`, target)
	writeHTML(w, "popup", head, "<h1>popup</h1>")
}

func (ps *pageServer) handleIframe(w http.ResponseWriter, r *http.Request) {
	src := ps.url(ps.otherHost(r), "/page")
	body := fmt.Sprintf(`<iframe src="%s" style="width:48%%;height:90%%;border:0"></iframe>
<iframe src="%s" style="width:48%%;height:90%%;border:0"></iframe>`, src, src)
	writeHTML(w, "iframe", "", body)
}

func (ps *pageServer) handleAlloc(w http.ResponseWriter, r *http.Request) {
	target := strconv.Quote(ps.url(ps.otherHost(r), "/page"))
	// ~50 MB: 20 steps of 100ms, each allocating 2.5 MB of float64 (8 bytes).
	head := fmt.Sprintf(`<script>
(function(){
  var keep=[], steps=20, perStep=327680, i=0;
  var timer=setInterval(function(){
    var a=new Array(perStep);
    for(var j=0;j<perStep;j++){a[j]=j+0.5;}
    keep.push(a);
    if(++i>=steps){clearInterval(timer); keep=null; location.href=%s;}
  },100);
})();
</script>`, target)
	writeHTML(w, "alloc", head, "<h1>alloc</h1>")
}
