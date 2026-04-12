import { useState, memo } from 'react';
import { api } from '../api/client';
import type { DRSRecommendation } from '../types';

interface DRSPanelProps {
  recommendations: DRSRecommendation[];
  onRefresh: () => void;
}

export const DRSPanel = memo(function DRSPanel({ recommendations, onRefresh }: DRSPanelProps) {
  const [loading, setLoading] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Handle null/undefined recommendations
  const recs = recommendations || [];

  const handleApply = async (rec: DRSRecommendation) => {
    setLoading(rec.id);
    setError(null);
    try {
      await api.applyDRSRecommendation(rec.cluster, rec.id);
      onRefresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to apply recommendation');
    } finally {
      setLoading(null);
    }
  };

  const handleDismiss = async (rec: DRSRecommendation) => {
    setLoading(rec.id);
    setError(null);
    try {
      await api.dismissDRSRecommendation(rec.cluster, rec.id);
      onRefresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to dismiss');
    } finally {
      setLoading(null);
    }
  };

  const getPriorityColor = (priority: number) => {
    switch (priority) {
      case 5: return 'bg-red-500';
      case 4: return 'bg-orange-500';
      case 3: return 'bg-yellow-500';
      case 2: return 'bg-blue-500';
      default: return 'bg-gray-500';
    }
  };

  const getPriorityLabel = (priority: number) => {
    switch (priority) {
      case 5: return 'Critical';
      case 4: return 'High';
      case 3: return 'Medium';
      case 2: return 'Low';
      default: return 'Info';
    }
  };

  if (recs.length === 0) {
    return (
      <div className="bg-gray-800 rounded-lg p-4">
        <h3 className="text-sm font-medium text-gray-300 mb-2 flex items-center gap-2">
          <span className="text-green-400">DRS</span>
          <span className="px-2 py-0.5 bg-green-600/20 text-green-400 text-xs rounded">Balanced</span>
        </h3>
        <p className="text-sm text-gray-500">
          No load balancing recommendations. Cluster resources are well-balanced.
        </p>
      </div>
    );
  }

  return (
    <div className="bg-gray-800 rounded-lg p-4">
      <h3 className="text-sm font-medium text-gray-300 mb-3 flex items-center gap-2">
        <span className="text-yellow-400">DRS Recommendations</span>
        <span className="px-2 py-0.5 bg-yellow-600/20 text-yellow-400 text-xs rounded">
          {recs.length} pending
        </span>
      </h3>

      {error && (
        <div className="mb-3 p-2 bg-red-900/30 text-red-400 text-sm rounded">
          {error}
        </div>
      )}

      <div className="space-y-2">
        {recs.map((rec) => (
          <div
            key={rec.id}
            className="bg-gray-700/50 rounded p-3 border-l-2"
            style={{ borderColor: `var(--${getPriorityColor(rec.priority).replace('bg-', '')})` }}
          >
            <div className="flex items-start justify-between gap-2">
              <div className="flex-1 min-w-0">
                {/* Priority badge and guest info */}
                <div className="flex items-center gap-2 mb-1">
                  <span className={`px-1.5 py-0.5 ${getPriorityColor(rec.priority)} text-white text-xs rounded`}>
                    {getPriorityLabel(rec.priority)}
                  </span>
                  <span className="text-gray-200 font-medium truncate">
                    {rec.guest_type === 'vm' ? 'VM' : 'CT'}: {rec.guest_name}
                  </span>
                  <span className="text-gray-500 text-sm">({rec.vmid})</span>
                </div>

                {/* Migration path */}
                <div className="text-sm text-gray-400 mb-1">
                  <span className="text-gray-500">{rec.from_node}</span>
                  <span className="mx-2">→</span>
                  <span className="text-gray-300">{rec.to_node}</span>
                </div>

                {/* Reason */}
                <div className="text-xs text-gray-500">
                  {rec.reason}
                </div>
              </div>

              {/* Actions */}
              <div className="flex items-center gap-1 flex-shrink-0">
                <button
                  onClick={() => handleApply(rec)}
                  disabled={loading === rec.id}
                  className="px-2 py-1 bg-blue-600 hover:bg-blue-700 text-white text-xs rounded disabled:opacity-50"
                  title="Apply recommendation (start migration)"
                >
                  {loading === rec.id ? '...' : 'Apply'}
                </button>
                <button
                  onClick={() => handleDismiss(rec)}
                  disabled={loading === rec.id}
                  className="px-2 py-1 bg-gray-600 hover:bg-gray-500 text-gray-300 text-xs rounded disabled:opacity-50"
                  title="Dismiss recommendation"
                >
                  Dismiss
                </button>
              </div>
            </div>
          </div>
        ))}
      </div>

      {/* Legend */}
      <div className="mt-3 pt-3 border-t border-gray-700">
        <div className="flex items-center gap-3 text-xs text-gray-500">
          <span>Priority:</span>
          <span className="flex items-center gap-1">
            <span className="w-2 h-2 rounded bg-red-500"></span> Critical
          </span>
          <span className="flex items-center gap-1">
            <span className="w-2 h-2 rounded bg-orange-500"></span> High
          </span>
          <span className="flex items-center gap-1">
            <span className="w-2 h-2 rounded bg-yellow-500"></span> Medium
          </span>
          <span className="flex items-center gap-1">
            <span className="w-2 h-2 rounded bg-blue-500"></span> Low
          </span>
        </div>
      </div>
    </div>
  );
});
