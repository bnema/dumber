package cef

import (
	_ "embed"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// vimPageRuntimeHarness is a DOM shim for the embedded runtime: three visible
// paragraphs and one below the fold, two links, a Selection and Range with
// Chromium-like semantics, and a fetch that records every message posted to
// the Go bridge. See testdata/vim_page_harness.js.
//
//go:embed testdata/vim_page_harness.js
var vimPageRuntimeHarness string

// runVimPageRuntime executes the embedded runtime inside the harness, then
// the given driver script, and returns every message posted to the bridge
// plus the driver's own output appended to the "posted" array.
func runVimPageRuntime(t *testing.T, driver string) []map[string]any {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		if os.Getenv("DUMBER_REQUIRE_NODE") == "1" {
			t.Fatal("node is required (DUMBER_REQUIRE_NODE=1) to execute the vim page runtime")
		}
		t.Skip("node is required to execute the vim page runtime")
	}
	script := vimPageRuntimeHarness + "\n" + vimPageRuntimeTemplateJS + "\n" + driver +
		"\nprocess.stdout.write(JSON.stringify(posted));"
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	require.NoError(t, err, "runtime failed: %s", output)
	var posted []map[string]any
	require.NoError(t, json.Unmarshal(output, &posted), "runtime output: %s", output)
	return posted
}

// named returns the driver snapshot recorded under name.
func named(t *testing.T, posted []map[string]any, name string) map[string]any {
	t.Helper()
	for _, entry := range posted {
		if entry["n"] == name {
			return entry
		}
	}
	require.FailNow(t, "snapshot not recorded", name)
	return nil
}

// bridgeMessages returns the runtime's own bridge messages, without snapshots.
func bridgeMessages(posted []map[string]any) []map[string]any {
	var out []map[string]any
	for _, entry := range posted {
		if _, ok := entry["token"]; ok {
			out = append(out, entry)
		}
	}
	return out
}

func modeSequence(posted []map[string]any) []any {
	var out []any
	for _, entry := range bridgeMessages(posted) {
		if entry["type"] == "mode" {
			out = append(out, entry["mode"])
		}
	}
	return out
}

func TestVimPageRuntimeVisualStartsWithTextAnchorHints(t *testing.T) {
	posted := runVimPageRuntime(t, `
window.__dumberVimPage.start("tok", { kind: "visual", color: "#fbbf24" });
posted.push({ n: "hints", labels: hintLabels(), selection: selection.rangeCount, caret: cursor() !== null });
`)
	hints := named(t, posted, "hints")
	require.Equal(t, []any{"s", "a", "d"}, hints["labels"], "one label per visible text block, none for text below the fold")
	require.EqualValues(t, 0, hints["selection"])
	require.Equal(t, false, hints["caret"], "no caret until an anchor is picked")
	require.Equal(t, []any{"hints"}, modeSequence(posted))
}

func TestVimPageRuntimeTextAnchorLabelPlacesCaret(t *testing.T) {
	posted := runVimPageRuntime(t, `
startCaret("tok", "a");
S("caret", { hints: hintLabels(), cursor: cursor() && cursor().style });
`)
	caret := named(t, posted, "caret")
	require.Empty(t, caret["text"])
	require.Equal(t, "1:0", caret["anchor"], "the label puts the caret at the start of that text")
	require.Equal(t, "1:0", caret["focus"])
	require.Empty(t, caret["hints"], "the hint overlay is gone once the caret is placed")
	style := caret["cursor"].(map[string]any)
	require.Equal(t, "fixed", style["position"])
	require.Equal(t, "#fbbf24", style["background"], "the caret uses the accent color")
	require.Equal(t, "none", style["pointer-events"])
	require.Equal(t, "2147483647", style["z-index"])
	require.Equal(t, "8px", style["left"])
	require.Equal(t, "37px", style["top"], "the caret is taller than the line, centered on it")
	require.Equal(t, "26px", style["height"])
	require.Equal(t, "4px", style["width"])
	require.Equal(t, []any{"hints", "caret"}, modeSequence(posted))
	require.Empty(t, bridgeMessagesOfType(posted, "end"))
}

func bridgeMessagesOfType(posted []map[string]any, messageType string) []map[string]any {
	var out []map[string]any
	for _, message := range bridgeMessages(posted) {
		if message["type"] == messageType {
			out = append(out, message)
		}
	}
	return out
}

