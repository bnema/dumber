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
            overflow-x: hidden; /* prevent horizontal scroll from long content */
        }
        
        .container {
            flex: 1;
            display: flex;
            max-width: 1200px;
            margin: 0 auto;
            width: 100%;
            padding: 2rem;
            gap: 2rem;
            overflow-x: hidden;
        }
        
        .history-section {
            flex: 1;
            min-height: 0;
            min-width: 0; /* allow flex child to shrink */
        }
        
        .shortcuts-section {
            flex: 1;
            min-height: 0;
            min-width: 0; /* allow flex child to shrink */
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
            overflow-x: hidden;
            max-height: calc(100vh - 8rem);
            max-width: 100%;
        }
        
        .history-item {
            padding: 0.75rem;
            margin-bottom: 0.5rem;
            background: #2d2d2d;
            border-radius: 6px;
            cursor: pointer;
            transition: background 0.2s;
            border-left: 3px solid #404040;
            overflow: hidden;
            max-width: 100%;
        }
        
        .history-item:hover {
            background: #3d3d3d;
            border-left-color: #0066cc;
        }
        
        .history-line {
            display: flex;
            gap: 0.5rem;
            white-space: nowrap;
            align-items: center;
            overflow: hidden;
            width: 100%;
            min-width: 0; /* allow flex children to shrink */
        }
        .history-favicon-chip {
            flex: 0 0 20px;
            width: 20px; height: 20px;
            border-radius: 50%;
            background: #ccc;
            border: 1px solid rgba(0,0,0,.12);
            box-shadow: 0 1px 2px rgba(0,0,0,.12);
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .history-favicon-img {
            width: 16px; height: 16px;
            filter: brightness(1.06) contrast(1.03);
            image-rendering: -webkit-optimize-contrast;
        }

        .history-title { font-size: 0.95rem; color: #e6e6e6; flex: 0 0 auto; }
        .history-domain { font-size: 0.9rem; color: #a5a5a5; flex: 0 0 auto; }
        .history-sep { color: #666; flex: 0 0 auto; }
        .history-url {
            font-size: 0.9rem;
            color: #7a7a7a; /* darker than domain */
            flex: 1 1 auto;
            min-width: 0; /* critical for flex truncation */
            overflow: hidden;
            text-overflow: ellipsis; /* fallback */
            -webkit-mask-image: linear-gradient(to right, rgba(0,0,0,1) 85%, rgba(0,0,0,0) 100%);
            mask-image: linear-gradient(to right, rgba(0,0,0,1) 85%, rgba(0,0,0,0) 100%);
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
