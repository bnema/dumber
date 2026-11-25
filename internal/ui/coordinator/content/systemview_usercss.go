package content

import (
	"context"
	"encoding/json"
	"fmt"

	"bnema.dev/dumber/internal/logging"
)

const (
	systemViewUserCSSKey = "user-css"
	userCSSStyleID       = "dumber-user-css"
)

// ApplySystemViewUserCSS applies the user stylesheet to every system view
// page, after the theme. An empty css removes it.
func (c *Coordinator) ApplySystemViewUserCSS(ctx context.Context, css string) {
	script, err := buildUserCSSScript(css)
	if err != nil {
		logging.FromContext(ctx).Warn().Err(err).Msg("failed to build user css script")
		return
	}
	logging.FromContext(ctx).Debug().Int("bytes", len(css)).Msg("broadcasting user css to system views")
	c.BroadcastToSystemViews(ctx, systemViewUserCSSKey, script)
}

// buildUserCSSScript returns JS that upserts <style id="dumber-user-css"> as
// the last child of <head>, or removes it when css is empty.
func buildUserCSSScript(css string) (string, error) {
	cssJSON, err := json.Marshal(css)
	if err != nil {
		return "", err
	}
	idJSON, err := json.Marshal(userCSSStyleID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`(function(){
  try {
    var cssText = %s;
    var style = document.getElementById(%s);
    if (!cssText) {
      if (style) { style.remove(); }
      return;
    }
    if (!style) {
      style = document.createElement('style');
      style.id = %s;
    }
    var head = document.head || document.documentElement;
    head.appendChild(style);
    style.textContent = cssText;
  } catch (e) {
    console.error('[dumber] failed to apply user css', e);
  }
})();`, cssJSON, idJSON, idJSON), nil
}
