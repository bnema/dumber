//go:build webkit_cgo

package webkit

/*
#cgo pkg-config: webkit2gtk-4.0 gtk+-3.0 javascriptcoregtk-4.0
#include <stdlib.h>
#include <gtk/gtk.h>
#include <webkit2/webkit2.h>
#include <glib-object.h>
#include <gdk/gdk.h>
#include <jsc/jsc.h>
#include <glib.h>
#include <gio/gio.h>

static GtkWidget* new_window() { return gtk_window_new(GTK_WINDOW_TOPLEVEL); }
static void connect_destroy_quit(GtkWidget* w) { g_signal_connect(w, "destroy", G_CALLBACK(gtk_main_quit), NULL); }
static WebKitWebView* as_webview(GtkWidget* w) { return WEBKIT_WEB_VIEW(w); }
static WebKitWebsiteDataManager* make_wdm(const gchar* data, const gchar* cache){
    return webkit_website_data_manager_new("base-data-directory", data, "base-cache-directory", cache, NULL);
}

static gboolean gtk_prefers_dark() {
    GtkSettings* settings = gtk_settings_get_default();
    if (!settings) return FALSE;
    gboolean prefer = FALSE;
    g_object_get(settings, "gtk-application-prefer-dark-theme", &prefer, NULL);
    return prefer;
}

// Note: preferred color scheme is handled via a user script injection to support
// older WebKitGTK versions without the color-scheme API. See enableUserContentManager.

// Forward declaration of Go callback
extern void goOnKeyPress(unsigned long id, guint keyval, GdkModifierType state);
extern void goOnButtonPress(unsigned long id, guint button, GdkModifierType state);
extern void goOnUcmMessage(unsigned long id, const char* json);
extern void goOnTitleChanged(unsigned long id, const char* title);
extern void goOnURIChanged(unsigned long id, const char* uri);
extern void* goResolveURIScheme(char* uri, size_t* out_len, char** out_mime);

gboolean on_key_press(GtkWidget* widget, GdkEventKey* event, gpointer user_data) {
    (void)widget;
    unsigned long id = (unsigned long)user_data;
    goOnKeyPress(id, event->keyval, event->state);
    return FALSE; // Propagate
}

gboolean on_button_press(GtkWidget* widget, GdkEventButton* event, gpointer user_data) {
    (void)widget;
    unsigned long id = (unsigned long)user_data;
    goOnButtonPress(id, event->button, event->state);
    return FALSE;
}

void on_title_notify(GObject* obj, GParamSpec* pspec, gpointer user_data) {
    (void)pspec;
    WebKitWebView* view = WEBKIT_WEB_VIEW(obj);
    const gchar* title = webkit_web_view_get_title(view);
    GtkWindow* win = GTK_WINDOW(user_data);
    if (title && win) {
        gtk_window_set_title(win, title);
    }
}

void on_title_notify_id(GObject* obj, GParamSpec* pspec, gpointer user_data) {
    (void)pspec; (void)obj;
    const gchar* title = webkit_web_view_get_title(WEBKIT_WEB_VIEW(obj));
    if (title) { goOnTitleChanged((unsigned long)user_data, title); }
}

void on_uri_notify(GObject* obj, GParamSpec* pspec, gpointer user_data) {
    (void)pspec; (void)obj;
    const gchar* uri = webkit_web_view_get_uri(WEBKIT_WEB_VIEW(obj));
    if (uri) { goOnURIChanged((unsigned long)user_data, uri); }
}

static gchar* js_result_to_utf8(WebKitJavascriptResult* r) {
    JSCValue* v = webkit_javascript_result_get_js_value(r);
    // Returns newly-allocated string; free with g_free()
    return jsc_value_to_string(v);
}

void on_ucm_message(WebKitUserContentManager* m, WebKitJavascriptResult* r, gpointer user_data) {
    (void)m;
    unsigned long id = (unsigned long)user_data;
    gchar* s = js_result_to_utf8(r);
    goOnUcmMessage(id, s);
    g_free(s);
}

void on_uri_scheme(WebKitURISchemeRequest* request, gpointer user_data) {
    (void)user_data;
    const gchar* uri = webkit_uri_scheme_request_get_uri(request);
    size_t n = 0;
    char* mime = NULL;
    void* buf = goResolveURIScheme((char*)uri, &n, &mime);
    if (buf && n > 0 && mime) {
        GInputStream* stream = g_memory_input_stream_new_from_data(buf, (gssize)n, g_free);
        webkit_uri_scheme_request_finish(request, stream, (gint64)n, mime);
        g_object_unref(stream);
        g_free(mime);
    } else {
        GError* err = g_error_new_literal(g_quark_from_string("dumber"), 404, "Not found");
        webkit_uri_scheme_request_finish_error(request, err);
        g_error_free(err);
    }
}
*/
import "C"

