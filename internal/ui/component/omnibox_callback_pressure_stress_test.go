package component

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bnema/purego"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gobject"
	"github.com/bnema/puregotk/v4/gtk"
	"github.com/stretchr/testify/require"
)

const gtkCallbackStressIterations = 3_200

type callbackLedgerEvent struct {
	Event string `json:"event"`
	Type  string `json:"type"`
}

// runGTKCallbackLifecycleStress exercises real GTK motion controllers and raw
// tick callbacks while p3 and p0 are repeatedly removed/rebuilt. It is never
// opt-in: GTK init is skipped only when the host has no usable native display.
func runGTKCallbackLifecycleStress(t *testing.T) {
	t.Helper()
	ledgerPath := t.TempDir() + "/purego-callback-ledger.jsonl"
	t.Setenv("PUREGO_CALLBACK_LEDGER", "1")
	t.Setenv("PUREGO_CALLBACK_LEDGER_FILE", ledgerPath)
	if !gtk.InitCheck() {
		t.Skip("GTK native display prerequisite unavailable (gtk.InitCheck returned false)")
	}

	mainContext := glib.MainContextDefault()
	require.NotNil(t, mainContext)
	done := make(chan struct{})
	stress := glib.SourceFunc(func(uintptr) bool {
		defer close(done)
		runGTKCallbackLifecycleStressOnMain(t, mainContext, ledgerPath)
		return false
	})
	// Invoke guarantees that the entire GTK mutation sequence is owned by the
	// default context, whether the test thread owns it or a native GTK loop does.
	mainContext.Invoke(&stress, 0)
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("GTK main context did not execute callback lifecycle stress")
	}
}

func runGTKCallbackLifecycleStressOnMain(t *testing.T, mainContext *glib.MainContext, ledgerPath string) {
	t.Helper()
	assertMainContextOwner := func() {
		t.Helper()
		require.True(t, mainContext.IsOwner(), "GTK mutation must own the default GLib main context")
	}

	assertMainContextOwner()
	root := gtk.NewBox(gtk.OrientationVerticalValue, 0)
	assertMainContextOwner()
	p0 := gtk.NewBox(gtk.OrientationVerticalValue, 0)
	assertMainContextOwner()
	p3 := gtk.NewBox(gtk.OrientationVerticalValue, 0)
	require.NotNil(t, root)
	require.NotNil(t, p0)
	require.NotNil(t, p3)
	assertMainContextOwner()
	root.Append(&p0.Widget)
	assertMainContextOwner()
	root.Append(&p3.Widget)

	enters, leaves := 0, 0
	for i := 0; i < gtkCallbackStressIterations; i++ {
		pane := p0
		if i%2 != 0 {
			pane = p3
		}
		assertMainContextOwner()
		controller := gtk.NewEventControllerMotion()
		require.NotNil(t, controller)
		enter := func(_ gtk.EventControllerMotion, _ float64, _ float64) { enters++ }
		leave := func(_ gtk.EventControllerMotion) { leaves++ }
		enterID := controller.ConnectEnter(&enter)
		leaveID := controller.ConnectLeave(&leave)
		assertMainContextOwner()
		pane.Widget.AddController(&controller.EventController)

		// Invoke the real connected handler functions in GTK enter/leave order
		// while the alternate pane is closed and the workspace rebuilt.
		enter(*controller, 0, 0)
		leave(*controller)

		tick := gtk.TickCallback(func(uintptr, uintptr, uintptr) bool { return false })
		tickCallback := glib.NewCallback(&tick)
		assertMainContextOwner()
		tickID := pane.Widget.AddTickCallback(&tick, 0, nil)
		require.NotZero(t, tickID)
		assertMainContextOwner()
		pane.Widget.RemoveTickCallback(tickID)
		require.NoError(t, purego.UnrefCallback(tickCallback), "raw tick slot must be released after GTK removal")

		assertMainContextOwner()
		gobject.SignalHandlerDisconnect(gobject.ObjectNewFromInternalPtr(controller.GoPointer()), enterID)
		assertMainContextOwner()
		gobject.SignalHandlerDisconnect(gobject.ObjectNewFromInternalPtr(controller.GoPointer()), leaveID)
		assertMainContextOwner()
		pane.Widget.RemoveController(&controller.EventController)

		// Alternate p3 <-> p0 close/rebuild mutations on the same main context.
		assertMainContextOwner()
		root.Remove(&pane.Widget)
		assertMainContextOwner()
		root.Append(&pane.Widget)
	}
	require.Equal(t, gtkCallbackStressIterations, enters)
	require.Equal(t, gtkCallbackStressIterations, leaves)

	events := readCallbackLedgerEvents(t, ledgerPath)
	var tickReleases, tickReuses int
	for _, event := range events {
		if !strings.Contains(event.Type, "TickCallback") {
			continue
		}
		switch event.Event {
		case "release-fnptr", "release":
			tickReleases++
		case "reuse":
			tickReuses++
		case "release-miss", "release-fnptr-miss", "exhausted":
			t.Fatalf("callback ledger recorded %s for tick callback", event.Event)
		}
	}
	require.Equal(t, gtkCallbackStressIterations, tickReleases, "every raw GTK tick slot must be released exactly once")
	require.GreaterOrEqual(t, tickReuses, gtkCallbackStressIterations-1, "released raw tick slots must be reused")
}

func readCallbackLedgerEvents(t *testing.T, path string) []callbackLedgerEvent {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()

	var events []callbackLedgerEvent
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event callbackLedgerEvent
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &event))
		events = append(events, event)
	}
	require.NoError(t, scanner.Err())
	return events
}
