import { BrowserService } from '../../bindings/github.com/bnema/dumber/services/index';
import { Window } from '@wailsio/runtime';
import { NotificationService } from './notifications.js';
import { DebugConsoleService } from './debug-console.js';

export class BrowserControlsService {
  private currentURL = '';
  private currentZoom = 1.0;
  private notifications = new NotificationService();
  private debugConsole = new DebugConsoleService();

  constructor() {
    // Debug logging with enhanced details
    this.debugConsole.addLog('info', 'frontend', 'init', 'BrowserControlsService initializing...');
    this.debugConsole.addLog('info', 'frontend', 'init', 'Using Wails v3 service bindings');
    
    this.initializeEventListeners();
    this.debugConsole.addLog('info', 'frontend', 'init', 'BrowserControlsService initialized ✓');
  }


  private initializeEventListeners(): void {
    // Keyboard event handler for browser controls
    document.addEventListener('keydown', this.handleKeyboardEvent.bind(this));
    
    // Mouse button handler for navigation
    document.addEventListener('mousedown', this.handleMouseEvent.bind(this));
    
    // URL change listener (for SPA navigation)
    window.addEventListener('popstate', this.handleURLChange.bind(this));
  }

  private async handleKeyboardEvent(event: KeyboardEvent): Promise<void> {
    // Skip F12 (debug console toggle)
    if (event.key === 'F12') {
      return;
    }

    // Log all keyboard events to debug console
    this.debugConsole.logKeyboardEvent(event);

    const { ctrlKey, metaKey, shiftKey, altKey, key } = event;
    const isCmd = ctrlKey || metaKey;

    // Zoom controls
    if (isCmd && (key === '+' || key === '=')) {
      this.debugConsole.addLog('info', 'frontend', 'action', 'Zoom In triggered');
      event.preventDefault();
      await this.zoomIn();
      return;
    }

    if (isCmd && key === '-') {
      this.debugConsole.addLog('info', 'frontend', 'action', 'Zoom Out triggered');
      event.preventDefault();
      await this.zoomOut();
      return;
    }

    if (isCmd && key === '0') {
      this.debugConsole.addLog('info', 'frontend', 'action', 'Zoom Reset triggered');
      event.preventDefault();
      await this.resetZoom();
      return;
    }

    // Copy URL
    if (isCmd && shiftKey && key === 'C') {
      this.debugConsole.addLog('info', 'frontend', 'action', 'Copy URL triggered');
      event.preventDefault();
      await this.copyCurrentURL();
      return;
    }

    // Navigation
    if (altKey && key === 'ArrowLeft') {
      this.debugConsole.addLog('info', 'frontend', 'action', 'Go Back triggered');
      event.preventDefault();
      await this.goBack();
      return;
    }

    if (altKey && key === 'ArrowRight') {
      this.debugConsole.addLog('info', 'frontend', 'action', 'Go Forward triggered');
      event.preventDefault();
      await this.goForward();
      return;
    }
  }

  private async handleMouseEvent(event: MouseEvent): Promise<void> {
    // Log all mouse events to debug console
    this.debugConsole.logMouseEvent(event);

    // Mouse button 3 (back button)
    if (event.button === 3) {
      this.debugConsole.addLog('info', 'frontend', 'action', 'Mouse Back button triggered');
      event.preventDefault();
      await this.goBack();
      return;
    }

    // Mouse button 4 (forward button)
    if (event.button === 4) {
      this.debugConsole.addLog('info', 'frontend', 'action', 'Mouse Forward button triggered');
      event.preventDefault();
      await this.goForward();
      return;
    }
  }

  private handleURLChange(): void {
    this.setCurrentURL(window.location.href);
    this.loadSavedZoom();
  }

  async zoomIn(): Promise<void> {
    const url = this.getCurrentURL();
    this.debugConsole.addLog('info', 'frontend', 'service', `Calling ZoomIn with URL: ${url}`);
    
    try {
      const newZoom = await BrowserService.ZoomIn(url);
      this.debugConsole.logServiceCall('BrowserService', 'ZoomIn', [url], newZoom);
      
      await this.applyZoom(newZoom);
      this.notifications.showZoom(`Zoom: ${Math.round(newZoom * 100)}%`);
      
      this.debugConsole.addLog('info', 'frontend', 'zoom', `Zoom level changed to ${Math.round(newZoom * 100)}%`);
    } catch (error) {
      this.debugConsole.logServiceCall('BrowserService', 'ZoomIn', [url], null, error as Error);
      console.error('Failed to zoom in:', error);
    }
  }

  async zoomOut(): Promise<void> {
    const url = this.getCurrentURL();
    this.debugConsole.addLog('info', 'frontend', 'service', `Calling ZoomOut with URL: ${url}`);
    
    try {
      const newZoom = await BrowserService.ZoomOut(url);
      this.debugConsole.logServiceCall('BrowserService', 'ZoomOut', [url], newZoom);
      
      await this.applyZoom(newZoom);
      this.notifications.showZoom(`Zoom: ${Math.round(newZoom * 100)}%`);
      
      this.debugConsole.addLog('info', 'frontend', 'zoom', `Zoom level changed to ${Math.round(newZoom * 100)}%`);
    } catch (error) {
      this.debugConsole.logServiceCall('BrowserService', 'ZoomOut', [url], null, error as Error);
      console.error('Failed to zoom out:', error);
    }
  }

