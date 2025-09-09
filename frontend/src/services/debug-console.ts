export interface DebugLogEntry {
  timestamp: string;
  level: 'info' | 'warn' | 'error' | 'debug';
  source: 'frontend' | 'backend';
  category: string;
  message: string;
  data?: any;
}

export class DebugConsoleService {
  private logs: DebugLogEntry[] = [];
  private maxLogs = 1000;
  private isVisible = false;
  private debugElement: HTMLElement | null = null;
  private logsContainer: HTMLElement | null = null;

  constructor() {
    this.createDebugUI();
    this.setupKeyboardShortcut();
    this.interceptConsole();
  }

  private createDebugUI(): void {
    // Create debug panel
    this.debugElement = document.createElement('div');
    this.debugElement.id = 'debug-console';
    this.debugElement.style.cssText = `
      position: fixed;
      top: 0;
      right: 0;
      width: 50%;
      height: 100%;
      background: rgba(0, 0, 0, 0.95);
      color: #00ff00;
      font-family: 'Courier New', monospace;
      font-size: 12px;
      z-index: 10000;
      display: none;
      border-left: 2px solid #00ff00;
    `;

    // Create header
    const header = document.createElement('div');
    header.style.cssText = `
      padding: 10px;
      background: rgba(0, 255, 0, 0.1);
      border-bottom: 1px solid #00ff00;
      font-weight: bold;
      display: flex;
      justify-content: space-between;
      align-items: center;
    `;
    header.innerHTML = `
      <span>🐛 DUMBER DEBUG CONSOLE</span>
      <div style="font-size: 10px;">
        <span style="color: #888; margin-right: 10px;">F11: Inspect Services | F12: Toggle</span>
        <button id="clear-logs" style="background: none; border: 1px solid #00ff00; color: #00ff00; padding: 2px 8px; margin-right: 5px; cursor: pointer;">Clear</button>
        <button id="close-debug" style="background: none; border: 1px solid #00ff00; color: #00ff00; padding: 2px 8px; cursor: pointer;">Close</button>
      </div>
    `;

    // Create logs container
    this.logsContainer = document.createElement('div');
    this.logsContainer.style.cssText = `
      height: calc(100% - 50px);
      overflow-y: auto;
      padding: 10px;
    `;

    this.debugElement.appendChild(header);
    this.debugElement.appendChild(this.logsContainer);
    document.body.appendChild(this.debugElement);

    // Setup button handlers
    header.querySelector('#clear-logs')?.addEventListener('click', () => this.clearLogs());
    header.querySelector('#close-debug')?.addEventListener('click', () => this.hide());
  }

  private setupKeyboardShortcut(): void {
    document.addEventListener('keydown', (event) => {
      // F12 to toggle debug console
      if (event.key === 'F12') {
        event.preventDefault();
        this.toggle();
      }
      
      // F11 to inspect window.go in detail
      if (event.key === 'F11') {
        event.preventDefault();
        this.inspectWailsServices();
      }
    });
  }

  private inspectWailsServices(): void {
    this.addLog('info', 'frontend', 'debug', '=== WAILS SERVICES INSPECTION ===');
    this.addLog('info', 'frontend', 'debug', `typeof window: ${typeof window}`);
    this.addLog('info', 'frontend', 'debug', `window.go exists: ${!!window.go}`);
    
    if (window.go) {
      this.addLog('info', 'frontend', 'debug', `typeof window.go: ${typeof window.go}`, window.go);
      this.addLog('info', 'frontend', 'debug', `window.go.services exists: ${!!window.go.services}`);
      
      if (window.go.services) {
        const services = window.go.services;
        this.addLog('info', 'frontend', 'debug', `typeof window.go.services: ${typeof services}`, services);
        
        const serviceNames = Object.keys(services);
        this.addLog('info', 'frontend', 'debug', `Service count: ${serviceNames.length}`);
        this.addLog('info', 'frontend', 'debug', `Service names: [${serviceNames.join(', ')}]`);
        
        serviceNames.forEach(name => {
          const service = services[name];
          this.addLog('info', 'frontend', 'debug', `${name}: ${typeof service}`, service);
          
          if (service && typeof service === 'object') {
            const methods = Object.getOwnPropertyNames(service).filter(prop => typeof service[prop] === 'function');
            this.addLog('info', 'frontend', 'debug', `${name} methods: [${methods.join(', ')}]`);
          }
        });
      } else {
        this.addLog('error', 'frontend', 'debug', 'window.go.services is null/undefined');
      }
    } else {
      this.addLog('error', 'frontend', 'debug', 'window.go is null/undefined');
    }
    
    this.addLog('info', 'frontend', 'debug', '=== END INSPECTION ===');
  }