func TestVimPageRuntimeEscapeInTextAnchorHintsPicksFirstLongText(t *testing.T) {
	posted := runVimPageRuntime(t, `
window.__dumberVimPage.start("tok", { kind: "visual" });
vp().key("tok", "<Escape>");
S("caret");
`)
	caret := named(t, posted, "caret")
	require.Equal(t, "1:0", caret["focus"], "the first visible text of at least 50 characters is chosen over short banners")
	require.Equal(t, []any{"hints", "caret"}, modeSequence(posted))
}

func TestVimPageRuntimeEscapeInTextAnchorHintsFallsBackToAnyVisibleText(t *testing.T) {
	posted := runVimPageRuntime(t, `
second.text.data = "short";
window.__dumberVimPage.start("tok", { kind: "visual" });
vp().key("tok", "<Escape>");
S("caret");
`)
	require.Equal(t, "0:0", named(t, posted, "caret")["focus"])
}

func TestVimPageRuntimeVisualEndsWithReasonWhenNoVisibleText(t *testing.T) {
	posted := runVimPageRuntime(t, `
for (const t of texts) t.parentElement.rect = rect(10, 5000, 600, 20);
window.__dumberVimPage.start("tok", { kind: "visual" });
`)
	require.Equal(t, []map[string]any{{"token": "tok", "type": "end", "reason": "no-visible-text"}}, bridgeMessages(posted))
}

func TestVimPageRuntimeEscapeFallbackEndsWhenNothingVisible(t *testing.T) {
	posted := runVimPageRuntime(t, `
window.__dumberVimPage.start("tok", { kind: "visual" });
for (const t of texts) t.parentElement.rect = rect(10, 5000, 600, 20);
vp().key("tok", "<Escape>");
`)
	ends := bridgeMessagesOfType(posted, "end")
	require.Len(t, ends, 1)
	require.Equal(t, "no-visible-text", ends[0]["reason"])
}

func TestVimPageRuntimeVisibleSelectionStartsVisualDirectly(t *testing.T) {
	posted := runVimPageRuntime(t, `
selection.setBaseAndExtent(first.text, 0, first.text, 5);
window.__dumberVimPage.start("tok", { kind: "visual" });
S("start", { hints: hintLabels(), cursor: cursor() !== null });
vp().key("tok", "l");
S("extended");
`)
	start := named(t, posted, "start")
	require.Equal(t, "Alpha", start["text"], "an existing visible selection is used as is")
	require.Empty(t, start["hints"])
	require.Equal(t, true, start["cursor"])
	require.Equal(t, "Alpha ", named(t, posted, "extended")["text"])
	require.Equal(t, []any{"visual"}, modeSequence(posted))
}

func TestVimPageRuntimeSelectionOutsideViewportIsIgnored(t *testing.T) {
	posted := runVimPageRuntime(t, `
selection.setBaseAndExtent(far.text, 0, far.text, 3);
window.__dumberVimPage.start("tok", { kind: "visual" });
posted.push({ n: "hints", labels: hintLabels() });
`)
	require.Equal(t, []any{"s", "a", "d"}, named(t, posted, "hints")["labels"])
	require.Equal(t, []any{"hints"}, modeSequence(posted))
}

func TestVimPageRuntimeCaretMotionsMoveWithoutSelecting(t *testing.T) {
	tests := []struct {
		name  string
		start string // hint label
		keys  []string
		want  string // focus
	}{
		{"l", "s", []string{"l", "l"}, "0:2"},
		{"counted l", "s", []string{"3", "l"}, "0:3"},
		{"h clamps", "s", []string{"l", "h", "h"}, "0:0"},
		{"w", "s", []string{"w"}, "0:6"},
		{"2w", "s", []string{"2", "w"}, "0:12"},
		{"w crosses line", "s", []string{"3", "w"}, "1:0"},
		{"e", "s", []string{"e"}, "0:5"},
		{"b", "s", []string{"w", "w", "b"}, "0:6"},
		{"b at start", "s", []string{"b"}, "0:0"},
		{"$", "s", []string{"$"}, "0:19"},
		{"0", "s", []string{"$", "0"}, "0:0"},
		{"^", "s", []string{"$", "^"}, "0:0"},
		{"j", "s", []string{"l", "l", "j"}, "1:2"},
		{"k", "a", []string{"l", "k"}, "0:1"},
		{"G", "s", []string{"G"}, "3:18"},
		{"gg", "a", []string{"G", "g", "g"}, "0:0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys, err := json.Marshal(tt.keys)
			require.NoError(t, err)
			posted := runVimPageRuntime(t, `
startCaret("tok", `+jsString(tt.start)+`);
keys("tok", ...`+string(keys)+`);
S("end");
`)
			end := named(t, posted, "end")
			require.Equal(t, tt.want, end["focus"])
			require.Equal(t, end["focus"], end["anchor"], "the caret stays collapsed")
			require.Empty(t, end["text"])
			require.Empty(t, bridgeMessagesOfType(posted, "end"))
		})
	}
}

