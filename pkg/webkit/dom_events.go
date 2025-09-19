package webkit

import (
	"encoding/json"
	"fmt"
)

// DispatchCustomEvent sends a CustomEvent into the page world with the provided detail payload.
func (w *WebView) DispatchCustomEvent(eventName string, detail map[string]any) error {
	if w == nil {
		return ErrNotImplemented
	}

	payload := "{}"
	if detail != nil {
		data, err := json.Marshal(detail)
		if err != nil {
			return fmt.Errorf("failed to marshal event detail: %w", err)
		}
		payload = string(data)
	}

	script := fmt.Sprintf(`(function(){try{var detail=%s;document.dispatchEvent(new CustomEvent(%q,{detail:detail}));}catch(e){console.error('[dumber] Failed to dispatch %s', e);}})();`, payload, eventName, eventName)
	return w.InjectScript(script)
}
