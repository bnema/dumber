#ifndef WEBKIT_LOG_CAPTURE_H
#define WEBKIT_LOG_CAPTURE_H

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <glib.h>

static int webkit_log_pipe[2];
static FILE* webkit_original_stdout;
static FILE* webkit_original_stderr;
static gboolean webkit_capture_enabled = FALSE;

// Initialize log capture pipes
static inline void webkit_init_log_capture() {
    if (webkit_capture_enabled) return;
    
    // Create pipe for capturing C output
    if (pipe(webkit_log_pipe) == 0) {
        // Duplicate stdout/stderr for restoration later
        webkit_original_stdout = fdopen(dup(1), "w");
        webkit_original_stderr = fdopen(dup(2), "w");
        webkit_capture_enabled = TRUE;
        
        g_debug("WebKit log capture initialized");
    } else {
        g_warning("Failed to initialize WebKit log capture");
    }
}

// Start capturing WebKit output
static inline void webkit_start_log_capture() {
    if (!webkit_capture_enabled) return;
    
    // Redirect stdout/stderr to pipe (for C printf statements)
    dup2(webkit_log_pipe[1], 1);
    dup2(webkit_log_pipe[1], 2);
}

// Stop capturing and restore original output
static inline void webkit_stop_log_capture() {
    if (!webkit_capture_enabled) return;
    
    // Restore original stdout/stderr
    if (webkit_original_stdout) {
        dup2(fileno(webkit_original_stdout), 1);
        fclose(webkit_original_stdout);
    }
    if (webkit_original_stderr) {
        dup2(fileno(webkit_original_stderr), 2);
        fclose(webkit_original_stderr);
    }
    
    close(webkit_log_pipe[0]);
    close(webkit_log_pipe[1]);
    webkit_capture_enabled = FALSE;
}

// Read captured output (non-blocking)
static inline int webkit_read_captured_log(char* buffer, int buffer_size) {
    if (!webkit_capture_enabled) return 0;
    
    fd_set read_fds;
    struct timeval timeout;
    
    FD_ZERO(&read_fds);
    FD_SET(webkit_log_pipe[0], &read_fds);
    
    // Non-blocking check
    timeout.tv_sec = 0;
    timeout.tv_usec = 0;
    
    int result = select(webkit_log_pipe[0] + 1, &read_fds, NULL, NULL, &timeout);
    if (result > 0 && FD_ISSET(webkit_log_pipe[0], &read_fds)) {
        int n = read(webkit_log_pipe[0], buffer, buffer_size - 1);
        if (n > 0) {
            buffer[n] = '\0';
            return n;
        }
    }
    return 0;
}

// GLib log handler to capture GLib/GTK/WebKit messages
static inline void webkit_glib_log_handler(const gchar *log_domain,
                                          GLogLevelFlags log_level,
                                          const gchar *message,
                                          gpointer user_data) {
    // Forward to Go callback
    extern void goWebKitLogHandler(char* domain, int level, char* message);
    goWebKitLogHandler((char*)log_domain, (int)log_level, (char*)message);
}

// Set up GLib log handlers for various domains
static inline void webkit_setup_glib_log_handlers() {
    // Set up log handlers for WebKit and GTK domains
    g_log_set_handler("WebKit", G_LOG_LEVEL_MASK | G_LOG_FLAG_FATAL | G_LOG_FLAG_RECURSION,
                      webkit_glib_log_handler, NULL);
    g_log_set_handler("Gtk", G_LOG_LEVEL_MASK | G_LOG_FLAG_FATAL | G_LOG_FLAG_RECURSION,
                      webkit_glib_log_handler, NULL);
    g_log_set_handler("GLib", G_LOG_LEVEL_MASK | G_LOG_FLAG_FATAL | G_LOG_FLAG_RECURSION,
                      webkit_glib_log_handler, NULL);
    g_log_set_handler(NULL, G_LOG_LEVEL_MASK | G_LOG_FLAG_FATAL | G_LOG_FLAG_RECURSION,
                      webkit_glib_log_handler, NULL);
}

#endif // WEBKIT_LOG_CAPTURE_H