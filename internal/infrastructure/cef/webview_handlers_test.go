package cef

import (
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
)

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
	wv := &WebView{}
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
