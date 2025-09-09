import { AppService } from './services/app.js';
import { BrowserControlsService } from './services/browser-controls.js';
import { DOMRenderer } from './utils/dom.js';

class DumberBrowserApp {
  private appService: AppService;
  private browserControls: BrowserControlsService;
  private domRenderer: DOMRenderer;

  constructor() {
    this.appService = new AppService();
    this.browserControls = new BrowserControlsService();
    this.domRenderer = new DOMRenderer();
  }

  async initialize(): Promise<void> {
    try {
      // Show loading states
      this.domRenderer.showLoading('history');
      this.domRenderer.showLoading('shortcuts');

      // Initialize services
      await this.appService.initialize();
      
      // Initialize browser controls
      await this.browserControls.initialize();

      // Render the UI
      this.render();

      console.log('✓ Dumber Browser initialized successfully');
    } catch (error) {
      console.error('Failed to initialize Dumber Browser:', error);
      this.handleInitializationError();
    }
  }

  private render(): void {
    try {
      // Render history
      const history = this.appService.getHistory();
      this.domRenderer.displayHistory(history);

      // Render shortcuts
      const shortcuts = this.appService.getShortcuts();
      this.domRenderer.displayShortcuts(shortcuts);
    } catch (error) {
      console.error('Failed to render UI:', error);
      this.domRenderer.showError('history', 'Failed to load history');
      this.domRenderer.showError('shortcuts', 'Failed to load shortcuts');
    }
  }

  private handleInitializationError(): void {
    this.domRenderer.showError('history', 'Failed to initialize application');
    this.domRenderer.showError('shortcuts', 'Failed to initialize application');
  }
}

// Initialize the application when the DOM is ready
function initializeApp(): void {
  const app = new DumberBrowserApp();
  app.initialize().catch(error => {
    console.error('Critical error during app initialization:', error);
  });
}

// Wait for DOM to be ready
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', initializeApp);
} else {
  initializeApp();
}