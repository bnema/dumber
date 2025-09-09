export class HTMLBuilder {
  static createBaseHTML(): string {
    return `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Dumber Browser</title>
    <style>
        ${this.getStyles()}
    </style>
</head>
<body>
    <div class="container">
        <div class="history-section">
            <h2 class="section-title">Recent History</h2>
            <div id="historyList" class="history-list">
                <div class="loading">Loading history...</div>
            </div>
        </div>
        
        <div class="shortcuts-section">
            <h2 class="section-title">Search Shortcuts</h2>
            <div id="shortcuts" class="shortcuts-grid">
                <div class="loading">Loading shortcuts...</div>
            </div>
        </div>
    </div>
    <script type="module" src="./main.js"></script>
</body>
</html>`;
  }

  private static getStyles(): string {
    return `
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #1a1a1a;
            color: #ffffff;
            height: 100vh;
            display: flex;
            flex-direction: column;
        }
        
        .container {
            flex: 1;
            display: flex;
            max-width: 1200px;
            margin: 0 auto;
            width: 100%;
            padding: 2rem;
            gap: 2rem;
        }
        
        .history-section {
            flex: 1;
            min-height: 0;
        }
        
        .shortcuts-section {
            flex: 1;
            min-height: 0;
        }
        
        .section-title {
            font-size: 1.5rem;
            margin-bottom: 1rem;
            color: #ffffff;
            border-bottom: 2px solid #404040;
            padding-bottom: 0.5rem;
        }
        
        .history-list {
            overflow-y: auto;
            max-height: calc(100vh - 8rem);
        }
        
        .history-item {
            padding: 0.75rem;
            margin-bottom: 0.5rem;
            background: #2d2d2d;
            border-radius: 6px;
            cursor: pointer;
            transition: background 0.2s;
            border-left: 3px solid #404040;
        }
        
        .history-item:hover {
            background: #3d3d3d;
            border-left-color: #0066cc;
        }
        
        .history-url {
            font-size: 0.9rem;
            color: #cccccc;
            margin-bottom: 0.25rem;
            word-break: break-all;
        }
        
        .history-title {
            font-size: 0.8rem;
            color: #888;
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
        }
        
        .shortcuts-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 1rem;
            max-height: calc(100vh - 8rem);
            overflow-y: auto;
        }
        
        .shortcut {
            padding: 1rem;
            background: #2d2d2d;
            border: 2px solid #404040;
            border-radius: 8px;
            cursor: pointer;
            transition: all 0.2s;
            text-align: center;
        }
        
        .shortcut:hover {
            background: #3d3d3d;
            border-color: #0066cc;
            transform: translateY(-2px);
        }
        
        .shortcut-key {
            font-weight: bold;
            color: #0066cc;
            margin-bottom: 0.5rem;
            font-size: 1.1rem;
        }
        
        .shortcut-desc {
            color: #888;
            font-size: 0.9rem;
        }
        
        .loading {
            padding: 2rem;
            text-align: center;
            color: #888;
        }
        
        .empty-state {
            padding: 3rem 2rem;
            text-align: center;
            color: #666;
        }
        
        .empty-state h3 {
            margin-bottom: 1rem;
            color: #888;
        }
        
        @media (max-width: 768px) {
            .container {
                flex-direction: column;
                padding: 1rem;
            }
            
            .shortcuts-grid {
                grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
            }
        }
    `;
  }
}