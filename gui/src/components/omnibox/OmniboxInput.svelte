<!--
  Omnibox Input Component

  Input field with keyboard handling and debounced search
-->
<script lang="ts">
  import { omniboxStore } from './stores.svelte.ts';
  import { debouncedQuery, debouncedFind, debouncedPrefixQuery, omniboxBridge } from './messaging';
  import { findInPage } from './find';

  // Reactive state
  let mode = $derived(omniboxStore.mode);
  let viewMode = $derived(omniboxStore.viewMode);
  let inputValue = $derived(omniboxStore.inputValue);
  let selectedIndex = $derived(omniboxStore.selectedIndex);
  let suggestions = $derived(omniboxStore.suggestions);
  let favorites = $derived(omniboxStore.favorites);
  let matches = $derived(omniboxStore.matches);
  let searchShortcuts = $derived(omniboxStore.searchShortcuts);
  let inlineCompletion = $derived(omniboxStore.inlineCompletion);
  let inlineSuggestion = $derived(omniboxStore.inlineSuggestion);

  // Cursor tracking for ghost text visibility
  let cursorAtEnd = $state(true);

  // Input element ref and responsive styles (props for parent to bind)
  interface Props {
    inputElement?: HTMLInputElement;
    responsiveStyles?: {
      fontSize: string;
      inputPadding: string;
    };
  }

  let { inputElement = $bindable(), responsiveStyles }: Props = $props();

  const ACCENT_FALLBACK = 'var(--dynamic-accent)';

  interface CommandBadge {
    prefix: string;
    label: string;
  }

  // Build command badges dynamically from search shortcuts
  let COMMAND_BADGES = $derived(
    Object.entries(searchShortcuts).map(([key, shortcut]) => ({
      prefix: `${key}:`,
      label: shortcut.description || key.toUpperCase()
    }))
  );

  const BADGE_LEFT_OFFSET = '64px';

  function resolveAccentColor(): string {
    if (!inputElement) return ACCENT_FALLBACK;
    const inlineAccent = inputElement.style.getPropertyValue('--dumber-input-accent');
    if (inlineAccent && inlineAccent.trim()) {
      return inlineAccent.trim();
    }
    try {
      const computedAccent = getComputedStyle(inputElement).getPropertyValue('--dynamic-accent');
      if (computedAccent && computedAccent.trim()) {
        return computedAccent.trim();
      }
    } catch { /* no-op */ }
    return ACCENT_FALLBACK;
  }

  // Computed placeholder
  let placeholder = $derived(
    mode === 'find' ? 'Find in page…' : 'Type URL or search…'
  );

  const DEFAULT_PADDING: [string, string, string, string] = ['10px', '12px', '10px', '12px'];

  function normalizePadding(padding?: string): [string, string, string, string] {
    if (!padding) return DEFAULT_PADDING;
    const parts = padding.trim().split(/\s+/).filter(Boolean);
    if (parts.length === 0) return DEFAULT_PADDING;
    const p0 = parts[0] ?? DEFAULT_PADDING[0];
    const p1 = parts[1] ?? p0;
    const p2 = parts[2] ?? p0;
    const p3 = parts[3] ?? p1;

    if (parts.length === 1) return [p0, p0, p0, p0];
    if (parts.length === 2) return [p0, p1, p0, p1];
    if (parts.length === 3) return [p0, p1, p2, p1];
    return [p0, p1, p2, p3];
  }

  let activeBadge = $state<CommandBadge | null>(null);
  // Ghost text visibility - only show when cursor is at end and no command badge active
  let showGhostText = $derived(
    mode === 'omnibox' &&
    inlineCompletion &&
    cursorAtEnd &&
    inputValue.length > 0 &&
    !activeBadge
  );
  let basePaddingSegments = $state<[string, string, string, string]>(DEFAULT_PADDING);
  let inputPadding = $state(DEFAULT_PADDING.join(' '));
  let baseLeftPadding = $state(DEFAULT_PADDING[3]);
  let inputFontSize = $derived(responsiveStyles?.fontSize || '16px');

  $effect(() => {
    basePaddingSegments = normalizePadding(responsiveStyles?.inputPadding);
  });

  // Debug: log when inline suggestion state changes
  $effect(() => {
    console.log('[INLINE] State changed:', {
      inlineCompletion,
      inlineSuggestion,
      showGhostText,
      cursorAtEnd,
      mode,
      inputValueLength: inputValue?.length,
      activeBadge: !!activeBadge
    });
  });

  $effect(() => {
    const value = inputValue || '';
    const trimmedValue = value.trimStart();
    activeBadge = COMMAND_BADGES.find(({ prefix }) => trimmedValue.startsWith(prefix)) || null;
  });

  $effect(() => {
    const [top, right, bottom, left] = basePaddingSegments;
    baseLeftPadding = left;
    const leftPadding = activeBadge ? `calc(${left} + ${BADGE_LEFT_OFFSET})` : left;
    inputPadding = `${top} ${right} ${bottom} ${leftPadding}`;
  });

  // Track cursor position for ghost text visibility
  function updateCursorPosition() {
    if (inputElement) {
      cursorAtEnd = inputElement.selectionStart === inputValue.length &&
                    inputElement.selectionEnd === inputValue.length;
    }
  }

  // Accept inline suggestion (full or word-by-word)
  function acceptInlineSuggestion(acceptMode: 'full' | 'word') {
    if (!inlineSuggestion || !inlineCompletion) return;

    switch (acceptMode) {
      case 'full':
        // Accept entire suggestion
        omniboxStore.setInputValue(inlineSuggestion);
        omniboxStore.clearInlineSuggestion();
        // Query for next prefix match
        debouncedPrefixQuery(inlineSuggestion);
        // Also update the search results
        debouncedQuery(inlineSuggestion);
        break;

      case 'word':
        // Accept next word at cursor position
        const cursorPos = inputElement?.selectionStart ?? inputValue.length;
        const remainingCompletion = inlineSuggestion.slice(cursorPos);

        // Find next word boundary (handles /, ., -, _ as boundaries)
        const wordMatch = remainingCompletion.match(/^([^\s\/\.\-\_]+[\s\/\.\-\_]?)/);
        if (wordMatch && wordMatch[1]) {
          const wordToInsert = wordMatch[1];
          const newValue = inputValue.slice(0, cursorPos) + wordToInsert + inputValue.slice(cursorPos);
          const newCursorPos = cursorPos + wordToInsert.length;

          omniboxStore.setInputValue(newValue);

          requestAnimationFrame(() => {
            if (inputElement) {
              inputElement.setSelectionRange(newCursorPos, newCursorPos);
              updateCursorPosition();
            }
          });

          debouncedPrefixQuery(newValue);
          debouncedQuery(newValue);
        }
        break;
    }
  }

  // Handle input changes
  function handleInput(event: Event) {
    const target = event.target as HTMLInputElement;
    const value = target.value || '';

    omniboxStore.setInputValue(value);
    omniboxStore.setFaded(false);
    updateCursorPosition();

    if (mode === 'omnibox') {
      // Only query backend if we're in history view
      // In favorites view, filtering happens locally via computed state
      if (viewMode === 'history') {
        if (value === '') {
          // Input is empty - fetch initial history based on config
          omniboxBridge.fetchInitialHistory();
          omniboxStore.clearInlineSuggestion();
        } else {
          // Input has content - perform search
          debouncedQuery(value);
          // Also query for inline suggestion (fish-style ghost text)
          debouncedPrefixQuery(value);
        }
      }
    } else if (mode === 'find') {
      // Debounced find with minimum length to avoid freeze on first letter
      debouncedFind(value, findInPage);
    }
  }

  // Handle key events
  function handleKeyDown(event: KeyboardEvent) {
    switch (event.key) {
      case 'Escape':
        event.preventDefault();
        event.stopPropagation();

        if (inputValue && inputValue.trim() !== '') {
          // Input has text - clear it but keep omnibox open
          omniboxStore.setInputValue('');

          // In find mode, clear the search highlights
          if (mode === 'find') {
            findInPage('');
          }
        } else {
          // Input is empty - close omnibox immediately
          omniboxStore.close();
        }
        break;

      case 'Enter':
        event.preventDefault();
        event.stopPropagation();
        handleEnterKey(event);
        break;

      case 'Tab':
        // Only handle Tab in omnibox mode (not in find mode)
        if (mode === 'omnibox') {
          event.preventDefault();
          event.stopPropagation();
          // Toggle between history and favorites views
          const newViewMode = viewMode === 'history' ? 'favorites' : 'history';
          omniboxStore.setViewMode(newViewMode);
        }
        break;

      case ' ':
        // Only handle Space in omnibox mode when an item is selected
        if (mode === 'omnibox' && selectedIndex >= 0) {
          event.preventDefault();
          event.stopPropagation();
          handleSpaceKey();
        }
        break;

      case 'ArrowDown':
      case 'ArrowUp':
        event.preventDefault();
        event.stopPropagation();
        handleArrowKeys(event.key);
        break;

      case 'ArrowRight':
        // Handle inline suggestion acceptance (fish-style)
        if (mode === 'omnibox' && inlineCompletion) {
          if (event.ctrlKey || event.metaKey) {
            // Ctrl+Right: accept next word
            event.preventDefault();
            event.stopPropagation();
            acceptInlineSuggestion('word');
          } else if (cursorAtEnd) {
            // Right at end: accept full suggestion
            event.preventDefault();
            event.stopPropagation();
            acceptInlineSuggestion('full');
          }
          // If cursor not at end and no modifier, let default behavior happen
        }
        break;

      case 'y':
        // Ctrl+Y: accept full inline suggestion (alternative to Right Arrow)
        if (event.ctrlKey && mode === 'omnibox' && inlineCompletion) {
          event.preventDefault();
          event.stopPropagation();
          acceptInlineSuggestion('full');
        }
        break;

      default:
        // Handle Ctrl+1 through Ctrl+0 for quick navigation to first 10 results
        // Use event.code for keyboard layout independence (QWERTY, AZERTY, QWERTZ, etc.)
        if (event.ctrlKey && mode === 'omnibox') {
          const code = event.code;
          let targetIndex = -1;

          // Map physical keys: Digit1->0, Digit2->1, ..., Digit9->8, Digit0->9
          if (code >= 'Digit1' && code <= 'Digit9') {
            targetIndex = parseInt(code.substring(5), 10) - 1;
          } else if (code === 'Digit0') {
            targetIndex = 9;
          }

          if (targetIndex >= 0) {
            event.preventDefault();
            event.stopPropagation();
            handleNumberKey(targetIndex);
            return;
          }
        }

        // For normal typing keys, only stop propagation to prevent page handlers
        // but don't prevent default so typing still works in the input
        event.stopPropagation();
        // Any other key should restore full opacity
        omniboxStore.setFaded(false);
        break;
    }
  }

  // Handle Enter key based on mode
  function handleEnterKey(event: KeyboardEvent) {
    if (mode === 'omnibox') {
      // Get selected item from current view (history or favorites)
      const currentList = viewMode === 'history' ? suggestions : favorites;
      const selectedItem = selectedIndex >= 0 ? currentList[selectedIndex] : null;
      const url = selectedItem?.url || inputValue || '';

      if (url) {
        omniboxBridge.navigate(url);
        omniboxStore.close();
      }
    } else if (mode === 'find') {
      if (event.shiftKey) {
        // Shift+Enter: jump to previous match
        jumpToMatch(-1);
      } else if (event.altKey) {
        // Alt+Enter: center on current match but keep overlay open
        revealCurrentMatch();
      } else {
        // Enter: center and close
        revealCurrentMatch();
        omniboxStore.close();
      }
    }
  }

  // Handle Space key to toggle favorite
  function handleSpaceKey() {
    if (mode !== 'omnibox' || selectedIndex < 0) return;

    // Get the selected item based on current view mode
    const item = viewMode === 'history'
      ? suggestions[selectedIndex]
      : favorites[selectedIndex];

    if (!item || !item.url) return;

    // Extract title and favicon with proper type narrowing
    const title = 'title' in item ? item.title : '';
    const faviconURL = 'favicon_url' in item ? item.favicon_url : ('favicon' in item ? item.favicon : '') ?? '';

    // Toggle the favorite status
    omniboxBridge.toggleFavorite(item.url, title, faviconURL);

    // Refresh the favorites list after toggling
    setTimeout(() => {
      omniboxBridge.fetchFavorites();
    }, 100);
  }

  // Handle Ctrl+Number key for quick navigation
  function handleNumberKey(targetIndex: number) {
    if (mode !== 'omnibox') return;

    // Get the current list based on view mode
    const currentList = viewMode === 'history' ? suggestions : favorites;

    // Check if the target index is valid
    if (targetIndex >= currentList.length) return;

    const item = currentList[targetIndex];
    if (!item || !item.url) return;

    // Navigate to the item and close omnibox
    omniboxBridge.navigate(item.url);
    omniboxStore.close();
  }

  // Handle arrow key navigation
  function handleArrowKeys(key: string) {
    let totalItems: number;
    if (mode === 'omnibox') {
      // Get count from current view (history or favorites)
      totalItems = viewMode === 'history' ? suggestions.length : favorites.length;
    } else {
      totalItems = matches.length;
    }

    if (totalItems > 0) {
      if (key === 'ArrowDown') {
        omniboxStore.selectNext();
      } else {
        omniboxStore.selectPrevious();
      }

      // Fade overlay while navigating results only for find mode
      if (mode === 'find') {
        omniboxStore.setFaded(true);
      } else {
        omniboxStore.setFaded(false);
      }

      // For find mode, reveal the current match
      if (mode === 'find') {
        revealCurrentMatch();
      }
    }
  }

  // Jump to match (for find mode)
  function jumpToMatch(delta: number) {
    if (mode !== 'find' || matches.length === 0) return;

    const currentIndex = selectedIndex || 0;
    const newIndex = ((currentIndex + delta) % matches.length + matches.length) % matches.length;

    omniboxStore.setSelectedIndex(newIndex);
    revealCurrentMatch();
  }

  // Reveal current match (for find mode)
  function revealCurrentMatch() {
    if (mode !== 'find' || selectedIndex < 0 || !matches[selectedIndex]) return;

    // Remove previous active class
    const prevMatch = matches[omniboxStore.activeIndex];
    if (prevMatch?.element?.classList) {
      prevMatch.element.classList.remove('find-active');
    }

    // Add active class to current match
    const currentMatch = matches[selectedIndex];
    if (currentMatch?.element?.classList) {
      currentMatch.element.classList.add('find-active');
      omniboxStore.setActiveIndex(selectedIndex);

      // Scroll into view
      try {
        currentMatch.element.scrollIntoView({
          block: 'center',
          inline: 'nearest'
        });
      } catch {
        currentMatch.element.scrollIntoView();
      }
    }
  }

  // Handle mouse events
  function handleMouseDown() {
    omniboxStore.setFaded(false);
  }

  function handleFocus() {
    omniboxStore.setFaded(false);
    if (inputElement) {
      const accent = resolveAccentColor();
      inputElement.style.setProperty('--dumber-input-border-color', accent);
    }
  }

  function handleBlur() {
    if (!inputElement) return;
    const base = inputElement.style.getPropertyValue('--dumber-input-border-base');
    const fallback = (base && base.trim()) || 'var(--dynamic-border)';
    inputElement.style.setProperty('--dumber-input-border-color', fallback);
  }

  function handleMouseEnter() {
    if (omniboxStore.visible && inputElement) {
      try {
        inputElement.focus({ preventScroll: true });
        inputElement.select();
      } catch {
        inputElement.focus();
      }
    }
  }

  // Handle selection changes (for cursor tracking)
  function handleSelect() {
    updateCursorPosition();
  }

  // Handle click with cursor tracking
  function handleClickWithCursor() {
    omniboxStore.setFaded(false);
    updateCursorPosition();
  }
