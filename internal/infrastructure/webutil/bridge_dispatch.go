package webutil

// inlineBridgeMessages are message types whose handlers run inline on the
// engine's dispatch thread instead of a goroutine. They are ordering
// sensitive (key press before release, copy events) and read toolkit state
// (focused input, clipboard). Every other handler may block on I/O and runs
// off-thread on both engines.
var inlineBridgeMessages = map[string]bool{
	"accent_key_press":    true,
	"accent_key_release":  true,
	"auto_copy_selection": true,
	"explicit_text_copy":  true,
}

// IsInlineBridgeMessage reports whether msgType must be handled inline.
func IsInlineBridgeMessage(msgType string) bool {
	return inlineBridgeMessages[msgType]
}

// InlineBridgeMessageTypes returns the inline message types (for tests).
func InlineBridgeMessageTypes() []string {
	out := make([]string, 0, len(inlineBridgeMessages))
	for t := range inlineBridgeMessages {
		out = append(out, t)
	}
	return out
}
