// Visual enhancements for dumb:// system views.
//
// The Go/wasm runtime owns state and logic. This file only provides visual
// helpers it may call through window.dumberSystemviews. Every page must keep
// working when this file is missing, so the runtime feature-detects each
// helper before using it.
(() => {
  'use strict';

  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)');

  // Rows that fade in after a filter change. The stagger is capped so long
  // lists settle quickly.
  const ROWS = '.sv-row';
  const MAX_STAGGER = 8;

  function markEntering(regions) {
    let index = 0;
    for (const region of regions) {
      for (const row of region.querySelectorAll(ROWS)) {
        row.classList.add('sv-enter');
        row.style.setProperty('--sv-enter-index', String(Math.min(index, MAX_STAGGER)));
        index += 1;
      }
    }
  }

  /** @type {Set<() => void>} */
  const afterSwapHooks = new Set();

  /**
   * Moves focus into a scrolled-to section so the next Tab continues there.
   * @param {HTMLElement} section
   */
  function focusSection(section) {
    if (!section.hasAttribute('tabindex')) section.setAttribute('tabindex', '-1');
    section.focus({ preventScroll: true });
  }

  /**
   * Replaces the inner markup of several regions as one visual change. The
   * DOM is updated synchronously; with `transition`, the new rows fade in.
   * @param {Array<[Element, string]>} entries region element and new markup
   * @param {{transition?: boolean}} options
   */
  function swap(entries, options = {}) {
    for (const [element, html] of entries) {
      element.innerHTML = html;
    }
    // Page hooks (the favorites filter) run before Go restores focus.
    afterSwapHooks.forEach((hook) => hook());
    if (options.transition && !reducedMotion.matches) {
      markEntering(entries.map(([element]) => element));
    }
  }

  // ----- Toast --------------------------------------------------------------

  /** @type {{host: HTMLElement, settle: (result: string) => void} | null} */
  let activeToast = null;

  // One persistent live region: screen readers announce text set into a
  // region that already exists, not a region inserted already filled.
  /** @type {HTMLElement | null} */
  let liveRegion = null;

  function ensureLiveRegion() {
    if (liveRegion?.isConnected || !document.body) return liveRegion;
    liveRegion = document.createElement('div');
    liveRegion.className = 'sv-sr-only';
    liveRegion.setAttribute('role', 'status');
    liveRegion.setAttribute('aria-live', 'polite');
    document.body.append(liveRegion);
    return liveRegion;
  }

  if (document.body) ensureLiveRegion();
  else document.addEventListener('DOMContentLoaded', ensureLiveRegion, { once: true });

  let announceTimer = 0;

  /** Announces text; an empty string clears the region. */
  function announce(text) {
    const region = ensureLiveRegion();
    if (!region) return;
    window.clearTimeout(announceTimer);
    announceTimer = 0;
    region.textContent = '';
    if (!text) return;
    // A separate task so a repeated message is announced again.
    announceTimer = window.setTimeout(() => {
      announceTimer = 0;
      region.textContent = text;
    }, 50);
  }

  // The toast lives under <body>, outside .sv-app, so it copies the app's
  // palette variables and theme classes.
  function inheritTheme(host, themeRoot) {
    if (!(themeRoot instanceof Element)) return;
    const style = themeRoot.getAttribute('style');
    if (style) host.setAttribute('style', style);
    for (const name of themeRoot.classList) {
      if (name !== 'sv-app') host.classList.add(name);
    }
  }

  function isEditable(target) {
    return target instanceof Element &&
      (target.closest('input, textarea, select, [contenteditable]') !== null);
  }

  /**
   * Shows a single action toast. A newer toast replaces the current one.
   * @param {{message: string, actionLabel: string, timeoutMs: number, theme?: Element}} options
   * @returns {Promise<'action'|'timeout'|'replaced'|'dismissed'>}
   */
  function toast(options) {
    activeToast?.settle('replaced');

    const host = document.createElement('div');
    host.className = 'sv-toast-host';
    inheritTheme(host, options.theme);
    const supportsPopover = typeof host.showPopover === 'function';
    if (supportsPopover) host.popover = 'manual';

    const box = document.createElement('div');
    box.className = 'sv-toast';

    const message = document.createElement('span');
    message.className = 'sv-toast-message';
    message.textContent = options.message;

    // Without an action label the toast is a plain status message.
    const hasAction = Boolean(options.actionLabel);
    const action = document.createElement('button');
    action.type = 'button';
    action.className = 'sv-toast-action';
    action.textContent = options.actionLabel || '';
    action.setAttribute('aria-keyshortcuts', 'U Control+Z');

    const hint = document.createElement('kbd');
    hint.className = 'sv-toast-key';
    hint.textContent = 'u';
    hint.setAttribute('aria-hidden', 'true');

    const progress = document.createElement('span');
    progress.className = 'sv-toast-progress';
    progress.style.animationDuration = `${options.timeoutMs}ms`;

    box.append(message);
    if (hasAction) box.append(action, hint);
    box.append(progress);
    host.append(box);
    document.body.append(host);
    if (supportsPopover) host.showPopover();
    announce(options.actionLabel ? `${options.message}. Press U to ${options.actionLabel.toLowerCase()}.` : options.message);

    return new Promise((resolve) => {
      let remaining = options.timeoutMs;
      let startedAt = performance.now();
      let timer = window.setTimeout(() => settle('timeout'), remaining);
      let done = false;

      const pause = () => {
        if (done || timer === 0) return;
        window.clearTimeout(timer);
        timer = 0;
        remaining -= performance.now() - startedAt;
        box.classList.add('sv-toast-paused');
      };
      const resume = () => {
        if (done || timer !== 0 || box.matches(':hover, :focus-within')) return;
        startedAt = performance.now();
        timer = window.setTimeout(() => settle('timeout'), Math.max(remaining, 0));
        box.classList.remove('sv-toast-paused');
      };
      const onKey = (event) => {
        if (!hasAction || document.querySelector('dialog[open], .sv-confirm-backdrop')) return;
        const undo = (event.key === 'u' && !event.ctrlKey && !event.metaKey && !event.altKey) ||
          ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'z' && !event.shiftKey);
        if (!undo || isEditable(event.target)) return;
        event.preventDefault();
        event.stopImmediatePropagation();
        settle('action');
      };

      function settle(result) {
        if (done) return;
        done = true;
        window.clearTimeout(timer);
        // A replacing toast announces itself; only clear when none follows.
        if (result !== 'replaced') announce('');
        document.removeEventListener('keydown', onKey, true);
        if (activeToast?.host === host) activeToast = null;
        const remove = () => {
          if (supportsPopover && host.matches(':popover-open')) host.hidePopover();
          host.remove();
        };
        if (reducedMotion.matches || result === 'replaced') {
          remove();
        } else {
          box.classList.add('sv-toast-leaving');
          window.setTimeout(remove, 160);
        }
        resolve(result);
      }

      action.addEventListener('click', () => settle('action'));
      box.addEventListener('pointerenter', pause);
      box.addEventListener('pointerleave', resume);
      box.addEventListener('focusin', pause);
      box.addEventListener('focusout', resume);
      // Capture phase so the page's own shortcuts (u is free, Ctrl+Z is not
      // used) never see the undo key while a toast is open.
      document.addEventListener('keydown', onKey, true);
      activeToast = { host, settle };
    });
  }

  /** Settles the open toast as dismissed (the page is going away). */
  function dismissToast() {
    activeToast?.settle('dismissed');
  }

  // ----- Action palette -----------------------------------------------------

  // Scores label/keyword matches: prefix > word start > substring > ordered
  // letters (so "dlh" finds "Delete … last hour"). 0 means no match.
  const WORDS = /[^a-z0-9+#%]+/;

  // Letters of token appear in order in text ("bgc" finds "background color").
  function inOrder(token, text) {
    let at = 0;
    for (const char of token) {
      at = text.indexOf(char, at);
      if (at === -1) return false;
      at += 1;
    }
    return true;
  }

  function tokenScore(token, label, haystack) {
    if (label.startsWith(token)) return 6;
    if (label.split(WORDS).some((word) => word.startsWith(token))) return 5;
    if (label.includes(token)) return 4;
    if (haystack.split(WORDS).some((word) => word.startsWith(token))) return 3;
    if (haystack.includes(token)) return 2;
    return inOrder(token, label) ? 1 : 0;
  }

  // Scores an item for a query: every word of the query must match the
  // label (best), the keywords, group or detail (weaker), or the label's
  // letters in order (weakest). 0 means no match.
  function matchScore(item, query) {
    if (!query) return item.searchOnly ? 0 : 1;
    const label = item.label.toLowerCase();
    const haystack = `${label} ${(item.keywords || '').toLowerCase()} ${(item.group || '').toLowerCase()} ` +
      `${(item.detail || '').toLowerCase()} ${(item.hint || '').toLowerCase()}`;
    let total = 0;
    for (const token of query.split(/\s+/)) {
      const score = tokenScore(token, label, haystack);
      if (score === 0) return 0;
      total += score;
    }
    return total * (item.weight ?? 1);
  }

  // ----- Settings index ------------------------------------------------------

  /** @type {Map<string, {row: Element, control: HTMLElement | null}>} */
  let settingTargets = new Map();
  const VALUE_MAX = 32;

  const textOf = (root, selector) => root.querySelector(selector)?.textContent.trim().replace(/\s+/g, ' ') ?? '';

  function controlValue(control) {
    if (!control) return '';
    let value = '';
    if (control instanceof HTMLSelectElement) value = control.selectedOptions[0]?.textContent ?? '';
    else if (control.type === 'checkbox') value = control.checked ? 'On' : 'Off';
    else if (control.type === 'radio') value = control.closest('label')?.textContent ?? control.value;
    else if ('value' in control) value = control.value;
    value = value.trim();
    return value.length > VALUE_MAX ? `${value.slice(0, VALUE_MAX - 1)}…` : value;
  }

  /**
   * Lists every setting on the settings page for the palette: its label,
   * help, current value and section. Choosing one calls jumpToSetting.
   * @param {Element} root
   */
  function settingsIndex(root) {
    settingTargets = new Map();
    const items = [];
    const rows = root.querySelectorAll(
      '[data-sv-setting], [data-sv-color], [data-sv-kb-row], [data-sv-shortcut-row]:not([data-sv-shortcut-new]), .sv-kv-row');
    for (const row of rows) {
      // Settings hidden by the page state (the Custom performance card)
      // are not offered; keybinding rows hidden by the filter are.
      if (!row.matches('[data-sv-kb-row]') && row.closest('details:not([open])') === null &&
          row.checkVisibility?.() === false) continue;
      const section = textOf(row.closest('[data-sv-config-section]') ?? row, 'h2');
      const card = textOf(row.closest('.sv-settings-card') ?? row, '.sv-settings-card-title, summary');
      let label = '';
      let help = '';
      let control = null;
      if (row.matches('[data-sv-setting]')) {
        label = textOf(row, '.sv-setting-label');
        help = textOf(row, '.sv-setting-help');
        const box = row.querySelector('.sv-setting-control');
        control = box?.querySelector('input:checked:not([type=checkbox]), input:not([type=hidden]):not([type=radio]), select, textarea, button') ?? null;
        // Segmented choices ("Dark", "Balanced") find their setting too.
        const choices = [...(box?.querySelectorAll('.sv-segment, option') ?? [])].map((choice) => choice.textContent.trim());
        help = [help, ...choices].join(' ');
      } else if (row.matches('[data-sv-color]')) {
        label = textOf(row, '.sv-color-label');
        control = row.querySelector('[data-sv-color-input]');
      } else if (row.matches('[data-sv-kb-row]')) {
        label = textOf(row, '.sv-kb-label');
        help = textOf(row, '.sv-kb-action');
        control = row.querySelector('[data-sv-kb-input]');
      } else if (row.matches('[data-sv-shortcut-row]')) {
        control = row.querySelector('input[name$=".key"]');
        label = `Search shortcut ${control?.value ?? ''}`.trim();
        help = [...row.querySelectorAll('input[name$=".url"], input[name$=".description"]')].map((input) => input.value).join(' ');
      } else {
        label = textOf(row, 'dt');
        help = textOf(row, 'dd');
      }
      if (!label) continue;
      const id = `setting:${items.length}`;
      settingTargets.set(id, { row, control });
      const value = row.matches('.sv-kv-row') ? help : controlValue(control);
      items.push({
        id,
        label,
        group: 'Settings',
        detail: [section, card].filter((part) => part && part !== label).join(' › '),
        hint: value || (row.matches('[data-sv-kb-row]') ? 'Unassigned' : ''),
        keywords: help,
        searchOnly: true,
        // Raw stored values rank below the editable settings.
        weight: row.matches('.sv-kv-row') ? 0.5 : 1,
      });
    }
    return items;
  }

  /**
   * Scrolls to the setting chosen in the palette, focuses its control and
   * flashes the row.
   * @param {string} id
   */
  function jumpToSetting(id) {
    const target = settingTargets.get(id);
    if (!target || !target.row.isConnected) return;
    const { row, control } = target;
    const details = row.closest('details');
    if (details && !details.open) details.open = true;
    if (row.matches('[data-sv-kb-row]') && row.checkVisibility?.() === false) {
      const filter = document.querySelector('[data-sv-keybinding-filter]');
      if (filter instanceof HTMLInputElement && filter.value) {
        filter.value = '';
        filter.dispatchEvent(new Event('input', { bubbles: true }));
      }
    }
    row.scrollIntoView({ behavior: reducedMotion.matches ? 'auto' : 'smooth', block: 'center' });
    control?.focus({ preventScroll: true });
    if (control instanceof HTMLInputElement && control.type === 'text') control.select();
    row.classList.remove('sv-flash');
    void row.offsetWidth;
    row.classList.add('sv-flash');
    row.addEventListener('animationend', () => row.classList.remove('sv-flash'), { once: true });
  }

  let paletteSeq = 0;

  /**
   * Opens the action palette. Resolves with the chosen item id, or null when
   * the palette is dismissed.
   * Items with searchOnly appear only once the user types.
   * @param {{placeholder: string, items: Array<{id: string, label: string, group?: string, detail?: string, hint?: string, keywords?: string, danger?: boolean, searchOnly?: boolean}>, theme?: Element}} options
   * @returns {Promise<string|null>}
   */
  function palette(options) {
    const uid = `sv-palette-${++paletteSeq}`;
    const dialog = document.createElement('dialog');
    dialog.className = 'sv-palette';
    dialog.setAttribute('aria-label', 'Actions');
    inheritTheme(dialog, options.theme);

    const field = document.createElement('div');
    field.className = 'sv-palette-field';
    const input = document.createElement('input');
    input.type = 'text';
    input.className = 'sv-palette-input';
    input.placeholder = options.placeholder;
    input.autocomplete = 'off';
    input.spellcheck = false;
    input.setAttribute('role', 'combobox');
    input.setAttribute('aria-label', 'Search actions');
    input.setAttribute('aria-expanded', 'true');
    input.setAttribute('aria-controls', `${uid}-list`);
    input.setAttribute('aria-autocomplete', 'list');
    const esc = document.createElement('kbd');
    esc.className = 'sv-palette-esc';
    esc.textContent = 'Esc';
    esc.setAttribute('aria-hidden', 'true');
    field.append(input, esc);

    const list = document.createElement('div');
    list.className = 'sv-palette-list';
    list.id = `${uid}-list`;
    list.setAttribute('role', 'listbox');
    list.setAttribute('aria-label', 'Actions');

    const empty = document.createElement('p');
    empty.className = 'sv-palette-empty';
    // A live region that is always present: its text is set only while
    // nothing matches, so the change is announced.
    empty.setAttribute('role', 'status');

    dialog.append(field, list, empty);
    document.body.append(dialog);

    /** @type {HTMLElement[]} */
    let options$ = [];
    let active = -1;

    function setActive(index) {
      if (options$.length === 0) {
        active = -1;
        input.removeAttribute('aria-activedescendant');
        return;
      }
      active = (index + options$.length) % options$.length;
      options$.forEach((option, i) => option.setAttribute('aria-selected', String(i === active)));
      const current = options$[active];
      input.setAttribute('aria-activedescendant', current.id);
      current.scrollIntoView({ block: 'nearest' });
    }

    function render() {
      const query = input.value.trim().toLowerCase();
      const matches = options.items
        .map((item, order) => ({ item, order, score: matchScore(item, query) }))
        .filter((entry) => entry.score > 0);
      if (query) matches.sort((a, b) => b.score - a.score || a.order - b.order);

      list.replaceChildren();
      options$ = [];
      let group = null;
      for (const { item } of matches) {
        // Group headings only while browsing; a ranked search is one list.
        if (!query && item.group && item.group !== group) {
          group = item.group;
          const heading = document.createElement('div');
          heading.className = 'sv-palette-group';
          heading.setAttribute('role', 'presentation');
          heading.textContent = group;
          list.append(heading);
        }
        const option = document.createElement('div');
        option.className = 'sv-palette-option';
        option.id = `${uid}-${options$.length}`;
        option.dataset.id = item.id;
        option.setAttribute('role', 'option');
        if (item.danger) option.classList.add('sv-palette-danger');
        const label = document.createElement('span');
        label.className = 'sv-palette-label';
        label.textContent = item.label;
        if (item.detail) {
          const text = document.createElement('span');
          text.className = 'sv-palette-text';
          const detail = document.createElement('span');
          detail.className = 'sv-palette-detail';
          detail.textContent = item.detail;
          text.append(label, detail);
          option.append(text);
        } else {
          option.append(label);
        }
        if (item.hint) {
          const hint = document.createElement('span');
          hint.className = 'sv-palette-hint';
          hint.textContent = item.hint;
          option.append(hint);
        }
        list.append(option);
        options$.push(option);
      }
      const emptyText = options$.length > 0 ? '' : 'No matching command';
      if (empty.textContent !== emptyText) empty.textContent = emptyText;
      setActive(0);
    }

    return new Promise((resolve) => {
      let result = null;
      const choose = (option) => {
        if (!option) return;
        result = option.dataset.id;
        dialog.close();
      };

      input.addEventListener('input', render);
      input.addEventListener('keydown', (event) => {
        const ctrl = event.ctrlKey && !event.altKey && !event.metaKey;
        if (event.key === 'ArrowDown' || (ctrl && (event.key === 'n' || event.key === 'j'))) {
          event.preventDefault();
          setActive(active + 1);
        } else if (event.key === 'ArrowUp' || (ctrl && (event.key === 'p' || event.key === 'k'))) {
          event.preventDefault();
          setActive(active - 1);
        } else if (event.key === 'Enter') {
          event.preventDefault();
          choose(options$[active]);
        }
      });
      list.addEventListener('pointermove', (event) => {
        const option = event.target instanceof Element ? event.target.closest('[role="option"]') : null;
        const index = option ? options$.indexOf(option) : -1;
        if (index >= 0 && index !== active) setActive(index);
      });
      list.addEventListener('click', (event) => {
        choose(event.target instanceof Element ? event.target.closest('[role="option"]') : null);
      });
      // A click on the backdrop lands on the dialog element itself.
      dialog.addEventListener('click', (event) => {
        if (event.target === dialog) dialog.close();
      });
      dialog.addEventListener('close', () => {
        dialog.remove();
        resolve(result);
      }, { once: true });

      render();
      dialog.showModal();
      input.focus();
    });
  }

  // ----- Settings page ------------------------------------------------------
  //
  // The settings form is plain HTML and saves without this file. Here we only
  // add feedback: the unsaved-changes bar, sidebar scrollspy, color swatches,
  // the keybinding filter, the key recorder and conflict hints.

  /** @type {(() => void) | null} */
  let teardownSettings = null;

  // Same key spellings as internal/domain/keychord, so two spellings of one
  // chord compare equal.
  const KEY_ALIASES = {
    esc: 'escape', enter: 'return', del: 'delete', page_up: 'pageup', page_down: 'pagedown',
    arrowleft: 'left', arrowright: 'right', arrowup: 'up', arrowdown: 'down',
    '+': 'plus', '-': 'minus', '=': 'equal',
    '[': 'bracketleft', ']': 'bracketright', '{': 'braceleft', '}': 'braceright',
  };

  function canonicalChord(raw) {
    const text = raw.trim();
    if (text === '+') return 'plus';
    const mods = new Set();
    let key = '';
    for (const part of text.split('+')) {
      const lower = part.trim().toLowerCase();
      if (lower === 'ctrl' || lower === 'control') mods.add('ctrl');
      else if (lower === 'alt' || lower === 'shift') mods.add(lower);
      else if (lower !== '') {
        if (key) return text;
        key = part.trim();
      }
    }
    if (!key && text.endsWith('++')) key = '+';
    if (!key) return text;
    if (/^[A-Z]$/.test(key)) {
      mods.add('shift');
      key = key.toLowerCase();
    }
    const lower = key.toLowerCase();
    key = KEY_ALIASES[lower] ?? (key.length === 1 ? key : lower);
    return [...['ctrl', 'alt', 'shift'].filter((mod) => mods.has(mod)), key].join('+');
  }

  // Mirrors parseKeyList in config_actions.go: a trailing comma separates
  // keys unless the comma is the key ("," or "ctrl+,").
  function keyPart(key) {
    let rest = key;
    for (;;) {
      const plus = rest.indexOf('+');
      if (plus <= 0) return rest;
      if (!['ctrl', 'control', 'alt', 'shift'].includes(rest.slice(0, plus).toLowerCase())) return rest;
      rest = rest.slice(plus + 1);
    }
  }

  function splitKeys(raw) {
    return raw.split(/\s+/).filter(Boolean).map((part) => {
      const trimmed = part.endsWith(',') ? part.slice(0, -1) : part;
      return trimmed !== part && keyPart(trimmed) !== '' ? trimmed : part;
    });
  }

  const NAMED_KEYS = {
    ' ': 'space', Escape: 'escape', Enter: 'enter', Tab: 'tab', Backspace: 'backspace',
    Delete: 'delete', Home: 'home', End: 'end', PageUp: 'pageup', PageDown: 'pagedown',
    ArrowUp: 'arrowup', ArrowDown: 'arrowdown', ArrowLeft: 'arrowleft', ArrowRight: 'arrowright',
    '+': 'plus', '-': 'minus', '=': 'equal',
  };
  const MODIFIER_KEYS = new Set(['Control', 'Shift', 'Alt', 'AltGraph', 'Meta', 'OS', 'CapsLock', 'Dead', 'Process', 'Unidentified']);

  // Turns a keydown into a config chord: "ctrl+shift+t", "alt+}", "f5".
  // Letters keep Shift; shifted symbols are stored bare, like the defaults.
  function chordFromEvent(event) {
    if (MODIFIER_KEYS.has(event.key)) return null;
    let key = NAMED_KEYS[event.key];
    let shift = event.shiftKey;
    if (key) {
      if (key === 'plus' || key === 'minus' || key === 'equal') shift = false;
    } else if (/^F\d{1,2}$/.test(event.key)) {
      key = event.key.toLowerCase();
    } else if (event.key.length === 1) {
      key = event.key.toLowerCase();
      if (!/[a-z]/.test(key)) shift = false;
    } else {
      return null;
    }
    const mods = [];
    if (event.ctrlKey) mods.push('ctrl');
    if (event.altKey) mods.push('alt');
    if (shift) mods.push('shift');
    return [...mods, key].join('+');
  }

  function fieldIsDirty(field) {
    if (field.type === 'checkbox' || field.type === 'radio') return field.checked !== field.defaultChecked;
    if (field instanceof HTMLSelectElement) {
      return [...field.options].some((option) => option.selected !== option.defaultSelected);
    }
    return field.value !== field.defaultValue;
  }

  /**
   * Adds the settings and favorites page behaviours under root. Safe to call
   * after every mount: the previous page's listeners and observers are
   * removed first.
   * @param {Element} root
   */
  function enhance(root) {
    teardownSettings?.();
    teardownSettings = null;
    teardownFavorites?.();
    teardownFavorites = null;
    const favorites = root.querySelector('[data-sv-favorites]');
    if (favorites) enhanceFavorites(favorites);
    const form = root.querySelector('[data-sv-config-form]');
    if (!(form instanceof HTMLFormElement)) return;

    const cleanups = [];
    const on = (target, type, handler, options) => {
      target.addEventListener(type, handler, options);
      cleanups.push(() => target.removeEventListener(type, handler, options));
    };
    teardownSettings = () => cleanups.forEach((cleanup) => cleanup());

    const savebar = root.querySelector('[data-sv-savebar]');
    const status = root.querySelector('[data-sv-region="config-status"]');
    const navLinks = [...root.querySelectorAll('[data-sv-config-nav]')];
    const afterReset = (fn) => window.setTimeout(fn, 0);

    // --- Unsaved changes ---------------------------------------------------

    const MARK_ROWS = '[data-sv-setting], [data-sv-shortcut-row], [data-sv-kb-row], [data-sv-color]';

    function refreshDirty() {
      for (const marked of root.querySelectorAll('[data-sv-dirty]')) {
        if (marked !== form) marked.removeAttribute('data-sv-dirty');
      }
      const changed = new Set();
      const sections = new Set();
      for (const field of form.elements) {
        if (!field.name || field.type === 'hidden' || field.disabled || !fieldIsDirty(field)) continue;
        changed.add(field.name);
        field.closest(MARK_ROWS)?.setAttribute('data-sv-dirty', '');
        const section = field.closest('[data-sv-config-section]');
        if (section) sections.add(section.id);
      }
      for (const link of navLinks) {
        link.toggleAttribute('data-sv-dirty', sections.has(link.dataset.svConfigNav));
      }
      const count = changed.size;
      if (count > 0) form.dataset.svDirty = String(count);
      else delete form.dataset.svDirty;
      savebar?.toggleAttribute('data-sv-open', count > 0 || Boolean(status?.querySelector('.sv-savebar-error')));
      const label = status?.querySelector('[data-sv-dirty-label]');
      if (label) label.textContent = count === 1 ? '1 unsaved change' : `${count} unsaved changes`;
    }

    // A few hundred fields at most: refreshing on every edit is cheap.
    const refresh = () => {
      refreshDirty();
      refreshKeybindings();
    };

    // --- Colors and fonts --------------------------------------------------

    function syncColor(input) {
      const field = input.closest('[data-sv-color]');
      const value = input.value.trim();
      const valid = value !== '' && CSS.supports('color', value);
      field?.querySelector('.sv-color-swatch')?.style.setProperty('--sv-swatch', valid ? value : 'transparent');
      const picker = field?.querySelector('[data-sv-color-picker]');
      if (picker && /^#[0-9a-f]{6}$/i.test(value)) picker.value = value.toLowerCase();
      const preview = root.querySelector(`[data-sv-palette-preview="${input.dataset.svPalette}"]`);
      const name = `--pp-${(input.dataset.svPaletteKey || '').replace(/_/g, '-')}`;
      if (preview && valid) preview.style.setProperty(name, value);
    }

    function syncFont(input) {
      input.style.fontFamily = input.value.trim() ? `${input.value}, var(--sv-font-sans, system-ui)` : '';
    }

    function syncAllFields() {
      root.querySelectorAll('[data-sv-color-input]').forEach(syncColor);
      root.querySelectorAll('[data-sv-font-input]').forEach(syncFont);
    }

    // --- Search shortcut rows ----------------------------------------------

    // Typing in the trailing blank row adds another blank row below it.
    function growShortcuts(row) {
      if (row.nextElementSibling || ![...row.querySelectorAll('input:not([type="hidden"])')].some((input) => input.value)) {
        return;
      }
      const rows = row.parentElement.querySelectorAll('[data-sv-shortcut-row]');
      const clone = row.cloneNode(true);
      clone.dataset.svShortcutClone = '';
      clone.removeAttribute('data-sv-dirty');
      for (const input of clone.querySelectorAll('input')) {
        input.name = input.name.replace(/^shortcut\.\d+\./, `shortcut.${rows.length}.`);
        input.value = '';
        input.removeAttribute('aria-invalid');
      }
      row.after(clone);
    }

    // --- Keybindings -------------------------------------------------------

    const kbRows = [...root.querySelectorAll('[data-sv-kb-row]')];
    const filter = root.querySelector('[data-sv-keybinding-filter]');
    const filterEmpty = root.querySelector('[data-sv-keybinding-empty]');

    function refreshKeybindings() {
      const byMode = new Map();
      for (const row of kbRows) {
        const input = row.querySelector('[data-sv-kb-input]');
        const chords = splitKeys(input.value).map(canonicalChord);
        const defaults = splitKeys(input.dataset.defaultKeys || '').map(canonicalChord);
        const reset = row.querySelector('[data-sv-kb-reset]');
        if (reset) reset.hidden = chords.join(' ') === defaults.join(' ');
        const mode = row.dataset.svKbMode || '';
        if (!byMode.has(mode)) byMode.set(mode, new Map());
        const owners = byMode.get(mode);
        for (const chord of new Set(chords)) {
          if (!owners.has(chord)) owners.set(chord, []);
          owners.get(chord).push(row);
        }
        row.svChords = chords;
      }
      for (const row of kbRows) {
        const owners = byMode.get(row.dataset.svKbMode || '');
        const others = new Set();
        for (const chord of row.svChords) {
          for (const other of owners.get(chord) || []) if (other !== row) others.add(other);
        }
        const note = row.querySelector('[data-sv-kb-conflict]');
        if (!note) continue;
        note.hidden = others.size === 0;
        note.textContent = others.size === 0 ? '' :
          `Also used by ${[...others].map((other) => other.querySelector('.sv-kb-label')?.textContent.trim()).join(', ')}`;
        row.toggleAttribute('data-sv-conflict', others.size > 0);
      }
    }

    function applyFilter() {
      const query = (filter?.value || '').trim().toLowerCase();
      let visible = 0;
      for (const row of kbRows) {
        const text = `${row.querySelector('.sv-kb-label')?.textContent} ${row.querySelector('.sv-kb-action')?.textContent} ${row.querySelector('[data-sv-kb-input]')?.value}`.toLowerCase();
        row.hidden = query !== '' && !text.includes(query);
        if (!row.hidden) visible += 1;
      }
      for (const group of root.querySelectorAll('[data-sv-kb-group]')) {
        group.hidden = query !== '' && group.querySelector('[data-sv-kb-row]:not([hidden])') === null;
      }
      if (filterEmpty) filterEmpty.hidden = visible > 0 || query === '';
    }

    /** @type {{input: HTMLInputElement, previous: string, button: HTMLElement} | null} */
    let recording = null;

    function stopRecording(keep) {
      if (!recording) return;
      const { input, previous, button } = recording;
      recording = null;
      input.closest('[data-sv-kb-row]')?.removeAttribute('data-sv-recording');
      button.setAttribute('aria-pressed', 'false');
      input.placeholder = 'Unassigned';
      if (!keep) input.value = previous;
      input.dispatchEvent(new Event('input', { bubbles: true }));
    }

    function startRecording(input, button) {
      stopRecording(true);
      recording = { input, previous: input.value, button };
      input.closest('[data-sv-kb-row]')?.setAttribute('data-sv-recording', '');
      button.setAttribute('aria-pressed', 'true');
      input.value = '';
      input.placeholder = 'Press keys… (Esc cancels)';
      // A focused, editable field tells Dumber to pass keys to the page.
      input.focus();
    }

    // The record buttons are rendered by the page (recordable groups only);
    // this only wires their behaviour.
    for (const button of root.querySelectorAll('[data-sv-kb-record]')) {
      const input = button.closest('[data-sv-kb-row]')?.querySelector('[data-sv-kb-input]');
      if (!input) continue;
      // Keep focus in the field so a second click stops the recording
      // instead of blurring (and cancelling) it first.
      on(button, 'pointerdown', (event) => event.preventDefault());
      on(button, 'click', () => {
        if (recording?.input === input) stopRecording(false);
        else startRecording(input, button);
      });
    }

    // Window capture runs before every other key handler, so a recorded
    // Ctrl+S or Ctrl+K is captured instead of executed.
    on(window, 'keydown', (event) => {
      if (!recording || event.target !== recording.input) return;
      if (event.key === 'Tab') {
        stopRecording(false);
        return;
      }
      event.preventDefault();
      event.stopImmediatePropagation();
      if (event.key === 'Escape' && !event.ctrlKey && !event.altKey && !event.shiftKey) {
        stopRecording(false);
        return;
      }
      const chord = chordFromEvent(event);
      if (!chord) return;
      recording.input.value = chord;
      stopRecording(true);
    }, true);

    // --- Sidebar -----------------------------------------------------------

    function setCurrent(id) {
      for (const link of navLinks) {
        if (link.dataset.svConfigNav === id) link.setAttribute('aria-current', 'true');
        else link.removeAttribute('aria-current');
      }
    }

    const sections = [...root.querySelectorAll('[data-sv-config-section]')];
    if (sections.length > 0 && 'IntersectionObserver' in window) {
      const visible = new Set();
      const observer = new IntersectionObserver((entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) visible.add(entry.target);
          else visible.delete(entry.target);
        }
        const first = sections.find((section) => visible.has(section));
        if (first) setCurrent(first.id);
      }, { rootMargin: '-15% 0px -70% 0px' });
      sections.forEach((section) => observer.observe(section));
      cleanups.push(() => observer.disconnect());
    }
    if (sections[0]) setCurrent(sections[0].id);

    for (const link of navLinks) {
      link.addEventListener('click', (event) => {
        const target = root.querySelector(`#${CSS.escape(link.dataset.svConfigNav)}`);
        if (!target) return;
        event.preventDefault();
        setCurrent(target.id);
        target.scrollIntoView({ behavior: reducedMotion.matches ? 'auto' : 'smooth', block: 'start' });
        focusSection(target);
      });
    }

    // --- Wiring ------------------------------------------------------------

    on(form, 'input', (event) => {
      const target = event.target;
      if (!(target instanceof HTMLElement)) return;
      target.removeAttribute('aria-invalid');
      if (target.matches('[data-sv-color-input]')) syncColor(target);
      else if (target.matches('[data-sv-color-picker]')) {
        const text = target.closest('[data-sv-color]')?.querySelector('[data-sv-color-input]');
        if (text) {
          text.value = target.value;
          syncColor(text);
        }
      } else if (target.matches('[data-sv-font-input]')) syncFont(target);
      else if (target.matches('[data-sv-keybinding-filter]')) {
        applyFilter();
        return;
      }
      // A validation error replaced the dirty label: bring it back.
      if (status && !status.querySelector('[data-sv-dirty-label]')) {
        status.innerHTML = '<p class="sv-savebar-message" data-sv-dirty-label></p>';
      }
      const shortcut = target.closest('[data-sv-shortcut-new]');
      if (shortcut) growShortcuts(shortcut);
      refresh();
    });
    on(form, 'change', refresh);
    on(form, 'reset', () => {
      stopRecording(false);
      afterReset(() => {
        root.querySelectorAll('[data-sv-shortcut-clone]').forEach((row) => row.remove());
        root.querySelectorAll('[aria-invalid]').forEach((field) => field.removeAttribute('aria-invalid'));
        if (status) status.innerHTML = '<p class="sv-savebar-message" data-sv-dirty-label></p>';
        syncAllFields();
        applyFilter();
        refreshDirty();
        refreshKeybindings();
      });
    });
    on(form, 'focusout', (event) => {
      if (recording && event.target === recording.input) stopRecording(false);
    });
    if (filter) {
      on(filter, 'keydown', (event) => {
        if (event.key !== 'Escape') return;
        event.preventDefault();
        if (filter.value) {
          filter.value = '';
          applyFilter();
        } else {
          filter.blur();
        }
      });
    }

    syncAllFields();
    refreshDirty();
    refreshKeybindings();
  }

  // ----- Favorites ----------------------------------------------------------

  /** @type {(() => void) | null} */
  let teardownFavorites = null;

  const anchorPositioning = typeof CSS !== 'undefined' && CSS.supports?.('anchor-name: --sv-a');

  /**
   * Adds the favorites page behaviours: edit panels with unsaved-change
   * status, the local text filter, tag popovers and the color picker.
   * Listeners are delegated on root, which region swaps never replace.
   * @param {Element} root
   */
  function enhanceFavorites(root) {
    const cleanups = [];
    const on = (target, type, handler, options) => {
      target.addEventListener(type, handler, options);
      cleanups.push(() => target.removeEventListener(type, handler, options));
    };
    teardownFavorites = () => cleanups.forEach((cleanup) => cleanup());
    let query = '';

    // --- Edit panels --------------------------------------------------------

    function panelOf(row) {
      return row?.querySelector('[data-sv-favorite-panel]') ?? null;
    }

    function setPanel(row, open) {
      const panel = panelOf(row);
      const toggle = row?.querySelector('[data-sv-favorite-edit]');
      if (!panel || !toggle) return;
      panel.hidden = !open;
      toggle.setAttribute('aria-expanded', String(open));
      row.toggleAttribute('data-sv-open', open);
      if (open) {
        const title = panel.querySelector('[name="title"]');
        title?.focus();
        title?.select?.();
      }
    }

    function refreshPanelDirty(form) {
      const fields = [...form.elements].filter((field) => field.name && field.type !== 'hidden');
      const names = new Set(fields.filter(fieldIsDirty).map((field) => field.name));
      form.toggleAttribute('data-sv-dirty', names.size > 0);
      const label = form.querySelector('[data-sv-favorite-dirty]');
      if (label) label.textContent = names.size === 0 ? '' : names.size === 1 ? '1 unsaved change' : `${names.size} unsaved changes`;
    }

    // --- Text filter --------------------------------------------------------

    function applyQuery() {
      const input = root.querySelector('[data-sv-favorite-filter]');
      if (input && input.value !== query) input.value = query;
      const needle = query.trim().toLowerCase();
      let shown = 0;
      for (const row of root.querySelectorAll('[data-sv-favorite-row]')) {
        const match = !needle || (row.dataset.svSearch ?? '').includes(needle);
        row.hidden = !match;
        if (match) shown += 1;
      }
      const none = root.querySelector('[data-sv-favorite-nomatch]');
      if (none) none.hidden = shown > 0;
    }

    // Region swaps (filters, undo) rebuild the list: keep the typed filter.
    // swap() runs this before Go restores focus, so focus never lands on a
    // row that is about to be hidden.
    afterSwapHooks.add(applyQuery);
    cleanups.push(() => afterSwapHooks.delete(applyQuery));

    // --- Tag popovers -------------------------------------------------------

    function syncCustomColor(picker) {
      const custom = picker.closest('[data-sv-tag-form]')?.querySelector('[data-sv-color-custom]');
      if (!custom) return;
      custom.value = picker.value;
      custom.checked = true;
      custom.closest('.sv-swatch')?.style.setProperty('--tag-color', picker.value);
    }

    function placePopover(popover) {
      if (anchorPositioning) return;
      const invoker = root.querySelector(`[popovertarget="${CSS.escape(popover.id)}"]`);
      if (!invoker) return;
      const rect = invoker.getBoundingClientRect();
      const width = popover.offsetWidth;
      const left = Math.max(8, Math.min(rect.left, window.innerWidth - width - 8));
      popover.style.left = `${left}px`;
      popover.style.top = `${Math.min(rect.bottom + 6, window.innerHeight - popover.offsetHeight - 8)}px`;
    }

    // --- Wiring ------------------------------------------------------------

    on(root, 'click', (event) => {
      const target = event.target instanceof Element ? event.target : null;
      if (!target) return;
      const edit = target.closest('[data-sv-favorite-edit]');
      if (edit) {
        const row = edit.closest('[data-sv-favorite-row]');
        setPanel(row, panelOf(row)?.hidden ?? false);
        return;
      }
      const close = target.closest('[data-sv-favorite-close]');
      if (close) {
        const row = close.closest('[data-sv-favorite-row]');
        setPanel(row, false);
        row?.querySelector('[data-sv-favorite-edit]')?.focus();
      }
    });
    on(root, 'keydown', (event) => {
      const target = event.target instanceof Element ? event.target : null;
      if (!target || event.key !== 'Escape') return;
      if (target.matches('[data-sv-favorite-filter]')) {
        if (query) {
          event.preventDefault();
          query = '';
          applyQuery();
        } else {
          target.blur();
        }
        return;
      }
      if (target.closest('[popover]')) return; // the popover closes natively
      const panel = target.closest('[data-sv-favorite-panel]');
      if (panel) {
        event.preventDefault();
        const row = panel.closest('[data-sv-favorite-row]');
        setPanel(row, false);
        row?.querySelector('[data-sv-favorite-edit]')?.focus();
      }
    });
    on(root, 'input', (event) => {
      const target = event.target instanceof Element ? event.target : null;
      if (!target) return;
      if (target.matches('[data-sv-favorite-filter]')) {
        query = target.value;
        applyQuery();
        return;
      }
      if (target.matches('[data-sv-color-picker]')) syncCustomColor(target);
      const form = target.closest('[data-sv-favorite-form]');
      if (form) refreshPanelDirty(form);
    });
    on(root, 'change', (event) => {
      const form = event.target instanceof Element ? event.target.closest('[data-sv-favorite-form]') : null;
      if (form) refreshPanelDirty(form);
    });
    on(root, 'reset', (event) => {
      const form = event.target;
      if (!(form instanceof HTMLFormElement)) return;
      window.setTimeout(() => {
        if (form.matches('[data-sv-favorite-form]')) refreshPanelDirty(form);
        const picker = form.querySelector('[data-sv-color-picker]');
        const custom = form.querySelector('[data-sv-color-custom]');
        if (picker && custom) custom.closest('.sv-swatch')?.style.setProperty('--tag-color', custom.value);
      }, 0);
    });
    // toggle does not bubble: listen in the capture phase.
    on(root, 'toggle', (event) => {
      const popover = event.target;
      if (!(popover instanceof HTMLElement) || !popover.hasAttribute('popover')) return;
      if (event.newState === 'open') {
        placePopover(popover);
        const name = popover.querySelector('[name="name"]');
        name?.focus();
        name?.select?.();
      } else {
        popover.querySelector('form')?.reset();
      }
    }, true);

    applyQuery();
  }

  const api = {
    version: 1,
    swap,
    toast,
    dismissToast,
    focusSection,
    palette,
    settingsIndex,
    jumpToSetting,
    enhance,
  };

  window.dumberSystemviews = Object.freeze(api);
  document.documentElement.dataset.svEnhanced = String(api.version);
})();
