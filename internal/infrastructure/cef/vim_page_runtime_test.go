package cef

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// vimPageRuntimeHarness is a minimal DOM shim for the embedded runtime: one
// paragraph of text and two links, a Selection with Chromium-like
// setBaseAndExtent/modify semantics over that text, and a fetch that records
// every message posted to the Go bridge.
const vimPageRuntimeHarness = `
const posted = [];
const TEXT = "Alpha bravo charlie";
const rect = (x, y, w, h) => ({ left: x, top: y, right: x + w, bottom: y + h, width: w, height: h });
class Node_ {
  constructor(type, tag) {
    this.nodeType = type; this.tagName = tag; this.children = []; this.parentElement = null;
    this.style = {}; this.attrs = {}; this.isConnected = true; this.shadowRoot = null;
    this.classList = { add() {}, remove() {} };
  }
  appendChild(child) { child.parentElement = this; this.children.push(child); return child; }
  append(...items) { for (const item of items) if (item instanceof Node_) this.appendChild(item); }
  remove() { if (this.parentElement) this.parentElement.children = this.parentElement.children.filter((c) => c !== this); }
  setAttribute(name, value) { this.attrs[name] = value; }
  closest() { return null; }
  contains(other) { for (let n = other; n; n = n.parentElement) if (n === this) return true; return false; }
  getClientRects() { return this.rect ? [this.rect] : []; }
  getBoundingClientRect() { return this.rect || rect(0, 0, 0, 0); }
  getRootNode() { return document; }
  querySelectorAll(selector) {
    const out = [];
    const visit = (node) => { for (const c of node.children) { if (c.nodeType === 1) { if (matches(c, selector)) out.push(c); visit(c); } } };
    visit(this);
    return out;
  }
  focus() { document.activeElement = this; }
  click() { posted.push({ clicked: this.href }); }
}
function matches(el, selector) {
  return selector.split(",").some((part) => {
    part = part.trim();
    if (part === "*") return true;
    if (part === "a[href]") return el.tagName === "A" && el.href;
    if (part.startsWith("[")) return false;
    const base = part.replace(/:not\(.*\)$/, "");
    return el.tagName === base.toUpperCase();
  });
}
class Element extends Node_ { constructor(tag) { super(1, tag.toUpperCase()); } }
class HTMLElement extends Element {}
class HTMLAnchorElement extends HTMLElement { constructor(href) { super("a"); this.href = href; this.target = ""; } }
class HTMLInputElement extends HTMLElement {}
class HTMLTextAreaElement extends HTMLElement {}
class HTMLSelectElement extends HTMLElement {}
const Element_ = HTMLElement;
class Text_ extends Node_ { constructor(data) { super(3, "#text"); this.data = data; } }
const html = new Element_("html");
const body = html.appendChild(new Element_("body"));
const p = body.appendChild(new Element_("p")); p.rect = rect(10, 10, 300, 20);
const text = p.appendChild(new Text_(TEXT));
const one = body.appendChild(new HTMLAnchorElement("file:///one.html")); one.rect = rect(10, 50, 40, 20);
const two = body.appendChild(new HTMLAnchorElement("https://example.com/two")); two.rect = rect(80, 50, 40, 20);
const selection = {
  anchorNode: null, anchorOffset: 0, focusNode: null, focusOffset: 0, rangeCount: 0,
  get isCollapsed() { return this.rangeCount === 0 || (this.anchorNode === this.focusNode && this.anchorOffset === this.focusOffset); },
  setBaseAndExtent(an, ao, fn, fo) { this.anchorNode = an; this.anchorOffset = ao; this.focusNode = fn; this.focusOffset = fo; this.rangeCount = 1; },
  modify(_alter, direction, granularity) {
    const step = direction === "forward" ? 1 : -1;
    if (granularity === "character") this.focusOffset = Math.max(0, Math.min(TEXT.length, this.focusOffset + step));
    else if (granularity === "word") {
      let i = this.focusOffset;
      if (step > 0) { while (i < TEXT.length && TEXT[i] !== " ") i++; while (i < TEXT.length && TEXT[i] === " ") i++; }
      else { while (i > 0 && TEXT[i - 1] === " ") i--; while (i > 0 && TEXT[i - 1] !== " ") i--; }
      this.focusOffset = i;
    } else if (granularity === "lineboundary" || granularity === "documentboundary" || granularity === "line") {
      this.focusOffset = step > 0 ? TEXT.length : 0;
    }
  },
  removeAllRanges() { this.rangeCount = 0; this.anchorNode = this.focusNode = null; },
  addRange() { this.rangeCount = 1; },
  getRangeAt() { return {}; },
  toString() { return this.rangeCount ? TEXT.slice(Math.min(this.anchorOffset, this.focusOffset), Math.max(this.anchorOffset, this.focusOffset)) : ""; },
};
global.Node = { TEXT_NODE: 3, ELEMENT_NODE: 1 };
global.NodeFilter = { SHOW_ELEMENT: 1, SHOW_TEXT: 4 };
global.Element = Element; global.HTMLElement = HTMLElement; global.HTMLAnchorElement = HTMLAnchorElement;
global.HTMLInputElement = HTMLInputElement; global.HTMLTextAreaElement = HTMLTextAreaElement; global.HTMLSelectElement = HTMLSelectElement;
global.document = {
  body, documentElement: html, activeElement: body,
  createElement: (tag) => new HTMLElement(tag),
  createRange: () => ({ setStart() {}, collapse() {}, selectNodeContents() {}, getBoundingClientRect: () => rect(10, 10, 300, 20) }),
  createTreeWalker: (root) => {
    const nodes = []; const visit = (n) => { nodes.push(n); for (const c of n.children) visit(c); }; visit(root);
    let i = 0; return { currentNode: nodes[0], nextNode: () => nodes[++i] || null };
  },
  querySelectorAll: (selector) => html.querySelectorAll(selector),
  elementFromPoint: (x, y) => [two, one, p].find((el) => el.rect && x >= el.rect.left && x < el.rect.right && y >= el.rect.top && y < el.rect.bottom) || body,
};
global.window = {
  innerWidth: 800, innerHeight: 600, scrollBy() {},
  getSelection: () => selection,
  getComputedStyle: () => ({ display: "block", visibility: "visible" }),
  fetch: (url, init) => { posted.push(JSON.parse(Buffer.from(init.headers["X-Dumber-Body"], "base64").toString("latin1"))); return Promise.resolve(); },
};
global.btoa = (s) => Buffer.from(s, "binary").toString("base64");
global.unescape = (s) => decodeURIComponent(s);
global.setTimeout = (fn) => { fn(); return 0; };
`

