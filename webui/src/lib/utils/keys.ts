/**
 * Format a keybinding string for display.
 * Converts internal key names to user-friendly display format.
 *
 * Examples:
 *   "ctrl+shift+a" -> "Ctrl + Shift + A"
 *   "arrowleft" -> "Left"
 *   "alt+[" -> "Alt + ["
 */
export function formatKey(key: string): string {
  // Split by '+' separator first, then format each part
  // This avoids replacing '+' characters that are actual key names
  const parts = key.split("+");

  const formattedParts = parts.map((part) => {
    const lower = part.toLowerCase();
    switch (lower) {
      case "arrowleft":
        return "Left";
      case "arrowright":
        return "Right";
      case "arrowup":
        return "Up";
      case "arrowdown":
        return "Down";
      case "ctrl":
        return "Ctrl";
      case "alt":
        return "Alt";
      case "shift":
        return "Shift";
      case "space":
        return "Space";
      case "enter":
        return "Enter";
      case "tab":
        return "Tab";
      case "backspace":
        return "Backspace";
      case "escape":
        return "Esc";
      default:
        // Capitalize single letters, leave symbols as-is
        return part.length === 1 && /[a-z]/i.test(part)
          ? part.toUpperCase()
          : part;
    }
  });

  return formattedParts.join(" + ");
}
