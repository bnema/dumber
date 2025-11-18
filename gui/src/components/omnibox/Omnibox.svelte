<!--
  Omnibox Component

  Main container for omnibox/find functionality using Svelte 5 runes
-->
<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import OmniboxInput from './OmniboxInput.svelte';
  import OmniboxSuggestions from './OmniboxSuggestions.svelte';
  import OmniboxFind from './OmniboxFind.svelte';
  import { omniboxStore } from './stores.svelte.ts';
  import { omniboxBridge } from './messaging';

  // Reactive state from store
  let visible = $derived(omniboxStore.visible);
  let mode = $derived(omniboxStore.mode);
  let faded = $derived(omniboxStore.faded);

  // Ready state - prevents flash before initialization
  let isReady = $state(false);

  // Component refs
  let boxElement: HTMLDivElement;
  let blurLayerElement: HTMLDivElement;
  let inputElement = $state<HTMLInputElement>();

  // Responsive styling state
  let responsiveStyles = $state({
    width: 'min(90vw, 720px)',
    padding: '8px 12px',
    fontSize: '16px',
    inputPadding: '10px 12px'
  });

  // Track theme mode derived from injected color-scheme module
  let isDarkMode = $state(false);
  let themeObserver: MutationObserver | null = null;

  function syncTheme() {
    if (typeof document === 'undefined') return;

    const globalWindow = typeof window === 'undefined' ? undefined : window;

    if (document.documentElement.classList.contains('dark')) {
      isDarkMode = true;
      return;
    }

    if (globalWindow?.matchMedia) {
      try {
        const prefersDark = globalWindow.matchMedia('(prefers-color-scheme: dark)').matches;
        if (typeof prefersDark === 'boolean') {
          isDarkMode = prefersDark;
          return;
        }
      } catch {
        // matchMedia might be unavailable in some environments
      }
    }

    // Fallback to injected GTK preference flag if available
    isDarkMode = Boolean((globalWindow as any)?.__dumber_gtk_prefers_dark);
  }

  function observeThemeChanges() {
    if (typeof document === 'undefined' || typeof MutationObserver === 'undefined') {
      return;
    }

    if (themeObserver) {
      themeObserver.disconnect();
    }

    themeObserver = new MutationObserver(syncTheme);
    themeObserver.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class']
    });
  }

  // Handle click outside to close
  function handleOverlayClick(event: MouseEvent) {
    const target = event.target as HTMLElement;
    if (target && boxElement && !boxElement.contains(target)) {
      event.preventDefault();
      event.stopPropagation();
      omniboxStore.close();
    }
  }

  // Handle box click to prevent propagation
  function handleBoxClick(event: MouseEvent) {
    event.stopPropagation();
  }

  // Handle mouse enter to focus input and clear fade
  function handleMouseEnter() {
    if (visible && inputElement) {
      try {
        inputElement.focus({ preventScroll: true });
      } catch {
        inputElement.focus();
      }
      omniboxStore.setFaded(false);
    }
  }

  // Update responsive styles based on viewport width (handles zoom levels)
  function updateResponsiveStyles() {
    const vw = window.innerWidth;
    const isSmall = vw < 640;
    const isLarge = vw > 1920;

    responsiveStyles = {
      width: isSmall ? '95vw' : isLarge ? 'min(90vw, 840px)' : 'min(90vw, 720px)',
      padding: isSmall ? '6px 8px' : '8px 12px',
      fontSize: isSmall ? '14px' : isLarge ? '17px' : '16px',
      inputPadding: isSmall ? '8px 10px' : '10px 12px'
    };
  }

  // Focus input when component becomes visible and manage page event blocking
  $effect(() => {
    if (visible && inputElement) {
      inputElement.focus();
      // Enable keyboard event blocking in main world when omnibox opens
      enablePageEventBlocking();

      // Fetch initial history if opening in omnibox mode with empty input
      if (mode === 'omnibox' && omniboxStore.inputValue === '') {
        omniboxBridge.fetchInitialHistory();
      }
    } else if (!visible) {
      // Disable keyboard event blocking when omnibox closes
      disablePageEventBlocking();
    }
  });

  // Functions to control main-world event blocking
  function enablePageEventBlocking() {
    try {
      // Send message to Go backend via bridge (isolated world → main world → Go)
      omniboxBridge.postMessage({
        type: 'keyboard_blocking',
        action: 'enable'
      });
      console.log('[omnibox] Sent enable keyboard blocking message');
    } catch (error) {
      console.error('[omnibox] Failed to enable page event blocking:', error);
    }
  }

  function disablePageEventBlocking() {
    try {
      // Send message to Go backend via bridge (isolated world → main world → Go)
      omniboxBridge.postMessage({
        type: 'keyboard_blocking',
        action: 'disable'
      });
      console.log('[omnibox] Sent disable keyboard blocking message');
    } catch (error) {
      console.error('[omnibox] Failed to disable page event blocking:', error);
    }
  }


  // Apply faded styling effect (matching original JS implementation)
  // NOTE: to avoid text becoming blurry while animating, we animate the
  // blur via an absolutely-positioned layer's opacity instead of
  // animating `backdrop-filter` on the container. This keeps the text
  // on a separate compositing layer and prevents browser antialiasing
  // artifacts during transitions.
  $effect(() => {
    if (!boxElement || !inputElement || !blurLayerElement) return;

    // Ensure subtle transitions are present on the container so
    // background-color (and other cheap properties) animate smoothly.
    try {
      if (!boxElement.style.transition) {
        boxElement.style.transition = 'background-color 120ms ease, border-color 120ms ease';
      }
    } catch (e) {
      // defensive: ignore if setting styles fails for some reason
    }

    const surfaceMix = isDarkMode ? '55%' : '88%';
    const blurMix = isDarkMode ? '22%' : '12%';

    const applyFade = faded && mode === 'find';

    // GTK4-style: Container is slightly raised (use surface)
    const containerBackground = applyFade
      ? `color-mix(in srgb, var(--dynamic-surface) ${surfaceMix}, transparent)`
      : 'var(--dynamic-surface)';

    // GTK4-style: Input is recessed (darker background)
    const inputBackground = applyFade
      ? isDarkMode
        ? `color-mix(in srgb, color-mix(in srgb, var(--dynamic-bg) 85%, black 15%) ${surfaceMix}, transparent)`
        : `color-mix(in srgb, color-mix(in srgb, var(--dynamic-bg) 95%, black 5%) ${surfaceMix}, transparent)`
      : isDarkMode
        ? 'color-mix(in srgb, var(--dynamic-bg) 85%, black 15%)'
        : 'color-mix(in srgb, var(--dynamic-bg) 95%, black 5%)';

    // GTK4-style: Use palette border directly, slightly darker for input
    const containerBorderColor = applyFade
      ? `color-mix(in srgb, var(--dynamic-border) ${surfaceMix}, transparent)`
      : 'var(--dynamic-border)';

    const inputBorderColor = applyFade
      ? isDarkMode
        ? `color-mix(in srgb, color-mix(in srgb, var(--dynamic-border) 80%, black 20%) ${surfaceMix}, transparent)`
        : `color-mix(in srgb, color-mix(in srgb, var(--dynamic-border) 85%, black 15%) ${surfaceMix}, transparent)`
      : isDarkMode
        ? 'color-mix(in srgb, var(--dynamic-border) 80%, black 20%)'
        : 'color-mix(in srgb, var(--dynamic-border) 85%, black 15%)';

    const blurTint = `color-mix(in srgb, var(--dynamic-bg) ${blurMix}, transparent)`;

    boxElement.style.background = containerBackground;
    boxElement.style.color = 'var(--dynamic-text)';
    boxElement.style.setProperty('--dumber-border-color', containerBorderColor);
    // Force the omnibox to use our neutral surfaces so it stays in sync with the shell palette
    boxElement.style.setProperty('--dumber-omnibox-surface', containerBackground);

    inputElement.style.background = inputBackground;
    inputElement.style.color = 'var(--dynamic-text)';
    inputElement.style.setProperty('--dumber-input-bg', inputBackground);
    inputElement.style.setProperty('--dumber-input-border-base', inputBorderColor);
    inputElement.style.setProperty('--dumber-placeholder-color', 'var(--dynamic-muted)');

    blurLayerElement.style.setProperty('--dumber-blur-color', blurTint);
    blurLayerElement.style.background = 'var(--dumber-blur-color)';

    let accentColor = 'var(--dynamic-accent)';
    try {
      const computedAccent = getComputedStyle(boxElement).getPropertyValue('--dynamic-accent');
      if (computedAccent && computedAccent.trim()) {
        accentColor = computedAccent.trim();
      }
    } catch {
      // If computed styles fail, keep fallback accent color
    }
    inputElement.style.setProperty('--dumber-input-accent', accentColor);
    boxElement.style.setProperty('--dumber-input-accent', 'var(--dynamic-muted)');

    const isFocused = document.activeElement === inputElement;

    if (!isFocused) {
      inputElement.style.setProperty('--dumber-input-border-color', 'var(--dynamic-border)');
    }

    if (applyFade) {
      blurLayerElement.style.opacity = '1';
    } else {
      blurLayerElement.style.opacity = '0';
    }
  });

  // Global click handler to close omnibox when clicking anywhere outside
  function handleGlobalClick(event: MouseEvent) {
    if (!visible) return;

    const target = event.target as HTMLElement;
    const omniboxContainer = document.getElementById('dumber-omnibox-root');

    // Close if clicking outside the omnibox container
    if (omniboxContainer && !omniboxContainer.contains(target)) {
      omniboxStore.close();
    }
  }



  onMount(async () => {
    syncTheme();
    observeThemeChanges();

    console.log('🔧 Omnibox component mounted');

    // Initialize responsive styles
    updateResponsiveStyles();

    // Add resize listener for responsive updates (handles zoom changes)
    window.addEventListener('resize', updateResponsiveStyles);

    // Add global click listener
    document.addEventListener('click', handleGlobalClick, true);

    // Load search shortcuts and favorites on mount
    try {
      await Promise.all([
        omniboxBridge.fetchSearchShortcuts(),
        omniboxBridge.fetchFavorites()
      ]);
      console.log('✅ Search shortcuts and favorites loaded on mount');
    } catch (error) {
      console.warn('Failed to load initial data:', error);
    }

    try {
      // Set up global API for Go bridge
      window.__dumber_omnibox = {
        setSuggestions: (suggestions) => {
          try {
            console.log('📋 setSuggestions called with:', suggestions?.length || 0, 'items');
            omniboxBridge.setSuggestions(suggestions);
          } catch (error) {
            console.error('Error in setSuggestions:', error);
          }
        },
        toggle: () => {
          try {
            console.log('🔄 Omnibox toggle called, current visible:', omniboxStore.visible);
            omniboxStore.toggle();
            console.log('🔄 After toggle, visible:', omniboxStore.visible);
          } catch (error) {
            console.error('Error in toggle:', error);
          }
        },
        open: (mode, query) => {
          try {
            console.log('🚪 Omnibox open called with mode:', mode, 'query:', query);
            omniboxStore.open(mode as any, query);
            console.log('🚪 After open, visible:', omniboxStore.visible, 'mode:', omniboxStore.mode);
          } catch (error) {
            console.error('Error in open:', error);
          }
        },
        close: () => {
          try {
            console.log('🚪 Omnibox close called');
            omniboxStore.close();
          } catch (error) {
            console.error('Error in close:', error);
          }
        },
        findQuery: (query) => {
          try {
            console.log('🔍 Find query called with:', query);
            if (omniboxStore.mode !== 'find') {
              omniboxStore.setMode('find');
            }
            if (!omniboxStore.visible) {
              omniboxStore.setVisible(true);
            }
            omniboxStore.setInputValue(query || '');
            // Find will be triggered by input value change
          } catch (error) {
            console.error('Error in findQuery:', error);
          }
        },
        setActive: (active) => {
          try {
            console.log('🎯 Omnibox setActive called with:', active);
            // For now, this is a no-op but maintains API compatibility
            // In the future, this could control focus management or visibility
          } catch (error) {
            console.error('Error in setActive:', error);
          }
        }
      };

      console.log('✅ Omnibox API exposed:', Object.keys(window.__dumber_omnibox));

      // Check for pending suggestions that arrived before API was ready
      if ((window as any).__dumber_omnibox_pending_suggestions) {
        console.log('🔄 [OMNIBOX] Processing pending suggestions');
        const pending = (window as any).__dumber_omnibox_pending_suggestions;
        omniboxBridge.setSuggestions(pending);
        delete (window as any).__dumber_omnibox_pending_suggestions;
      }
    } catch (error) {
      console.error('❌ Failed to set up omnibox global API:', error);
    }

    // Mark as ready - removes initial hiding styles
    isReady = true;
    console.log('✅ Omnibox fully initialized and ready');
  });

  onDestroy(() => {
    // Cleanup resize listener
    window.removeEventListener('resize', updateResponsiveStyles);

    // Cleanup global click listener
    document.removeEventListener('click', handleGlobalClick, true);

    if (themeObserver) {
      themeObserver.disconnect();
      themeObserver = null;
    }

    // Reset omnibox state completely
    omniboxStore.reset();

    // Cleanup global API
    if (window.__dumber_omnibox) {
      delete window.__dumber_omnibox;
    }
  });