// Sentence and paragraph motions are native Selection.modify granularities
// the DOM shim cannot emulate, so the contract checked here is the call: the
// alter mode follows the phase and the direction and granularity are exact.
func TestVimPageRuntimeNativeGranularityMotions(t *testing.T) {
	tests := []struct {
		key         string
		direction   string
		granularity string
	}{
		{"(", "backward", "sentence"},
		{")", "forward", "sentence"},
		{"{", "backward", "paragraph"},
		{"}", "forward", "paragraph"},
		{"0", "backward", "lineboundary"},
		{"$", "forward", "lineboundary"},
		{"G", "forward", "documentboundary"},
	}
	for _, tt := range tests {
		for _, phase := range []struct{ name, setup, alter string }{
			{"caret", "", "move"},
			{"visual", `keys("tok", "v");`, "extend"},
		} {
			t.Run(phase.name+" "+tt.key, func(t *testing.T) {
				posted := runVimPageRuntime(t, `
startCaret("tok", "s");
`+phase.setup+`
modifyCalls.length = 0;
keys("tok", `+jsString(tt.key)+`);
posted.push({ n: "calls", calls: modifyCalls });
`)
				require.Equal(t, []any{[]any{phase.alter, tt.direction, tt.granularity}}, named(t, posted, "calls")["calls"])
			})
		}
	}
}

func TestVimPageRuntimeWordMotionsSkipPunctuation(t *testing.T) {
	posted := runVimPageRuntime(t, `
startCaret("tok", "d");
keys("tok", "w");
S("w1");
keys("tok", "w");
S("w2");
keys("tok", "b");
S("b1");
`)
	// "Third line, end.": w from "Third" -> "line"; w -> "end" (the comma and
	// space are skipped); b -> back to "line".
	require.Equal(t, "2:6", named(t, posted, "w1")["focus"])
	require.Equal(t, "2:12", named(t, posted, "w2")["focus"])
	require.Equal(t, "2:6", named(t, posted, "b1")["focus"])
}

func TestVimPageRuntimeCaretMotionScrollsFocusIntoView(t *testing.T) {
	posted := runVimPageRuntime(t, `
startCaret("tok", "s");
scrolls.length = 0;
keys("tok", "G");
posted.push({ n: "scrolled", scrolls });
`)
	scrolled := named(t, posted, "scrolled")["scrolls"].([]any)
	require.NotEmpty(t, scrolled, "moving the caret below the fold scrolls it into view")
	require.Greater(t, scrolled[0].([]any)[1].(float64), float64(1000))
}

func TestVimPageRuntimeCaretFollowsScrollAndIsRemovedOnEnd(t *testing.T) {
	posted := runVimPageRuntime(t, `
startCaret("tok", "s");
const bar = cursor();
first.p.rect = rect(10, -300, 600, 20); // the page scrolled the paragraph away
fire("scroll");
flushFrames();
const hiddenWhileOffscreen = bar.style.display;
first.p.rect = rect(10, 100, 600, 20);
fire("scroll");
flushFrames();
posted.push({ n: "follow", hidden: hiddenWhileOffscreen, top: bar.style.top, display: bar.style.display });
keys("tok", "<Escape>");
posted.push({ n: "cleanup", caret: cursor() === null, styles: overlays("data-dumber-vim-visual").length, scrollListeners: (listeners.scroll || []).length, resize: (listeners.resize || []).length, selection: selection.rangeCount });
`)
	follow := named(t, posted, "follow")
	require.Equal(t, "none", follow["hidden"], "the caret hides while its text is off screen")
	require.Equal(t, "97px", follow["top"], "the caret follows its text when the page scrolls")
	require.Empty(t, follow["display"])
	cleanup := named(t, posted, "cleanup")
	require.Equal(t, true, cleanup["caret"], "the caret overlay is removed on cleanup")
	require.EqualValues(t, 0, cleanup["styles"])
	require.EqualValues(t, 0, cleanup["scrollListeners"])
	require.EqualValues(t, 0, cleanup["resize"])
	require.EqualValues(t, 0, cleanup["selection"])
}

