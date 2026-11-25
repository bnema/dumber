package homehtml

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var forbiddenTags = map[string]bool{
	"script": true, "iframe": true, "svg": true, "style": true, "math": true,
	"object": true, "embed": true, "form": true, "noscript": true, "template": true,
}

// unsafeReason re-parses out and reports the first unsafe element or
// attribute, or "" when the output is safe.
func unsafeReason(t testing.TB, out string) string {
	t.Helper()
	if strings.Contains(strings.ToLower(out), "<script") {
		return "raw <script"
	}
	ctx := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(out), ctx)
	if err != nil {
		return "unparseable: " + err.Error()
	}
	for _, n := range nodes {
		if r := unsafeNode(n); r != "" {
			return r
		}
	}
	return ""
}

func unsafeNode(n *html.Node) string {
	if n.Type == html.ElementNode {
		if forbiddenTags[n.Data] || n.Namespace != "" {
			return "tag " + n.Data
		}
		for _, a := range n.Attr {
			if r := unsafeAttr(n.Data, a); r != "" {
				return r
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if r := unsafeNode(c); r != "" {
			return r
		}
	}
	return ""
}

func unsafeAttr(tag string, a html.Attribute) string {
	key := strings.ToLower(a.Key)
	val := normalize(a.Val)
	switch {
	case strings.HasPrefix(key, "on") || key == "srcset" || key == "formaction" || a.Namespace != "",
		key == "id" || key == "name" || strings.HasPrefix(key, "data-sv-") || key == "data-sv":
		return "attr " + key
	case strings.Contains(val, "javascript:") || strings.Contains(val, "vbscript:"):
		return "script url in " + key
	case strings.HasPrefix(val, "data:") && (tag != "img" || key != "src"):
		return "data url in " + key
	case key == "style" && (strings.Contains(val, "url(") || strings.Contains(val, "expression")):
		return "style " + val
	}
	return ""
}

func TestSanitize_XSSCorpus(t *testing.T) {
	t.Parallel()
	corpus := []string{
		`<img src=x onerror=alert(1)>`,
		`<IMG SRC="javascript:alert('XSS');">`,
		`<a href="jAvAsCrIpT:alert(1)">x</a>`,
		`<a href="&#106;&#97;&#118;&#97;&#115;&#99;&#114;&#105;&#112;&#116;&#58;alert(1)">x</a>`,
		`<a href="&#x6A;avascript&colon;alert(1)">x</a>`,
		`<a href=" java	script:alert(1)">x</a>`,
		`<a href="java&#x0A;script:alert(1)">x</a>`,
		`<a href="vbscript:msgbox(1)">x</a>`,
		`<a href="data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==">x</a>`,
		`<a href="//evil.example/x">x</a>`,
		`<svg onload=alert(1)>`,
		`<svg><script>alert(1)</script></svg>`,
		`<style>body{background:expression(alert(1))}</style>`,
		`<div style="background:url(javascript:alert(1))">x</div>`,
		`<div style="width:expression(alert(1))">x</div>`,
		`<div style="color:red;background-image:url(x)">x</div>`,
		`<div style="color:\72 ed">x</div>`,
		`<div style="color:red/*x*/">x</div>`,
		`<iframe srcdoc="<script>alert(1)</script>"></iframe>`,
		`<iframe src="javascript:alert(1)"></iframe>`,
		`<math><mtext><table><mglyph><style><img src=x onerror=alert(1)>`,
		`<math><mi><malignmark><style><img src=x onerror=alert(1)></style>`,
		`<noscript><p title="</noscript><img src=x onerror=alert(1)>">`,
		`<template><img src=x onerror=alert(1)></template>`,
		`<form><button formaction="javascript:alert(1)">x</button></form>`,
		`<input autofocus onfocus=alert(1)>`,
		`<object data="javascript:alert(1)"></object>`,
		`<embed src="javascript:alert(1)">`,
		`<link rel=stylesheet href="javascript:alert(1)">`,
		`<meta http-equiv="refresh" content="0;url=javascript:alert(1)">`,
		`<base href="javascript:alert(1)//">`,
		`<img src="data:image/svg+xml;base64,PHN2Zz4=">`,
		`<img srcset="javascript:alert(1) 1x">`,
		`<img src="x" ONERROR="alert(1)">`,
		`<img/src="x"/onerror="alert(1)">`,
		`<div data-x="javascript:alert(1)">x</div>`,
		`<a xlink:href="javascript:alert(1)">x</a>`,
		`<scr<script>ipt>alert(1)</scr</script>ipt>`,
		`<<script>alert(1)//<</script>`,
		`<xmp><img src=x onerror=alert(1)></xmp>`,
		`<plaintext><img src=x onerror=alert(1)>`,
		`<details open ontoggle=alert(1)>`,
		`<body onload=alert(1)>`,
		`<table background="javascript:alert(1)"><td>x</td></table>`,
		`<!--<img src=x onerror=alert(1)>-->`,
		`<p title="&lt;img src=x onerror=alert(1)&gt;">x</p>`,
		`<a href="https://ok.example" onclick="alert(1)">x</a>`,
		`<video><source onerror="alert(1)"></video>`,
		`<div style="display:expression(alert(1))">x</div>`,
		`<div style="display:contents">x</div>`,
	}
	for _, in := range corpus {
		out := Sanitize(in)
		assert.Empty(t, unsafeReason(t, out), "input %q produced %q", in, out)
		assert.Equal(t, out, Sanitize(out), "not idempotent for %q", in)
	}
}

func TestSanitize_KeepsAllowedMarkup(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{`<div class="card" aria-label="x" data-id="7"><strong>Hi</strong></div>`,
			`<div class="card" aria-label="x" data-id="7"><strong>Hi</strong></div>`},
		{`<a href="https://example.com/a?b=1">x</a>`,
			`<a href="https://example.com/a?b=1" rel="noopener noreferrer" target="_blank">x</a>`},
		{`<a href="https://e.com" rel="opener" target="_self">x</a>`,
			`<a href="https://e.com" rel="noopener noreferrer" target="_blank">x</a>`},
		{`<img src="https://e.com/i.png" alt="i" width="16" height="16">`,
			`<img src="https://e.com/i.png" alt="i" width="16" height="16"/>`},
		{`<img src="data:image/png;base64,AAAA">`, `<img src="data:image/png;base64,AAAA"/>`},
		{`<progress value="3" max="10"></progress><meter min="0" max="1" value="0.5" low="0.2" high="0.8" optimum="1"></meter>`,
			`<progress value="3" max="10"></progress><meter min="0" max="1" value="0.5" low="0.2" high="0.8" optimum="1"></meter>`},
		{`<time datetime="2026-11-25">today</time>`, `<time datetime="2026-11-25">today</time>`},
		{`<table><tr><td colspan="2">x</td></tr></table>`,
			`<table><tbody><tr><td colspan="2">x</td></tr></tbody></table>`},
		{`<div style="color: red; position: fixed; margin-top: 4px; display: flex; display: table">x</div>`,
			`<div style="color: red; margin-top: 4px; display: flex">x</div>`},
		{`<nav><p>kept text</p></nav>`, `<p>kept text</p>`},
		{`<custom-el>text</custom-el>`, `text`},
		{`<p>a &lt; b &amp; c</p>`, `<p>a &lt; b &amp; c</p>`},
		{`<span title="x" value="1" datetime="y">s</span>`, `<span title="x">s</span>`},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, Sanitize(tc.in), "input %q", tc.in)
	}
}

