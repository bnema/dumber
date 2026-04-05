package cef

import (
	"context"
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
)

func newTestPipeline(w, h, s int32) *renderPipeline {
	rp := &renderPipeline{scale: s}
	rp.widthAtomic.Store(w)
	rp.heightAtomic.Store(h)
	return rp
}

type stubFrame struct {
	main bool
	url  string
}

func (f stubFrame) IsValid() bool                                                { return true }
func (f stubFrame) Undo()                                                        {}
func (f stubFrame) Redo()                                                        {}
func (f stubFrame) Cut()                                                         {}
func (f stubFrame) Copy()                                                        {}
func (f stubFrame) Paste()                                                       {}
func (f stubFrame) PasteAndMatchStyle()                                          {}
func (f stubFrame) Del()                                                         {}
func (f stubFrame) SelectAll()                                                   {}
func (f stubFrame) ViewSource()                                                  {}
func (f stubFrame) GetSource(purecef.StringVisitor)                              {}
func (f stubFrame) GetText(purecef.StringVisitor)                                {}
func (f stubFrame) LoadRequest(purecef.Request)                                  {}
func (f stubFrame) LoadURL(string)                                               {}
func (f stubFrame) ExecuteJavaScript(string, string, int32)                      {}
func (f stubFrame) IsMain() bool                                                 { return f.main }
func (f stubFrame) IsFocused() bool                                              { return false }
func (f stubFrame) GetName() string                                              { return "" }
func (f stubFrame) GetIdentifier() string                                        { return "" }
func (f stubFrame) GetParent() purecef.Frame                                     { return nil }
func (f stubFrame) GetURL() string                                               { return f.url }
func (f stubFrame) GetBrowser() purecef.Browser                                  { return nil }
func (f stubFrame) GetV8Context() purecef.V8Context                              { return nil }
func (f stubFrame) VisitDom(purecef.Domvisitor)                                  {}
func (f stubFrame) SendProcessMessage(purecef.ProcessID, purecef.ProcessMessage) {}
func (f stubFrame) CreateUrlrequest(purecef.Request, purecef.UrlrequestClient) purecef.Urlrequest {
	return nil
}

func TestOnLoadStartFiresCommittedAndUpdatesURI(t *testing.T) {
	wv := &WebView{ctx: context.Background()}
	var gotEvents []port.LoadEvent
	wv.SetCallbacks(&port.WebViewCallbacks{
		OnLoadChanged: func(event port.LoadEvent) {
			gotEvents = append(gotEvents, event)
		},
	})

	h := &handlerSet{wv: wv}
	h.OnLoadStart(nil, stubFrame{main: true, url: "https://google.com"}, 0)

	require.Len(t, gotEvents, 1)
	require.Equal(t, port.LoadCommitted, gotEvents[0])
	require.Equal(t, "https://google.com", wv.URI())
}

func TestGetViewRectUsesDIPCoordinates(t *testing.T) {
	rect := &purecef.Rect{}
	h := &handlerSet{
		wv: &WebView{
			ctx:      context.Background(),
			pipeline: newTestPipeline(800, 600, 2),
		},
	}

	h.GetViewRect(nil, rect)

	require.Equal(t, int32(0), rect.X)
	require.Equal(t, int32(0), rect.Y)
	require.Equal(t, int32(400), rect.Width)
	require.Equal(t, int32(300), rect.Height)
}

func TestGetScreenInfoUsesDIPRectAndScale(t *testing.T) {
	si := purecef.NewScreenInfo()
	info := &si
	h := &handlerSet{
		wv: &WebView{
			ctx:      context.Background(),
			pipeline: newTestPipeline(1500, 900, 3),
		},
	}

	ok := h.GetScreenInfo(nil, info)

	require.Equal(t, int32(1), ok)
	require.InEpsilon(t, float32(3), info.DeviceScaleFactor, 0.001)
	require.Equal(t, int32(500), info.Rect.Width)
	require.Equal(t, int32(300), info.Rect.Height)
	require.Equal(t, int32(500), info.AvailableRect.Width)
	require.Equal(t, int32(300), info.AvailableRect.Height)
}

func TestOptionalHandlersRespectFactoryFlags(t *testing.T) {
	h := &handlerSet{}

	// AudioHandler is always enabled (required for media decoding).
	require.Same(t, h, h.GetAudioHandler())
	require.Nil(t, h.GetContextMenuHandler())

	h.enableContextMenuHandler = true

	require.Same(t, h, h.GetContextMenuHandler())
}
