import { NotificationService } from "./notifications.js";
import { DebugConsoleService } from "./debug-console.js";

export class BrowserControlsService {
  private currentURL = "";
  private notifications = new NotificationService();
  private debugConsole = new DebugConsoleService();

  constructor() {
    // Debug logging with enhanced details
    this.debugConsole.addLog(
      "info",
      "frontend",
      "init",
      "BrowserControlsService initializing...",
    );
    this.debugConsole.addLog(
      "info",
      "frontend",
      "init",
      "Running without Wails bindings",
    );

    this.initializeEventListeners();
    this.debugConsole.addLog(
      "info",
      "frontend",
      "init",
      "BrowserControlsService initialized ✓",
    );
  }

  private initializeEventListeners(): void {
    // Keyboard event handler for browser controls
    document.addEventListener("keydown", this.handleKeyboardEvent.bind(this));

    // Mouse button handler for navigation
    document.addEventListener("mousedown", this.handleMouseEvent.bind(this));

    // URL change listener (for SPA navigation)
    window.addEventListener("popstate", this.handleURLChange.bind(this));
  }

  private async handleKeyboardEvent(event: KeyboardEvent): Promise<void> {
    // Skip F12 (debug console toggle)
    if (event.key === "F12") {
      return;
    }

    // Log all keyboard events to debug console
    this.debugConsole.logKeyboardEvent(event);

    const { ctrlKey, metaKey, shiftKey, altKey, key } = event;
    const isCmd = ctrlKey || metaKey;

    // Zoom controls handled natively by GTK; don't intercept
    if (isCmd && (key === "+" || key === "=" || key === "-" || key === "0")) {
      return; // allow native handler to process
    }

    // Copy URL
    if (isCmd && shiftKey && key === "C") {
      this.debugConsole.addLog(
        "info",
        "frontend",
        "action",
        "Copy URL triggered",
      );
      event.preventDefault();
      await this.copyCurrentURL();
      return;
    }

    // Navigation
    if (altKey && key === "ArrowLeft") {
      this.debugConsole.addLog(
        "info",
        "frontend",
        "action",
        "Go Back triggered",
      );
      event.preventDefault();
      history.back();
      return;
    }

    if (altKey && key === "ArrowRight") {
      this.debugConsole.addLog(
        "info",
        "frontend",
        "action",
        "Go Forward triggered",
      );
      event.preventDefault();
      history.forward();
      return;
    }
  }

  private async handleMouseEvent(event: MouseEvent): Promise<void> {
    // Log all mouse events to debug console
    this.debugConsole.logMouseEvent(event);

    // Mouse button 3 (back button)
    if (event.button === 3) {
      this.debugConsole.addLog(
        "info",
        "frontend",
        "action",
        "Mouse Back button triggered",
      );
      event.preventDefault();
      await this.goBack();
      return;
    }

    // Mouse button 4 (forward button)
    if (event.button === 4) {
      this.debugConsole.addLog(
        "info",
        "frontend",
        "action",
        "Mouse Forward button triggered",
      );
      event.preventDefault();
      await this.goForward();
      return;
    }
  }

  private handleURLChange(): void {
    this.setCurrentURL(window.location.href);
    this.loadSavedZoom();
  }

  // Zoom is handled natively; no JS-based zoom adjustments needed.

  async copyCurrentURL(): Promise<void> {
    try {
      const url = this.getCurrentURL();
      await navigator.clipboard.writeText(url);

      // URL copied successfully - no backend notification needed

      this.notifications.show("URL copied to clipboard!", "success");
    } catch (error) {
      console.error("Failed to copy URL:", error);
      this.notifications.show("Failed to copy URL", "error");
    }
  }

  async goBack(): Promise<void> {
    this.debugConsole.addLog("info", "frontend", "service", "Calling GoBack");

    try {
      // await BrowserService.GoBack(); // TODO: Implement BrowserService
      this.debugConsole.logServiceCall("BrowserService", "GoBack", []);
      history.back();
      this.debugConsole.addLog(
        "info",
        "frontend",
        "navigation",
        "Browser navigated back",
      );
    } catch (error) {
      this.debugConsole.logServiceCall(
        "BrowserService",
        "GoBack",
        [],
        null,
        error as Error,
      );
      console.error("Failed to go back:", error);
    }
  }

  async goForward(): Promise<void> {
    this.debugConsole.addLog(
      "info",
      "frontend",
      "service",
      "Calling GoForward",
    );

    try {
      // await BrowserService.GoForward(); // TODO: Implement BrowserService
      this.debugConsole.logServiceCall("BrowserService", "GoForward", []);
      history.forward();
      this.debugConsole.addLog(
        "info",
        "frontend",
        "navigation",
        "Browser navigated forward",
      );
    } catch (error) {
      this.debugConsole.logServiceCall(
        "BrowserService",
        "GoForward",
        [],
        null,
        error as Error,
      );
      console.error("Failed to go forward:", error);
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
      let title = "Dumber Browser";
      if (url && url !== "/") {
        const urlObj = new URL(url);
        title = `Dumber - ${urlObj.hostname}`;
      }

      document.title = title;

      // Backend title updates handled natively; no frontend call
    } catch (error) {
      console.error("Failed to update title:", error);
    }
  }

  async loadSavedZoom(): Promise<void> {
    // Zoom persistence applied natively on page load
  }

  async initialize(): Promise<void> {
    // Check if there's an initial URL from backend (direct navigation)
    // Set initial URL from browser location (homepage)
    this.setCurrentURL(window.location.href);

    // Load saved zoom level (only if we're staying on homepage)
    // Zoom handled natively
  }
}
