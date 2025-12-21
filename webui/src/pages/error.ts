import { mount } from "svelte";
import "../styles/app.css";
import ErrorPage from "./ErrorPage.svelte";

console.log("[dumber] Mounting ErrorPage component");
try {
  const app = mount(ErrorPage, { target: document.body });
  console.log("[dumber] ErrorPage mounted successfully", app);
} catch (error) {
  console.error("[dumber] Failed to mount ErrorPage:", error);
}