import (
    "errors"
    "log"
    "os"
    "path/filepath"
    "runtime"
    "unsafe"
)

const omniboxDefaultScript = `(() => {
  try {
    if (window.__dumber_omnibox_loaded) return; window.__dumber_omnibox_loaded = true;
    const H = { el:null,input:null,list:null,visible:false,suggestions:[],debounceTimer:0,
      post(m){ try{ window.webkit?.messageHandlers?.dumber?.postMessage(JSON.stringify(m)); }catch(_){} },
      render(){ if(!this.el) this.mount(); this.el.style.display=this.visible?'block':'none'; if(this.visible) this.input.focus(); },
      mount(){ const r=document.createElement('div'); r.style.cssText='position:fixed;inset:0;z-index:2147483647;display:none;';
        const b=document.createElement('div'); b.style.cssText='max-width:720px;margin:8vh auto;padding:10px 12px;background:#16181a;color:#e6e6e6;border:1px solid #2a2e33;border-radius:10px;box-shadow:0 12px 36px rgba(0,0,0,.55);backdrop-filter:saturate(120%) blur(2px);font-family:system-ui,-apple-system,Segoe UI,Roboto,Ubuntu,\"Helvetica Neue\",Arial,sans-serif;';
        const i=document.createElement('input'); i.type='text'; i.placeholder='Type URL or search…'; i.style.cssText='display:block;width:100%;box-sizing:border-box;padding:12px 14px;border-radius:8px;border:1px solid #3a3f45;background:#0f1113;color:#e6e6e6;font-size:15px;outline:none;';
        const l=document.createElement('div'); l.style.cssText='margin-top:10px;max-height:52vh;overflow:auto;border-top:1px solid #2a2e33;';
        b.appendChild(i); b.appendChild(l); r.appendChild(b); document.documentElement.appendChild(r);
        i.addEventListener('keydown', (e)=>{ if(e.key==='Escape'){H.toggle(false);} else if(e.key==='Enter'){ const pick=H.suggestions[H.selectedIndex|0]; const v=(pick&&pick.url)||i.value||''; if(v) H.post({type:'navigate', url:v}); H.toggle(false);} else if(e.key==='ArrowDown' || e.key==='ArrowUp'){ e.preventDefault(); const n=H.suggestions.length; if(n){ H.selectedIndex=(H.selectedIndex||0)+(e.key==='ArrowDown'?1:-1); if(H.selectedIndex<0)H.selectedIndex=n-1; if(H.selectedIndex>=n)H.selectedIndex=0; H.paintList(); } } });
        i.addEventListener('input', ()=>{ clearTimeout(H.debounceTimer); const q=i.value; H.debounceTimer=setTimeout(()=>H.post({type:'query', q, limit:10}), 120); });
        this.el=r; this.input=i; this.list=l; this.selectedIndex=-1; this.paintList(); },
      paintList(){ const l=this.list; if(!l) return; l.textContent=''; this.suggestions.forEach((s,i)=>{ const it=document.createElement('div'); it.style.cssText='padding:10px 12px;display:flex;gap:10px;align-items:center;cursor:pointer;border-bottom:1px solid #252a2f;'+(i===this.selectedIndex?'background:#0c0f12;':'');
          const icon=document.createElement('img'); icon.src=s.favicon||''; icon.width=18; icon.height=18; icon.loading='lazy'; icon.style.cssText='flex:0 0 18px;width:18px;height:18px;border-radius:4px;opacity:.95;'; icon.onerror=()=>{ icon.style.display='none'; };
          const url=document.createElement('div'); url.textContent=s.url||''; url.style.cssText='flex:1;min-width:0;color:#cfe6ff;font-size:13.5px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;';
          it.appendChild(icon); it.appendChild(url);
          it.addEventListener('mouseenter',()=>{ H.selectedIndex=i; H.paintList(); });
          it.addEventListener('click',()=>{ H.post({type:'navigate', url:s.url}); H.toggle(false); });
          l.appendChild(it);
        }); },
      toggle(v){ this.visible=(typeof v==='boolean')?v:!this.visible; this.render(); }, setSuggestions(a){ this.suggestions=Array.isArray(a)?a:[]; this.selectedIndex=-1; this.paintList(); }
    };
    window.addEventListener('keydown', (e)=>{ if((e.ctrlKey||e.metaKey) && (e.key==='l'||e.key==='L')){ e.preventDefault(); H.toggle(true); } }, true);
    window.__dumber_setSuggestions = (a)=> H.setSuggestions(a);
    window.__dumber_toggle = ()=> H.toggle();
  } catch(_){}
})();`

