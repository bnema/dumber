<script lang="ts">
  import uPlot from 'uplot';
  import 'uplot/dist/uPlot.min.css';
  import type { DailyVisitCount } from '../types';

  interface Props {
    dailyVisits: DailyVisitCount[];
  }

  let { dailyVisits }: Props = $props();

  let container: HTMLDivElement | undefined = $state();
  let chart: uPlot | null = null;

  // Convert data to uPlot format
  const chartData = $derived.by((): uPlot.AlignedData => {
    if (!dailyVisits || dailyVisits.length === 0) {
      return [[], []];
    }
    const timestamps = dailyVisits.map(d => {
      const date = new Date(d.day + 'T12:00:00');
      return Math.floor(date.getTime() / 1000);
    });
    const visits = dailyVisits.map(d => d.visits);
    return [timestamps, visits];
  });

  // Create chart
  $effect(() => {
    if (!container || chartData[0].length === 0) return;

    const width = container.clientWidth;
    if (width <= 0) return;

    if (chart) {
      chart.destroy();
      chart = null;
    }

    const opts: uPlot.Options = {
      width,
      height: 180,
      padding: [10, 10, 0, 0],
      cursor: {
        show: true,
        points: { show: true, size: 6 },
      },
      legend: { show: false },
      scales: {
        x: { time: true },
        y: {
          auto: true,
          range: (u, min, max) => [0, Math.max(max * 1.1, 10)],
        },
      },
      axes: [
        {
          stroke: '#737373',
          grid: { show: true, stroke: '#333', width: 1 },
          ticks: { show: true, stroke: '#444', width: 1, size: 4 },
          font: '10px monospace',
          size: 24,
        },
        {
          stroke: '#737373',
          grid: { show: true, stroke: '#333', width: 1 },
          ticks: { show: true, stroke: '#444', width: 1, size: 4 },
          font: '10px monospace',
          size: 40,
        },
      ],
      series: [
        {},
        {
          label: 'Visits',
          stroke: '#4ade80',
          width: 2,
          fill: 'rgba(74, 222, 128, 0.15)',
          paths: uPlot.paths.spline?.(),
        },
      ],
    };

    chart = new uPlot(opts, chartData, container);

    return () => {
      chart?.destroy();
      chart = null;
    };
  });

  // Handle resize
  $effect(() => {
    if (!container) return;

    const resizeObserver = new ResizeObserver(entries => {
      const width = entries[0]?.contentRect.width;
      if (width && width > 0 && chart) {
        chart.setSize({ width, height: 180 });
      }
    });

    resizeObserver.observe(container);
    return () => resizeObserver.disconnect();
  });
</script>

<div class="chart-wrapper">
  <div class="chart-header">
    <span class="chart-label">Visits</span>
    <span class="chart-value">{dailyVisits.reduce((sum, d) => sum + d.visits, 0)} total</span>
  </div>
  <div bind:this={container} class="uplot-target"></div>
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
    font-family: 'JetBrains Mono NF', monospace;
  }

  .chart-label {
    color: #4ade80;
    font-weight: 600;
  }

  .chart-value {
    color: var(--dynamic-muted, #737373);
  }

  .uplot-target {
    width: 100%;
    height: 180px;
  }

  .uplot-target :global(.u-wrap) {
    font-family: 'JetBrains Mono NF', monospace;
  }
</style>
