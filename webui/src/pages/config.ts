import { mount } from "svelte";
import ConfigPage from "./ConfigPage.svelte";
import "../styles/app.css";

// Mount the config page
const app = mount(ConfigPage, {
  target: document.body,
});

export default app;
