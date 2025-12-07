<script lang="ts">
  import { Chart } from 'frappe-charts';
  import { onMount } from 'svelte';
  import type { HourlyDistribution } from '../types';

  interface Props {
    hourlyDistribution: HourlyDistribution[];
  }

  let { hourlyDistribution }: Props = $props();

  let container: HTMLDivElement;
  let chart: Chart | null = null;

  // Convert data to Frappe Charts format (ensure all 24 hours)
  const chartData = $derived.by(() => {
    const visitsByHour = new Map<number, number>();
    hourlyDistribution.forEach(h => visitsByHour.set(h.hour, h.visit_count));

    const labels = Array.from({ length: 24 }, (_, i) =>
      i % 6 === 0 ? `${String(i).padStart(2, '0')}h` : ''
    );
    const values = Array.from({ length: 24 }, (_, i) => visitsByHour.get(i) ?? 0);

    return {
      labels,
      datasets: [{ name: 'Activity', values }]
    };
  });

  // Peak hour for display
  const peakHour = $derived.by(() => {
    let maxVisits = 0;
    let peak = 0;
    hourlyDistribution.forEach(h => {
      if (h.visit_count > maxVisits) {
        maxVisits = h.visit_count;
        peak = h.hour;
      }
    });
    return `${String(peak).padStart(2, '0')}:00`;
  });

  // Initialize chart on mount
  onMount(() => {
    chart = new Chart(container, {
      data: chartData,
      type: 'bar',
      height: 120,
      colors: ['#4ade80'],
      axisOptions: { xAxisMode: 'tick' },
      barOptions: { spaceRatio: 0.3 },
      tooltipOptions: { formatTooltipY: (d: number) => `${d} visits` }
    });

    return () => {
      chart?.destroy();
      chart = null;
    };
  });

  // Update chart when data changes (after initial mount)
  $effect(() => {
    if (!chart) return;

    const data = chartData;
    if (data.labels.length === 0) return;

    chart.update(data);
  });
</script>

<div class="chart-wrapper">
  <div class="chart-header">
    <span class="chart-label">Activity</span>
    <span class="chart-value">peak at {peakHour}</span>
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
    color: var(--dynamic-muted, #737373);
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
