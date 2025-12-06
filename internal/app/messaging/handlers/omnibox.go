package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/bnema/dumber/internal/logging"
)

// OmniboxMessage contains fields for omnibox operations.
type OmniboxMessage struct {
	Q     string `json:"q"`
	Limit int    `json:"limit"`
}

type suggestion struct {
	URL     string `json:"url"`
	Favicon string `json:"favicon,omitempty"`
}

// HandleQuery computes omnibox suggestions and returns them to the GUI.
func HandleQuery(c *Context, msg OmniboxMessage) {
	if !c.IsReady() {
		return
	}

	q := msg.Q
	limit := msg.Limit
	if limit <= 0 {
		limit = 10
	}

	results := make([]suggestion, 0, limit)
	seen := make(map[string]struct{}, limit*2)

	// History search
	if len(results) < limit {
		remaining := limit - len(results)
		if entries, err := c.BrowserService.SearchHistory(c.Ctx(), q, remaining); err == nil {
			for _, e := range entries {
				bb, _ := json.Marshal(e)
				var m map[string]any
				_ = json.Unmarshal(bb, &m)
				var url string
				if s, ok := m["url"].(string); ok {
					url = s
				} else if s, ok := m["URL"].(string); ok {
					url = s
				}
				if url == "" {
					continue
				}
				if _, ok := seen[url]; ok {
					continue
				}
				favicon := ""
				if s, ok := m["favicon_url"].(string); ok && s != "" {
					favicon = s
				}
				results = append(results, suggestion{URL: url, Favicon: favicon})
				seen[url] = struct{}{}
				if len(results) >= limit {
					break
				}
			}
		}
	}

	// Inject back to GUI
	if b, err := json.Marshal(results); err == nil {
		script := "(window.__dumber?.omnibox?.suggestions ? window.__dumber.omnibox.suggestions(" + string(b) + ") : (window.__dumber_omnibox_suggestions && window.__dumber_omnibox_suggestions(" + string(b) + ")))"
		if err := c.WebView.InjectScript(script); err != nil {
			logging.Error(fmt.Sprintf("[handlers] Failed to inject omnibox suggestions: %v", err))
		}
	}
}

// HandleOmniboxInitialHistory processes initial history display for empty omnibox.
func HandleOmniboxInitialHistory(c *Context, msg OmniboxMessage) {
	if !c.IsReady() {
		return
	}

	limit := msg.Limit
	if limit <= 0 {
		limit = 10
	}

	cfg, err := c.BrowserService.GetConfig(c.Ctx())
	if err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to get config: %v", err))
		return
	}
	behavior := cfg.Omnibox.InitialBehavior

	results := make([]suggestion, 0, limit)
	seen := make(map[string]struct{}, limit*2)

	var entries []interface{}

	switch behavior {
	case "recent":
		historyEntries, histErr := c.BrowserService.GetRecentHistory(c.Ctx(), limit)
		err = histErr
		for _, e := range historyEntries {
			entries = append(entries, e)
		}
	case "most_visited":
		historyEntries, histErr := c.BrowserService.GetMostVisited(c.Ctx(), limit)
		err = histErr
		for _, e := range historyEntries {
			entries = append(entries, e)
		}
	case "none":
		entries = []interface{}{}
	}

	if err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to fetch initial history: %v", err))
		return
	}

	for _, entry := range entries {
		bb, _ := json.Marshal(entry)
		var m map[string]any
		_ = json.Unmarshal(bb, &m)

		var url string
		if s, ok := m["url"].(string); ok {
			url = s
		} else if s, ok := m["URL"].(string); ok {
			url = s
		}
		if url == "" {
			continue
		}

		if _, ok := seen[url]; ok {
			continue
		}

		favicon := ""
		if s, ok := m["favicon_url"].(string); ok && s != "" {
			favicon = s
		}

		results = append(results, suggestion{URL: url, Favicon: favicon})
		seen[url] = struct{}{}
		if len(results) >= limit {
			break
		}
	}

	if b, err := json.Marshal(results); err == nil {
		script := "(window.__dumber?.omnibox?.suggestions ? window.__dumber.omnibox.suggestions(" + string(b) + ") : (window.__dumber_omnibox_suggestions && window.__dumber_omnibox_suggestions(" + string(b) + ")))"
		if err := c.WebView.InjectScript(script); err != nil {
			logging.Error(fmt.Sprintf("[handlers] Failed to inject initial history: %v", err))
		}
	}
}

// HandlePrefixQuery returns the best prefix-matching URL for inline suggestions.
func HandlePrefixQuery(c *Context, msg OmniboxMessage) {
	if !c.IsReady() {
		return
	}

	query := msg.Q
	if query == "" {
		_ = c.WebView.InjectScript("window.__dumber_omnibox_inline_suggestion && window.__dumber_omnibox_inline_suggestion(null)")
		return
	}

	prefixURL := c.BrowserService.GetBestPrefixMatch(c.Ctx(), query)

	if prefixURL != "" {
		escaped, err := json.Marshal(prefixURL)
		if err != nil {
			_ = c.WebView.InjectScript("window.__dumber_omnibox_inline_suggestion && window.__dumber_omnibox_inline_suggestion(null)")
			return
		}
		_ = c.WebView.InjectScript("window.__dumber_omnibox_inline_suggestion && window.__dumber_omnibox_inline_suggestion(" + string(escaped) + ")")
	} else {
		_ = c.WebView.InjectScript("window.__dumber_omnibox_inline_suggestion && window.__dumber_omnibox_inline_suggestion(null)")
	}
}
