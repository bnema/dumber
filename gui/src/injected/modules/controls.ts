/**
 * Browser Controls Module
 *
 * Provides global keyboard and mouse controls that work on all domains.
 * Only includes essential controls that can work cross-origin:
 * - Navigation (back/forward)
 * - Mouse navigation buttons
 *
 * Zoom controls are handled natively by the GTK backend
 */

export interface ControlsConfig {
  // Future configuration options can be added here
  _placeholder?: never; // Prevent empty interface warning
}

let controlsInitialized = false;

export function initializeControls(_config?: ControlsConfig): void {
  if (controlsInitialized) {
    console.log('🔄 Browser controls already initialized, skipping');
    return;
  }

  console.log('🚀 Initializing browser controls on:', window.location.href);

  // Keyboard event handler for global controls
  function handleKeyboardEvent(event: KeyboardEvent): void {
    const { altKey, key } = event;
    // Note: ctrlKey, metaKey reserved for future use

    try {
      // Navigation: Alt + Left Arrow (Back)
      if (altKey && key === 'ArrowLeft') {
        event.preventDefault();
        event.stopPropagation();

        if (window.history.length > 1) {
          console.log('⬅️ Navigating back');
          window.history.back();
        }
        return;
      }

      // Navigation: Alt + Right Arrow (Forward)
      if (altKey && key === 'ArrowRight') {
        event.preventDefault();
        event.stopPropagation();

        console.log('➡️ Navigating forward');
        window.history.forward();
        return;
      }

    } catch (error) {
      console.error('❌ Error in keyboard handler:', error);
    }
  }

  // Mouse event handler for navigation buttons
  function handleMouseEvent(event: MouseEvent): void {
    try {
      // Mouse button 3 (back button)
      if (event.button === 3) {
        event.preventDefault();
        event.stopPropagation();

        if (window.history.length > 1) {
          console.log('⬅️ Mouse back button pressed');
          window.history.back();
        }
        return;
      }

      // Mouse button 4 (forward button)
      if (event.button === 4) {
        event.preventDefault();
        event.stopPropagation();

        console.log('➡️ Mouse forward button pressed');
        window.history.forward();
        return;
      }

    } catch (error) {
      console.error('❌ Error in mouse handler:', error);
    }
  }

  // Add event listeners with high priority (capture phase)
  try {
    document.addEventListener('keydown', handleKeyboardEvent, true);
    document.addEventListener('mousedown', handleMouseEvent, true);

    controlsInitialized = true;
    console.log('✅ Browser controls initialized');

    // Cleanup function for when page is unloaded
    window.addEventListener('beforeunload', () => {
      document.removeEventListener('keydown', handleKeyboardEvent, true);
      document.removeEventListener('mousedown', handleMouseEvent, true);
      controlsInitialized = false;
      console.log('🧹 Browser controls cleaned up');
    });

  } catch (error) {
    console.error('❌ Failed to initialize browser controls:', error);
  }
}