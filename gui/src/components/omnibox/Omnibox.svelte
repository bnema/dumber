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
  let inputElement = $state<HTMLInputElement>();

  // Responsive styling state
  let responsiveStyles = $state({
    width: 'min(90vw, 720px)',
    padding: '8px 12px',
    fontSize: '16px',
    inputPadding: '10px 12px'
  });

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

  // Focus input when component becomes visible
  $effect(() => {
    if (visible && inputElement) {
      inputElement.focus();
    }
  });


  // Apply faded styling effect (matching original JS implementation)
  $effect(() => {
    if (!boxElement || !inputElement) return;

    if (faded) {
      // Faded state - semi-transparent with backdrop blur
      boxElement.style.background = 'rgba(27,27,27,0.25)';
      boxElement.style.backdropFilter = 'blur(2px) saturate(110%)';
      (boxElement.style as any).webkitBackdropFilter = 'blur(2px) saturate(110%)';
      inputElement.style.background = 'rgba(18,18,18,0.35)';
      inputElement.style.color = '#eee';
    } else {
      // Normal state - solid background
      boxElement.style.background = '#1b1b1b';
      boxElement.style.backdropFilter = '';
      (boxElement.style as any).webkitBackdropFilter = '';
      inputElement.style.background = '#121212';
      inputElement.style.color = '#eee';
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
        }
      };

      console.log('✅ Omnibox API exposed:', Object.keys(window.__dumber_omnibox));
    } catch (error) {
      console.error('❌ Failed to set up omnibox global API:', error);
    }
  });

  onDestroy(() => {
    // Cleanup resize listener
    window.removeEventListener('resize', updateResponsiveStyles);

    // Cleanup global click listener
    document.removeEventListener('click', handleGlobalClick, true);

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
    class="bg-[#1b1b1b] border border-[#444] rounded-lg shadow-[0_10px_30px_rgba(0,0,0,0.6)] text-[#eee]"
    style="position: relative !important;
           left: 50% !important;
           transform: translateX(-50%) !important;
           margin-top: 8vh !important;
           margin-left: 0 !important;
           margin-right: 0 !important;
           width: {responsiveStyles.width};
           padding: {responsiveStyles.padding};
           font-family: system-ui, -apple-system, 'Segoe UI', Roboto, Ubuntu, 'Helvetica Neue', Arial, sans-serif;
           pointer-events: auto !important;
           box-sizing: border-box !important;"
    onmousedown={handleBoxClick}
    onmouseenter={handleMouseEnter}
    role="dialog"
    tabindex="-1"
    aria-modal="true"
    aria-label={mode === 'find' ? 'Find in page' : 'Omnibox search'}
  >
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

<style>
  /* Additional custom styles if needed */
</style>