func TestVimPageRuntimeVisualFromCaretExtendsAndYankReturnsToCaret(t *testing.T) {
	posted := runVimPageRuntime(t, `
startCaret("tok", "s");
keys("tok", "v");
S("v");
keys("tok", "l", "l");
S("extended");
keys("tok", "w");
S("word");
keys("tok", "y");
S("after");
keys("tok", "l");
S("moved");
keys("tok", "y");
S("caretYank");
keys("tok", "v", "e");
S("second");
keys("tok", "<Return>");
S("afterReturn");
`)
	require.Equal(t, "A", named(t, posted, "v")["text"], "v starts charwise from the caret")
	require.Equal(t, "Alp", named(t, posted, "extended")["text"])
	require.Equal(t, "Alpha ", named(t, posted, "word")["text"])

	after := named(t, posted, "after")
	require.Empty(t, after["text"], "y returns to a collapsed caret")
	require.Equal(t, "0:5", after["focus"], "on the last selected character, where v put the caret")
	require.Equal(t, after["focus"], after["anchor"])
	require.Equal(t, "0:6", named(t, posted, "moved")["focus"], "the caret keeps moving after a yank")
	require.Equal(t, "0:6", named(t, posted, "caretYank")["focus"], "y in caret mode does nothing")

	copies := bridgeMessagesOfType(posted, "copy")
	require.Len(t, copies, 2, "y in caret mode copies nothing; Enter chains a second copy")
	require.Equal(t, map[string]any{"token": "tok", "type": "copy", "text": "Alpha "}, copies[0])
	require.Equal(t, "bravo", copies[1]["text"], "v includes the character under the caret and e extends to the word end")
	require.Empty(t, named(t, posted, "afterReturn")["text"])
	require.Empty(t, bridgeMessagesOfType(posted, "end"), "copying never ends the interaction")
	require.Equal(t, []any{"hints", "caret", "visual", "caret", "visual", "caret"}, modeSequence(posted))
}

func TestVimPageRuntimeVisualOSwapsEnds(t *testing.T) {
	posted := runVimPageRuntime(t, `
startCaret("tok", "s");
keys("tok", "l", "l", "l", "v");
S("before");
keys("tok", "o");
S("swapped");
keys("tok", "h", "h");
S("shrunk");
keys("tok", "o", "l");
S("back");
`)
	before := named(t, posted, "before")
	require.Equal(t, "0:3", before["anchor"])
	require.Equal(t, "0:4", before["focus"])
	swapped := named(t, posted, "swapped")
	require.Equal(t, "0:4", swapped["anchor"])
	require.Equal(t, "0:3", swapped["focus"])
	require.Equal(t, "0:4", named(t, posted, "shrunk")["anchor"])
	require.Equal(t, "0:1", named(t, posted, "shrunk")["focus"])
	require.Equal(t, "lph", named(t, posted, "shrunk")["text"])
	back := named(t, posted, "back")
	require.Equal(t, "0:1", back["anchor"])
	require.Equal(t, "0:5", back["focus"], "after o the motions extend the other end")
}

func TestVimPageRuntimeVisualLine(t *testing.T) {
	posted := runVimPageRuntime(t, `
startCaret("tok", "a");
keys("tok", "l", "l");
keys("tok", "V");
S("line");
keys("tok", "j");
S("down");
keys("tok", "k", "k");
S("up");
keys("tok", "o");
S("swapped");
keys("tok", "y");
S("yanked");
`)
	require.Equal(t, "The quick brown fox jumps over the lazy dog near the river", named(t, posted, "line")["text"], "V selects the whole line")
	require.Equal(t, "The quick brown fox jumps over the lazy dog near the river\nThird line, end.", named(t, posted, "down")["text"])
	up := named(t, posted, "up")
	require.Equal(t, "Alpha bravo charlie\nThe quick brown fox jumps over the lazy dog near the river", up["text"], "motions extend by whole lines")
	require.Equal(t, "1:58", up["anchor"])
	require.Equal(t, "0:0", up["focus"])
	require.Equal(t, "0:0", named(t, posted, "swapped")["anchor"], "o swaps the ends")
	yanked := named(t, posted, "yanked")
	require.Empty(t, yanked["text"])
	require.Equal(t, yanked["focus"], yanked["anchor"], "y returns to a caret at the focus")
	copies := bridgeMessagesOfType(posted, "copy")
	require.Len(t, copies, 1)
	require.Equal(t, "Alpha bravo charlie\nThe quick brown fox jumps over the lazy dog near the river", copies[0]["text"])
	require.Equal(t, []any{"hints", "caret", "visual-line", "caret"}, modeSequence(posted))
}

