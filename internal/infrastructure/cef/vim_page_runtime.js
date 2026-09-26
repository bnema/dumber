// Dumber Vim Mode page runtime: link hints, visual selection, text-object
// yanks. Installed once per document; Go calls window.__dumberVimPage.* with a
// per-interaction token. Each interaction reports exactly one terminal message
// (copy, open-new, or end) to dumb:///api/vim-page; Go accepts it only while
// that token is armed, so the token grants at most one result.
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
  const VISUAL_MOTIONS = {
    h: ["backward", "character"], "<Left>": ["backward", "character"],
    l: ["forward", "character"], "<Right>": ["forward", "character"],
    j: ["forward", "line"], "<Down>": ["forward", "line"],
    k: ["backward", "line"], "<Up>": ["backward", "line"],
    w: ["forward", "word"], e: ["forward", "word"], b: ["backward", "word"],
    "0": ["backward", "lineboundary"], "^": ["backward", "lineboundary"],
    "$": ["forward", "lineboundary"],
    "(": ["backward", "sentence"], ")": ["forward", "sentence"],
    "{": ["backward", "paragraph"], "}": ["forward", "paragraph"],
    G: ["forward", "documentboundary"],
  };

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
      const x = Math.min(Math.max(rect.left + rect.width / 2, 0), window.innerWidth - 1);
      const y = Math.min(Math.max(rect.top + rect.height / 2, 0), window.innerHeight - 1);
      const root = element.getRootNode();
      const hit = (root.elementFromPoint ? root : document).elementFromPoint(x, y);
      if (hit && !element.contains(hit) && !hit.contains(element) && !(hit.shadowRoot && hit.shadowRoot.contains(element))) continue;
      seen.add(element);
      targets.push({ element, rect });
    }
    return targets;
  }

  function startHints(kind, color) {
    const targets = hintTargets(kind !== "hint-follow");
    if (targets.length === 0) return false;
    const labels = hintLabels(targets.length);
    const root = document.createElement("div");
    root.setAttribute("data-dumber-vim-hints", "");
    root.style.cssText = "position:fixed;inset:0;pointer-events:none;z-index:2147483647;";
    const hints = targets.map((target, index) => {
      const node = document.createElement("span");
      node.style.cssText = [
        "position:fixed", "left:" + Math.max(target.rect.left, 0) + "px",
        "top:" + Math.max(target.rect.top, 0) + "px", "padding:1px 4px", "border-radius:3px",
        "font:bold 11px/1.3 monospace", "text-transform:uppercase", "color:#111",
        "background:" + color, "box-shadow:0 1px 3px rgba(0,0,0,.4)",
      ].join(";");
      node.textContent = labels[index];
      root.appendChild(node);
      return { label: labels[index], element: target.element, node };
    });
    document.documentElement.appendChild(root);
    state.root = root;
    state.hints = hints;
    state.typed = "";
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

  function hintKey(key) {
    if (key === "<Escape>") return finish({ type: "end" });
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
      const kind = state.kind;
      const element = matches[0].element;
      // Remove the overlay before activation so the click hits the page.
      if (state.root) state.root.remove();
      finish(activateHint(kind, element));
      return;
    }
    renderHintFilter();
  }

  // --- Visual selection -----------------------------------------------------

  function firstVisibleTextPosition() {
    for (const node of deepTextNodes(document.body || document.documentElement)) {
      if (!visibleRect(node.parentElement)) continue;
      const range = document.createRange();
      range.selectNodeContents(node);
      const rect = range.getBoundingClientRect();
      if (rect.bottom <= 0 || rect.top >= window.innerHeight) continue;
      return { node, offset: Math.max(node.data.search(/\S/), 0) };
    }
    return null;
  }

  // Visual mode paints the selection with the accent so it stands out even on
  // pages that restyle ::selection; the style is removed when visual ends.
  function styleVisual(color) {
    const style = document.createElement("style");
    style.setAttribute("data-dumber-vim-visual", "");
    style.textContent = "::selection { background: " + color + " !important; color: #111 !important; }";
    document.documentElement.appendChild(style);
    state.root = style;
  }

  function startVisual(color) {
    const selection = window.getSelection();
    if (!selection) return false;
    if (selection.rangeCount === 0 || selection.isCollapsed) {
      const start = firstVisibleTextPosition();
      if (!start) return false;
      selection.setBaseAndExtent(start.node, start.offset, start.node, start.offset);
      selection.modify("extend", "forward", "character");
    }
    state.count = "";
    state.pendingG = false;
    styleVisual(color);
    return true;
  }

  function revealSelectionFocus(selection) {
    if (!selection.focusNode) return;
    const range = document.createRange();
    range.setStart(selection.focusNode, selection.focusOffset);
    range.collapse(true);
    let rect = range.getBoundingClientRect();
    if (rect.top === 0 && rect.bottom === 0) {
      const element = selection.focusNode.nodeType === Node.ELEMENT_NODE
        ? selection.focusNode : selection.focusNode.parentElement;
      if (!element) return;
      rect = element.getBoundingClientRect();
    }
    const margin = 40;
    if (rect.top < margin) window.scrollBy(0, rect.top - margin);
    else if (rect.bottom > window.innerHeight - margin) window.scrollBy(0, rect.bottom - window.innerHeight + margin);
  }

  function visualKey(key) {
    const selection = window.getSelection();
    if (!selection || selection.rangeCount === 0) return finish({ type: "end" });
    if (key === "<Escape>" || key === "v") {
      selection.removeAllRanges();
      return finish({ type: "end" });
    }
    if (key === "y" || key === "<Return>") {
      const text = selection.toString();
      selection.removeAllRanges();
      return finish(text ? { type: "copy", text } : { type: "end" });
    }
    if (key === "o") {
      selection.setBaseAndExtent(selection.focusNode, selection.focusOffset, selection.anchorNode, selection.anchorOffset);
      return revealSelectionFocus(selection);
    }
    if (/^[1-9]$/.test(key) || (key === "0" && state.count !== "")) {
      state.count += key;
      return;
    }
    let motion = VISUAL_MOTIONS[key];
    if (key === "g") {
      if (!state.pendingG) {
        state.pendingG = true;
        return;
      }
      motion = ["backward", "documentboundary"];
    }
    state.pendingG = false;
    const count = Math.min(parseInt(state.count || "1", 10), 500);
    state.count = "";
    if (!motion) return;
    for (let i = 0; i < count; i++) selection.modify("extend", motion[0], motion[1]);
    revealSelectionFocus(selection);
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
    if (state && state.root) state.root.remove();
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
        state = { kind: request.kind, token };
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
        if (state.kind === "visual") return visualKey(key);
        return hintKey(key);
      },
      cancel(token) {
        if (!state || state.token !== token) return;
        if (state && state.kind === "visual") {
          const selection = window.getSelection();
          if (selection) selection.removeAllRanges();
        }
        cleanup();
      },
    }),
  });
})();