type nativeView struct {
    win  *C.GtkWidget
    view *C.GtkWidget
    wv   *C.WebKitWebView
    ucm  *C.WebKitUserContentManager
}

// WebView represents a browser view powered by WebKit2GTK.
type WebView struct {
    config    *Config
    zoom      float64
    url       string
    destroyed bool
    native    *nativeView
    window    *Window
    id        uintptr
    msgHandler func(payload string)
    titleHandler func(title string)
    uriHandler   func(uri string)
}

// NewWebView constructs a new WebView instance with native WebKit2GTK widgets.
func NewWebView(cfg *Config) (*WebView, error) {
    log.Printf("[webkit] Initializing GTK and creating WebView (CGO)")
    if C.gtk_init_check(nil, nil) == 0 {
        return nil, errors.New("failed to initialize GTK")
    }

    if cfg == nil {
        cfg = &Config{}
    }

    // Prepare persistent website data/caches
    dataDir := cfg.DataDir
    cacheDir := cfg.CacheDir
    if dataDir == "" { dataDir = filepath.Join(os.TempDir(), "dumber-webkit-data") }
    if cacheDir == "" { cacheDir = filepath.Join(os.TempDir(), "dumber-webkit-cache") }
    _ = os.MkdirAll(dataDir, 0o755)
    _ = os.MkdirAll(cacheDir, 0o755)

    cData := C.CString(dataDir)
    cCache := C.CString(cacheDir)
    defer C.free(unsafe.Pointer(cData))
    defer C.free(unsafe.Pointer(cCache))
    wdm := C.make_wdm((*C.gchar)(cData), (*C.gchar)(cCache))
    if wdm == nil { return nil, errors.New("failed to create WebsiteDataManager") }
    ctx := C.webkit_web_context_new_with_website_data_manager(wdm)
    if ctx == nil { return nil, errors.New("failed to create WebContext") }

    // Register custom URI scheme handler for "dumb://"
    {
        sch := C.CString("dumb")
        C.webkit_web_context_register_uri_scheme(ctx, sch, (C.WebKitURISchemeRequestCallback)(C.on_uri_scheme), nil, nil)
        C.free(unsafe.Pointer(sch))
    }

    // Cookie manager persistent storage
    cm := C.webkit_web_context_get_cookie_manager(ctx)
    if cm != nil {
        cookiePath := filepath.Join(dataDir, "cookies.sqlite")
        cCookie := C.CString(cookiePath)
        defer C.free(unsafe.Pointer(cCookie))
        C.webkit_cookie_manager_set_persistent_storage(cm, (*C.gchar)(cCookie), C.WEBKIT_COOKIE_PERSISTENT_STORAGE_SQLITE)
        C.webkit_cookie_manager_set_accept_policy(cm, C.WEBKIT_COOKIE_POLICY_ACCEPT_ALWAYS)
    }

    // Create WebView with context
    viewWidget := C.webkit_web_view_new_with_context(ctx) // GtkWidget*
    if viewWidget == nil {
        return nil, errors.New("failed to create WebKitWebView")
    }

    // Create a top-level window to host the view
    win := C.new_window()
    if win == nil {
        return nil, errors.New("failed to create GtkWindow")
    }

    // Pack view widget into the window
    C.gtk_container_add((*C.GtkContainer)(unsafe.Pointer(win)), viewWidget)
    C.gtk_window_set_default_size((*C.GtkWindow)(unsafe.Pointer(win)), 1024, 768)
    C.connect_destroy_quit(win)

    v := &WebView{
        config: cfg,
        zoom:   1.0,
        native: &nativeView{win: win, view: viewWidget, wv: C.as_webview(viewWidget), ucm: nil},
        window: &Window{win: win},
    }
    // Allow receiving button press events
    C.gtk_widget_add_events(win, C.GDK_BUTTON_PRESS_MASK|C.GDK_BUTTON_RELEASE_MASK)
    C.gtk_widget_add_events(viewWidget, C.GDK_BUTTON_PRESS_MASK|C.GDK_BUTTON_RELEASE_MASK)

    // Assign an ID for accelerator dispatch and connect input events
    v.id = nextViewID()
    registerView(v.id, v)
    C.g_signal_connect_data(C.gpointer(unsafe.Pointer(win)), C.CString("key-press-event"), C.GCallback(C.on_key_press), C.gpointer(v.id), nil, 0)
    C.g_signal_connect_data(C.gpointer(unsafe.Pointer(viewWidget)), C.CString("button-press-event"), C.GCallback(C.on_button_press), C.gpointer(v.id), nil, 0)
    C.g_signal_connect_data(C.gpointer(unsafe.Pointer(win)), C.CString("button-press-event"), C.GCallback(C.on_button_press), C.gpointer(v.id), nil, 0)
    // Update window title when page title changes
    C.g_signal_connect_data(C.gpointer(unsafe.Pointer(viewWidget)), C.CString("notify::title"), C.GCallback(C.on_title_notify), C.gpointer(unsafe.Pointer(win)), nil, 0)
    // Also dispatch title change to Go with view id
    C.g_signal_connect_data(C.gpointer(unsafe.Pointer(viewWidget)), C.CString("notify::title"), C.GCallback(C.on_title_notify_id), C.gpointer(v.id), nil, 0)
    // Notify URI changes to Go to keep current URL in sync
    C.g_signal_connect_data(C.gpointer(unsafe.Pointer(viewWidget)), C.CString("notify::uri"), C.GCallback(C.on_uri_notify), C.gpointer(v.id), nil, 0)
    if cfg.ZoomDefault > 0 {
        v.zoom = cfg.ZoomDefault
        C.webkit_web_view_set_zoom_level(v.native.wv, C.gdouble(v.zoom))
    }
    if cfg.EnableDeveloperExtras {
        settings := C.webkit_web_view_get_settings(v.native.wv)
        if settings != nil {
            C.webkit_settings_set_enable_developer_extras(settings, C.gboolean(1))
        }
    }
    // Apply default fonts if provided in cfg
    if settings := C.webkit_web_view_get_settings(v.native.wv); settings != nil {
        if cfg.DefaultSansFont != "" {
            csans := C.CString(cfg.DefaultSansFont)
            C.webkit_settings_set_sans_serif_font_family(settings, (*C.gchar)(csans))
            C.free(unsafe.Pointer(csans))
            // Also set default font family to sans
            csans2 := C.CString(cfg.DefaultSansFont)
            C.webkit_settings_set_default_font_family(settings, (*C.gchar)(csans2))
            C.free(unsafe.Pointer(csans2))
        }
        if cfg.DefaultSerifFont != "" {
            cserif := C.CString(cfg.DefaultSerifFont)
            C.webkit_settings_set_serif_font_family(settings, (*C.gchar)(cserif))
            C.free(unsafe.Pointer(cserif))
        }
        if cfg.DefaultMonospaceFont != "" {
            cmono := C.CString(cfg.DefaultMonospaceFont)
            C.webkit_settings_set_monospace_font_family(settings, (*C.gchar)(cmono))
            C.free(unsafe.Pointer(cmono))
        }
        if cfg.DefaultFontSize > 0 {
            C.webkit_settings_set_default_font_size(settings, C.guint(cfg.DefaultFontSize))
        }
    }
    if cfg.InitialURL != "" {
        _ = v.LoadURL(cfg.InitialURL)
    }
    // Ensure native resources are released if user forgets to call Destroy
    runtime.SetFinalizer(v, func(v *WebView) { _ = v.Destroy() })
    log.Printf("[webkit] WebView created (CGO)")

    // Obtain the view's UserContentManager and inject script
    if v.native != nil && v.native.wv != nil {
        u := C.webkit_web_view_get_user_content_manager(v.native.wv)
        if u != nil {
            v.native.ucm = u
            v.enableUserContentManager()
        }
    }
    return v, nil
}