func TestVimPageRuntimeModeSwitchingKeys(t *testing.T) {
	posted := runVimPageRuntime(t, `
startCaret("tok", "s");
keys("tok", "l", "l");
keys("tok", "v", "l");
S("visual");
keys("tok", "V");
S("line");
keys("tok", "v");
S("visualAgain");
keys("tok", "V");
keys("tok", "V");
S("caretFromLine");
keys("tok", "v");
keys("tok", "v");
S("caretFromVisual");
keys("tok", "v", "l", "<Escape>");
S("escapeVisual");
keys("tok", "V", "<Escape>");
S("escapeLine");
`)
	require.Equal(t, "ph", named(t, posted, "visual")["text"], "v includes the character under the caret, then l extends")
	require.Equal(t, "Alpha bravo charlie", named(t, posted, "line")["text"])
	require.Equal(t, "Alpha bravo charlie", named(t, posted, "visualAgain")["text"], "v in visual line switches to charwise without changing the range")
	require.Empty(t, named(t, posted, "caretFromLine")["text"], "V in visual line goes back to caret")
	require.Empty(t, named(t, posted, "caretFromVisual")["text"], "v in visual goes back to caret")
	require.Empty(t, named(t, posted, "escapeVisual")["text"], "Escape in visual goes back to caret")
	require.Empty(t, named(t, posted, "escapeLine")["text"], "Escape in visual line goes back to caret")
	require.Empty(t, bridgeMessagesOfType(posted, "end"), "none of these end the interaction")
	require.Equal(t, []any{
		"hints", "caret", "visual", "visual-line", "visual", "visual-line", "caret",
		"visual", "caret", "visual", "caret", "visual-line", "caret",
	}, modeSequence(posted))
}

func TestVimPageRuntimeEscapeInCaretEndsInteraction(t *testing.T) {
	posted := runVimPageRuntime(t, `
startCaret("tok", "s");
keys("tok", "l", "<Escape>");
posted.push({ n: "after", selection: selection.rangeCount, caret: cursor() === null });
`)
	require.Equal(t, []map[string]any{{"token": "tok", "type": "end"}}, bridgeMessagesOfType(posted, "end"))
	after := named(t, posted, "after")
	require.EqualValues(t, 0, after["selection"])
	require.Equal(t, true, after["caret"])
}

func TestVimPageRuntimeCancelRemovesCaretAndSelection(t *testing.T) {
	posted := runVimPageRuntime(t, `
startCaret("tok", "s");
keys("tok", "v", "l");
vp().cancel("tok");
posted.push({ n: "after", selection: selection.rangeCount, caret: cursor() === null, styles: overlays("data-dumber-vim-visual").length });
`)
	after := named(t, posted, "after")
	require.EqualValues(t, 0, after["selection"])
	require.Equal(t, true, after["caret"])
	require.EqualValues(t, 0, after["styles"])
	require.Empty(t, bridgeMessagesOfType(posted, "end"), "cancel is Go-initiated and reports nothing")
}

func TestVimPageRuntimeCancelRemovesHintOverlay(t *testing.T) {
	posted := runVimPageRuntime(t, `
window.__dumberVimPage.start("tok", { kind: "visual" });
vp().cancel("tok");
posted.push({ n: "after", labels: hintLabels() });
`)
	require.Empty(t, named(t, posted, "after")["labels"])
}

func TestVimPageRuntimeKeyErrorReleasesCapture(t *testing.T) {
	posted := runVimPageRuntime(t, `
startCaret("tok", "s");
selection.modify = () => { throw new Error("page quirk"); };
keys("tok", "l");
`)
	ends := bridgeMessagesOfType(posted, "end")
	require.Len(t, ends, 1, "a failing motion must not leave the keyboard captured")
	require.Equal(t, "error", ends[0]["reason"])
}

