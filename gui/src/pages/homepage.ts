import { mount } from 'svelte';
import Homepage from './Homepage.svelte';

// Mount the Homepage component to the DOM
const app = mount(Homepage, {
  target: document.body,
});

export default app;