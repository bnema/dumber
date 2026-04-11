package webutil

import (
	"fmt"
	"html"
	"net/url"
	"strings"
)

// SanitizeCrashPageOriginalURI validates and sanitizes a URI for display on a crash page.
// Only http, https, and dumb schemes are allowed. Non-allowlisted schemes such as
// javascript:, data:, and vbscript: are intentionally rejected to prevent XSS when
// the URI is rendered into crash page HTML.
func SanitizeCrashPageOriginalURI(originalURI string) string {
	originalURI = strings.TrimSpace(originalURI)
	if originalURI == "" {
		return ""
	}
	parsed, err := url.Parse(originalURI)
	if err != nil {
		return ""
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		if parsed.Host == "" {
			return ""
		}
		return parsed.String()
	case "dumb":
		if parsed.Host == "" && parsed.Opaque == "" {
			return ""
		}
		return parsed.String()
	default: // Security: reject all non-allowlisted schemes (javascript:, data:, vbscript:, etc.)
		return ""
	}
}

// BuildCrashPageHTML returns a self-contained HTML page for renderer crash recovery.
func BuildCrashPageHTML(originalURI string) string {
	escapedURI := html.EscapeString(originalURI)
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Renderer crashed</title>
    <style>
        :root { color-scheme: dark; font-family: "IBM Plex Sans", "Segoe UI", sans-serif; }
        body {
            margin: 0;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            background: radial-gradient(circle at top, #253447, #101622 55%%);
            color: #f2f6fa;
            padding: 24px;
        }
        .card {
            width: min(640px, 100%%);
            background: rgba(10, 16, 26, 0.86);
            border: 1px solid rgba(144, 173, 205, 0.35);
            border-radius: 16px;
            box-shadow: 0 24px 64px rgba(0, 0, 0, 0.45);
            padding: 28px;
        }
        .url {
            margin: 16px 0 20px;
            padding: 12px;
            border-radius: 10px;
            background: rgba(26, 38, 56, 0.85);
            border: 1px solid rgba(139, 167, 194, 0.28);
            font-family: "IBM Plex Mono", "Fira Code", monospace;
            overflow-wrap: anywhere;
        }
        .actions { display: flex; gap: 12px; flex-wrap: wrap; }
        button {
            border: 0;
            border-radius: 10px;
            padding: 10px 16px;
            cursor: pointer;
            font-size: 0.95rem;
            font-weight: 600;
        }
        .primary { background: #4dd0e1; color: #061018; }
        .secondary { background: #233346; color: #d6e5f5; }
    </style>
</head>
<body>
    <div class="card">
        <h1>Renderer process ended</h1>
        <p>The current page was interrupted. You can reload it to continue browsing.</p>
        <div class="url">%s</div>
        <div class="actions">
            <button class="primary" id="reload-btn" data-target="%s">Reload page</button>
            <button class="secondary" id="stay-btn">Stay on this page</button>
        </div>
    </div>
    <script>
        const reloadButton = document.getElementById('reload-btn');
        const targetUrl = (reloadButton.getAttribute('data-target') || '').trim();
        reloadButton.addEventListener('click', function() {
            if (targetUrl) {
                window.location.href = targetUrl;
                return;
            }
            window.location.reload();
        });
        document.getElementById('stay-btn').addEventListener('click', function() {
            this.disabled = true;
            this.textContent = 'Staying on page';
        });
    </script>
</body>
</html>`, escapedURI, escapedURI)
}