func TestVimPageRuntimeHintsFollowAndYank(t *testing.T) {
	posted := runVimPageRuntime(t, `
const vpage = window.__dumberVimPage;
vpage.start("a", { kind: "hint-follow" });
posted.push({ n: "labels", labels: hintLabels() });
vpage.key("a", "a");
vpage.start("b", { kind: "hint-yank-url" });
vpage.key("b", "s");
vpage.start("c", { kind: "hint-follow-new" });
vpage.key("c", "a");
vpage.start("d", { kind: "hint-follow" });
vpage.key("d", "<Escape>");
`)
	require.ElementsMatch(t, []any{"s", "a"}, named(t, posted, "labels")["labels"], "two visible links get one-letter labels")
	require.Equal(t, "https://example.com/two", recordedField(posted, "clicked"), "follow clicks the chosen link")
	messages := bridgeMessages(posted)
	require.Equal(t, []map[string]any{
		{"token": "a", "type": "end"},
		{"token": "b", "type": "copy", "text": "file:///one.html"},
		{"token": "c", "type": "open-new", "url": "https://example.com/two"},
		{"token": "d", "type": "end"},
	}, messages)
	require.NotContains(t, modeSequence(posted), "caret", "link hints never report caret modes")
}

// recordedField returns a field recorded by a harness side effect (such as a click).
func recordedField(posted []map[string]any, key string) any {
	for _, entry := range posted {
		if value, ok := entry[key]; ok {
			return value
		}
	}
	return nil
}

func TestVimPageRuntimeYankParagraph(t *testing.T) {
	posted := runVimPageRuntime(t, `
first.p.innerText = first.text.data;
window.__dumberVimPage.start("tok", { kind: "yank", object: "paragraph" });
`)
	messages := bridgeMessages(posted)
	require.Len(t, messages, 1)
	require.Equal(t, "copy", messages[0]["type"])
	require.True(t, strings.HasPrefix(messages[0]["text"].(string), "Alpha"))
}

func TestVimPageRuntimeStaleTokenKeyReleasesCapture(t *testing.T) {
	posted := runVimPageRuntime(t, `
startCaret("live", "s");
vp().key("stale", "j");
posted.push({ n: "live", selection: selection.rangeCount });
`)
	require.Equal(t, []map[string]any{{"token": "stale", "type": "end"}}, bridgeMessagesOfType(posted, "end"))
	require.EqualValues(t, 1, named(t, posted, "live")["selection"], "a stale key must not disturb the live interaction")
}

// Entering visual selects the character under the caret and leaving it must
// put the caret back on that character; the caret used to move one position
// right on every v then Escape.
func TestVimPageRuntimeVisualRoundTripDoesNotDriftCaret(t *testing.T) {
	posted := runVimPageRuntime(t, `
startCaret("tok", "s");
keys("tok", "l", "l");
S("start");
for (let i = 0; i < 5; i++) keys("tok", "v", "<Escape>");
S("afterEscapes");
for (let i = 0; i < 3; i++) keys("tok", "v", "v");
S("afterToggles");
for (let i = 0; i < 3; i++) keys("tok", "v", "y");
S("afterYanks");
keys("tok", "V", "V");
S("afterLine");
`)
	start := named(t, posted, "start")
	require.Equal(t, "0:2", start["focus"])
	for _, name := range []string{"afterEscapes", "afterToggles", "afterYanks"} {
		after := named(t, posted, name)
		require.Equal(t, start["focus"], after["focus"], name)
		require.Equal(t, start["anchor"], after["anchor"], name)
		require.Empty(t, after["text"], name)
	}
	copies := bridgeMessagesOfType(posted, "copy")
	require.Len(t, copies, 3)
	for _, c := range copies {
		require.Equal(t, "p", c["text"], "each yank copies the character under the caret")
	}
	require.Equal(t, "0:18", named(t, posted, "afterLine")["focus"], "V then V returns to the end of the line")
}

func TestVimPageRuntimeVisualRoundTripAtEndOfTextKeepsCaret(t *testing.T) {
	posted := runVimPageRuntime(t, `
startCaret("tok", "d");
keys("tok", "G");
S("end");
keys("tok", "v");
S("visual");
keys("tok", "<Escape>");
S("back");
`)
	end := named(t, posted, "end")
	require.Equal(t, "3:18", end["focus"])
	visual := named(t, posted, "visual")
	require.Equal(t, "d", visual["text"], "at the end of the text v selects the last character")
	require.Equal(t, "3:18", visual["focus"])
	require.Equal(t, "3:17", named(t, posted, "back")["focus"], "the caret returns onto the last character")
}

