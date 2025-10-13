/**
 * Unified GUI bundle entry point.
 *
 * Delegates to the shared bootstrap so special pages can reuse the same setup
 * without depending on this injected build.
 */

import { bootstrapGUI } from "./bootstrap";

const href = window.location.href;
const protocol = window.location.protocol;
const host = window.location.host || "";

const isDumbProtocol = protocol === "dumb:" || href.startsWith("dumb://");
const dumbHost = host.toLowerCase();
const shouldSkip =
  isDumbProtocol &&
  (href.toLowerCase().startsWith("dumb://home") ||
    href.toLowerCase().startsWith("dumb://homepage") ||
    dumbHost === "home" ||
    dumbHost === "homepage");

if (!shouldSkip) {
  bootstrapGUI();
}
