import { mount } from 'svelte';
import Homepage from './Homepage.svelte';

// Proactively unregister any Service Workers and clear caches that may have been
// installed by previous builds (e.g., SvelteKit), which can otherwise attempt to
// request non-existent _app/immutable/* assets under dumb://homepage.
async function cleanupServiceWorkersAndCaches() {
  try {
    if ('serviceWorker' in navigator) {
      const regs = await navigator.serviceWorker.getRegistrations();
      await Promise.all(regs.map((r) => r.unregister().catch(() => {})));
    }
  } catch { /* ignore */ }

  try {
    if ('caches' in window) {
      const keys = await caches.keys();
      await Promise.all(keys.map((k) => caches.delete(k).catch(() => false)));
    }
  } catch { /* ignore */ }
}


// Mount the Homepage component to the DOM
cleanupServiceWorkersAndCaches().finally(() => {
  mount(Homepage, { target: document.body });
});
