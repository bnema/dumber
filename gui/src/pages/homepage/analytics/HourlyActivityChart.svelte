<script lang="ts">
  import uPlot from 'uplot';
  import 'uplot/dist/uPlot.min.css';
  import type { HourlyDistribution } from '../types';

  interface Props {
    hourlyDistribution: HourlyDistribution[];
  }

  let { hourlyDistribution }: Props = $props();

  let container: HTMLDivElement | undefined = $state();
  let chart: uPlot | null = null;

  // Convert data to uPlot format (ensure all 24 hours)
  const chartData = $derived.by((): uPlot.AlignedData => {
    const visitsByHour = new Map<number, number>();
    hourlyDistribution.forEach(h => visitsByHour.set(h.hour, h.visit_count));

    const hours = Array.from({ length: 24 }, (_, i) => i);
    const visits = hours.map(h => visitsByHour.get(h) ?? 0);
    return [hours, visits];
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

  // Create chart
  $effect(() => {
    if (!container) return;

    const width = container.clientWidth;
    if (width <= 0) return;

    if (chart) {
      chart.destroy();
      chart = null;
    }

    const opts: uPlot.Options = {
      width,
      height: 140,
      padding: [10, 10, 0, 0],
      cursor: {
        show: true,
        points: { show: false },
      },
      legend: { show: false },
      scales: {
        x: {
          time: false,
          range: [-0.5, 23.5],
        },
        y: {
          auto: true,
          range: (u, min, max) => [0, Math.max(max * 1.15, 5)],
        },
      },
      axes: [
        {
          stroke: '#737373',
          grid: { show: false },
          ticks: { show: true, stroke: '#444', width: 1, size: 4 },
          font: '10px monospace',
          size: 24,
          values: (u, vals) => vals.map(v => {
            const h = Math.round(v);
            if (h % 6 === 0) return `${String(h).padStart(2, '0')}h`;
            return '';
          }),
        },
        {
          stroke: '#737373',
          grid: { show: true, stroke: '#333', width: 1 },
          ticks: { show: true, stroke: '#444', width: 1, size: 4 },
          font: '10px monospace',
          size: 32,
        },
      ],
      series: [
        {},
        {
          label: 'Activity',
          stroke: '#4ade80',
          fill: 'rgba(74, 222, 128, 0.5)',
          width: 0,
          paths: uPlot.paths.bars?.({ size: [0.6, Infinity], gap: 2 }),
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
        chart.setSize({ width, height: 140 });
      }
    });

    resizeObserver.observe(container);
    return () => resizeObserver.disconnect();
  });
</script>

<div class="chart-wrapper">
  <div class="chart-header">
    <span class="chart-label">Activity</span>
    <span class="chart-value">peak at {peakHour}</span>
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
    height: 140px;
  }

  .uplot-target :global(.u-wrap) {
    font-family: 'JetBrains Mono NF', monospace;
  }
</style>
