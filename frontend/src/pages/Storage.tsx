import { useState, useEffect } from 'react';
import { useSearchParams } from 'react-router-dom';
import { Layout } from '../components/Layout';
import { InventoryTree } from '../components/InventoryTree';
import { ObjectDetail } from '../components/ObjectDetail';
import { useCluster } from '../context/ClusterContext';
import { formatBytes } from '../api/client';
import type { SmartDisk } from '../types';

// Known Ceph health check fixes with API command mappings
const CEPH_FIXES: Record<string, {
  description: string;
  command: string;
  apiCommand?: string;  // Command name for API
  needsPgId?: boolean;  // Whether it needs a PG ID from the detail
}> = {
  OSD_SCRUB_ERRORS: {
    description: 'Scrub errors indicate data inconsistencies between replicas. Run repair on the affected PG.',
    command: 'ceph pg repair <pg_id>',
    apiCommand: 'pg_repair',
    needsPgId: true,
  },
  PG_DAMAGED: {
    description: 'Placement group has inconsistent data. Repair will choose the authoritative copy.',
    command: 'ceph pg repair <pg_id>',
    apiCommand: 'pg_repair',
    needsPgId: true,
  },
  PG_DEGRADED: {
    description: 'Not enough replicas available. Check OSD status and ensure all OSDs are up.',
    command: 'ceph osd tree',
    apiCommand: 'osd_tree',
  },
  OSD_DOWN: {
    description: 'One or more OSDs are down. Check the OSD service status.',
    command: 'systemctl status ceph-osd@<id>',
    // No API command - manual intervention required
  },
  MON_DOWN: {
    description: 'Monitor(s) are down. Check monitor service status.',
    command: 'systemctl status ceph-mon@<hostname>',
    // No API command - manual intervention required
  },
  POOL_NO_REDUNDANCY: {
    description: 'Pool has no redundancy configured. Consider increasing replica count.',
    command: 'ceph osd pool set <pool> size <n>',
    // No API command - requires pool name input
  },
};

// Extract PG ID from detail message like "pg 4.3b is active+clean+inconsistent"
function extractPgId(detail: string): string | null {
  const match = detail.match(/pg\s+(\d+\.[0-9a-fA-F]+)/);
  return match ? match[1] : null;
}

// Check if repair is already in progress from the PG state
function isRepairInProgress(detail: string): boolean {
  // State format: "pg 4.3b is active+clean+scrubbing+deep+inconsistent+repair, acting [4,7]"
  return detail.includes('+repair');
}

interface CommandLog {
  timestamp: Date;
  command: string;
  success: boolean;
  output: string;
}

