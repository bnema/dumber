#ifndef WEBVIEW_CALLBACKS_H
#define WEBVIEW_CALLBACKS_H

#include <gtk/gtk.h>
#include <webkit/webkit.h>
#include <glib-object.h>

// Forward declarations for Go callbacks
extern void goOnTitleChanged(unsigned long id, char* title);
extern void goOnURIChanged(unsigned long id, char* uri);
extern void goOnThemeChanged(unsigned long id, int prefer_dark);
extern void goOnFaviconURIChanged(unsigned long id, char* page_uri, char* favicon_uri);
extern void* goResolveURIScheme(char* uri, size_t* out_len, char** out_mime);
extern void goQuitMainLoop();

// Static inline functions to avoid multiple definition errors
static inline void on_title_notify(GObject* obj, GParamSpec* pspec, gpointer user_data) {
    (void)pspec;
    WebKitWebView* view = WEBKIT_WEB_VIEW(obj);
    const gchar* title = webkit_web_view_get_title(view);
    GtkWindow* win = GTK_WINDOW(user_data);
    if (title && win) {
        gtk_window_set_title(win, title);
    }
}

static inline void on_title_notify_id(GObject* obj, GParamSpec* pspec, gpointer user_data) {
    (void)pspec; (void)obj;
    const gchar* title = webkit_web_view_get_title(WEBKIT_WEB_VIEW(obj));
    if (title) { goOnTitleChanged((unsigned long)user_data, (char*)title); }
}

static inline void on_uri_notify(GObject* obj, GParamSpec* pspec, gpointer user_data) {
    (void)pspec; (void)obj;
    const gchar* uri = webkit_web_view_get_uri(WEBKIT_WEB_VIEW(obj));
    if (uri) { goOnURIChanged((unsigned long)user_data, (char*)uri); }
}

static inline void on_theme_changed(GObject* obj, GParamSpec* pspec, gpointer user_data) {
    (void)pspec;
    GtkSettings* settings = GTK_SETTINGS(obj);
    if (!settings) return;
    gboolean prefer = FALSE;
    g_object_get(settings, "gtk-application-prefer-dark-theme", &prefer, NULL);
    goOnThemeChanged((unsigned long)user_data, prefer ? 1 : 0);
}

static inline void on_uri_scheme(WebKitURISchemeRequest* request, gpointer user_data) {
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

static inline void on_favicon_changed(WebKitFaviconDatabase* database, gchar* page_uri, gchar* favicon_uri, gpointer user_data) {
    (void)database;
    printf("[favicon-c] Favicon changed - page: %s, favicon: %s, WebView ID: %lu\n",
           page_uri ? page_uri : "NULL", favicon_uri ? favicon_uri : "NULL", (unsigned long)user_data);

    if (!favicon_uri || !page_uri) {
        printf("[favicon-c] Invalid favicon or page URI for WebView ID %lu\n", (unsigned long)user_data);
        return;
    }

    // Pass the page URI and favicon URI to Go
    // WebKit manages the actual favicon data - we just need to store the URI mapping
    goOnFaviconURIChanged((unsigned long)user_data, page_uri, favicon_uri);
}

// GTK4 close-request signal handler
static inline gboolean on_close_request(GtkWindow* window, gpointer user_data) {
    (void)window; (void)user_data;
    goQuitMainLoop();
    return FALSE; // Allow the window to close
}

#endif // WEBVIEW_CALLBACKS_H