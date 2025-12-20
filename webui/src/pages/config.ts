import { mount } from "svelte";
import ConfigPage from "./ConfigPage.svelte";
import "../lib/styles.css";

// Mount the config page
const app = mount(ConfigPage, {
  target: document.body,
});

export default app;
