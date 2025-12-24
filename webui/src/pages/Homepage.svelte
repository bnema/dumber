<script lang="ts">
  import { ModeWatcher } from 'mode-watcher';
  import { HomepageShell } from './homepage/shell';
  import { HistoryPanel, FavoritesPanel, AnalyticsPanel } from './homepage/panels';
  import { homepageState } from './homepage/state.svelte';
</script>

<ModeWatcher />
<HomepageShell>
  {#if homepageState.activePanel === 'history'}
    <HistoryPanel />
  {:else if homepageState.activePanel === 'favorites'}
    <FavoritesPanel />
  {:else if homepageState.activePanel === 'analytics'}
    <AnalyticsPanel />
  {/if}
</HomepageShell>
