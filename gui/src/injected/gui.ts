/**
 * Unified GUI bundle entry point.
 *
 * Delegates to the shared bootstrap so special pages can reuse the same setup
 * without depending on this injected build.
 *
 * Always calls bootstrapGUI() - deduplication is handled inside bootstrap.ts
 * via window.__dumber_gui_ready_for check. This ensures GUI loads in all
 * contexts including new split panes and about:blank pages.
 */

import { bootstrapGUI } from "./bootstrap";

bootstrapGUI();
