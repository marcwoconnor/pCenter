import { useState, useEffect, useRef } from 'react';
import type { MetricsResponse } from '../types';

type TimeRange = '1h' | '6h' | '24h' | '7d' | '30d';

interface UseMetricsOptions {
  resourceType?: 'node' | 'vm' | 'ct' | 'storage' | 'ceph';
  resourceId?: string;
  cluster?: string;
  metrics?: string[];
  timeRange: TimeRange;
  refreshInterval?: number;
  enabled?: boolean;
}

function getStartTimestamp(range: TimeRange): number {
  const now = Math.floor(Date.now() / 1000);
  switch (range) {
    case '1h': return now - 3600;
    case '6h': return now - 6 * 3600;
    case '24h': return now - 24 * 3600;
    case '7d': return now - 7 * 24 * 3600;
    case '30d': return now - 30 * 24 * 3600;
    default: return now - 3600;
  }
}

export function useMetrics(options: UseMetricsOptions) {
  const {
    resourceType,
    resourceId,
    cluster,
    metrics = ['cpu', 'mem_percent'],
    timeRange,
    refreshInterval = 30000,
    enabled = true,
  } = options;

  const [data, setData] = useState<MetricsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Use refs to store current values without triggering re-renders
  const optionsRef = useRef({ cluster, resourceType, resourceId, metrics, timeRange, enabled });
  const hasLoadedOnce = useRef(false);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // Update ref when options change (for use in interval callback)
  optionsRef.current = { cluster, resourceType, resourceId, metrics, timeRange, enabled };

  useEffect(() => {
    let cancelled = false;

    const fetchMetrics = async () => {
      const opts = optionsRef.current;
      if (!opts.enabled) return;

      try {
        const params = new URLSearchParams({
          start: getStartTimestamp(opts.timeRange).toString(),
          end: Math.floor(Date.now() / 1000).toString(),
          metrics: opts.metrics.join(','),
          resolution: 'auto',
        });

        if (opts.cluster) params.set('cluster', opts.cluster);
        if (opts.resourceType) params.set('resource_type', opts.resourceType);
        if (opts.resourceId) params.set('resource_id', opts.resourceId);

        let endpoint = '/api/metrics';
        if (opts.cluster) {
          endpoint = `/api/clusters/${opts.cluster}/metrics`;
        } else if (opts.resourceType && opts.resourceId) {
          endpoint = `/api/metrics/${opts.resourceType}/${opts.resourceId}`;
        }

        const res = await fetch(`${endpoint}?${params}`);
        if (cancelled) return;

        if (!res.ok) {
          const errData = await res.json().catch(() => ({}));
          throw new Error(errData.error || `HTTP ${res.status}`);
        }

        const json: MetricsResponse = await res.json();
        if (cancelled) return;

        setData(json);
        setError(null);
        hasLoadedOnce.current = true;
      } catch (e) {
        if (cancelled) return;
        if (!hasLoadedOnce.current) {
          setError(e instanceof Error ? e.message : 'Failed to fetch metrics');
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };

    // Initial fetch
    fetchMetrics();

    // Set up interval for refresh
    if (refreshInterval && enabled) {
      intervalRef.current = setInterval(fetchMetrics, refreshInterval);
    }

    return () => {
      cancelled = true;
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
      }
    };
  // Only re-run effect when these key values change
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cluster, resourceType, resourceId, timeRange, enabled, refreshInterval, metrics.join(',')]);

  return { data, loading, error, refetch: () => {} };
}

export function useNodeMetrics(node: string, timeRange: TimeRange) {
  return useMetrics({
    resourceType: 'node',
    resourceId: node,
    metrics: ['cpu', 'mem_percent', 'loadavg_1m'],
    timeRange,
  });
}

export function useClusterMetrics(cluster: string, timeRange: TimeRange) {
  return useMetrics({
    cluster,
    resourceType: 'node',
    metrics: ['cpu', 'mem_percent'],
    timeRange,
  });
}
