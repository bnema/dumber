import { mount } from "svelte";
import "../styles/tailwind.css";
import BlockedPage from "./BlockedPage.svelte";
import { bootstrapGUI } from "../injected/bootstrap";

bootstrapGUI();

// Mount the BlockedPage component to the DOM
console.log("[dumber] Mounting BlockedPage component to document.body");
try {
  const app = mount(BlockedPage, { target: document.body });
  console.log("[dumber] BlockedPage component mounted successfully", app);
} catch (error) {
  console.error("[dumber] Failed to mount BlockedPage component:", error);
}
