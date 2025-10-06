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
    } else if (!visible) {
      // Disable keyboard event blocking when omnibox closes
      disablePageEventBlocking();
    }
  });

  // Functions to control main-world event blocking
  function enablePageEventBlocking() {
    try {
      // Send message to Go backend to enable keyboard blocking
      if (window.webkit?.messageHandlers?.dumber) {
        window.webkit.messageHandlers.dumber.postMessage(JSON.stringify({
          type: 'keyboard_blocking',
          action: 'enable'
        }));
      }
    } catch (error) {
      console.warn('[omnibox] Failed to enable page event blocking:', error);
    }
  }

  function disablePageEventBlocking() {
    try {
      // Send message to Go backend to disable keyboard blocking
      if (window.webkit?.messageHandlers?.dumber) {
        window.webkit.messageHandlers.dumber.postMessage(JSON.stringify({
          type: 'keyboard_blocking',
          action: 'disable'
        }));
      }
    } catch (error) {
      console.warn('[omnibox] Failed to disable page event blocking:', error);
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
    const inputMix = isDarkMode ? '52%' : '90%';
    const borderMix = isDarkMode ? '62%' : '85%';
    const blurMix = isDarkMode ? '22%' : '12%';

    const applyFade = faded && mode === 'find';

    const containerBackground = applyFade
      ? `color-mix(in srgb, var(--dynamic-surface) ${surfaceMix}, transparent)`
      : 'var(--dynamic-surface)';
    const inputBackground = applyFade
      ? `color-mix(in srgb, var(--dynamic-bg) ${inputMix}, transparent)`
      : 'var(--dynamic-bg)';
    const borderColor = applyFade
      ? `color-mix(in srgb, var(--dynamic-border) ${borderMix}, transparent)`
      : 'var(--dynamic-border)';
    const blurTint = `color-mix(in srgb, var(--dynamic-bg) ${blurMix}, transparent)`;

    boxElement.style.background = containerBackground;
    boxElement.style.color = 'var(--dynamic-text)';
    boxElement.style.setProperty('--dumber-border-color', borderColor);
    // Force the omnibox to use our neutral surfaces so it stays in sync with the shell palette
    boxElement.style.setProperty('--dumber-omnibox-surface', 'color-mix(in srgb, var(--dynamic-bg) 88%, var(--dynamic-surface) 12%)');

    inputElement.style.background = inputBackground;
    inputElement.style.color = 'var(--dynamic-text)';
    inputElement.style.setProperty('--dumber-input-bg', inputBackground);
    inputElement.style.setProperty('--dumber-input-border-base', borderColor);
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



  onMount(() => {
    syncTheme();
    observeThemeChanges();

    console.log('🔧 Omnibox component mounted');

    // Initialize responsive styles
    updateResponsiveStyles();

    // Add resize listener for responsive updates (handles zoom changes)
    window.addEventListener('resize', updateResponsiveStyles);

    // Add global click listener
    document.addEventListener('click', handleGlobalClick, true);

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
         z-index: 2147483647 !important;
         pointer-events: none !important;
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
           font-family: 'JetBrains Mono', 'Fira Code', 'SFMono-Regular', Menlo, monospace;
           pointer-events: auto !important;
           box-sizing: border-box !important;
           background: var(--dumber-omnibox-surface, color-mix(in srgb, var(--dynamic-bg) 88%, var(--dynamic-surface) 12%));
           color: var(--dynamic-text);
           border: 1px solid var(--dumber-border-color, var(--dynamic-border));
           box-shadow:
             0 20px 60px rgba(0, 0, 0, 0.45),
             0 10px 30px rgba(0, 0, 0, 0.35),
             0 5px 15px rgba(0, 0, 0, 0.25),
             inset 0 0 0 1px color-mix(in srgb, var(--dynamic-border) 22%, transparent);
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
    background: var(--dumber-omnibox-surface, color-mix(in srgb, var(--dynamic-bg) 88%, var(--dynamic-surface) 12%));
    color: var(--dynamic-text);
    border: 1px solid var(--dumber-border-color, var(--dynamic-border));
    transition: background-color 140ms ease, border-color 140ms ease, box-shadow 140ms ease;
    border-radius: 0;
    box-shadow:
      0 20px 60px rgba(0, 0, 0, 0.45),
      0 10px 30px rgba(0, 0, 0, 0.35),
      0 5px 15px rgba(0, 0, 0, 0.25),
      inset 0 0 0 1px color-mix(in srgb, var(--dynamic-border) 22%, transparent);
  }

  .dumber-omnibox-container:focus-within {
    border-color: color-mix(in srgb, var(--dumber-border-color, var(--dynamic-border)) 55%, var(--dynamic-text) 45%);
    box-shadow:
      0 25px 75px rgba(0, 0, 0, 0.5),
      0 15px 40px rgba(0, 0, 0, 0.4),
      0 8px 20px rgba(0, 0, 0, 0.3),
      inset 0 0 0 1px color-mix(in srgb, var(--dynamic-border) 28%, transparent);
  }

  :global(.dumber-omnibox-container input) {
    background: var(--dumber-input-bg, color-mix(in srgb, var(--dynamic-bg) 92%, var(--dynamic-surface) 8%));
    color: var(--dynamic-text);
    border: 1px solid var(--dumber-input-border-color, var(--dynamic-border));
    transition: border-color 120ms ease, background-color 120ms ease, color 120ms ease;
    font-family:
      "JetBrains Mono",
      "Fira Code",
      "SFMono-Regular",
      Menlo,
      monospace;
    font-size: inherit;
    letter-spacing: 0.05em;
    text-transform: none;
  }

  :global(.dumber-omnibox-container input::placeholder) {
    color: var(--dynamic-muted);
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }

  /* Blur layer styles: use `backdrop-filter` but animate opacity only. */
  .dumber-omnibox-blur-layer {
    backdrop-filter: blur(1.5px) saturate(105%);
    -webkit-backdrop-filter: blur(1.5px) saturate(105%);
    background: var(--dumber-blur-color, color-mix(in srgb, var(--dynamic-bg) 12%, transparent));
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