// Motions must not read the whole selection text per step: that made long
// selections quadratic.
func TestVimPageRuntimeMotionsDoNotReadSelectionText(t *testing.T) {
	posted := runVimPageRuntime(t, `
startCaret("tok", "s");
keys("tok", "v");
toStringCalls = 0;
keys("tok", "w", "w", "e", "b", "$", "^");
posted.push({ n: "reads", calls: toStringCalls });
S("sel");
`)
	require.EqualValues(t, 0, named(t, posted, "reads")["calls"])
	require.NotEmpty(t, named(t, posted, "sel")["focus"])
}

func TestVimPageRuntimeStartAndCancelFailuresReleaseCapture(t *testing.T) {
	posted := runVimPageRuntime(t, `
const realGetSelection = window.getSelection;
window.getSelection = () => { throw new Error("page quirk"); };
window.__dumberVimPage.start("bad", { kind: "visual" });
window.getSelection = realGetSelection;
posted.push({ n: "afterStart", hints: hintLabels().length, caret: cursor() === null });

startCaret("tok", "s");
selection.removeAllRanges = () => { throw new Error("page quirk"); };
vp().cancel("tok");
posted.push({ n: "afterCancel", caret: cursor() === null, styles: overlays("data-dumber-vim-visual").length, listeners: (listeners.scroll || []).length });
`)
	ends := bridgeMessagesOfType(posted, "end")
	require.Equal(t, []map[string]any{{"token": "bad", "type": "end", "reason": "error"}}, ends, "a failing start reports the end")
	afterStart := named(t, posted, "afterStart")
	require.EqualValues(t, 0, afterStart["hints"])
	cancel := named(t, posted, "afterCancel")
	require.Equal(t, true, cancel["caret"], "cancel removes the overlay even when the selection cannot be cleared")
	require.EqualValues(t, 0, cancel["styles"])
	require.EqualValues(t, 0, cancel["listeners"])
}

func TestVimPageRuntimePageHideCleansUp(t *testing.T) {
	posted := runVimPageRuntime(t, `
startCaret("tok", "s");
fire("pagehide");
posted.push({ n: "gone", caret: cursor() === null, scroll: (listeners.scroll || []).length, frames: frames.size });
fire("pagehide"); // without an interaction it is a no-op
`)
	gone := named(t, posted, "gone")
	require.Equal(t, true, gone["caret"])
	require.EqualValues(t, 0, gone["scroll"])
	require.Empty(t, bridgeMessagesOfType(posted, "end"), "the document is leaving; nothing is reported")
}

// SVG anchors expose href as an SVGAnimatedString and have no click().
func TestVimPageRuntimeSVGLinks(t *testing.T) {
	posted := runVimPageRuntime(t, `
const svg = body.appendChild(new SVGAElement_("../rel.html"));
svg.rect = rect(150, 120, 40, 20);
const w = window.__dumberVimPage;
w.start("y", { kind: "hint-yank-url" });
w.key("y", "d");
w.start("n", { kind: "hint-follow-new" });
w.key("n", "d");
w.start("f", { kind: "hint-follow" });
w.key("f", "d");
`)
	require.Equal(t, []map[string]any{
		{"token": "y", "type": "copy", "text": "https://example.com/rel.html"},
		{"token": "n", "type": "open-new", "url": "https://example.com/rel.html"},
		{"token": "f", "type": "end"},
	}, bridgeMessages(posted), "relative SVG hrefs resolve against the document")
	require.Equal(t, "click", recordedField(posted, "dispatched"), "following dispatches a click when the element has no click()")
	require.Equal(t, true, recordedField(posted, "bubbles"))
	require.Equal(t, true, recordedField(posted, "cancelable"))
}