</script>

<div class="omnibox-input-wrapper">
  {#if activeBadge}
    <span
      class="omnibox-prefix-badge"
      aria-hidden="true"
      style={`left: ${baseLeftPadding};`}
    >{activeBadge.label}</span>
  {/if}

  <!-- Ghost text overlay (fish-style inline suggestion) -->
  {#if showGhostText}
    <div
      class="omnibox-ghost-text"
      aria-hidden="true"
      style={`padding: ${inputPadding}; font-size: ${inputFontSize};`}
    >
      <span class="ghost-spacer">{inputValue}</span><span class="ghost-completion">{inlineCompletion}</span>
    </div>
  {/if}

  <input
    bind:this={inputElement}
    type="text"
    {placeholder}
    value={inputValue}
    class="w-full box-border omnibox-input omnibox-input-field focus:outline-none"
    class:has-ghost-text={showGhostText}
    style={`padding: ${inputPadding}; font-size: ${inputFontSize}; box-sizing: border-box;`}
    oninput={handleInput}
    onkeydown={handleKeyDown}
    onmousedown={handleMouseDown}
    onclick={handleClickWithCursor}
    onselect={handleSelect}
    onfocus={handleFocus}
    onblur={handleBlur}
    onmouseenter={handleMouseEnter}
    autocomplete="off"
    spellcheck="false"
    role="combobox"
    aria-expanded={omniboxStore.hasContent}
    aria-controls="omnibox-list"
    aria-haspopup="listbox"
    aria-owns="omnibox-list"
    aria-activedescendant={selectedIndex >= 0 ? `omnibox-item-${selectedIndex}` : undefined}
  />
</div>

<style>
  .omnibox-input-wrapper {
    position: relative;
  }

  .omnibox-input-field {
    background: var(--dumber-input-bg, var(--dynamic-bg));
    color: var(--dynamic-text);
    border: 1px solid var(--dumber-input-border-color, var(--dynamic-border));
    border-radius: 2px;
    box-shadow:
      inset 0 1px 2px rgba(0, 0, 0, 0.15),
      inset 0 0 0 1px rgba(0, 0, 0, 0.03);
    transition: border-color 120ms ease, background-color 120ms ease, color 120ms ease, box-shadow 120ms ease;
    font-family: 'Fira Sans', system-ui, -apple-system, 'Segoe UI', 'Ubuntu', 'Cantarell', sans-serif;
    letter-spacing: normal;
  }

  .omnibox-input-field::placeholder {
    color: var(--dynamic-muted);
    letter-spacing: normal;
  }

  .omnibox-input-field:focus {
    color: var(--dynamic-text);
    border-color: var(--dynamic-accent);
    box-shadow:
      inset 0 1px 2px rgba(0, 0, 0, 0.15),
      0 0 0 1px var(--dynamic-accent);
  }

  .omnibox-prefix-badge {
    position: absolute;
    top: 50%;
    transform: translateY(-50%);
    padding: 0.2rem 0.6rem;
    border-radius: 0;
    border: 1px solid var(--dynamic-border, transparent);
    background: var(--dumber-input-badge-bg, var(--dynamic-accent));
    color: var(--dumber-input-badge-text, var(--dynamic-bg));
    font-size: 0.75rem;
    font-weight: 600;
    display: inline-flex;
    align-items: center;
    gap: 4px;
    line-height: 1;
    pointer-events: none;
    white-space: nowrap;
    box-shadow: 0 0 0 1px var(--dynamic-border, transparent);
    z-index: 1;
  }

  /* Ghost text overlay for fish-style inline suggestions */
  .omnibox-ghost-text {
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    pointer-events: none;
    display: flex;
    align-items: center;
    white-space: nowrap;
    overflow: hidden;
    font-family: 'Fira Sans', system-ui, -apple-system, 'Segoe UI', 'Ubuntu', 'Cantarell', sans-serif;
    letter-spacing: normal;
    z-index: 0;
    box-sizing: border-box;
    border: 1px solid transparent;
  }

  .ghost-spacer {
    visibility: hidden;
    white-space: pre;
  }

  .ghost-completion {
    color: var(--dynamic-muted);
    opacity: 0.5;
    white-space: pre;
    transition: opacity 100ms ease-out;
  }

  .omnibox-ghost-text {
    transition: opacity 80ms ease-out;
  }

  /* Make input transparent when showing ghost text */
  .omnibox-input-field.has-ghost-text {
    background: transparent !important;
  }

  /* Ensure wrapper provides background when ghost text is active */
  .omnibox-input-wrapper:has(.has-ghost-text) {
    background: var(--dumber-input-bg, var(--dynamic-bg));
    border-radius: 2px;
  }
</style>