function CephDetailPanel() {
  const { ceph } = useCluster();
  const [commandLogs, setCommandLogs] = useState<CommandLog[]>([]);
  const [runningCommand, setRunningCommand] = useState<string | null>(null);

  const runCommand = async (apiCommand: string, pgId?: string) => {
    const cmdKey = `${apiCommand}-${pgId || 'none'}`;
    setRunningCommand(cmdKey);

    try {
      const response = await fetch('/api/ceph/command', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ command: apiCommand, pg_id: pgId }),
      });

      const result = await response.json();

      setCommandLogs(prev => [{
        timestamp: new Date(),
        command: pgId ? `ceph pg repair ${pgId}` : `ceph ${apiCommand.replace('_', ' ')}`,
        success: result.success,
        output: result.output || result.error || 'No output',
      }, ...prev]);
    } catch (err) {
      setCommandLogs(prev => [{
        timestamp: new Date(),
        command: pgId ? `ceph pg repair ${pgId}` : `ceph ${apiCommand.replace('_', ' ')}`,
        success: false,
        output: `Request failed: ${err}`,
      }, ...prev]);
    } finally {
      setRunningCommand(null);
    }
  };

  if (!ceph) {
    return (
      <div className="p-6 text-gray-500">
        Ceph not available
      </div>
    );
  }

  const hasIssues = ceph.health !== 'HEALTH_OK';

  return (
    <div className="p-6">
      <h2 className="text-xl font-bold text-gray-900 dark:text-white mb-4">Ceph Cluster Health</h2>

      {/* Status Overview */}
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4 mb-6">
        <div className="flex items-center justify-between">
          <div>
            <div className={`text-3xl font-bold ${
              ceph.health === 'HEALTH_OK' ? 'text-green-500' :
              ceph.health === 'HEALTH_WARN' ? 'text-yellow-500' : 'text-red-500'
            }`}>
              {ceph.health}
            </div>
            <div className="text-sm text-gray-500 mt-1">
              {formatBytes(ceph.bytes_used)} used of {formatBytes(ceph.bytes_total)} ({((ceph.bytes_used / ceph.bytes_total) * 100).toFixed(1)}%)
            </div>
          </div>
        </div>
      </div>

      {/* Health Checks */}
      {hasIssues && ceph.checks && (
        <div className="space-y-4">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white">Health Issues</h3>
          {Object.entries(ceph.checks).map(([name, check]) => {
            const fix = CEPH_FIXES[name];
            const pgId = check.detail ? extractPgId(check.detail) : null;
            const repairAlreadyRunning = check.detail ? isRepairInProgress(check.detail) : false;
            const canRun = fix?.apiCommand && (!fix.needsPgId || pgId) && !repairAlreadyRunning;
            const cmdKey = `${fix?.apiCommand}-${pgId || 'none'}`;
            const isRunning = runningCommand === cmdKey;

            return (
              <div key={name} className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
                <div className="flex items-start gap-3">
                  <div className={`mt-1 w-3 h-3 rounded-full flex-shrink-0 ${
                    check.severity === 'HEALTH_ERR' ? 'bg-red-500' : 'bg-yellow-500'
                  }`} />
                  <div className="flex-1">
                    <div className="font-medium text-gray-900 dark:text-white">
                      {name.replace(/_/g, ' ')}
                    </div>
                    <div className={`text-sm ${
                      check.severity === 'HEALTH_ERR' ? 'text-red-500' : 'text-yellow-500'
                    }`}>
                      {check.summary}
                    </div>
                    {check.detail && (
                      <div className="text-sm text-gray-500 dark:text-gray-400 mt-1 font-mono bg-gray-100 dark:bg-gray-900 p-2 rounded max-h-60 overflow-y-auto whitespace-pre-wrap">
                        {check.detail}
                      </div>
                    )}
                    {fix && (
                      <div className="mt-3 p-3 bg-blue-50 dark:bg-blue-900/20 rounded border border-blue-200 dark:border-blue-800">
                        <div className="text-sm text-gray-700 dark:text-gray-300">
                          {fix.description}
                        </div>
                        <div className="mt-2 flex items-center gap-2">
                          <div className="flex-1 font-mono text-sm bg-gray-900 text-green-400 p-2 rounded">
                            $ {fix.needsPgId && pgId ? fix.command.replace('<pg_id>', pgId) : fix.command}
                          </div>
                          {repairAlreadyRunning ? (
                            <>
                              <button
                                disabled
                                className="px-3 py-2 text-sm font-medium rounded bg-yellow-600 text-white cursor-not-allowed animate-pulse"
                              >
                                Repairing...
                              </button>
                              {fix.needsPgId && pgId && (
                                <button
                                  onClick={() => runCommand('pg_query', pgId)}
                                  disabled={runningCommand === `pg_query-${pgId}`}
                                  className={`px-3 py-2 text-sm font-medium rounded ${
                                    runningCommand === `pg_query-${pgId}`
                                      ? 'bg-gray-400 text-gray-200 cursor-not-allowed'
                                      : 'bg-blue-600 hover:bg-blue-700 text-white'
                                  }`}
                                >
                                  {runningCommand === `pg_query-${pgId}` ? 'Checking...' : 'Status'}
                                </button>
                              )}
                            </>
                          ) : canRun && (
                            <>
                              <button
                                onClick={() => runCommand(fix.apiCommand!, pgId || undefined)}
                                disabled={isRunning}
                                className={`px-3 py-2 text-sm font-medium rounded ${
                                  isRunning
                                    ? 'bg-gray-400 text-gray-200 cursor-not-allowed'
                                    : 'bg-green-600 hover:bg-green-700 text-white'
                                }`}
                              >
                                {isRunning ? 'Running...' : 'Run'}
                              </button>
                              {fix.needsPgId && pgId && (
                                <button
                                  onClick={() => runCommand('pg_query', pgId)}
                                  disabled={runningCommand === `pg_query-${pgId}`}
                                  className={`px-3 py-2 text-sm font-medium rounded ${
                                    runningCommand === `pg_query-${pgId}`
                                      ? 'bg-gray-400 text-gray-200 cursor-not-allowed'
                                      : 'bg-blue-600 hover:bg-blue-700 text-white'
                                  }`}
                                >
                                  {runningCommand === `pg_query-${pgId}` ? 'Checking...' : 'Status'}
                                </button>
                              )}
                            </>
                          )}
                        </div>
                      </div>
                    )}
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {!hasIssues && (
        <div className="bg-green-50 dark:bg-green-900/20 rounded-lg p-4 text-green-700 dark:text-green-400">
          Ceph cluster is healthy. No issues detected.
        </div>
      )}

      {/* Command Output Log */}
      {commandLogs.length > 0 && (
        <div className="mt-6">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-3">Command Log</h3>
          <div className="bg-gray-900 rounded-lg p-4 max-h-80 overflow-y-auto">
            {commandLogs.map((log, idx) => (
              <div key={idx} className="mb-4 last:mb-0">
                <div className="flex items-center gap-2 text-xs text-gray-400 mb-1">
                  <span>{log.timestamp.toLocaleTimeString()}</span>
                  <span className={log.success ? 'text-green-400' : 'text-red-400'}>
                    {log.success ? '✓' : '✗'}
                  </span>
                  <span className="text-blue-400">$ {log.command}</span>
                </div>
                <pre className="text-sm text-gray-300 whitespace-pre-wrap font-mono pl-4 border-l-2 border-gray-700">
                  {log.output}
                </pre>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

// Format power-on hours to years/days
function formatPowerOnTime(hours: number): string {
  const years = Math.floor(hours / 8760);
  const days = Math.floor((hours % 8760) / 24);
  if (years > 0) {
    return `${years}y ${days}d`;
  }
  return `${days}d`;
}

// Critical SMART attribute IDs
const CRITICAL_ATTRS = new Set([5, 10, 196, 197, 198]);

function SmartPanel() {
  const [disks, setDisks] = useState<SmartDisk[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [expandedDisk, setExpandedDisk] = useState<string | null>(null);

  useEffect(() => {
    const fetchSmartData = async () => {
      try {
        const response = await fetch('/api/smart');
        if (!response.ok) throw new Error('Failed to fetch SMART data');
        const data = await response.json();
        setDisks(data || []);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Unknown error');
      } finally {
        setLoading(false);
      }
    };
    fetchSmartData();
  }, []);

  if (loading) {
    return (
      <div className="p-6 text-gray-500">
        Loading SMART data...
      </div>
    );
  }

  if (error) {
    return (
      <div className="p-6 text-red-500">
        Error: {error}
      </div>
    );
  }

  // Group disks by node
  const disksByNode = disks.reduce((acc, disk) => {
    if (!acc[disk.node]) acc[disk.node] = [];
    acc[disk.node].push(disk);
    return acc;
  }, {} as Record<string, SmartDisk[]>);

  const getHealthColor = (health: string) => {
    switch (health) {
      case 'PASSED': return 'text-green-500';
      case 'WARNING': return 'text-yellow-500';
      case 'FAILED': return 'text-red-500';
      default: return 'text-gray-500';
    }
  };

  const getHealthBg = (health: string) => {
    switch (health) {
      case 'PASSED': return 'bg-green-500';
      case 'WARNING': return 'bg-yellow-500';
      case 'FAILED': return 'bg-red-500';
      default: return 'bg-gray-500';
    }
  };

  const getTempColor = (temp: number) => {
    if (temp >= 60) return 'text-red-500';
    if (temp >= 50) return 'text-yellow-500';
    return 'text-gray-600 dark:text-gray-400';
  };

  const getDiskIcon = (type: string) => {
    switch (type) {
      case 'nvme': return '⚡';
      case 'ssd': return '◻️';
      default: return '🗄️';
    }
  };

  const getDiskTypeLabel = (type: string) => {
    switch (type) {
      case 'nvme': return 'NVMe SSD';
      case 'ssd': return 'SATA SSD';
      default: return 'Hard Disk Drive';
    }
  };

  return (
    <div className="p-6">
      <h2 className="text-xl font-bold text-gray-900 dark:text-white mb-4">Disk Health (SMART)</h2>

      {Object.entries(disksByNode).map(([node, nodeDisks]) => (
        <div key={node} className="mb-6">
          <h3 className="text-lg font-semibold text-gray-700 dark:text-gray-300 mb-3">{node}</h3>
          <div className="space-y-2">
            {nodeDisks.map((disk) => {
              const diskKey = `${disk.node}-${disk.device}`;
              const isExpanded = expandedDisk === diskKey;

              return (
                <div key={diskKey} className="bg-white dark:bg-gray-800 rounded-lg shadow">
                  {/* Disk header - always visible */}
                  <button
                    onClick={() => setExpandedDisk(isExpanded ? null : diskKey)}
                    className="w-full p-4 flex items-center gap-4 text-left hover:bg-gray-50 dark:hover:bg-gray-700 rounded-lg"
                  >
                    <div
                      className={`w-3 h-3 rounded-full ${getHealthBg(disk.health)}`}
                      title={`SMART Health: ${disk.health}`}
                    />
                    <span className="text-lg" title={getDiskTypeLabel(disk.type)}>{getDiskIcon(disk.type)}</span>
                    <div className="flex-1 min-w-0">
                      <div className="font-medium text-gray-900 dark:text-white">
                        <span className="font-mono">{disk.device}</span>
                        {disk.model && <span className="ml-2 text-gray-600 dark:text-gray-400">— {disk.model}</span>}
                      </div>
                      <div className="text-sm text-gray-500 dark:text-gray-400">
                        {formatBytes(disk.capacity)} • {disk.type.toUpperCase()}
                      </div>
                    </div>
                    <div className="text-right">
                      <div className={`font-medium ${getHealthColor(disk.health)}`}>
                        {disk.health}
                      </div>
                      <div className="text-sm text-gray-500">
                        <span className={getTempColor(disk.temperature)} title="Current Temperature">{disk.temperature}°C</span>
                        {' • '}
                        <span title={`${disk.power_on_hours.toLocaleString()} hours powered on`}>{formatPowerOnTime(disk.power_on_hours)}</span>
                      </div>
                    </div>
                    <svg
                      className={`w-5 h-5 text-gray-400 transition-transform ${isExpanded ? 'rotate-180' : ''}`}
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                    >
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                    </svg>
                  </button>

                  {/* Expanded details */}
                  {isExpanded && (
                    <div className="px-4 pb-4 border-t border-gray-200 dark:border-gray-700">
                      <div className="mt-4 grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
                        <div>
                          <span className="text-gray-500">Serial:</span>
                          <span className="ml-2 font-mono text-gray-900 dark:text-white">{disk.serial}</span>
                        </div>
                        <div>
                          <span className="text-gray-500">Protocol:</span>
                          <span className="ml-2 text-gray-900 dark:text-white">{disk.protocol}</span>
                        </div>
                        <div>
                          <span className="text-gray-500">Power On:</span>
                          <span className="ml-2 text-gray-900 dark:text-white">{disk.power_on_hours.toLocaleString()} hours</span>
                        </div>
                        <div>
                          <span className="text-gray-500">Temperature:</span>
                          <span className={`ml-2 ${getTempColor(disk.temperature)}`}>{disk.temperature}°C</span>
                        </div>
                      </div>

                      {/* NVMe Health */}
                      {disk.nvme_health && (
                        <div className="mt-4">
                          <h4 className="font-medium text-gray-900 dark:text-white mb-2">NVMe Health</h4>
                          <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
                            <div>
                              <span className="text-gray-500">Available Spare:</span>
                              <span className={`ml-2 ${disk.nvme_health.available_spare < 20 ? 'text-yellow-500' : 'text-green-500'}`}>
                                {disk.nvme_health.available_spare}%
                              </span>
                            </div>
                            <div>
                              <span className="text-gray-500">Wear Level:</span>
                              <span className={`ml-2 ${disk.nvme_health.percent_used > 80 ? 'text-yellow-500' : 'text-green-500'}`}>
                                {disk.nvme_health.percent_used}%
                              </span>
                            </div>
                            <div>
                              <span className="text-gray-500">Media Errors:</span>
                              <span className={`ml-2 ${disk.nvme_health.media_errors > 0 ? 'text-red-500' : 'text-green-500'}`}>
                                {disk.nvme_health.media_errors}
                              </span>
                            </div>
                            <div>
                              <span className="text-gray-500">Unsafe Shutdowns:</span>
                              <span className="ml-2 text-gray-900 dark:text-white">{disk.nvme_health.unsafe_shutdowns}</span>
                            </div>
                          </div>
                        </div>
                      )}

                      {/* SMART Attributes for HDD/SSD */}
                      {disk.attributes && disk.attributes.length > 0 && (
                        <div className="mt-4">
                          <h4 className="font-medium text-gray-900 dark:text-white mb-2">SMART Attributes</h4>
                          <div className="overflow-x-auto">
                            <table className="w-full text-sm">
                              <thead>
                                <tr className="text-left text-gray-500 border-b border-gray-200 dark:border-gray-700">
                                  <th className="pb-2">ID</th>
                                  <th className="pb-2">Attribute</th>
                                  <th className="pb-2 text-right">Value</th>
                                  <th className="pb-2 text-right">Worst</th>
                                  <th className="pb-2 text-right">Thresh</th>
                                  <th className="pb-2 text-right">Raw</th>
                                </tr>
                              </thead>
                              <tbody>
                                {disk.attributes
                                  .filter(attr => CRITICAL_ATTRS.has(attr.id) || attr.raw > 0)
                                  .map((attr) => (
                                    <tr
                                      key={attr.id}
                                      className={`border-b border-gray-100 dark:border-gray-700 ${
                                        attr.critical && attr.raw > 0 ? 'bg-red-50 dark:bg-red-900/20' : ''
                                      }`}
                                    >
                                      <td className="py-2 text-gray-600 dark:text-gray-400">{attr.id}</td>
                                      <td className={`py-2 ${attr.critical ? 'font-medium text-gray-900 dark:text-white' : 'text-gray-700 dark:text-gray-300'}`}>
                                        {attr.name.replace(/_/g, ' ')}
                                        {attr.critical && <span className="ml-1 text-xs text-red-500">*</span>}
                                      </td>
                                      <td className="py-2 text-right text-gray-900 dark:text-white">{attr.value}</td>
                                      <td className="py-2 text-right text-gray-600 dark:text-gray-400">{attr.worst}</td>
                                      <td className="py-2 text-right text-gray-600 dark:text-gray-400">{attr.threshold}</td>
                                      <td className={`py-2 text-right font-mono ${
                                        attr.critical && attr.raw > 0 ? 'text-red-500 font-bold' : 'text-gray-900 dark:text-white'
                                      }`}>
                                        {attr.raw.toLocaleString()}
                                      </td>
                                    </tr>
                                  ))}
                              </tbody>
                            </table>
                          </div>
                          <div className="mt-2 text-xs text-gray-500">
                            * Critical attributes - non-zero raw values indicate potential issues
                          </div>
                        </div>
                      )}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </div>
      ))}

      {disks.length === 0 && (
        <div className="text-gray-500">No disk SMART data available.</div>
      )}
    </div>
  );
}

export function StoragePage() {
  const { selectedObject } = useCluster();
  const [filter, setFilter] = useState('');
  const [searchParams, setSearchParams] = useSearchParams();
  const activeTab = searchParams.get('tab') || 'storage';

  const sidebar = (
    <div className="flex flex-col h-full">
      <div className="p-2 border-b border-gray-200 dark:border-gray-700">
        <input
          type="text"
          placeholder="Search storage..."
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200"
        />
      </div>
      <div className="flex-1 overflow-y-auto">
        <InventoryTree view="storage" filter={filter} />
      </div>
    </div>
  );

  return (
    <Layout sidebar={sidebar}>
      <div className="flex-1 overflow-auto">
        {/* Tab bar */}
        <div className="border-b border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
          <div className="flex gap-4 px-4">
            <button
              onClick={() => setSearchParams({})}
              className={`py-3 px-1 text-sm font-medium border-b-2 ${
                activeTab === 'storage'
                  ? 'border-blue-500 text-blue-500'
                  : 'border-transparent text-gray-500 hover:text-gray-700'
              }`}
            >
              Storage
            </button>
            <button
              onClick={() => setSearchParams({ tab: 'ceph' })}
              className={`py-3 px-1 text-sm font-medium border-b-2 ${
                activeTab === 'ceph'
                  ? 'border-blue-500 text-blue-500'
                  : 'border-transparent text-gray-500 hover:text-gray-700'
              }`}
            >
              Ceph
            </button>
            <button
              onClick={() => setSearchParams({ tab: 'disks' })}
              className={`py-3 px-1 text-sm font-medium border-b-2 ${
                activeTab === 'disks'
                  ? 'border-blue-500 text-blue-500'
                  : 'border-transparent text-gray-500 hover:text-gray-700'
              }`}
            >
              Disks
            </button>
          </div>
        </div>

        {/* Tab content */}
        {activeTab === 'ceph' ? (
          <CephDetailPanel />
        ) : activeTab === 'disks' ? (
          <SmartPanel />
        ) : selectedObject ? (
          <ObjectDetail />
        ) : (
          <div className="flex-1 flex items-center justify-center text-gray-500 p-8">
            Select a storage from the tree
          </div>
        )}
      </div>
    </Layout>
  );
}
