// DOM shim for the Vim page runtime tests. It models a page of single-line
// paragraphs (10px per character, 20px lines) with Chromium-like Selection and
// Range semantics: crossing a paragraph boundary is one character step that
// adds one "\n" to the selected text. Native "word" granularity is a trap: the
// runtime must implement word motions itself (Vimium #1441).
const posted = [];
const scrolls = [];
const listeners = {};
const CHAR_WIDTH = 10;
const LINE_HEIGHT = 20;
const rect = (x, y, w, h) => ({ left: x, top: y, right: x + w, bottom: y + h, width: w, height: h });

class Node_ {
  constructor(type, tag) {
    this.nodeType = type; this.tagName = tag; this.children = []; this.parentElement = null;
    // style.cssText assignments are parsed so tests can read properties.
    const style = {};
    Object.defineProperty(style, "cssText", { enumerable: false, set(text) {
      for (const decl of text.split(";")) { const i = decl.indexOf(":"); if (i > 0) style[decl.slice(0, i).trim()] = decl.slice(i + 1).trim(); }
    } });
    this.style = style; this.attrs = {}; this.isConnected = true; this.shadowRoot = null;
  }
  appendChild(child) { child.parentElement = this; this.children.push(child); return child; }
  append(...items) { for (const item of items) if (item instanceof Node_) this.appendChild(item); }
  remove() { if (this.parentElement) this.parentElement.children = this.parentElement.children.filter((c) => c !== this); this.parentElement = null; }
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
class Text_ extends Node_ { constructor(data) { super(3, "#text"); this.data = data; } }

const html = new HTMLElement("html");
const body = html.appendChild(new HTMLElement("body"));
const texts = [];
function paragraph(data, top) {
  const p = body.appendChild(new HTMLElement("p"));
  p.rect = rect(10, top, 600, LINE_HEIGHT);
  const text = p.appendChild(new Text_(data));
  texts.push(text);
  return { p, text };
}
const LONG = "The quick brown fox jumps over the lazy dog near the river";
const first = paragraph("Alpha bravo charlie", 10);
const second = paragraph(LONG, 40);
const third = paragraph("Third line, end.", 70);
const far = paragraph("Far below the fold", 2000);
const p = first.p; // legacy name used by the yank test
const one = body.appendChild(new HTMLAnchorElement("file:///one.html")); one.rect = rect(10, 120, 40, 20);
const two = body.appendChild(new HTMLAnchorElement("https://example.com/two")); two.rect = rect(80, 120, 40, 20);
const TEXT = first.text.data;

const index = (node) => texts.indexOf(node);
const order = (node, offset) => index(node) * 100000 + offset;
class Range_ {
  constructor() { this.startContainer = this.endContainer = null; this.startOffset = this.endOffset = 0; }
  get collapsed() { return this.startContainer === this.endContainer && this.startOffset === this.endOffset; }
  setStart(node, offset) {
    this.startContainer = node; this.startOffset = offset;
    if (!this.endContainer || order(node, offset) > order(this.endContainer, this.endOffset)) { this.endContainer = node; this.endOffset = offset; }
  }
  setEnd(node, offset) {
    this.endContainer = node; this.endOffset = offset;
    if (!this.startContainer || order(node, offset) < order(this.startContainer, this.startOffset)) { this.startContainer = node; this.startOffset = offset; }
  }
  collapse(toStart) { if (toStart) { this.endContainer = this.startContainer; this.endOffset = this.startOffset; } else { this.startContainer = this.endContainer; this.startOffset = this.endOffset; } }
  selectNodeContents(node) { this.startContainer = this.endContainer = node; this.startOffset = 0; this.endOffset = node.data ? node.data.length : 0; }
  getClientRects() {
    const node = this.startContainer;
    const box = node && node.parentElement && node.parentElement.rect;
    if (!box) return [];
    const from = this.startOffset;
    const to = this.endContainer === node ? this.endOffset : node.data.length;
    return [rect(box.left + from * CHAR_WIDTH, box.top, (to - from) * CHAR_WIDTH, LINE_HEIGHT)];
  }
  getBoundingClientRect() { return this.getClientRects()[0] || rect(0, 0, 0, 0); }
}

const selection = {
  anchorNode: null, anchorOffset: 0, focusNode: null, focusOffset: 0, rangeCount: 0,
  get isCollapsed() { return this.rangeCount === 0 || (this.anchorNode === this.focusNode && this.anchorOffset === this.focusOffset); },
  setBaseAndExtent(an, ao, fn, fo) { this.anchorNode = an; this.anchorOffset = ao; this.focusNode = fn; this.focusOffset = fo; this.rangeCount = 1; },
  collapse(node, offset) { this.setBaseAndExtent(node, offset, node, offset); },
  setFocus(node, offset) { this.focusNode = node; this.focusOffset = offset; },
  modify(alter, direction, granularity) {
    if (granularity === "word") throw new Error("native word granularity must not be used");
    const step = direction === "forward" ? 1 : -1;
    let i = index(this.focusNode);
    let o = this.focusOffset;
    const len = (n) => texts[n].data.length;
    if (granularity === "character") {
      if (step > 0) { if (o < len(i)) o++; else if (i < texts.length - 1) { i++; o = 0; } }
      else if (o > 0) o--; else if (i > 0) { i--; o = len(i); }
    } else if (granularity === "lineboundary") {
      o = step > 0 ? len(i) : 0;
    } else if (granularity === "line") {
      const j = i + step;
      if (j >= 0 && j < texts.length) { i = j; o = Math.min(o, len(j)); } else o = step > 0 ? len(i) : 0;
    } else if (granularity === "documentboundary") {
      i = step > 0 ? texts.length - 1 : 0; o = step > 0 ? len(i) : 0;
    } else if (granularity === "paragraph") {
      if (step > 0) { if (o < len(i)) o = len(i); else if (i < texts.length - 1) { i++; o = len(i); } }
      else if (o > 0) o = 0; else if (i > 0) { i--; o = 0; }
    } else if (granularity === "sentence") {
      o = step > 0 ? len(i) : 0;
    }
    this.focusNode = texts[i]; this.focusOffset = o;
    if (alter === "move") { this.anchorNode = this.focusNode; this.anchorOffset = this.focusOffset; }
  },
  removeAllRanges() { this.rangeCount = 0; this.anchorNode = this.focusNode = null; },
  addRange(range) { this.setBaseAndExtent(range.startContainer, range.startOffset, range.endContainer, range.endOffset); },
  getRangeAt() {
    const range = new Range_();
    const forward = order(this.anchorNode, this.anchorOffset) <= order(this.focusNode, this.focusOffset);
    const [s, e] = forward ? [[this.anchorNode, this.anchorOffset], [this.focusNode, this.focusOffset]] : [[this.focusNode, this.focusOffset], [this.anchorNode, this.anchorOffset]];
    range.startContainer = s[0]; range.startOffset = s[1]; range.endContainer = e[0]; range.endOffset = e[1];
    return range;
  },
  toString() {
    if (!this.rangeCount) return "";
    const r = this.getRangeAt();
    const a = index(r.startContainer);
    const b = index(r.endContainer);
    if (a === b) return r.startContainer.data.slice(r.startOffset, r.endOffset);
    const parts = [r.startContainer.data.slice(r.startOffset)];
    for (let n = a + 1; n < b; n++) parts.push(texts[n].data);
    parts.push(r.endContainer.data.slice(0, r.endOffset));
    return parts.join("\n");
  },
};

global.Node = { TEXT_NODE: 3, ELEMENT_NODE: 1 };
global.NodeFilter = { SHOW_ELEMENT: 1, SHOW_TEXT: 4 };
global.Element = Element; global.HTMLElement = HTMLElement; global.HTMLAnchorElement = HTMLAnchorElement;
global.HTMLInputElement = HTMLInputElement; global.HTMLTextAreaElement = HTMLTextAreaElement; global.HTMLSelectElement = HTMLSelectElement;
global.document = {
  body, documentElement: html, activeElement: body,
  createElement: (tag) => new HTMLElement(tag),
  createRange: () => new Range_(),
  createTreeWalker: (root) => {
    const nodes = []; const visit = (n) => { nodes.push(n); for (const c of n.children) visit(c); }; visit(root);
    let i = 0; return { currentNode: nodes[0], nextNode: () => nodes[++i] || null };
  },
  querySelectorAll: (selector) => html.querySelectorAll(selector),
  elementFromPoint: (x, y) => [two, one, ...texts.map((t) => t.parentElement)].find((el) => el.rect && x >= el.rect.left && x < el.rect.right && y >= el.rect.top && y < el.rect.bottom) || body,
};
global.window = {
  innerWidth: 800, innerHeight: 600,
  scrollBy(x, y) { scrolls.push([x, y]); },
  addEventListener(type, fn) { (listeners[type] = listeners[type] || []).push(fn); },
  removeEventListener(type, fn) { listeners[type] = (listeners[type] || []).filter((f) => f !== fn); },
  getSelection: () => selection,
  getComputedStyle: () => ({ display: "block", visibility: "visible" }),
  fetch: (url, init) => { posted.push(JSON.parse(Buffer.from(init.headers["X-Dumber-Body"], "base64").toString("latin1"))); return Promise.resolve(); },
};
global.btoa = (s) => Buffer.from(s, "binary").toString("base64");
global.unescape = (s) => decodeURIComponent(s);
global.setTimeout = (fn) => { fn(); return 0; };

// Helpers for drivers.
const pos = (node, offset) => index(node) + ":" + offset;
const snap = () => ({
  text: selection.toString(),
  anchor: selection.rangeCount ? pos(selection.anchorNode, selection.anchorOffset) : null,
  focus: selection.rangeCount ? pos(selection.focusNode, selection.focusOffset) : null,
});
const overlays = (name) => html.children.filter((c) => c.attrs && name in c.attrs);
const cursor = () => overlays("data-dumber-vim-caret")[0] || null;
const hintLabels = () => overlays("data-dumber-vim-hints").flatMap((root) => root.children.map((s) => s.textContent));
const fire = (type) => (listeners[type] || []).slice().forEach((fn) => fn({}));
const modes = () => posted.filter((m) => m.type === "mode").map((m) => m.mode);
const vp = () => window.__dumberVimPage;
const keys = (token, ...list) => list.forEach((k) => window.__dumberVimPage.key(token, k));
const rec = (name, extra) => posted.push(Object.assign({ r: name }, extra));
// startCaret begins the visual flow and places the caret at the start of the
// paragraph with the given hint label ("s", "a", "d" for the first, second,
// third visible paragraph).
const startCaret = (token, label, color) => {
  window.__dumberVimPage.start(token, { kind: "visual", color: color || "#fbbf24" });
  window.__dumberVimPage.key(token, label);
};
// S records a named snapshot of the selection (and any extra fields).
const S = (n, extra) => posted.push(Object.assign({ n }, snap(), extra));
