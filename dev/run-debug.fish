#!/usr/bin/env fish
# dev/run-debug.fish - launch ./dist/dumber with verbose debug instrumentation

# Enable core dumps so we get a crash backtrace if GLib aborts
ulimit -c unlimited

# Go runtime crash dumps
set -gx GOTRACEBACK crash

# Make GLib fatal on warnings/criticals and emit stack traces
set -gx G_DEBUG "fatal-criticals,fatal-warnings,stack-trace-on-fatal"
set -gx G_MESSAGES_DEBUG all

# Execute the browser with any arguments forwarded
./dist/dumber $argv