func (w *WebView) LoadURL(rawURL string) error {
    if w == nil || w.destroyed || w.native == nil || w.native.wv == nil {
        return ErrNotImplemented
    }
    w.url = rawURL
    curl := C.CString(rawURL)
    defer C.free(unsafe.Pointer(curl))
    C.webkit_web_view_load_uri(w.native.wv, (*C.gchar)(curl))
    log.Printf("[webkit] LoadURL: %s", rawURL)
    return nil
}

func (w *WebView) Show() error {
    if w == nil || w.destroyed || w.native == nil || w.native.win == nil {
        return ErrNotImplemented
    }
    C.gtk_widget_show(w.native.win)
    C.gtk_widget_show(w.native.view)
    log.Printf("[webkit] Show window")
    return nil
}

func (w *WebView) Hide() error {
    if w == nil || w.destroyed || w.native == nil || w.native.win == nil {
        return ErrNotImplemented
    }
    C.gtk_widget_hide(w.native.win)
    return nil
}

func (w *WebView) Destroy() error {
    if w == nil || w.native == nil || w.native.win == nil {
        return ErrNotImplemented
    }
    C.gtk_widget_destroy(w.native.win)
    w.destroyed = true
    unregisterView(w.id)
    log.Printf("[webkit] Destroy window")
    return nil
}

