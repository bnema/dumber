package cef

import (
	"runtime"
	"testing"
	"unicode/utf16"
	"unsafe"

	purecef "github.com/bnema/purego-cef/cef"
	cefmocks "github.com/bnema/purego-cef/cef/mocks"
	"github.com/bnema/puregotk/v4/gtk"
	"github.com/stretchr/testify/require"
)

func withTooltipString(text string, fn func(uintptr)) {
	units := utf16.Encode([]rune(text))
	value := cefTooltipString{Length: uintptr(len(units))}
	if len(units) > 0 {
		value.Str = &units[0]
	}
	fn(uintptr(unsafe.Pointer(&value)))
	runtime.KeepAlive(value)
	runtime.KeepAlive(units)
}

func TestDecodeCEFTooltipString(t *testing.T) {
	require.Empty(t, decodeCEFTooltipString(0))
	for _, text := range []string{"", "title", "日本語 😀", "a\x00b"} {
		withTooltipString(text, func(ptr uintptr) { require.Equal(t, text, decodeCEFTooltipString(ptr)) })
	}
	value := cefTooltipString{Length: 10}
	require.Empty(t, decodeCEFTooltipString(uintptr(unsafe.Pointer(&value))))
	units := []uint16{65}
	value = cefTooltipString{Str: &units[0]}
	require.Empty(t, decodeCEFTooltipString(uintptr(unsafe.Pointer(&value))))
	runtime.KeepAlive(units)
}

func tooltipTestView(t *testing.T) (*WebView, purecef.Browser, *[]*string) {
	t.Helper()
	browser := cefmocks.NewMockBrowser(t)
	browser.EXPECT().GetIdentifier().Return(int32(8)).Maybe()
	calls := []*string{}
	wv := &WebView{browser: browser, nativeWidget: &gtk.Widget{}}
	wv.tooltipSetFunc = func(text *string) { calls = append(calls, text) }
	return wv, browser, &calls
}

func TestOnTooltipSetsClearsAndDeduplicates(t *testing.T) {
	wv, browser, calls := tooltipTestView(t)
	h := &handlerSet{wv: wv}
	for _, text := range []string{"hello 😀", "hello 😀", "", "again"} {
		withTooltipString(text, func(ptr uintptr) { require.Zero(t, h.OnTooltip(browser, ptr)) })
	}
	require.Zero(t, h.OnTooltip(browser, 0))
	require.Len(t, *calls, 4)
	require.Equal(t, "hello 😀", *(*calls)[0])
	require.Nil(t, (*calls)[1])
	require.Equal(t, "again", *(*calls)[2])
	require.Nil(t, (*calls)[3])
}

func TestOnTooltipGuardsAndQueuedUpdates(t *testing.T) {
	var h *handlerSet
	require.Zero(t, h.OnTooltip(nil, 0))
	require.Zero(t, (&handlerSet{}).OnTooltip(nil, 0))
	for _, scenario := range []string{"destroyed", "navigation", "browser", "widget"} {
		t.Run(scenario, func(t *testing.T) {
			wv, browser, calls := tooltipTestView(t)
			var queued func()
			wv.gtkDispatch = func(fn func()) { queued = fn }
			handler := &handlerSet{wv: wv}
			withTooltipString("borrowed", func(ptr uintptr) { handler.OnTooltip(browser, ptr) })
			require.NotNil(t, queued)
			switch scenario {
			case "destroyed":
				wv.destroyed.Store(true)
			case "navigation":
				wv.documentSeq++
			case "browser":
				wv.browser = nil
			case "widget":
				wv.nativeWidget = nil
			}
			queued()
			require.Empty(t, *calls)
		})
	}
	wv, browser, calls := tooltipTestView(t)
	stale := cefmocks.NewMockBrowser(t)
	stale.EXPECT().GetIdentifier().Return(int32(9))
	h = &handlerSet{wv: wv}
	require.Zero(t, h.OnTooltip(stale, 0))
	require.Zero(t, h.OnTooltip(nil, 0))
	wv.destroyed.Store(true)
	require.Zero(t, h.OnTooltip(browser, 0))
	require.Empty(t, *calls)
}

func TestDestroyClearsTooltip(t *testing.T) {
	wv, _, calls := tooltipTestView(t)
	wv.setTooltipOnGTK("title")
	wv.Destroy()
	require.Len(t, *calls, 2)
	require.Nil(t, (*calls)[1])
}

func TestOnLoadStartClearsTooltipOnlyForMainFrame(t *testing.T) {
	wv, _, calls := tooltipTestView(t)
	wv.setTooltipOnGTK("title")
	h := &handlerSet{wv: wv}
	h.OnLoadStart(nil, stubFrame{main: false}, 0)
	require.Len(t, *calls, 1)
	h.OnLoadStart(nil, stubFrame{main: true, url: "https://example.com"}, 0)
	require.Len(t, *calls, 2)
	require.Nil(t, (*calls)[1])
}
