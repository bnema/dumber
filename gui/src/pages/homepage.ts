import { mount } from "svelte";
import "../styles/tailwind.css";
import Homepage from "./Homepage.svelte";
import { bootstrapGUI } from "../injected/bootstrap";

// Proactively unregister any Service Workers and clear caches that may have been
// installed by previous builds (e.g., SvelteKit), which can otherwise attempt to
// request non-existent _app/immutable/* assets under dumb://homepage.
async function cleanupServiceWorkersAndCaches() {
  try {
    if ("serviceWorker" in navigator) {
      const regs = await navigator.serviceWorker.getRegistrations();
      await Promise.all(regs.map((r) => r.unregister().catch(() => {})));
    }
  } catch {
    /* ignore */
  }

  try {
    if ("caches" in window) {
      const keys = await caches.keys();
      await Promise.all(keys.map((k) => caches.delete(k).catch(() => false)));
    }
  } catch {
    /* ignore */
  }
}

bootstrapGUI();

// Mount the Homepage component to the DOM
cleanupServiceWorkersAndCaches().finally(() => {
  console.log("[dumber] Mounting Homepage component to document.body");
  try {
    const app = mount(Homepage, { target: document.body });
    console.log("[dumber] Homepage component mounted successfully", app);
  } catch (error) {
    console.error("[dumber] Failed to mount Homepage component:", error);
  }
});