// Window returns the associated native window.
func (w *WebView) Window() *Window { return w.window }

// GetCurrentURL returns the current URI from WebKit.
func (w *WebView) GetCurrentURL() string {
    if w == nil || w.native == nil || w.native.wv == nil { return "" }
    uri := C.webkit_web_view_get_uri(w.native.wv)
    if uri == nil { return "" }
    return C.GoString((*C.char)(unsafe.Pointer(uri)))
}

func (w *WebView) GoBack() error {
    if w == nil || w.native == nil || w.native.wv == nil { return ErrNotImplemented }
    C.webkit_web_view_go_back(w.native.wv)
    return nil
}

func (w *WebView) GoForward() error {
    if w == nil || w.native == nil || w.native.wv == nil { return ErrNotImplemented }
    C.webkit_web_view_go_forward(w.native.wv)
    return nil
}

// RegisterScriptMessageHandler registers a callback invoked when the content script posts a message.
func (w *WebView) RegisterScriptMessageHandler(cb func(payload string)) { w.msgHandler = cb }

func (w *WebView) dispatchScriptMessage(payload string) {
    if w != nil && w.msgHandler != nil { w.msgHandler(payload) }
}

// RegisterTitleChangedHandler registers a callback invoked when the page title changes.
func (w *WebView) RegisterTitleChangedHandler(cb func(title string)) { w.titleHandler = cb }

func (w *WebView) dispatchTitleChanged(title string) {
    if w != nil && w.titleHandler != nil { w.titleHandler(title) }
}

// RegisterURIChangedHandler registers a callback invoked when the current page URI changes.
func (w *WebView) RegisterURIChangedHandler(cb func(uri string)) { w.uriHandler = cb }

func (w *WebView) dispatchURIChanged(uri string) {
    if w != nil && w.uriHandler != nil { w.uriHandler(uri) }
}

