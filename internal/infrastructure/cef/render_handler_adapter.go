package cef

import (
	purecef "github.com/bnema/purego-cef/cef"
	cef2gtk "github.com/bnema/purego-cef2gtk"

	"github.com/bnema/dumber/internal/logging"
)

func newDumberRenderHandler(wv *WebView) purecef.RenderHandler {
	hooks := cef2gtk.Hooks{
		OnUnsupportedPaint: func() {
			if wv != nil && wv.ctx != nil {
				logging.FromContext(wv.ctx).Warn().Msg("cef: unsupported CPU paint from accelerated bridge")
			}
		},
		OnError: func(err error) {
			if wv != nil && wv.ctx != nil {
				logging.FromContext(wv.ctx).Warn().Err(err).Msg("cef: accelerated render bridge error")
			}
		},
		OnTextSelectionChanged: func(selectedText string, _ *purecef.Range) {
			handleRenderTextSelectionChanged(wv, selectedText)
		},
	}
	if wv == nil || wv.viewBridge == nil {
		var view *cef2gtk.View
		return view.RenderHandler(hooks)
	}
	return wv.viewBridge.RenderHandler(hooks)
}

func handleRenderTextSelectionChanged(wv *WebView, selectedText string) {
	if wv == nil {
		return
	}
	previous, changed := wv.setSelectedText(selectedText)
	if !changed {
		return
	}
	if wv.ctx != nil && selectedText == "" {
		if previous != "" {
			logging.FromContext(wv.ctx).Debug().
				Int("prev_text_len", len(previous)).
				Msg("cef: text selection cleared")
		}
	} else if wv.ctx != nil {
		logging.FromContext(wv.ctx).Debug().
			Int("text_len", len(selectedText)).
			Msg("cef: text selection changed")
	}
	if wv.engine != nil {
		wv.scheduleSelectionUpdate(selectedText)
	}
}
