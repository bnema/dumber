<script lang="ts">
  import { Chart } from 'frappe-charts';
  import { onMount } from 'svelte';
  import type { DailyVisitCount } from '../types';

  interface Props {
    dailyVisits: DailyVisitCount[];
  }

  let { dailyVisits }: Props = $props();

  let container: HTMLDivElement;
  let chart: Chart | null = null;

  // Convert data to Frappe Charts format
  const chartData = $derived.by(() => {
    if (!dailyVisits || dailyVisits.length === 0) {
      return { labels: [], datasets: [{ values: [] }] };
    }

    const labels = dailyVisits.map(d => {
      const date = new Date(d.day + 'T12:00:00');
      return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
    });
    const values = dailyVisits.map(d => d.visits);

    return {
      labels,
      datasets: [{ name: 'Visits', values }]
    };
  });

  const totalVisits = $derived(dailyVisits.reduce((sum, d) => sum + d.visits, 0));

  // Initialize chart on mount
  onMount(() => {
    if (chartData.labels.length === 0) return;

    chart = new Chart(container, {
      data: chartData,
      type: 'line',
      height: 160,
      colors: ['#4ade80'],
      axisOptions: { xAxisMode: 'tick', xIsSeries: true },
      lineOptions: { regionFill: 1, hideDots: 0, dotSize: 4 },
      tooltipOptions: { formatTooltipY: (d: number) => `${d} visits` }
    });

    return () => {
      chart?.destroy();
      chart = null;
    };
  });

  // Update chart when data changes (after initial mount)
  $effect(() => {
    // Skip if chart not initialized yet or no data
    if (!chart) return;

    // Read chartData to track changes
    const data = chartData;
    if (data.labels.length === 0) return;

    chart.update(data);
  });
</script>

<div class="chart-wrapper">
  <div class="chart-header">
    <span class="chart-label">Visits</span>
    <span class="chart-value">{totalVisits} total</span>
  </div>
  <div bind:this={container} class="chart-container"></div>
</div>

<style>
  .chart-wrapper {
    width: 100%;
    display: flex;
    flex-direction: column;
  }

  .chart-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0 0.25rem 0.5rem;
    font-size: 0.65rem;
    font-family: ui-monospace, 'Fira Code', 'Cascadia Code', Menlo, Monaco, Consolas, monospace;
  }

  .chart-label {
    color: #4ade80;
    font-weight: 600;
  }

  .chart-value {
    color: var(--muted-foreground, #737373);
  }

  .chart-container {
    width: 100%;
  }

  .chart-container :global(.frappe-chart) {
    font-family: ui-monospace, 'Fira Code', 'Cascadia Code', Menlo, Monaco, Consolas, monospace;
  }

  .chart-container :global(.frappe-chart .axis text),
  .chart-container :global(.frappe-chart .chart-label) {
    fill: #737373;
    font-size: 10px;
  }

  .chart-container :global(.frappe-chart .axis line),
  .chart-container :global(.frappe-chart .chart-strokes) {
    stroke: #333;
  }
</style>