// runVimPageRuntime executes the embedded runtime inside the harness, then
// the given driver script, and returns every message posted to the bridge
// plus the driver's own output appended to the "posted" array.
func runVimPageRuntime(t *testing.T, driver string) []map[string]any {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required to execute the vim page runtime")
	}
	script := vimPageRuntimeHarness + "\n" + vimPageRuntimeTemplateJS + "\n" + driver +
		"\nprocess.stdout.write(JSON.stringify(posted));"
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	require.NoError(t, err, "runtime failed: %s", output)
	var posted []map[string]any
	require.NoError(t, json.Unmarshal(output, &posted), "runtime output: %s", output)
	return posted
}

func TestVimPageRuntimeVisualExtendsAndYanks(t *testing.T) {
	posted := runVimPageRuntime(t, `
const vp = window.__dumberVimPage;
vp.start("tok", { kind: "visual", color: "#fbbf24" });
posted.push({ started: selection.toString() });
for (const key of ["l", "l", "w"]) vp.key("tok", key);
posted.push({ extended: selection.toString() });
vp.key("tok", "y");
posted.push({ after: selection.rangeCount });
`)
	require.Len(t, posted, 4)
	require.Equal(t, "A", posted[0]["started"], "visual starts on the first visible character")
	require.Equal(t, "Alpha ", posted[1]["extended"], "motions extend the selection")
	require.Equal(t, map[string]any{"token": "tok", "type": "copy", "text": "Alpha "}, posted[2])
	require.EqualValues(t, 0, posted[3]["after"], "yank clears the selection")
}

func TestVimPageRuntimeVisualExitsOnVAndEscapeWithoutCopy(t *testing.T) {
	for _, key := range []string{"v", "<Escape>"} {
		posted := runVimPageRuntime(t, `
const vp = window.__dumberVimPage;
vp.start("tok", { kind: "visual" });
vp.key("tok", "l");
vp.key("tok", `+jsString(key)+`);
`)
		require.Len(t, posted, 1, "key %s", key)
		require.Equal(t, "end", posted[0]["type"], "key %s", key)
	}
}

func TestVimPageRuntimeStartDoesNotEndImmediately(t *testing.T) {
	posted := runVimPageRuntime(t, `
window.__dumberVimPage.start("tok", { kind: "visual" });
window.__dumberVimPage.start("hint", { kind: "hint-follow" });
`)
	require.Empty(t, posted, "starting an interaction with visible content posts nothing until a key ends it")
}

func TestVimPageRuntimeHintsFollowAndYank(t *testing.T) {
	posted := runVimPageRuntime(t, `
const vp = window.__dumberVimPage;
vp.start("a", { kind: "hint-follow" });
posted.push({ labels: html.querySelectorAll("span").map((s) => s.textContent) });
vp.key("a", "a");
vp.start("b", { kind: "hint-yank-url" });
vp.key("b", "s");
vp.start("c", { kind: "hint-follow-new" });
vp.key("c", "a");
`)
	require.Len(t, posted, 5)
	require.ElementsMatch(t, []any{"s", "a"}, posted[0]["labels"], "two visible links get one-letter labels")
	require.Equal(t, "https://example.com/two", posted[1]["clicked"], "follow clicks the chosen link")
	require.Equal(t, "end", posted[2]["type"])
	require.Equal(t, map[string]any{"token": "b", "type": "copy", "text": "file:///one.html"}, posted[3])
	require.Equal(t, map[string]any{"token": "c", "type": "open-new", "url": "https://example.com/two"}, posted[4])
}

func TestVimPageRuntimeYankParagraph(t *testing.T) {
	posted := runVimPageRuntime(t, `
p.innerText = TEXT;
window.__dumberVimPage.start("tok", { kind: "yank", object: "paragraph" });
`)
	require.Len(t, posted, 1)
	require.Equal(t, "copy", posted[0]["type"])
	require.True(t, strings.HasPrefix(posted[0]["text"].(string), "Alpha"))
}

func TestVimPageRuntimeStaleTokenKeyReleasesCapture(t *testing.T) {
	posted := runVimPageRuntime(t, `
const vp = window.__dumberVimPage;
vp.start("live", { kind: "visual" });
vp.key("stale", "j");
posted.push({ liveSelection: selection.rangeCount });
`)
	require.Len(t, posted, 2)
	require.Equal(t, map[string]any{"token": "stale", "type": "end"}, posted[0])
	require.EqualValues(t, 1, posted[1]["liveSelection"], "a stale key must not disturb the live interaction")
}