// enableUserContentManager registers the 'dumber' message handler and injects the omnibox script.
func (w *WebView) enableUserContentManager() {
    if w == nil || w.native == nil || w.native.ucm == nil { return }
    // Register handler "dumber"
    cname := C.CString("dumber")
    defer C.free(unsafe.Pointer(cname))
    C.webkit_user_content_manager_register_script_message_handler(w.native.ucm, (*C.gchar)(cname))
    C.g_signal_connect_data(C.gpointer(unsafe.Pointer(w.native.ucm)), C.CString("script-message-received::dumber"), C.GCallback(C.on_ucm_message), C.gpointer(w.id), nil, 0)

    // Inject color-scheme preference script at document-start to inform sites of system theme
    preferDark := C.gtk_prefers_dark() != 0
    if preferDark {
        log.Printf("[theme] GTK prefers: dark")
    } else {
        log.Printf("[theme] GTK prefers: light")
    }
    var schemeJS string
    if preferDark {
        schemeJS = "(() => { try { const d=true; const cs=d?'dark':'light'; console.log('[dumber] color-scheme set:', cs); try{ window.webkit?.messageHandlers?.dumber?.postMessage(JSON.stringify({type:'theme', value: cs})) }catch(_){} const meta=document.createElement('meta'); meta.name='color-scheme'; meta.content='dark light'; document.documentElement.appendChild(meta); const s=document.createElement('style'); s.textContent=':root{color-scheme:dark;}'; document.documentElement.appendChild(s); const qD='(prefers-color-scheme: dark)'; const qL='(prefers-color-scheme: light)'; const orig=window.matchMedia; window.matchMedia=function(q){ if(typeof q==='string'&&(q.includes(qD)||q.includes(qL))){ const m={matches:q.includes('dark')?d:!d,media:q,onchange:null,addListener(){},removeListener(){},addEventListener(){},removeEventListener(){},dispatchEvent(){return false;}}; return m;} return orig.call(window,q); }; } catch(e){ console.warn('[dumber] color-scheme injection failed', e) } })();"
    } else {
        schemeJS = "(() => { try { const d=false; const cs=d?'dark':'light'; console.log('[dumber] color-scheme set:', cs); try{ window.webkit?.messageHandlers?.dumber?.postMessage(JSON.stringify({type:'theme', value: cs})) }catch(_){} const meta=document.createElement('meta'); meta.name='color-scheme'; meta.content='light dark'; document.documentElement.appendChild(meta); const s=document.createElement('style'); s.textContent=':root{color-scheme:light;}'; document.documentElement.appendChild(s); const qD='(prefers-color-scheme: dark)'; const qL='(prefers-color-scheme: light)'; const orig=window.matchMedia; window.matchMedia=function(q){ if(typeof q==='string'&&(q.includes(qD)||q.includes(qL))){ const m={matches:q.includes('dark')?d:!d,media:q,onchange:null,addListener(){},removeListener(){},addEventListener(){},removeEventListener(){},dispatchEvent(){return false;}}; return m;} return orig.call(window,q); }; } catch(e){ console.warn('[dumber] color-scheme injection failed', e) } })();"
    }
    cScheme := C.CString(schemeJS)
    defer C.free(unsafe.Pointer(cScheme))
    schemeScript := C.webkit_user_script_new((*C.gchar)(cScheme), C.WEBKIT_USER_CONTENT_INJECT_ALL_FRAMES, C.WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_START, nil, nil)
    if schemeScript != nil {
        C.webkit_user_content_manager_add_script(w.native.ucm, schemeScript)
        C.webkit_user_script_unref(schemeScript)
    }

    // Add user script at document-start
    src := C.CString(omniboxDefaultScript)
    defer C.free(unsafe.Pointer(src))
    script := C.webkit_user_script_new((*C.gchar)(src), C.WEBKIT_USER_CONTENT_INJECT_ALL_FRAMES, C.WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_START, nil, nil)
    if script != nil {
        C.webkit_user_content_manager_add_script(w.native.ucm, script)
        C.webkit_user_script_unref(script)
        log.Printf("[webkit] UCM omnibox script injected at document-start")
    }

    // Inject Wails runtime fetch interceptor for homepage bridging
    wailsBridge := `(() => { try { const origFetch = window.fetch.bind(window); const waiters = Object.create(null); window.__dumber_wails_resolve = (id, json) => { const w = waiters[id]; if(!w) return; delete waiters[id]; try { const headers = new Headers({"Content-Type":"application/json"}); w.resolve(new Response(json, { headers })); } catch(e){ w.reject(e); } }; window.fetch = (input, init) => { try { const url = new URL(input instanceof Request ? input.url : input, window.location.origin); if (url.pathname === '/wails/runtime') { const args = url.searchParams.get('args'); let payload = {}; try { payload = args ? JSON.parse(args) : {}; } catch(_){} const id = String(Date.now()) + '-' + Math.random().toString(36).slice(2); return new Promise((resolve, reject) => { waiters[id] = { resolve, reject }; try { window.webkit?.messageHandlers?.dumber?.postMessage(JSON.stringify({ type: 'wails', id, payload })); } catch (e) { reject(e); } }); } } catch(_){} return origFetch(input, init); }; } catch(_){} })();`
    cWails := C.CString(wailsBridge)
    defer C.free(unsafe.Pointer(cWails))
    wailsScript := C.webkit_user_script_new((*C.gchar)(cWails), C.WEBKIT_USER_CONTENT_INJECT_ALL_FRAMES, C.WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_START, nil, nil)
    if wailsScript != nil {
        C.webkit_user_content_manager_add_script(w.native.ucm, wailsScript)
        C.webkit_user_script_unref(wailsScript)
    }
}
