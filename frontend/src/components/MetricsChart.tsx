import { useMemo, memo } from 'react';
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  Legend,
  CartesianGrid,
} from 'recharts';
import type { MetricSeries } from '../types';

type TimeRange = '1h' | '6h' | '24h' | '7d' | '30d';

interface MetricsChartProps {
  series: MetricSeries[];
  height?: number;
  showLegend?: boolean;
  timeRange: TimeRange;
  title?: string;
}

const COLORS = [
  '#3B82F6', // blue
  '#10B981', // green
  '#F59E0B', // amber
  '#EF4444', // red
  '#8B5CF6', // purple
  '#EC4899', // pink
  '#06B6D4', // cyan
  '#84CC16', // lime
];

const METRIC_LABELS: Record<string, string> = {
  cpu: 'CPU',
  mem_percent: 'Memory',
  mem: 'Memory (bytes)',
  disk_percent: 'Disk',
  loadavg_1m: 'Load (1m)',
  loadavg_5m: 'Load (5m)',
  loadavg_15m: 'Load (15m)',
  netin: 'Net In',
  netout: 'Net Out',
  diskread: 'Disk Read',
  diskwrite: 'Disk Write',
  pgpgin: 'Page In',
  pgpgout: 'Page Out',
  swap_percent: 'Swap',
  ceph_health: 'Ceph Health',
};

function formatXAxis(ts: number, timeRange: TimeRange): string {
  const date = new Date(ts * 1000);
  if (timeRange === '1h' || timeRange === '6h') {
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }
  if (timeRange === '24h') {
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }
  return date.toLocaleDateString([], { month: 'short', day: 'numeric' });
}

function formatValue(value: number, unit: string): string {
  if (unit === 'percent') return `${value.toFixed(1)}%`;
  if (unit === 'bytes') return formatBytes(value);
  if (unit === 'bytes_per_sec') return `${formatBytes(value)}/s`;
  if (unit === 'pages_per_sec') return `${formatPages(value)}/s`;
  if (unit === 'ratio') return value.toFixed(2);
  return value.toFixed(1);
}

function formatPages(pages: number): string {
  // Convert pages to KB (page size = 4KB typically)
  const kb = pages * 4;
  if (kb < 1024) return `${kb.toFixed(0)} KB`;
  const mb = kb / 1024;
  if (mb < 1024) return `${mb.toFixed(1)} MB`;
  const gb = mb / 1024;
  return `${gb.toFixed(2)} GB`;
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
}

interface ChartDataPoint {
  ts: number;
  [key: string]: number;
}

export const MetricsChart = memo(function MetricsChart({
  series,
  height = 200,
  showLegend = true,
  timeRange,
  title,
}: MetricsChartProps) {
  // Merge all series data by timestamp
  const chartData = useMemo(() => {
    if (!series || series.length === 0) return [];

    const dataMap = new Map<number, ChartDataPoint>();

    series.forEach((s) => {
      s.data.forEach((point) => {
        const existing = dataMap.get(point.ts) || { ts: point.ts };
        const key = `${s.resource_id}_${s.metric}`;
        existing[key] = point.value;
        dataMap.set(point.ts, existing);
      });
    });

    return Array.from(dataMap.values()).sort((a, b) => a.ts - b.ts);
  }, [series]);

  // Build line configs
  const lines = useMemo(() => {
    if (!series) return [];

    return series.map((s, i) => {
      const key = `${s.resource_id}_${s.metric}`;
      const label = series.length > 1
        ? `${s.resource_id} ${METRIC_LABELS[s.metric] || s.metric}`
        : METRIC_LABELS[s.metric] || s.metric;

      return {
        key,
        label,
        color: COLORS[i % COLORS.length],
        unit: s.unit,
      };
    });
  }, [series]);

  if (chartData.length === 0) {
    return (
      <div className="flex items-center justify-center h-48 text-gray-500 dark:text-gray-400">
        No data available
      </div>
    );
  }

  return (
    <div style={{ minHeight: height + 24 }}>
      {title && (
        <h4 className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
          {title}
        </h4>
      )}
      <ResponsiveContainer width="100%" height={height} debounce={100}>
        <LineChart data={chartData} margin={{ top: 5, right: 5, bottom: 5, left: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="#374151" opacity={0.3} />
          <XAxis
            dataKey="ts"
            tickFormatter={(ts) => formatXAxis(ts, timeRange)}
            stroke="#9CA3AF"
            tick={{ fontSize: 11 }}
            interval="preserveStartEnd"
          />
          <YAxis
            stroke="#9CA3AF"
            tick={{ fontSize: 11 }}
            width={45}
            tickFormatter={(v) => {
              if (lines.length > 0 && lines[0].unit === 'percent') {
                return `${v}%`;
              }
              return v;
            }}
          />
          <Tooltip
            contentStyle={{
              backgroundColor: '#1F2937',
              border: 'none',
              borderRadius: '0.375rem',
              color: '#F3F4F6',
            }}
            labelFormatter={(ts) => new Date((ts as number) * 1000).toLocaleString()}
            formatter={(value, name) => {
              const line = lines.find((l) => l.key === name);
              return [formatValue(value as number, line?.unit || 'percent'), line?.label || name];
            }}
          />
          {showLegend && lines.length > 1 && (
            <Legend
              wrapperStyle={{ fontSize: 11 }}
              formatter={(value) => {
                const line = lines.find((l) => l.key === value);
                return line?.label || value;
              }}
            />
          )}
          {lines.map((line) => (
            <Line
              key={line.key}
              type="monotone"
              dataKey={line.key}
              name={line.key}
              stroke={line.color}
              dot={false}
              strokeWidth={2}
              isAnimationActive={false}
            />
          ))}
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
});

// Compact chart for inline display
export function MiniMetricsChart({
  series,
  height = 60,
}: {
  series: MetricSeries[];
  height?: number;
}) {
  const chartData = useMemo(() => {
    if (!series || series.length === 0) return [];

    const dataMap = new Map<number, ChartDataPoint>();

    series.forEach((s) => {
      s.data.forEach((point) => {
        const existing = dataMap.get(point.ts) || { ts: point.ts };
        existing[s.metric] = point.value;
        dataMap.set(point.ts, existing);
      });
    });

    return Array.from(dataMap.values()).sort((a, b) => a.ts - b.ts);
  }, [series]);

  if (chartData.length === 0) return null;

  return (
    <ResponsiveContainer width="100%" height={height}>
      <LineChart data={chartData} margin={{ top: 0, right: 0, bottom: 0, left: 0 }}>
        {series.map((s, i) => (
          <Line
            key={s.metric}
            type="monotone"
            dataKey={s.metric}
            stroke={COLORS[i % COLORS.length]}
            dot={false}
            strokeWidth={1.5}
            isAnimationActive={false}
          />
        ))}
      </LineChart>
    </ResponsiveContainer>
  );
}
