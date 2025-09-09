import type { HistoryEntry, SearchShortcut } from '../types/wails.js';
import { BrowserService } from '../../bindings/github.com/bnema/dumber/services/index';
import { Window } from '@wailsio/runtime';

export class DOMRenderer {
  private historyListElement: HTMLElement | null = null;
  private shortcutsElement: HTMLElement | null = null;

  constructor() {
    this.historyListElement = document.getElementById('historyList');
    this.shortcutsElement = document.getElementById('shortcuts');
  }

  displayHistory(historyEntries: HistoryEntry[]): void {
    if (!this.historyListElement) return;

    if (!historyEntries || historyEntries.length === 0) {
      this.historyListElement.innerHTML = `
        <div class="empty-state">
          <h3>No history yet</h3>
          <p>Use CLI commands to browse websites</p>
        </div>
      `;
      return;
    }

    this.historyListElement.innerHTML = '';
    historyEntries.forEach(item => {
      const historyItem = this.createHistoryItem(item);
      this.historyListElement!.appendChild(historyItem);
    });
  }

  displayShortcuts(shortcutsData: Record<string, SearchShortcut>): void {
    if (!this.shortcutsElement) return;

    if (!shortcutsData || Object.keys(shortcutsData).length === 0) {
      this.shortcutsElement.innerHTML = `
        <div class="empty-state">
          <h3>No shortcuts configured</h3>
          <p>Add shortcuts to your config file</p>
        </div>
      `;
      return;
    }

    this.shortcutsElement.innerHTML = '';
    Object.entries(shortcutsData).forEach(([key, shortcut]) => {
      const shortcutEl = this.createShortcutElement(key, shortcut);
      this.shortcutsElement!.appendChild(shortcutEl);
    });
  }

  showLoading(container: 'history' | 'shortcuts'): void {
    const element = container === 'history' ? this.historyListElement : this.shortcutsElement;
    if (element) {
      element.innerHTML = '<div class="loading">Loading...</div>';
    }
  }

  showError(container: 'history' | 'shortcuts', message: string): void {
    const element = container === 'history' ? this.historyListElement : this.shortcutsElement;
    if (element) {
      element.innerHTML = `
        <div class="empty-state">
          <h3>Error</h3>
          <p>${message}</p>
        </div>
      `;
    }
  }

  private createHistoryItem(item: HistoryEntry): HTMLElement {
    const historyItem = document.createElement('div');
    historyItem.className = 'history-item';
    historyItem.innerHTML = `
      <div class="history-url">${this.escapeHtml(item.url)}</div>
      <div class="history-title">${this.escapeHtml(item.title || 'Untitled')}</div>
    `;
    
    historyItem.addEventListener('click', () => {
      this.navigateToUrl(item.url);
    });
    
    return historyItem;
  }

  private createShortcutElement(key: string, shortcut: SearchShortcut): HTMLElement {
    const shortcutEl = document.createElement('div');
    shortcutEl.className = 'shortcut';
    shortcutEl.innerHTML = `
      <div class="shortcut-key">${this.escapeHtml(key)}:</div>
      <div class="shortcut-desc">${this.escapeHtml(shortcut.description)}</div>
    `;
    
    shortcutEl.addEventListener('click', () => {
      // Navigate to the base URL of the shortcut (remove %s template parameter)
      const baseUrl = this.extractBaseUrl(shortcut.url);
      this.navigateToUrl(baseUrl);
    });
    
    return shortcutEl;
  }

  private async navigateToUrl(url: string): Promise<void> {
    console.log('Navigating to:', url);
    try {
      const savedZoom = await BrowserService.GetZoomLevel(url);
      if (savedZoom && savedZoom > 0) {
        try { Window.SetZoom(savedZoom); } catch {}
      }
    } catch (e) {
      // Ignore zoom retrieval errors
    }
    window.location.href = url;
  }

  private extractBaseUrl(templateUrl: string): string {
    try {
      const url = new URL(templateUrl);
      // Remove query parameters that contain %s template
      const params = new URLSearchParams(url.search);
      for (const [key, value] of params.entries()) {
        if (value.includes('%s')) {
          params.delete(key);
        }
      }
      url.search = params.toString();
      return url.toString();
    } catch (error) {
      // If URL parsing fails, return the original URL
      console.warn('Failed to parse shortcut URL:', templateUrl);
      return templateUrl;
    }
  }

  private async copyToClipboard(text: string): Promise<void> {
    try {
      await navigator.clipboard.writeText(text);
      console.log('Copied to clipboard:', text);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  }

  private escapeHtml(text: string): string {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
  }
}
