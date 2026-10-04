// Dumber Vim Mode page runtime: link hints, caret and visual selection,
// text-object yanks. Installed once per document; Go calls
// window.__dumberVimPage.* with a per-interaction token and accepts results
// at dumb:///api/vim-page only while that token is armed.
//
// Link hints and yanks report exactly one terminal message (copy, open-new, or
// end). The caret/visual interaction is long-lived: it reports "mode" messages
// as it moves between hints, caret, visual, and visual line, "copy" messages
// that do not end it, and finally one "end".
//
// The caret and visual selection logic follows Vimium's mode_visual.js
// (MIT, Copyright (c) Phil Crosby, Ilya Sukhar): character-by-character word
// motions to avoid the native selection.modify word bugs on Linux (Vimium
// #1441), focus-direction detection, and the line-mode extension. The text
// anchor hints and the visible caret follow the flow of Surfingkeys' visual
// mode.
(() => {
  if (window.__dumberVimPage) return;

  const nativeFetch = window.fetch.bind(window);
  const HINT_CHARS = "sadfjklewcmpgh";
  const ALL_TARGETS = [
    "a[href]", "button", "input:not([type=\"hidden\"])", "select", "textarea", "summary",
    "[role=\"button\"]", "[role=\"link\"]", "[role=\"tab\"]", "[role=\"checkbox\"]",
    "[role=\"menuitem\"]", "[onclick]", "[contenteditable=\"\"]", "[contenteditable=\"true\"]",
    "[tabindex]:not([tabindex=\"-1\"])",
  ].join(",");
  const OBJECT_SELECTORS = {
    paragraph: ["p"],
    code: ["pre", "code"],
    table: ["table"],
    list: ["ul,ol,dl"],
  };
  // Motions that map directly to Selection.modify. w, e, b, ^ and gg are
  // handled in code.
  const CARET_MOTIONS = {
    h: ["backward", "character"], "<Left>": ["backward", "character"],
    l: ["forward", "character"], "<Right>": ["forward", "character"],
    j: ["forward", "line"], "<Down>": ["forward", "line"],
    k: ["backward", "line"], "<Up>": ["backward", "line"],
    "0": ["backward", "lineboundary"], "$": ["forward", "lineboundary"],
    "(": ["backward", "sentence"], ")": ["forward", "sentence"],
    "{": ["backward", "paragraph"], "}": ["forward", "paragraph"],
    G: ["forward", "documentboundary"],
  };
  const WORD_CHARACTER = /[\p{L}\p{N}_]/u;
  const MAX_MOTION_STEPS = 20000;
  const LONG_TEXT_LENGTH = 50;
  const MODE_NAMES = { hints: "hints", caret: "caret", visual: "visual", line: "visual-line" };

  let state = null;

  // deepQueryAll matches selector across the document and every open shadow
  // root; component-heavy sites (for example Reddit) render content there.
  function deepQueryAll(selector, root = document) {
    const results = Array.from(root.querySelectorAll(selector));
    for (const element of root.querySelectorAll("*")) {
      if (element.shadowRoot) results.push(...deepQueryAll(selector, element.shadowRoot));
    }
    return results;
  }

  // deepTextNodes yields non-blank text nodes in document order, descending
  // into open shadow roots.
  function* deepTextNodes(root) {
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_ELEMENT | NodeFilter.SHOW_TEXT);
    for (let node = walker.currentNode; node; node = walker.nextNode()) {
      if (node.nodeType === Node.TEXT_NODE) {
        if (node.data.trim()) yield node;
      } else if (node.shadowRoot) {
        yield* deepTextNodes(node.shadowRoot);
      }
    }
  }

  function post(token, message) {
    let encoded = "";
    try {
      encoded = btoa(unescape(encodeURIComponent(JSON.stringify(Object.assign({ token }, message)))));
    } catch (_) {
      return;
    }
    nativeFetch("dumb:///api/vim-page", {
      method: "POST",
      headers: { "X-Dumber-Body": encoded },
    }).catch(() => {});
  }

  function visibleRect(element) {
    if (!(element instanceof Element) || !element.isConnected) return null;
    if (element.closest("[hidden],[inert],[aria-hidden=\"true\"]")) return null;
    const style = window.getComputedStyle(element);
    if (style.display === "none" || style.visibility === "hidden" || style.visibility === "collapse") return null;
    for (const rect of element.getClientRects()) {
      if (rect.width < 1 || rect.height < 1) continue;
      if (rect.bottom <= 0 || rect.right <= 0 || rect.top >= window.innerHeight || rect.left >= window.innerWidth) continue;
      return rect;
    }
    return null;
  }

  function isEditable(element) {
    if (element.isContentEditable) return true;
    if (element instanceof HTMLTextAreaElement || element instanceof HTMLSelectElement) return true;
    if (!(element instanceof HTMLInputElement)) return false;
    return !["button", "checkbox", "color", "file", "image", "radio", "range", "reset", "submit"].includes(element.type);
  }

  function flash(element, color) {
    if (!(element instanceof HTMLElement)) return;
    const previousOutline = element.style.outline;
    const previousOffset = element.style.outlineOffset;
    element.style.outline = "3px solid " + color;
    element.style.outlineOffset = "3px";
    setTimeout(() => {
      element.style.outline = previousOutline;
      element.style.outlineOffset = previousOffset;
    }, 400);
  }

  // --- Link hints -----------------------------------------------------------

  function hintLabels(count) {
    let length = 1;
    while (Math.pow(HINT_CHARS.length, length) < count) length++;
    const labels = [];
    for (let i = 0; i < count; i++) {
      let n = i;
      let label = "";
      for (let j = 0; j < length; j++) {
        label = HINT_CHARS[n % HINT_CHARS.length] + label;
        n = Math.floor(n / HINT_CHARS.length);
      }
      labels.push(label);
    }
    return labels;
  }

  function hintTargets(linksOnly) {
    const seen = new Set();
    const targets = [];
    for (const element of deepQueryAll(linksOnly ? "a[href]" : ALL_TARGETS)) {
      if (seen.has(element) || element.disabled) continue;
      const rect = visibleRect(element);
      if (!rect) continue;
      if (!reachable(element, rect)) continue;
      seen.add(element);
      targets.push({ element, rect });
    }
    return targets;
  }

  // buildHintOverlay renders one label per item at its rect and arms the
  // typed-label filter. offsetLeft puts labels before the rect so they do not
  // cover the text a hint points at.
  function buildHintOverlay(items, color, offsetLeft) {
    const labels = hintLabels(items.length);
    const root = document.createElement("div");
    root.setAttribute("data-dumber-vim-hints", "");
    root.style.cssText = "position:fixed;inset:0;pointer-events:none;z-index:2147483647;";
    const labelWidth = labels.length > 0 ? labels[0].length * 8 + 10 : 0;
    const hints = items.map((item, index) => {
      const node = document.createElement("span");
      const left = offsetLeft && item.rect.left >= labelWidth ? item.rect.left - labelWidth : Math.max(item.rect.left, 0);
      node.style.cssText = [
        "position:fixed", "left:" + left + "px",
        "top:" + Math.max(item.rect.top, 0) + "px", "padding:1px 4px", "border-radius:3px",
        "font:bold 11px/1.3 monospace", "text-transform:uppercase", "color:#111",
        "background:" + color, "box-shadow:0 1px 3px rgba(0,0,0,.4)",
      ].join(";");
      node.textContent = labels[index];
      root.appendChild(node);
      return { label: labels[index], item, node };
    });
    document.documentElement.appendChild(root);
    state.root = root;
    state.hints = hints;
    state.typed = "";
  }

  function removeHintOverlay() {
    if (state.root) state.root.remove();
    state.root = null;
    state.hints = [];
  }

  function startHints(kind, color) {
    const targets = hintTargets(kind !== "hint-follow");
    if (targets.length === 0) return false;
    buildHintOverlay(targets, color, false);
    return true;
  }

  function renderHintFilter() {
    for (const hint of state.hints) {
      const visible = hint.label.startsWith(state.typed);
      hint.node.style.display = visible ? "" : "none";
      if (visible) {
        hint.node.textContent = "";
        const typed = document.createElement("span");
        typed.style.opacity = "0.45";
        typed.textContent = state.typed;
        hint.node.append(typed, hint.label.slice(state.typed.length));
      }
    }
  }

  // activateHint returns the terminal message for the chosen hint.
  function activateHint(kind, element) {
    const url = element instanceof HTMLAnchorElement ? element.href : "";
    if (kind === "hint-yank-url") return url ? { type: "copy", text: url } : { type: "end" };
    if (kind === "hint-follow-new" || (url && element.target === "_blank")) {
      return url ? { type: "open-new", url } : { type: "end" };
    }
    element.focus({ preventScroll: true });
    if (!isEditable(element)) element.click();
    return { type: "end" };
  }

  // hintKey handles typing labels for link hints and for the caret's
  // text-anchor hints (the "visual" kind), where Escape places the caret at a
  // default position instead of ending.
  function hintKey(key) {
    const textAnchors = state.kind === "visual";
    if (key === "<Escape>") return textAnchors ? placeCaretAtFallback() : finish({ type: "end" });
    if (key === "<BackSpace>") {
      state.typed = state.typed.slice(0, -1);
      return renderHintFilter();
    }
    if (key.length !== 1) return;
    const typed = state.typed + key.toLowerCase();
    const matches = state.hints.filter((hint) => hint.label.startsWith(typed));
    if (matches.length === 0) return;
    state.typed = typed;
    if (matches.length === 1 && matches[0].label === typed) {
      const item = matches[0].item;
      // Remove the overlay before activation so the click hits the page.
      removeHintOverlay();
      if (textAnchors) return enterCaretAt(item.node, item.offset);
      finish(activateHint(state.kind, item.element));
      return;
    }
    renderHintFilter();
  }

  // --- Caret and visual selection ------------------------------------------
  //
  // One interaction moves between four phases: "hints" (text-anchor labels),
  // "caret" (collapsed selection plus a visible cursor), "visual" (charwise
  // selection), and "line" (linewise selection). Each phase change is reported
  // to Go as a "mode" message so the indicator follows it.

  const SKIP_TEXT_PARENTS = /^(SCRIPT|STYLE|NOSCRIPT|TEMPLATE|TEXTAREA|OPTION)$/;

  function setPhase(phase) {
    if (state.phase === phase) return;
    state.phase = phase;
    post(state.token, { type: "mode", mode: MODE_NAMES[phase] });
  }

  // reachable reports whether element is what the user would hit when pointing
  // at rect, so hints skip content covered by overlays.
  function reachable(element, rect) {
    const x = Math.min(Math.max(rect.left + rect.width / 2, 0), window.innerWidth - 1);
    const y = Math.min(Math.max(rect.top + rect.height / 2, 0), window.innerHeight - 1);
    const root = element.getRootNode();
    const hit = (root.elementFromPoint ? root : document).elementFromPoint(x, y);
    return !hit || element.contains(hit) || hit.contains(element) || !!(hit.shadowRoot && hit.shadowRoot.contains(element));
  }

  // blockOf returns the nearest non-inline ancestor, so a paragraph with inline
  // markup offers one anchor instead of one per fragment.
  function blockOf(element) {
    let current = element;
    while (current.parentElement && /^(inline|contents)$/.test(window.getComputedStyle(current).display || "")) {
      current = current.parentElement;
    }
    return current;
  }

  function offsetAtPoint(node, x, y) {
    let container = null;
    let offset = 0;
    if (document.caretRangeFromPoint) {
      const range = document.caretRangeFromPoint(x, y);
      if (range) { container = range.startContainer; offset = range.startOffset; }
    } else if (document.caretPositionFromPoint) {
      const position = document.caretPositionFromPoint(x, y);
      if (position) { container = position.offsetNode; offset = position.offset; }
    }
    return container === node ? offset : -1;
  }

  // firstVisibleAnchor finds where visible text of node starts: the first
  // non-blank character when its first line is visible, otherwise the start of
  // the first fully visible line.
  function firstVisibleAnchor(node, element) {
    const range = document.createRange();
    range.selectNodeContents(node);
    const rects = Array.from(range.getClientRects());
    for (let i = 0; i < rects.length; i++) {
      const rect = rects[i];
      if (rect.width < 1 || rect.height < 1) continue;
      if (rect.top < 0 || rect.left < 0 || rect.bottom > window.innerHeight || rect.right > window.innerWidth) continue;
      const offset = i === 0 ? Math.max(node.data.search(/\S/), 0) : offsetAtPoint(node, rect.left + 1, rect.top + rect.height / 2);
      if (offset < 0 || !visibleRect(element) || !reachable(element, rect)) continue;
      return { node, offset, rect, element };
    }
    return null;
  }

  // collectTextAnchors lists visible text starts in document order. perBlock
  // keeps the first anchor of each block; minLength skips short texts.
  function collectTextAnchors({ perBlock, minLength, limit }) {
    const anchors = [];
    const blocks = new Set();
    for (const node of deepTextNodes(document.body || document.documentElement)) {
      const element = node.parentElement;
      if (!element || SKIP_TEXT_PARENTS.test(element.tagName) || isEditable(element)) continue;
      if (node.data.trim().length < minLength) continue;
      const anchor = firstVisibleAnchor(node, element);
      if (!anchor) continue;
      if (perBlock) {
        const block = blockOf(element);
        if (blocks.has(block)) continue;
        blocks.add(block);
      }
      anchors.push(anchor);
      if (anchors.length >= limit) break;
    }
    return anchors;
  }

  // Esc during the text-anchor hints: Vimium's heuristic picks the first
  // visible text of at least LONG_TEXT_LENGTH characters (shorter ones are
  // likely banners); any visible text is the fallback.
  function fallbackAnchor() {
    const long = collectTextAnchors({ perBlock: false, minLength: LONG_TEXT_LENGTH, limit: 1 });
    if (long.length > 0) return long[0];
    return collectTextAnchors({ perBlock: false, minLength: 1, limit: 1 })[0] || null;
  }

  function placeCaretAtFallback() {
    const anchor = fallbackAnchor();
    removeHintOverlay();
    if (!anchor) return finish({ type: "end", reason: "no-visible-text" });
    enterCaretAt(anchor.node, anchor.offset);
  }

  // The accent paints selections so they stand out on pages that restyle
  // ::selection; the style is removed with the interaction.
  function styleVisual(color) {
    const style = document.createElement("style");
    style.setAttribute("data-dumber-vim-visual", "");
    style.textContent = "::selection { background: " + color + " !important; color: #111 !important; }";
    document.documentElement.appendChild(style);
    state.style = style;
  }

  // focusRect measures the selection focus. A collapsed range can report an
  // empty rect at node boundaries, so the adjacent character is measured then.
  function focusRect(selection) {
    const node = selection.focusNode;
    if (!node) return null;
    const offset = selection.focusOffset;
    const range = document.createRange();
    range.setStart(node, offset);
    range.collapse(true);
    const first = (r) => Array.from(r.getClientRects()).find((rect) => rect.height > 0) || null;
    const direct = first(range);
    if (direct) return direct;
    if (node.nodeType === Node.TEXT_NODE) {
      if (offset < node.data.length) {
        range.setEnd(node, offset + 1);
        const next = first(range);
        if (next) return { left: next.left, right: next.left, top: next.top, bottom: next.bottom, width: 0, height: next.height };
      }
      if (offset > 0) {
        const before = document.createRange();
        before.setStart(node, offset - 1);
        before.setEnd(node, offset);
        const previous = first(before);
        if (previous) return { left: previous.right, right: previous.right, top: previous.top, bottom: previous.bottom, width: 0, height: previous.height };
      }
    }
    const element = node.nodeType === Node.ELEMENT_NODE ? node : node.parentElement;
    const rect = element && element.getBoundingClientRect();
    return rect && rect.height > 0 ? rect : null;
  }

  // The caret overlay is a blinking accent bar at the focus. It is page
  // content, so it follows scrolling and is removed with the interaction.
  function beginCaretUI(selection) {
    if (!state.style) styleVisual(state.color);
    if (!state.cursor) {
      const cursor = document.createElement("div");
      cursor.setAttribute("data-dumber-vim-caret", "");
      cursor.style.cssText = [
        "position:fixed", "width:4px", "pointer-events:none", "z-index:2147483647",
        "background:" + state.color, "border-radius:2px",
        "box-shadow:0 0 0 2px rgba(17,17,17,.85), 0 0 10px 2px " + state.color,
      ].join(";");
      document.documentElement.appendChild(cursor);
      if (typeof cursor.animate === "function") {
        cursor.animate([{ opacity: 1 }, { opacity: 1, offset: 0.6 }, { opacity: 0.15 }], { duration: 1000, iterations: Infinity });
      }
      state.cursor = cursor;
      state.onViewportChange = () => updateCursor(window.getSelection());
      window.addEventListener("scroll", state.onViewportChange, { capture: true, passive: true });
      window.addEventListener("resize", state.onViewportChange, { passive: true });
    }
    updateCursor(selection);
  }

  function updateCursor(selection) {
    if (!state || !state.cursor || !selection) return;
    const rect = focusRect(selection);
    const visible = rect && rect.bottom > 0 && rect.top < window.innerHeight && rect.right > -4 && rect.left < window.innerWidth;
    state.cursor.style.display = visible ? "" : "none";
    if (!visible) return;
    // Taller than the text line so the caret reads at a glance.
    const height = Math.max(rect.height, 14) + 6;
    state.cursor.style.left = Math.round(rect.left - 2) + "px";
    state.cursor.style.top = Math.round(rect.top + (rect.height - height) / 2) + "px";
    state.cursor.style.height = Math.round(height) + "px";
  }

  function revealFocus(selection) {
    const rect = focusRect(selection);
    if (rect) {
      const margin = 40;
      if (rect.top < margin) window.scrollBy(0, rect.top - margin);
      else if (rect.bottom > window.innerHeight - margin) window.scrollBy(0, rect.bottom - window.innerHeight + margin);
    }
    updateCursor(selection);
  }

  function enterCaretAt(node, offset) {
    const selection = window.getSelection();
    selection.setBaseAndExtent(node, offset, node, offset);
    state.count = "";
    state.pendingG = false;
    beginCaretUI(selection);
    setPhase("caret");
    revealFocus(selection);
  }

  // A visible range selection is used as is, as in Vimium; a selection that
  // scrolled out of view is stale and ignored.
  function hasVisibleRangeSelection(selection) {
    if (selection.rangeCount === 0 || selection.isCollapsed) return false;
    const rect = selection.getRangeAt(0).getBoundingClientRect();
    return rect.bottom > 0 && rect.top < window.innerHeight && rect.right > 0 && rect.left < window.innerWidth;
  }

  function startVisual(color) {
    const selection = window.getSelection();
    if (!selection) return false;
    state.color = color;
    state.count = "";
    state.pendingG = false;
    if (hasVisibleRangeSelection(selection)) {
      beginCaretUI(selection);
      setPhase("visual");
      return true;
    }
    const anchors = collectTextAnchors({ perBlock: true, minLength: 1, limit: HINT_CHARS.length * HINT_CHARS.length });
    if (anchors.length === 0) return false;
    buildHintOverlay(anchors, color, true);
    setPhase("hints");
    return true;
  }

  // --- Selection primitives (after Vimium's Movement class) ---

  function isBackward(selection) {
    if (selection.anchorNode === selection.focusNode && selection.anchorOffset === selection.focusOffset) return false;
    const range = document.createRange();
    range.setStart(selection.anchorNode, selection.anchorOffset);
    range.setEnd(selection.focusNode, selection.focusOffset);
    return range.collapsed;
  }

  function swapEnds(selection) {
    selection.setBaseAndExtent(selection.focusNode, selection.focusOffset, selection.anchorNode, selection.anchorOffset);
  }

  function collapseToFocus(selection) {
    selection.collapse(selection.focusNode, selection.focusOffset);
  }

  // extendByOne extends the focus one character and reports how the selected
  // text length changed; 0 means the focus could not move.
  function extendByOne(selection, direction) {
    const length = selection.toString().length;
    selection.modify("extend", direction, "character");
    return selection.toString().length - length;
  }

  // nextCharacter returns the character after the focus without moving it.
  function nextCharacter(selection) {
    const before = selection.toString();
    if (before.length === 0 || !isBackward(selection)) {
      selection.modify("extend", "forward", "character");
      const after = selection.toString();
      if (after === before) return undefined;
      selection.modify("extend", "backward", "character");
      return after[after.length - 1];
    }
    return before[0];
  }

  // previousCharacter returns the character before the focus without moving it.
  function previousCharacter(selection) {
    const before = selection.toString();
    if (before.length === 0 || isBackward(selection)) {
      selection.modify("extend", "backward", "character");
      const after = selection.toString();
      if (after === before) return undefined;
      selection.modify("extend", "forward", "character");
      return after[0];
    }
    return before[before.length - 1];
  }

  function isWordCharacter(character) {
    return character !== undefined && WORD_CHARACTER.test(character);
  }

  // Word motions run character by character: the native word granularity
  // differs between Linux and other platforms (Vimium #1441).
  function wordForward(selection, wordFirst) {
    const skipWord = () => {
      for (let i = 0; i < MAX_MOTION_STEPS && isWordCharacter(nextCharacter(selection)); i++) {
        if (extendByOne(selection, "forward") === 0) return false;
      }
      return true;
    };
    const skipOther = () => {
      for (let i = 0; i < MAX_MOTION_STEPS; i++) {
        const character = nextCharacter(selection);
        if (character === undefined || isWordCharacter(character)) return true;
        if (extendByOne(selection, "forward") === 0) return false;
      }
      return true;
    };
    // w: finish the current word, then skip to the next one. e: skip to a
    // word, then finish it.
    if (wordFirst) {
      if (skipWord()) skipOther();
    } else if (skipOther()) {
      skipWord();
    }
  }

  function wordBackward(selection) {
    for (let i = 0; i < MAX_MOTION_STEPS; i++) {
      const character = previousCharacter(selection);
      if (character === undefined || isWordCharacter(character)) break;
      if (extendByOne(selection, "backward") === 0) return;
    }
    for (let i = 0; i < MAX_MOTION_STEPS && isWordCharacter(previousCharacter(selection)); i++) {
      if (extendByOne(selection, "backward") === 0) return;
    }
  }

  function firstNonBlank(selection, alter) {
    selection.modify(alter, "backward", "lineboundary");
    if (alter === "move") {
      // Measure with extend, then settle the caret where it ended.
      for (let i = 0; i < MAX_MOTION_STEPS; i++) {
        const character = nextCharacter(selection);
        if (character === undefined || !/[ \t\u00a0]/.test(character)) break;
        if (extendByOne(selection, "forward") === 0) break;
      }
      collapseToFocus(selection);
      return;
    }
    for (let i = 0; i < MAX_MOTION_STEPS; i++) {
      const character = nextCharacter(selection);
      if (character === undefined || !/[ \t\u00a0]/.test(character)) break;
      if (extendByOne(selection, "forward") === 0) break;
    }
  }

  // extendToLines grows the selection so both ends sit on line boundaries.
  function extendToLines(selection) {
    const forward = !isBackward(selection);
    for (const direction of forward ? ["forward", "backward"] : ["backward", "forward"]) {
      selection.modify("extend", direction, "lineboundary");
      swapEnds(selection);
    }
  }

  // moveOnce applies one motion. In caret phase the selection stays collapsed:
  // native granularities use "move", and the character-by-character motions
  // extend and then collapse to the focus.
  function moveOnce(selection, key) {
    const caret = state.phase === "caret";
    const alter = caret ? "move" : "extend";
    if (CARET_MOTIONS[key]) {
      selection.modify(alter, CARET_MOTIONS[key][0], CARET_MOTIONS[key][1]);
      return true;
    }
    switch (key) {
      case "w": wordForward(selection, true); break;
      case "e": wordForward(selection, false); break;
      case "b": wordBackward(selection); break;
      case "^": return firstNonBlank(selection, alter), true;
      case "gg": selection.modify(alter, "backward", "documentboundary"); return true;
      default: return false;
    }
    if (caret) collapseToFocus(selection);
    return true;
  }

  function runMotion(selection, key, count) {
    for (let i = 0; i < count; i++) {
      if (state.phase === "line") {
        const saved = [selection.anchorNode, selection.anchorOffset, selection.focusNode, selection.focusOffset];
        if (!moveOnce(selection, key)) return;
        if (selection.isCollapsed) selection.setBaseAndExtent(...saved);
        extendToLines(selection);
      } else if (!moveOnce(selection, key)) {
        return;
      }
    }
  }

  // --- Phase transitions ---

  function toCaret(selection) {
    collapseToFocus(selection);
    setPhase("caret");
    revealFocus(selection);
  }

  function toVisual(selection) {
    // Include the character under the caret, or the one before at the end.
    if (extendByOne(selection, "forward") === 0) extendByOne(selection, "backward");
    setPhase("visual");
    revealFocus(selection);
  }

  function toLine(selection) {
    extendToLines(selection);
    setPhase("line");
    revealFocus(selection);
  }

  // yankSelection copies the selection and returns to the caret at its focus,
  // so further copies can be chained without restarting.
  function yankSelection(selection) {
    const text = selection.toString();
    if (text) post(state.token, { type: "copy", text });
    toCaret(selection);
  }

  function caretKey(key) {
    const selection = window.getSelection();
    if (!selection || selection.rangeCount === 0) return finish({ type: "end" });
    const phase = state.phase;
    if (key === "<Escape>") {
      state.count = "";
      state.pendingG = false;
      if (phase === "caret") {
        selection.removeAllRanges();
        return finish({ type: "end" });
      }
      return toCaret(selection);
    }
    if (key === "v") {
      state.count = "";
      state.pendingG = false;
      if (phase === "caret") return toVisual(selection);
      if (phase === "visual") return toCaret(selection);
      setPhase("visual");
      return updateCursor(selection);
    }
    if (key === "V") {
      state.count = "";
      state.pendingG = false;
      return phase === "line" ? toCaret(selection) : toLine(selection);
    }
    if ((key === "y" || key === "<Return>")) {
      state.count = "";
      state.pendingG = false;
      return phase === "caret" ? undefined : yankSelection(selection);
    }
    if (key === "o") {
      state.count = "";
      state.pendingG = false;
      if (phase === "caret") return;
      swapEnds(selection);
      return revealFocus(selection);
    }
    if (/^[1-9]$/.test(key) || (key === "0" && state.count !== "")) {
      state.count += key;
      return;
    }
    let motion = key;
    if (key === "g") {
      if (!state.pendingG) {
        state.pendingG = true;
        return;
      }
      motion = "gg";
    }
    state.pendingG = false;
    const count = Math.min(parseInt(state.count || "1", 10), 500);
    state.count = "";
    runMotion(selection, motion, count);
    revealFocus(selection);
  }

  // --- Text objects ---------------------------------------------------------

  function selectionText(range) {
    const selection = window.getSelection();
    if (!selection) return range.toString();
    const saved = [];
    for (let i = 0; i < selection.rangeCount; i++) saved.push(selection.getRangeAt(i));
    selection.removeAllRanges();
    selection.addRange(range);
    const text = selection.toString();
    selection.removeAllRanges();
    saved.forEach((savedRange) => selection.addRange(savedRange));
    return text;
  }

  function sectionText(color) {
    const headings = deepQueryAll("h1,h2,h3,h4,h5,h6")
      .filter((heading) => heading.getClientRects().length > 0);
    if (headings.length === 0) return "";
    const threshold = window.innerHeight * 0.25;
    let index = -1;
    headings.forEach((heading, i) => {
      if (heading.getBoundingClientRect().top <= threshold) index = i;
    });
    if (index < 0) index = 0;
    const heading = headings[index];
    const level = Number(heading.tagName[1]);
    const boundary = headings.slice(index + 1).find((next) => Number(next.tagName[1]) <= level);
    const range = document.createRange();
    range.setStartBefore(heading);
    if (boundary) range.setEndBefore(boundary);
    else range.setEndAfter(document.body ? document.body.lastChild || heading : heading);
    flash(heading, color);
    return selectionText(range);
  }

  function objectText(object, color) {
    if (object === "section") return sectionText(color);
    for (const selector of OBJECT_SELECTORS[object] || []) {
      const candidates = deepQueryAll(selector);
      const element = candidates.find((candidate) =>
        visibleRect(candidate) && !candidates.some((other) => other !== candidate && other.contains(candidate)));
      if (element) {
        flash(element, color);
        return element.innerText;
      }
    }
    return "";
  }

  // --- Lifecycle ------------------------------------------------------------

  function cleanup() {
    if (state) {
      for (const node of [state.root, state.style, state.cursor]) if (node) node.remove();
      if (state.onViewportChange) {
        window.removeEventListener("scroll", state.onViewportChange, { capture: true });
        window.removeEventListener("resize", state.onViewportChange);
      }
    }
    state = null;
  }

  // finish tears down the interaction and reports its single terminal message.
  function finish(message) {
    const token = state ? state.token : "";
    cleanup();
    if (token) post(token, message);
  }

  Object.defineProperty(window, "__dumberVimPage", {
    configurable: false,
    enumerable: false,
    value: Object.freeze({
      start(token, request) {
        cleanup();
        const color = request.color || "#fbbf24";
        if (request.kind === "yank") {
          const text = objectText(request.object, color).trim();
          post(token, text ? { type: "copy", text } : { type: "end" });
          return;
        }
        state = { kind: request.kind, token, color };
        const started = request.kind === "visual" ? startVisual(color) : startHints(request.kind, color);
        if (!started) finish({ type: "end", reason: request.kind === "visual" ? "no-visible-text" : "no-visible-targets" });
      },
      key(token, key) {
        if (!state || state.token !== token) {
          // Unknown interaction (for example the runtime was reinstalled):
          // release that token's key capture without touching live state.
          post(token, { type: "end" });
          return;
        }
        try {
          if (state.phase === "hints" || state.kind !== "visual") return hintKey(key);
          return caretKey(key);
        } catch (_) {
          // A page quirk must never leave the user's keys captured.
          if (state) finish({ type: "end", reason: "error" });
        }
      },
      cancel(token) {
        if (!state || state.token !== token) return;
        if (state.kind === "visual" && state.phase !== "hints") {
          const selection = window.getSelection();
          if (selection) selection.removeAllRanges();
        }
        cleanup();
      },
    }),
  });
})();
