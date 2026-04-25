interface HAStatusBadgeProps {
  enabled: boolean;
  quorum: boolean;
  manager?: string;
  // nodeCount and resourceCount let us suppress a misleading "HA: OK" on a
  // single-node cluster (no failover possible) or when nothing is HA-managed.
  nodeCount?: number;
  resourceCount?: number;
  className?: string;
}

export function HAStatusBadge({
  enabled,
  quorum,
  manager,
  nodeCount,
  resourceCount,
  className = '',
}: HAStatusBadgeProps) {
  // Single-node cluster: HA is technically running but failover is impossible.
  if (nodeCount !== undefined && nodeCount < 2) {
    return (
      <span
        className={`px-1.5 py-0.5 text-xs rounded bg-gray-600/30 text-gray-400 ${className}`}
        title="Single-node cluster — HA failover requires ≥2 nodes"
      >
        HA: N/A
      </span>
    );
  }

  if (!enabled) {
    return (
      <span className={`px-1.5 py-0.5 text-xs rounded bg-gray-600/30 text-gray-400 ${className}`}>
        HA: Off
      </span>
    );
  }

  if (!quorum) {
    return (
      <span className={`px-1.5 py-0.5 text-xs rounded bg-red-600/30 text-red-400 ${className}`} title="No quorum - HA services degraded">
        HA: No Quorum
      </span>
    );
  }

  // Quorate but nothing is HA-managed → don't claim "OK".
  if (resourceCount !== undefined && resourceCount === 0) {
    return (
      <span
        className={`px-1.5 py-0.5 text-xs rounded bg-gray-600/30 text-gray-400 ${className}`}
        title="HA service running but no VMs/CTs are HA-managed"
      >
        HA: No Resources
      </span>
    );
  }

  return (
    <span
      className={`px-1.5 py-0.5 text-xs rounded bg-green-600/30 text-green-400 ${className}`}
      title={manager ? `HA Manager: ${manager}` : 'HA services running'}
    >
      HA: OK
    </span>
  );
}

// Compact badge for tree items
export function HAGuestBadge({ haState }: { haState?: string }) {
  if (!haState) return null;

  const getStateStyle = () => {
    switch (haState) {
      case 'started':
        return 'bg-green-500/20 text-green-400';
      case 'stopped':
        return 'bg-gray-500/20 text-gray-400';
      case 'fence':
        return 'bg-red-500/20 text-red-400';
      case 'freeze':
        return 'bg-blue-500/20 text-blue-400';
      case 'migrate':
        return 'bg-purple-500/20 text-purple-400';
      case 'relocate':
        return 'bg-purple-500/20 text-purple-400';
      case 'error':
        return 'bg-red-500/20 text-red-400';
      default:
        return 'bg-gray-500/20 text-gray-400';
    }
  };

  const getStateIcon = () => {
    switch (haState) {
      case 'started':
        return 'HA';
      case 'stopped':
        return 'HA';
      case 'fence':
        return 'FENCE';
      case 'freeze':
        return 'FREEZE';
      case 'migrate':
      case 'relocate':
        return 'HA';
      case 'error':
        return 'ERR';
      default:
        return 'HA';
    }
  };

  return (
    <span
      className={`px-1 py-0.5 text-xs rounded ${getStateStyle()}`}
      title={`HA State: ${haState}`}
    >
      {getStateIcon()}
    </span>
  );
}