  private interceptConsole(): void {
    const originalConsole = {
      log: console.log,
      warn: console.warn,
      error: console.error,
      info: console.info,
      debug: console.debug
    };

    console.log = (...args) => {
      this.addLog('info', 'frontend', 'console', args.join(' '));
      originalConsole.log(...args);
    };

    console.warn = (...args) => {
      this.addLog('warn', 'frontend', 'console', args.join(' '));
      originalConsole.warn(...args);
    };

    console.error = (...args) => {
      this.addLog('error', 'frontend', 'console', args.join(' '));
      originalConsole.error(...args);
    };

    console.info = (...args) => {
      this.addLog('info', 'frontend', 'console', args.join(' '));
      originalConsole.info(...args);
    };

    console.debug = (...args) => {
      this.addLog('debug', 'frontend', 'console', args.join(' '));
      originalConsole.debug(...args);
    };
  }

  public addLog(level: DebugLogEntry['level'], source: DebugLogEntry['source'], category: string, message: string, data?: any): void {
    const entry: DebugLogEntry = {
      timestamp: new Date().toISOString().split('T')[1].split('.')[0], // HH:MM:SS format
      level,
      source,
      category,
      message,
      data
    };

    this.logs.push(entry);
    
    // Keep only the last maxLogs entries
    if (this.logs.length > this.maxLogs) {
      this.logs.shift();
    }

    this.updateDisplay();
  }

  private updateDisplay(): void {
    if (!this.logsContainer) return;

    const logHTML = this.logs.map(log => {
      const levelColor = {
        info: '#00ff00',
        warn: '#ffff00', 
        error: '#ff0000',
        debug: '#00ffff'
      }[log.level];

      const sourceIcon = log.source === 'backend' ? '🖥️' : '🌐';

      return `
        <div style="margin-bottom: 2px; font-size: 11px;">
          <span style="color: #888;">[${log.timestamp}]</span>
          <span style="color: ${levelColor};">${sourceIcon} ${log.level.toUpperCase()}</span>
          <span style="color: #aaa;">[${log.category}]</span>
          <span style="color: #fff;">${log.message}</span>
          ${log.data ? `<pre style="color: #ccc; font-size: 10px; margin: 2px 0;">${JSON.stringify(log.data, null, 2)}</pre>` : ''}
        </div>
      `;
    }).join('');

    this.logsContainer.innerHTML = logHTML;
    
    // Auto-scroll to bottom
    this.logsContainer.scrollTop = this.logsContainer.scrollHeight;
  }

  public logKeyboardEvent(event: KeyboardEvent): void {
    const modifiers = [];
    if (event.ctrlKey) modifiers.push('Ctrl');
    if (event.shiftKey) modifiers.push('Shift');
    if (event.altKey) modifiers.push('Alt');
    if (event.metaKey) modifiers.push('Meta');

    const shortcut = modifiers.length > 0 ? `${modifiers.join('+')}+${event.key}` : event.key;
    
    this.addLog('info', 'frontend', 'keyboard', `Key pressed: ${shortcut}`, {
      key: event.key,
      code: event.code,
      modifiers: modifiers,
      target: event.target?.constructor.name
    });
  }

  public logMouseEvent(event: MouseEvent): void {
    const buttonNames = {
      0: 'Left',
      1: 'Middle', 
      2: 'Right',
      3: 'Back',
      4: 'Forward'
    };

    this.addLog('info', 'frontend', 'mouse', `Mouse ${event.type}: ${buttonNames[event.button] || `Button${event.button}`}`, {
      button: event.button,
      clientX: event.clientX,
      clientY: event.clientY,
      target: event.target?.constructor.name
    });
  }

  public logServiceCall(serviceName: string, methodName: string, params: any[], result?: any, error?: Error): void {
    if (error) {
      this.addLog('error', 'frontend', 'service', `${serviceName}.${methodName}() failed`, {
        params,
        error: error.message
      });
    } else {
      this.addLog('info', 'frontend', 'service', `${serviceName}.${methodName}() called`, {
        params,
        result
      });
    }
  }

  public logBackendMessage(message: string, data?: any): void {
    this.addLog('info', 'backend', 'service', message, data);
  }

  public show(): void {
    if (this.debugElement) {
      this.debugElement.style.display = 'block';
      this.isVisible = true;
      this.addLog('info', 'frontend', 'debug', 'Debug console opened');
    }
  }

  public hide(): void {
    if (this.debugElement) {
      this.debugElement.style.display = 'none';
      this.isVisible = false;
    }
  }

  public toggle(): void {
    if (this.isVisible) {
      this.hide();
    } else {
      this.show();
    }
  }

  public clearLogs(): void {
    this.logs = [];
    this.updateDisplay();
    this.addLog('info', 'frontend', 'debug', 'Logs cleared');
  }

  public isDebugVisible(): boolean {
    return this.isVisible;
  }
}