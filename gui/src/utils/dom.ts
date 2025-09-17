import type { History, Shortcut } from '../types/generated.js';

export class DOMRenderer {
  private historyListElement: HTMLElement | null = null;
  private shortcutsElement: HTMLElement | null = null;

  constructor() {
    this.historyListElement = document.getElementById('historyList');
    this.shortcutsElement = document.getElementById('shortcuts');
  }

  displayHistory(historyEntries: History[]): void {
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

  displayShortcuts(shortcutsData: Record<string, Shortcut>): void {
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

  private createHistoryItem(item: History): HTMLElement {
    const historyItem = document.createElement('div');
    historyItem.className = 'history-item';
    const parsed = this.safeParseURL(item.url);
    const title = item.title && item.title.trim() !== '' ? item.title : (parsed.host || 'Untitled');

    const line = document.createElement('div');
    line.className = 'history-line';

    // Favicon chip (wrapper ensures perfect circle sizing)
    const iconWrap = document.createElement('div');
    iconWrap.className = 'history-favicon-chip';
    const icon = document.createElement('img');
    icon.className = 'history-favicon-img';
    icon.width = 16; icon.height = 16;
    icon.loading = 'lazy';
    icon.referrerPolicy = 'no-referrer';
    icon.src = this.buildFaviconURL(item.url);
    icon.addEventListener('error', () => { iconWrap.style.display = 'none'; });
    iconWrap.appendChild(icon);

    const titleEl = document.createElement('span');
    titleEl.className = 'history-title';
    titleEl.textContent = title;

    const sep1 = document.createElement('span');
    sep1.className = 'history-sep';
    sep1.textContent = ' - ';

    const domainEl = document.createElement('span');
    domainEl.className = 'history-domain';
    domainEl.textContent = parsed.host || '';

    const sep2 = document.createElement('span');
    sep2.className = 'history-sep';
    sep2.textContent = ' - ';

    const urlEl = document.createElement('span');
    urlEl.className = 'history-url';
    urlEl.textContent = item.url;

    line.appendChild(iconWrap);
    line.appendChild(titleEl);
    line.appendChild(sep1);
    line.appendChild(domainEl);
    line.appendChild(sep2);
    line.appendChild(urlEl);

    historyItem.appendChild(line);
    
    historyItem.addEventListener('click', () => {
      this.navigateToUrl(item.url);
    });
    
    return historyItem;
  }

  private createShortcutElement(key: string, shortcut: Shortcut): HTMLElement {
    const shortcutEl = document.createElement('div');
    shortcutEl.className = 'shortcut';
    shortcutEl.innerHTML = `
      <div class="shortcut-key">${this.escapeHtml(key)}:</div>
      <div class="shortcut-desc">${this.escapeHtml(shortcut.description || 'No description')}</div>
    `;
    
    shortcutEl.addEventListener('click', () => {
      // Navigate to the base URL of the shortcut (remove %s template parameter)
      const baseUrl = this.extractBaseUrl(shortcut.url_template);
      this.navigateToUrl(baseUrl);
    });
    
    return shortcutEl;
  }

  private async navigateToUrl(url: string): Promise<void> {
    console.log('Navigating to:', url);
    window.location.href = url;
  }

  private extractBaseUrl(templateUrl: string): string {
    try {
      // Support both %s and {query} placeholders
      const candidate = templateUrl.replace(/%s|\{query\}/g, "");
      const url = new URL(candidate);
      // Remove empty query params left after placeholder removal
      const params = new URLSearchParams(url.search);
      for (const key of Array.from(params.keys())) {
        const v = params.get(key);
        if (v === null || v === "") {
          params.delete(key);
        }
      }
      url.search = params.toString();
      return url.toString();
    } catch (error) {
      console.error('Failed to parse shortcut URL:', templateUrl, error);
      throw error;
    }
  }

  private escapeHtml(text: string): string {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
  }

  private safeParseURL(raw: string): { host: string } {
    try {
      const u = new URL(raw);
      return { host: u.host };
    } catch {
      return { host: '' };
    }
  }

  private buildFaviconURL(raw: string): string {
    try {
      const u = new URL(raw);
      const scheme = u.protocol && u.protocol !== ':' ? u.protocol : 'https:';
      return `${scheme}//${u.host}/favicon.ico`;
    } catch {
      return '';
    }
  }
}
