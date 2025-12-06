declare module 'frappe-charts' {
  export interface ChartData {
    labels: string[];
    datasets: Array<{
      name?: string;
      values: number[];
      chartType?: string;
    }>;
  }

  export interface ChartOptions {
    data: ChartData;
    type: 'line' | 'bar' | 'axis-mixed' | 'pie' | 'percentage' | 'heatmap';
    height?: number;
    colors?: string[];
    axisOptions?: {
      xAxisMode?: 'tick' | 'span';
      xIsSeries?: boolean;
      yAxisMode?: 'tick' | 'span';
    };
    barOptions?: {
      spaceRatio?: number;
      stacked?: boolean;
    };
    lineOptions?: {
      regionFill?: number;
      hideDots?: number;
      hideLine?: number;
      heatline?: number;
      dotSize?: number;
    };
    tooltipOptions?: {
      formatTooltipX?: (d: string) => string;
      formatTooltipY?: (d: number) => string;
    };
    isNavigable?: boolean;
    valuesOverPoints?: boolean;
    maxSlices?: number;
  }

  export class Chart {
    constructor(container: HTMLElement, options: ChartOptions);
    update(data: ChartData): void;
    destroy(): void;
    export(): void;
  }
}
