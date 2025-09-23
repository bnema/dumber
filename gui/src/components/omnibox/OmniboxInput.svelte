<!--
  Omnibox Input Component

  Input field with keyboard handling and debounced search
-->
<script lang="ts">
  import { omniboxStore } from './stores.svelte.ts';
  import { debouncedQuery, omniboxBridge } from './messaging';
  import { findInPage } from './find';

  // Reactive state
  let mode = $derived(omniboxStore.mode);
  let inputValue = $derived(omniboxStore.inputValue);
  let selectedIndex = $derived(omniboxStore.selectedIndex);
  let suggestions = $derived(omniboxStore.suggestions);
  let matches = $derived(omniboxStore.matches);

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

  // Handle input changes
  function handleInput(event: Event) {
    const target = event.target as HTMLInputElement;
    const value = target.value || '';

    omniboxStore.setInputValue(value);
    omniboxStore.setFaded(false);

    if (mode === 'omnibox') {
      // Debounced search for omnibox
      debouncedQuery(value);
    } else if (mode === 'find') {
      // Immediate find for search
      findInPage(value);
    }
  }

  // Handle key events
  function handleKeyDown(event: KeyboardEvent) {
    switch (event.key) {
      case 'Escape':
        event.preventDefault();
        event.stopPropagation();
        omniboxStore.close();
        break;

      case 'Enter':
        event.preventDefault();
        event.stopPropagation();
        handleEnterKey(event);
        break;

      case 'ArrowDown':
      case 'ArrowUp':
        event.preventDefault();
        event.stopPropagation();
        handleArrowKeys(event.key);
        break;

      default:
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
      const selectedSuggestion = selectedIndex >= 0 ? suggestions[selectedIndex] : null;
      const url = selectedSuggestion?.url || inputValue || '';

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

  // Handle arrow key navigation
  function handleArrowKeys(key: string) {
    const totalItems = mode === 'omnibox' ? suggestions.length : matches.length;

    if (totalItems > 0) {
      if (key === 'ArrowDown') {
        omniboxStore.selectNext();
      } else {
        omniboxStore.selectPrevious();
      }

      // Fade overlay while navigating results
      omniboxStore.setFaded(true);

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

  function handleClick() {
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
</script>

<input
  bind:this={inputElement}
  type="text"
  {placeholder}
  value={inputValue}
  class="w-full box-border omnibox-input omnibox-input-field rounded focus:outline-none"
  style="padding: {responsiveStyles?.inputPadding || '10px 12px'};
         font-size: {responsiveStyles?.fontSize || '16px'};
         box-sizing: border-box;"
  oninput={handleInput}
  onkeydown={handleKeyDown}
  onmousedown={handleMouseDown}
  onclick={handleClick}
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

<style>
  .omnibox-input-field {
    background: var(--dumber-input-bg, var(--dynamic-bg));
    color: var(--dynamic-text);
    border: 2px solid var(--dumber-input-border-color, var(--dynamic-border));
    transition: border-color 120ms ease, background-color 120ms ease, color 120ms ease;
  }

  .omnibox-input-field::placeholder {
    color: var(--dynamic-muted);
  }

  .omnibox-input-field:focus {
    color: var(--dynamic-text);
  }
</style>