</script>

<!-- Overlay container -->
<div
  class="fixed inset-0 z-[2147483647]"
  style="display: {visible ? 'block' : 'none'};
         position: fixed !important;
         top: 0 !important;
         left: 0 !important;
         right: 0 !important;
         bottom: 0 !important;
         z-index: {isReady ? '2147483647' : '-9999'} !important;
         visibility: {isReady ? 'visible' : 'hidden'} !important;
         opacity: {isReady ? '1' : '0'} !important;
         pointer-events: {isReady ? 'none' : 'none'} !important;
         margin: 0 !important;
         padding: 0 !important;
         isolation: isolate;"
  onmousedown={handleOverlayClick}
  role="presentation"
>
  <!-- Main omnibox container -->
  <div
    bind:this={boxElement}
    class="dumber-omnibox-container omnibox-base terminal-omnibox"
    style="position: relative !important;
           left: 50% !important;
           transform: translateX(-50%) !important;
           margin-top: 20vh !important;
           margin-left: 0 !important;
           margin-right: 0 !important;
           width: {responsiveStyles.width};
           padding: {responsiveStyles.padding};
           font-family: 'Fira Sans', system-ui, -apple-system, 'Segoe UI', 'Ubuntu', 'Cantarell', sans-serif;
           pointer-events: auto !important;
           box-sizing: border-box !important;
           background: var(--dumber-omnibox-surface, var(--dynamic-surface));
           color: var(--dynamic-text);
           border: 1px solid var(--dumber-border-color, var(--dynamic-border));
           border-radius: 3px;
           box-shadow:
             0 4px 12px rgba(0, 0, 0, 0.2),
             0 2px 6px rgba(0, 0, 0, 0.15),
             0 1px 3px rgba(0, 0, 0, 0.1),
             inset 0 1px 0 rgba(255, 255, 255, 0.04);
           --dumber-border-color: var(--dynamic-border);"
    onmousedown={handleBoxClick}
    onmouseenter={handleMouseEnter}
    role="dialog"
    tabindex="-1"
    aria-modal="true"
    aria-label={mode === 'find' ? 'Find in page' : 'Omnibox search'}
  >
    <!-- Blur layer: absolute full-cover element providing backdrop blur.
         We animate its opacity rather than the container's backdrop-filter
         to avoid causing text to blur during transitions. -->
    <div
      bind:this={blurLayerElement}
      class="dumber-omnibox-blur-layer"
      aria-hidden="true"
      style="position: absolute; inset: 0; pointer-events: none; border-radius: 0; opacity: 0;"
    ></div>
    <div class="dumber-omnibox-content">
      <!-- Input component -->
      <OmniboxInput bind:inputElement {responsiveStyles} />

      <!-- Content based on mode -->
      {#if mode === 'omnibox'}
        <OmniboxSuggestions />
      {:else if mode === 'find'}
        <OmniboxFind />
      {/if}
    </div>
  </div>
</div>

<style>
  .dumber-omnibox-container {
    background: var(--dumber-omnibox-surface, var(--dynamic-surface));
    color: var(--dynamic-text);
    border: 1px solid var(--dumber-border-color, var(--dynamic-border));
    transition: background-color 140ms ease, border-color 140ms ease, box-shadow 140ms ease;
    border-radius: 3px;
    box-shadow:
      0 4px 12px rgba(0, 0, 0, 0.2),
      0 2px 6px rgba(0, 0, 0, 0.15),
      0 1px 3px rgba(0, 0, 0, 0.1),
      inset 0 1px 0 rgba(255, 255, 255, 0.04);
  }

  .dumber-omnibox-container:focus-within {
    border-color: var(--dynamic-accent);
    box-shadow:
      0 6px 16px rgba(0, 0, 0, 0.24),
      0 3px 8px rgba(0, 0, 0, 0.18),
      0 2px 4px rgba(0, 0, 0, 0.12),
      inset 0 1px 0 rgba(255, 255, 255, 0.06),
      0 0 0 1px var(--dynamic-accent);
  }

  :global(.dumber-omnibox-container input) {
    background: var(--dumber-input-bg, var(--dynamic-bg));
    color: var(--dynamic-text);
    border: 1px solid var(--dumber-input-border-color, var(--dynamic-border));
    border-radius: 2px;
    box-shadow:
      inset 0 1px 2px rgba(0, 0, 0, 0.15),
      inset 0 0 0 1px rgba(0, 0, 0, 0.03);
    transition: border-color 120ms ease, background-color 120ms ease, color 120ms ease, box-shadow 120ms ease;
    font-family:
      "Fira Sans",
      system-ui,
      -apple-system,
      "Segoe UI",
      "Ubuntu",
      "Cantarell",
      sans-serif;
    font-size: inherit;
    letter-spacing: normal;
    text-transform: none;
  }

  :global(.dumber-omnibox-container input::placeholder) {
    color: var(--dynamic-muted);
    letter-spacing: normal;
    text-transform: none;
  }

  /* Blur layer styles: use `backdrop-filter` but animate opacity only. */
  /* GTK4-style: Minimal blur for faded state */
  .dumber-omnibox-blur-layer {
    backdrop-filter: blur(0.5px) saturate(102%);
    -webkit-backdrop-filter: blur(0.5px) saturate(102%);
    background: var(--dumber-blur-color, color-mix(in srgb, var(--dynamic-bg) 8%, transparent));
    transition: opacity 140ms ease;
    will-change: opacity;
    z-index: 0;
    mix-blend-mode: normal;
    border-radius: 0;
  }

  /* Ensure the actual content (input, suggestions) sits above the blur layer */
  .dumber-omnibox-content {
    position: relative;
    z-index: 1;
    /* promote to its own layer to avoid being rasterized with the blur */
    transform: translateZ(0);
    backface-visibility: hidden;
  }
  /* Additional custom styles if needed */
</style>