func TestSanitize_DropsPageHooksAndClobbering(t *testing.T) {
	t.Parallel()
	in := `<div id="app" name="x" data-sv-action="home.openConfig" DATA-SV-REGION="home" data-sv="1" data-svc="ok" data-id="7">` +
		`<img name="getElementById" id="dumberSystemviews" src="https://e.com/i.png"><a id="x" name="y" href="https://e.com">l</a></div>`
	assert.Equal(t,
		`<div data-svc="ok" data-id="7"><img src="https://e.com/i.png"/><a href="https://e.com" rel="noopener noreferrer" target="_blank">l</a></div>`,
		Sanitize(in))
}

func TestSanitize_Caps(t *testing.T) {
	t.Parallel()
	big := strings.Repeat("a", MaxInputBytes+100)
	assert.Len(t, Sanitize(big), MaxInputBytes)

	many := strings.Repeat("<b>x</b>", MaxNodes)
	out := Sanitize(many)
	require.NotEmpty(t, out)
	assert.LessOrEqual(t, strings.Count(out, "<b>"), MaxNodes/2)
}

func FuzzSanitize(f *testing.F) {
	for _, seed := range []string{
		`<img src=x onerror=alert(1)>`, `<a href="jaVa&#x09;script:x">x</a>`,
		`<math><mtext><table><mglyph><style><img src=x onerror=alert(1)>`,
		`<noscript><p title="</noscript><img src=x onerror=alert(1)>">`,
		`<div style="color:red;background:url(x)">x</div>`, `<table><td>x<p>y</table>`,
		`<a href="https://e.com"><div>x</div></a>`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		out := Sanitize(in)
		if reason := unsafeReason(t, out); reason != "" {
			t.Fatalf("unsafe output (%s) for %q: %q", reason, in, out)
		}
		if again := Sanitize(out); again != out {
			t.Fatalf("not idempotent for %q:\n first: %q\nsecond: %q", in, out, again)
		}
	})
}