// Chromium reports Selection positions inside a shadow tree relative to the
// host, so shadow text never anchors the caret, while link hints and text
// objects keep reaching into shadow roots.
func TestVimPageRuntimeShadowTextIsNotACaretAnchor(t *testing.T) {
	posted := runVimPageRuntime(t, `
shadowParagraph("Shadow text that is visible", 200);
window.__dumberVimPage.start("tok", { kind: "visual" });
posted.push({ n: "hints", labels: hintLabels() });
vp().key("tok", "<Escape>");
S("fallback");
vp().cancel("tok");

for (const t of texts) if (t !== texts[texts.length - 1]) t.parentElement.rect = rect(10, 5000, 600, 20);
window.__dumberVimPage.start("only", { kind: "visual" });
`)
	require.Equal(t, []any{"s", "a", "d"}, named(t, posted, "hints")["labels"], "no label for the shadow paragraph")
	require.Equal(t, "1:0", named(t, posted, "fallback")["focus"], "the fallback anchor ignores shadow text too")
	ends := bridgeMessagesOfType(posted, "end")
	require.Len(t, ends, 1)
	require.Equal(t, "no-visible-text", ends[0]["reason"], "shadow text alone offers no anchor")
}

func TestVimPageRuntimeYankObjectStillReadsShadowDOM(t *testing.T) {
	posted := runVimPageRuntime(t, `
for (const t of texts.slice(0, 4)) t.parentElement.rect = rect(10, 5000, 600, 20);
const shadow = shadowParagraph("Inside shadow", 200);
shadow.p.innerText = "Inside shadow";
window.__dumberVimPage.start("tok", { kind: "yank", object: "paragraph" });
`)
	require.Equal(t, []map[string]any{{"token": "tok", "type": "copy", "text": "Inside shadow"}}, bridgeMessages(posted))
}

// Off-screen elements are rejected from their bounding box, before the costly
// closest() and range measurements run.
func TestVimPageRuntimeSkipsOffscreenWork(t *testing.T) {
	posted := runVimPageRuntime(t, `
for (let i = 0; i < 5; i++) {
  const link = body.appendChild(new HTMLAnchorElement("https://example.com/" + i));
  link.rect = rect(10, 4000 + i * 30, 40, 20);
}
closestCalls = 0;
window.__dumberVimPage.start("h", { kind: "hint-follow" });
posted.push({ n: "hints", closest: closestCalls, labels: hintLabels().length });
vp().cancel("h");
rangeRectCalls = 0;
window.__dumberVimPage.start("v", { kind: "visual" });
posted.push({ n: "anchors", ranges: rangeRectCalls, labels: hintLabels().length });
`)
	hints := named(t, posted, "hints")
	require.EqualValues(t, 2, hints["labels"])
	require.EqualValues(t, 2, hints["closest"], "only the two on-screen links reach closest()")
	anchors := named(t, posted, "anchors")
	require.EqualValues(t, 3, anchors["labels"])
	require.EqualValues(t, 3, anchors["ranges"], "the paragraph below the fold is never measured")
}

// Scroll and resize repaint the caret once per animation frame, and a pending
// frame is canceled with the interaction.
func TestVimPageRuntimeBatchesViewportUpdatesPerFrame(t *testing.T) {
	posted := runVimPageRuntime(t, `
startCaret("tok", "s");
const bar = cursor();
first.p.rect = rect(10, 100, 600, 20);
for (let i = 0; i < 10; i++) { fire("scroll"); fire("resize"); }
const pending = frames.size;
const stale = bar.style.top;
flushFrames();
posted.push({ n: "batched", pending, stale, top: bar.style.top });
fire("scroll");
const queued = frames.size;
vp().cancel("tok");
posted.push({ n: "canceled", queued, left: frames.size });
`)
	batched := named(t, posted, "batched")
	require.EqualValues(t, 1, batched["pending"], "a burst of events schedules one repaint")
	require.NotEqual(t, batched["stale"], batched["top"], "the repaint happens in the frame")
	canceled := named(t, posted, "canceled")
	require.EqualValues(t, 1, canceled["queued"])
	require.EqualValues(t, 0, canceled["left"], "cleanup cancels a pending frame")
}

// The bridge carries UTF-8: accents, CJK and astral characters must reach Go
// exactly as the page holds them.
func TestVimPageRuntimeCopiesNonASCIIText(t *testing.T) {
	const text = "héllo wörld — 日本語 😀 ñ"
	posted := runVimPageRuntime(t, `
first.p.innerText = `+jsString(text)+`;
window.__dumberVimPage.start("tok", { kind: "yank", object: "paragraph" });
`)
	require.Equal(t, []map[string]any{{"token": "tok", "type": "copy", "text": text}}, bridgeMessages(posted))
}
