#include <glib.h>
#include <pthread.h>

static pthread_t main_thread_id;

void store_main_thread_id() {
    main_thread_id = pthread_self();
}

int is_main_thread() {
    return pthread_equal(pthread_self(), main_thread_id);
}

int iterate_main_loop() {
    GMainContext *ctx = g_main_context_default();
    return g_main_context_iteration(ctx, FALSE);
}
