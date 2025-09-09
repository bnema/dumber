/**
 * Injectable Controls Script
 * 
 * This script is injected into every page loaded in the browser to provide
 * global keyboard and mouse controls that work on all domains.
 * 
 * Only includes essential controls that can work cross-origin:
 * - Navigation (back/forward)
 * - URL copying
 * - Mouse navigation buttons
 * 
 * Zoom controls are handled natively by Wails in main.go
 */

(function() {
  'use strict';
  
  // Prevent multiple injections with a unique timestamp
  const injectionId = '__dumberControlsInjected_' + Date.now();
  if ((window as any).__dumberControlsInjected) {
    console.log('🔄 Dumber Browser controls already injected, skipping');
    return;
  }
  (window as any).__dumberControlsInjected = injectionId;
  
  console.log('🚀 Dumber Browser controls injected on:', window.location.href);
  
  // Keyboard event handler for global controls
  function handleKeyboardEvent(event: KeyboardEvent): void {
    const { ctrlKey, metaKey, shiftKey, altKey, key } = event;
    const isCmd = ctrlKey || metaKey;
    
    try {
      // Copy URL (Cmd+Shift+C)
      if (isCmd && shiftKey && key === 'C') {
        event.preventDefault();
        event.stopPropagation();
        
        // Copy current URL to clipboard
        if (navigator.clipboard && navigator.clipboard.writeText) {
          navigator.clipboard.writeText(window.location.href)
            .then(() => console.log('🔗 URL copied:', window.location.href))
            .catch((err) => console.error('❌ Failed to copy URL:', err));
        } else {
          console.warn('⚠️ Clipboard API not available');
        }
        return;
      }
      
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
    
    console.log('✅ Dumber Browser global controls active');
    
    // Cleanup function for when page is unloaded
    window.addEventListener('beforeunload', () => {
      document.removeEventListener('keydown', handleKeyboardEvent, true);
      document.removeEventListener('mousedown', handleMouseEvent, true);
      console.log('🧹 Dumber Browser controls cleaned up');
    });
    
  } catch (error) {
    console.error('❌ Failed to initialize Dumber Browser controls:', error);
  }
})();