  async resetZoom(): Promise<void> {
    const url = this.getCurrentURL();
    this.debugConsole.addLog('info', 'frontend', 'service', `Calling ResetZoom with URL: ${url}`);
    
    try {
      const newZoom = await BrowserService.ResetZoom(url);
      this.debugConsole.logServiceCall('BrowserService', 'ResetZoom', [url], newZoom);
      
      await this.applyZoom(newZoom);
      this.notifications.showZoom(`Zoom: ${Math.round(newZoom * 100)}%`);
      
      this.debugConsole.addLog('info', 'frontend', 'zoom', `Zoom reset to ${Math.round(newZoom * 100)}%`);
    } catch (error) {
      this.debugConsole.logServiceCall('BrowserService', 'ResetZoom', [url], null, error as Error);
      console.error('Failed to reset zoom:', error);
    }
  }

  private async applyZoom(zoomLevel: number): Promise<void> {
    this.currentZoom = zoomLevel;
    // Reset CSS zoom by default so we don't double-scale when native works
    try { document.body.style.zoom = ''; } catch {}
    try {
      if (zoomLevel >= 1.0) {
        await Window.SetZoom(zoomLevel);
        return;
      }
      // Try native first for <1.0; if platform clamps (e.g., Linux/WebKitGTK),
      // detect mismatch and fallback to CSS-based zoom for "unzoom" behavior.
      await Window.SetZoom(zoomLevel);
      const actual = await Window.GetZoom();
      if (actual >= 1.0 && zoomLevel < 1.0 && Math.abs(actual - zoomLevel) > 0.01) {
        await Window.SetZoom(1.0);
        document.body.style.zoom = zoomLevel.toString();
      }
    } catch (e) {
      console.warn('Failed to apply native zoom, falling back to CSS zoom', e);
      document.body.style.zoom = zoomLevel.toString();
    }
  }

  async copyCurrentURL(): Promise<void> {
    try {
      const url = this.getCurrentURL();
      await navigator.clipboard.writeText(url);
      
      // URL copied successfully - no backend notification needed
      
      this.notifications.show('URL copied to clipboard!', 'success');
    } catch (error) {
      console.error('Failed to copy URL:', error);
      this.notifications.show('Failed to copy URL', 'error');
    }
  }

  async goBack(): Promise<void> {
    this.debugConsole.addLog('info', 'frontend', 'service', 'Calling GoBack');
    
    try {
      await BrowserService.GoBack();
      this.debugConsole.logServiceCall('BrowserService', 'GoBack', []);
      history.back();
      this.debugConsole.addLog('info', 'frontend', 'navigation', 'Browser navigated back');
    } catch (error) {
      this.debugConsole.logServiceCall('BrowserService', 'GoBack', [], null, error as Error);
      console.error('Failed to go back:', error);
    }
  }

  async goForward(): Promise<void> {
    this.debugConsole.addLog('info', 'frontend', 'service', 'Calling GoForward');
    
    try {
      await BrowserService.GoForward();
      this.debugConsole.logServiceCall('BrowserService', 'GoForward', []);
      history.forward();
      this.debugConsole.addLog('info', 'frontend', 'navigation', 'Browser navigated forward');
    } catch (error) {
      this.debugConsole.logServiceCall('BrowserService', 'GoForward', [], null, error as Error);
      console.error('Failed to go forward:', error);
    }
  }

  getCurrentURL(): string {
    return this.currentURL || window.location.href;
  }

  setCurrentURL(url: string): void {
    this.currentURL = url;
    this.updateWindowTitle(url);
  }

  private async updateWindowTitle(url: string): Promise<void> {
    try {
      // Extract domain or path for title
      let title = 'Dumber Browser';
      if (url && url !== '/') {
        const urlObj = new URL(url);
        title = `Dumber - ${urlObj.hostname}`;
      }
      
      document.title = title;
      
      // Notify backend service
      try {
        await BrowserService.UpdatePageTitle(url, title);
      } catch (error) {
        console.warn('Failed to update page title on backend:', error);
      }
    } catch (error) {
      console.error('Failed to update title:', error);
    }
  }

  async loadSavedZoom(): Promise<void> {
    try {
      const savedZoom = await BrowserService.GetZoomLevel(this.getCurrentURL());
      if (savedZoom && savedZoom !== 1.0) {
        await this.applyZoom(savedZoom);
      }
    } catch (error) {
      console.error('Failed to load saved zoom:', error);
    }
  }

  async initialize(): Promise<void> {
    // Check if there's an initial URL from backend (direct navigation)
    try {
      const initialURL = await BrowserService.GetInitialURL();
      if (initialURL && initialURL !== '') {
        this.debugConsole.addLog('info', 'frontend', 'init', `Found initial URL from backend: ${initialURL}`);
        this.debugConsole.addLog('info', 'frontend', 'navigation', `Auto-navigating to: ${initialURL}`);
        
        // Set the URL and navigate to it
        this.setCurrentURL(initialURL);
        // Apply saved zoom natively before navigation so it persists into the next page
        try {
          const savedZoom = await BrowserService.GetZoomLevel(initialURL);
          if (savedZoom && savedZoom > 0) {
            this.applyZoom(savedZoom);
          }
        } catch {}
        
        // Navigate to the URL in the browser
        window.location.href = initialURL;
        return; // Don't load zoom for homepage, we're navigating away
      } else {
        // Set initial URL from browser location (homepage)
        this.setCurrentURL(window.location.href);
      }
    } catch (error) {
      console.error('Failed to get initial URL from backend:', error);
      // Fallback to browser location
      this.setCurrentURL(window.location.href);
    }
    
    // Load saved zoom level (only if we're staying on homepage)
    await this.loadSavedZoom();
  }
}
