import { useState, useEffect, type FormEvent } from 'react';
import { useCluster } from '../context/ClusterContext';
import { api, formatBytes } from '../api/client';
import { DRSPanel } from './DRSPanel';
import type { AlarmDefinition, NotificationChannel } from '../types';

interface Tab { id: string; label: string; }

const rootTabs: Tab[] = [
  { id: 'summary', label: 'Summary' },
  { id: 'configure', label: 'Configure' },
];

export function PCenterRootDetail({ defaultTab }: { defaultTab?: string }) {
  const { nodes, guests, clusters } = useCluster();
  const [activeTab, setActiveTab] = useState(defaultTab || 'summary');

  useEffect(() => { if (defaultTab) setActiveTab(defaultTab); }, [defaultTab]);

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      {/* Header */}
      <div className="bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-4 py-3 flex-shrink-0">
        <div className="flex items-center gap-3">
          <span className="text-2xl">🖧</span>
          <div>
            <h1 className="text-lg font-semibold text-gray-900 dark:text-white">pCenter</h1>
            <div className="text-sm text-gray-500">
              {clusters.length} cluster{clusters.length !== 1 ? 's' : ''} &middot;{' '}
              {nodes.length} nodes &middot; {guests.length} guests
            </div>
          </div>
        </div>
      </div>

      {/* Tabs */}
      <div className="bg-gray-50 dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-4 flex-shrink-0">
        <div className="flex gap-1">
          {rootTabs.map(tab => (
            <button key={tab.id} onClick={() => setActiveTab(tab.id)}
              className={`px-4 py-2 text-sm font-medium transition-colors ${
                activeTab === tab.id
                  ? 'text-blue-600 border-b-2 border-blue-600 bg-white dark:bg-gray-700'
                  : 'text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white'
              }`}>{tab.label}</button>
          ))}
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-auto p-4 bg-gray-50 dark:bg-gray-900">
        {activeTab === 'summary' && <RootSummary />}
        {activeTab === 'configure' && <RootConfigure />}
      </div>
    </div>
  );
}

// ─── Summary ────────────────────────────────────────────────────────────────

function RootSummary() {
  const { nodes, guests, clusters, drsRecommendations, alarms, ceph } = useCluster();
  const safeNodes = nodes || [];
  const safeGuests = guests || [];

  const onlineNodes = safeNodes.filter(n => n.status === 'online');
  const runningVMs = safeGuests.filter(g => g.type === 'qemu' && g.status === 'running');
  const totalVMs = safeGuests.filter(g => g.type === 'qemu');
  const runningCTs = safeGuests.filter(g => g.type === 'lxc' && g.status === 'running');
  const totalCTs = safeGuests.filter(g => g.type === 'lxc');

  const totalCPU = safeNodes.reduce((s, n) => s + n.maxcpu, 0);
  const usedCPU = safeNodes.reduce((s, n) => s + n.cpu * n.maxcpu, 0);
  const totalMem = safeNodes.reduce((s, n) => s + n.maxmem, 0);
  const usedMem = safeNodes.reduce((s, n) => s + n.mem, 0);

  const cpuPct = totalCPU > 0 ? (usedCPU / totalCPU) * 100 : 0;
  const memPct = totalMem > 0 ? (usedMem / totalMem) * 100 : 0;

  const cephHealth = ceph?.health || 'N/A';

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 lg:grid-cols-5 gap-4">
        <StatusCard title="Clusters" value={String(clusters.length)} subtitle="configured" color="blue" />
        <StatusCard title="Nodes" value={`${onlineNodes.length}/${safeNodes.length}`} subtitle="online"
          color={onlineNodes.length === safeNodes.length ? 'green' : 'yellow'} />
        <StatusCard title="VMs" value={`${runningVMs.length}/${totalVMs.length}`} subtitle="running" color="blue" />
        <StatusCard title="Containers" value={`${runningCTs.length}/${totalCTs.length}`} subtitle="running" color="blue" />
        <StatusCard title="Ceph"
          value={cephHealth === 'HEALTH_OK' ? 'Healthy' : cephHealth === 'HEALTH_WARN' ? 'Warning' : cephHealth === 'HEALTH_ERR' ? 'Error' : cephHealth}
          subtitle={ceph ? `${formatBytes(ceph.bytes_used || 0)} used` : ''}
          color={cephHealth === 'HEALTH_OK' ? 'green' : cephHealth === 'HEALTH_WARN' ? 'yellow' : cephHealth === 'HEALTH_ERR' ? 'red' : 'gray'} />
      </div>

      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Global Resources</h3>
        <div className="space-y-4">
          <ResourceBar label="CPU" value={cpuPct} detail={`${usedCPU.toFixed(1)} / ${totalCPU} cores`} />
          <ResourceBar label="Memory" value={memPct} detail={`${formatBytes(usedMem)} / ${formatBytes(totalMem)}`} />
        </div>
      </div>

      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Clusters</h3>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-gray-500 border-b border-gray-200 dark:border-gray-700">
                <th className="pb-2 font-medium">Cluster</th>
                <th className="pb-2 font-medium">Nodes</th>
                <th className="pb-2 font-medium">VMs</th>
                <th className="pb-2 font-medium">CTs</th>
                <th className="pb-2 font-medium">HA</th>
              </tr>
            </thead>
            <tbody>
              {clusters.map(c => (
                <tr key={c.name} className="border-b border-gray-100 dark:border-gray-700/50">
                  <td className="py-2 font-medium text-gray-900 dark:text-white">{c.name}</td>
                  <td className="py-2 text-gray-700 dark:text-gray-300">{c.summary?.OnlineNodes || 0}/{c.summary?.TotalNodes || 0}</td>
                  <td className="py-2 text-gray-700 dark:text-gray-300">{c.summary?.RunningVMs || 0}/{c.summary?.TotalVMs || 0}</td>
                  <td className="py-2 text-gray-700 dark:text-gray-300">{c.summary?.RunningCTs || 0}/{c.summary?.TotalContainers || 0}</td>
                  <td className="py-2">
                    {c.ha ? (
                      <span className={`px-2 py-0.5 rounded text-xs ${c.ha.quorum
                        ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                        : 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
                      }`}>{c.ha.quorum ? 'Quorum' : 'No Quorum'}</span>
                    ) : <span className="text-gray-400">N/A</span>}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {alarms && alarms.length > 0 && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h3 className="font-medium mb-3 text-gray-900 dark:text-white flex items-center gap-2">
            Active Alarms
            <span className="px-2 py-0.5 bg-red-500/20 text-red-500 text-xs rounded-full">{alarms.length}</span>
          </h3>
          <div className="space-y-2">
            {alarms.slice(0, 5).map(a => (
              <div key={a.id} className="flex items-center gap-3 text-sm">
                <span className={`w-2 h-2 rounded-full ${a.state === 'critical' ? 'bg-red-500' : 'bg-yellow-500'}`} />
                <span className="text-gray-900 dark:text-white">{a.definition_name}</span>
                <span className="text-gray-500">{a.resource_name || a.resource_id}</span>
                <span className="text-gray-400">{a.current_value.toFixed(1)}%</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {drsRecommendations && drsRecommendations.length > 0 && (
        <DRSPanel recommendations={drsRecommendations} onRefresh={() => window.location.reload()} />
      )}
    </div>
  );
}

// ─── Configure ──────────────────────────────────────────────────────────────

function RootConfigure() {
  const [config, setConfig] = useState<Record<string, unknown> | null>(null);
  const [configLoading, setConfigLoading] = useState(true);
  const [activeSection, setActiveSection] = useState('server');

  useEffect(() => {
    fetch('/api/config', { credentials: 'include' })
      .then(r => r.ok ? r.json() : null)
      .then(setConfig)
      .catch(() => {})
      .finally(() => setConfigLoading(false));
  }, []);

  const sections = [
    { id: 'server', label: 'Server' },
    { id: 'drs', label: 'DRS' },
    { id: 'metrics', label: 'Metrics' },
    { id: 'auth', label: 'Authentication' },
    { id: 'alarms', label: 'Alarms' },
    { id: 'notifications', label: 'Notifications' },
  ];

  return (
    <div className="flex gap-6">
      {/* Sidebar navigation */}
      <div className="w-48 flex-shrink-0">
        <div className="space-y-1">
          {sections.map(s => (
            <button key={s.id} onClick={() => setActiveSection(s.id)}
              className={`w-full text-left px-3 py-2 text-sm rounded transition-colors ${
                activeSection === s.id
                  ? 'bg-blue-600 text-white'
                  : 'text-gray-600 dark:text-gray-400 hover:bg-gray-200 dark:hover:bg-gray-800'
              }`}>{s.label}</button>
          ))}
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 min-w-0">
        {configLoading && <div className="text-gray-500">Loading configuration...</div>}
        {!configLoading && activeSection === 'server' && <ServerConfig config={config} />}
        {!configLoading && activeSection === 'drs' && <DRSConfig config={config} />}
        {!configLoading && activeSection === 'metrics' && <MetricsConfig config={config} />}
        {!configLoading && activeSection === 'auth' && <AuthConfig config={config} />}
        {!configLoading && activeSection === 'alarms' && <AlarmsConfig />}
        {!configLoading && activeSection === 'notifications' && <NotificationsConfig />}
      </div>
    </div>
  );
}

// ─── Config Sections (read-only from config.yaml) ───────────────────────────

function ServerConfig({ config }: { config: Record<string, unknown> | null }) {
  const srv = (config?.server || {}) as Record<string, unknown>;
  return (
    <ConfigSection title="Server" description="Server settings are configured in config.yaml">
      <ConfigRow label="Port" value={String(srv.port || 8080)} />
      <ConfigRow label="CORS Origins" value={Array.isArray(srv.cors_origins) ? srv.cors_origins.join(', ') || '(none)' : '(none)'} />
    </ConfigSection>
  );
}

function DRSConfig({ config }: { config: Record<string, unknown> | null }) {
  const drs = (config?.drs || {}) as Record<string, unknown>;
  return (
    <ConfigSection title="DRS (Distributed Resource Scheduler)" description="DRS settings are configured in config.yaml">
      <ConfigRow label="Enabled" value={String(drs.enabled ?? false)} />
      <ConfigRow label="Mode" value={String(drs.mode || 'manual')} />
      <ConfigRow label="Check Interval" value={`${drs.check_interval || 300}s`} />
      <ConfigRow label="CPU Threshold" value={`${((drs.cpu_threshold as number) || 0.8) * 100}%`} />
      <ConfigRow label="Memory Threshold" value={`${((drs.mem_threshold as number) || 0.85) * 100}%`} />
      <ConfigRow label="Max Concurrent Migrations" value={String(drs.migration_rate || 2)} />
    </ConfigSection>
  );
}

function MetricsConfig({ config }: { config: Record<string, unknown> | null }) {
  const m = (config?.metrics || {}) as Record<string, unknown>;
  const r = (m.retention || {}) as Record<string, unknown>;
  return (
    <ConfigSection title="Metrics" description="Metrics settings are configured in config.yaml">
      <ConfigRow label="Enabled" value={String(m.enabled ?? true)} />
      <ConfigRow label="Collection Interval" value={`${m.collection_interval || 30}s`} />
      <ConfigRow label="Raw Retention" value={`${r.raw_hours || 24} hours`} />
      <ConfigRow label="Hourly Rollup" value={`${r.hourly_days || 7} days`} />
      <ConfigRow label="Daily Rollup" value={`${r.daily_days || 30} days`} />
      <ConfigRow label="Weekly Rollup" value={`${r.weekly_months || 12} months`} />
    </ConfigSection>
  );
}

function AuthConfig({ config }: { config: Record<string, unknown> | null }) {
  const a = (config?.auth || {}) as Record<string, unknown>;
  const sess = (a.session || {}) as Record<string, unknown>;
  const lock = (a.lockout || {}) as Record<string, unknown>;
  const totp = (a.totp || {}) as Record<string, unknown>;
  const rl = (a.rate_limit || {}) as Record<string, unknown>;
  return (
    <ConfigSection title="Authentication" description="Auth settings are configured in config.yaml">
      <ConfigRow label="Enabled" value={String(a.enabled ?? true)} />
      <div className="mt-4 mb-2 text-xs font-medium text-gray-500 uppercase tracking-wider">Sessions</div>
      <ConfigRow label="Duration" value={`${sess.duration_hours || 24} hours`} />
      <ConfigRow label="Idle Timeout" value={`${sess.idle_timeout_hours || 8} hours`} />
      <div className="mt-4 mb-2 text-xs font-medium text-gray-500 uppercase tracking-wider">Lockout</div>
      <ConfigRow label="Max Attempts" value={String(lock.max_attempts || 5)} />
      <ConfigRow label="Lockout Duration" value={`${lock.lockout_minutes || 15} min`} />
      <ConfigRow label="Progressive" value={String(lock.progressive ?? true)} />
      <div className="mt-4 mb-2 text-xs font-medium text-gray-500 uppercase tracking-wider">Two-Factor Auth</div>
      <ConfigRow label="Enabled" value={String(totp.enabled ?? true)} />
      <ConfigRow label="Required" value={String(totp.required ?? false)} />
      <ConfigRow label="Trust IP Duration" value={`${totp.trust_ip_hours || 24} hours`} />
      <div className="mt-4 mb-2 text-xs font-medium text-gray-500 uppercase tracking-wider">Rate Limiting</div>
      <ConfigRow label="Login Requests/min" value={String(rl.requests_per_minute || 10)} />
    </ConfigSection>
  );
}

// ─── Alarms Config (interactive) ────────────────────────────────────────────

function AlarmsConfig() {
  const [definitions, setDefinitions] = useState<AlarmDefinition[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [name, setName] = useState('');
  const [metricType, setMetricType] = useState('cpu');
  const [resourceType, setResourceType] = useState('node');
  const [warningThreshold, setWarningThreshold] = useState(90);
  const [criticalThreshold, setCriticalThreshold] = useState(95);
  const [clearThreshold, setClearThreshold] = useState(85);
  const [durationSamples, setDurationSamples] = useState(3);
  const [creating, setCreating] = useState(false);

  const load = () => {
    api.getAlarmDefinitions().then(d => setDefinitions(d || [])).catch(e => setError(String(e))).finally(() => setLoading(false));
  };
  useEffect(load, []);

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault();
    setCreating(true);
    try {
      await api.createAlarmDefinition({
        name, metric_type: metricType, resource_type: resourceType,
        scope: 'global', condition: 'above',
        warning_threshold: warningThreshold, critical_threshold: criticalThreshold,
        clear_threshold: clearThreshold, duration_samples: durationSamples,
        notify_channels: [],
      } as Partial<AlarmDefinition>);
      setShowCreate(false); setName(''); load();
    } catch (e) { setError(e instanceof Error ? e.message : 'Failed'); }
    finally { setCreating(false); }
  };

  const handleDelete = (id: string) => api.deleteAlarmDefinition(id).then(load);
  const handleToggle = (def: AlarmDefinition) => api.updateAlarmDefinition(def.id, { ...def, enabled: !def.enabled }).then(load);

  if (loading) return <div className="text-gray-500">Loading...</div>;

  return (
    <div className="space-y-4">
      <div className="flex justify-between items-center">
        <div>
          <h3 className="text-lg font-medium text-gray-900 dark:text-white">Alarm Definitions</h3>
          <p className="text-sm text-gray-500">Threshold-based alerts evaluated every 30 seconds</p>
        </div>
        <button onClick={() => setShowCreate(!showCreate)}
          className="px-3 py-1.5 bg-blue-600 text-white text-sm rounded hover:bg-blue-700">
          {showCreate ? 'Cancel' : '+ New Alarm'}
        </button>
      </div>

      {error && <div className="p-2 bg-red-100 dark:bg-red-900/30 text-red-600 dark:text-red-400 text-sm rounded">{error}</div>}

      {showCreate && (
        <form onSubmit={handleCreate} className="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-4 space-y-3">
          <div className="grid grid-cols-2 gap-3">
            <Input label="Name" value={name} onChange={setName} placeholder="Node CPU High" required />
            <Select label="Metric" value={metricType} onChange={setMetricType}
              options={[['cpu','CPU %'], ['mem_percent','Memory %'], ['disk_percent','Disk %']]} />
            <Select label="Resource Type" value={resourceType} onChange={setResourceType}
              options={[['node','Node'], ['vm','VM'], ['ct','Container']]} />
            <div>
              <Input label="Duration (samples)" value={String(durationSamples)} onChange={v => setDurationSamples(parseInt(v) || 3)} type="number" />
              <span className="text-xs text-gray-400">{durationSamples * 30}s</span>
            </div>
            <Input label="Warning %" value={String(warningThreshold)} onChange={v => setWarningThreshold(parseFloat(v))} type="number" />
            <Input label="Critical %" value={String(criticalThreshold)} onChange={v => setCriticalThreshold(parseFloat(v))} type="number" />
            <div>
              <Input label="Clear %" value={String(clearThreshold)} onChange={v => setClearThreshold(parseFloat(v))} type="number" />
              <span className="text-xs text-gray-400">Hysteresis</span>
            </div>
          </div>
          <button type="submit" disabled={creating}
            className="px-4 py-2 bg-blue-600 text-white text-sm rounded hover:bg-blue-700 disabled:opacity-50">
            {creating ? 'Creating...' : 'Create'}
          </button>
        </form>
      )}

      <div className="space-y-2">
        {definitions.map(def => (
          <div key={def.id} className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-3 flex items-center justify-between">
            <div className="flex items-center gap-3">
              <button onClick={() => handleToggle(def)}
                className={`w-9 h-5 rounded-full relative transition-colors ${def.enabled ? 'bg-blue-600' : 'bg-gray-400'}`}>
                <span className={`absolute top-0.5 w-4 h-4 rounded-full bg-white transition-transform ${def.enabled ? 'left-4' : 'left-0.5'}`} />
              </button>
              <div>
                <span className="font-medium text-sm text-gray-900 dark:text-white">{def.name}</span>
                <div className="text-xs text-gray-500">
                  {def.resource_type} &middot; {def.metric_type} &middot;
                  warn &gt;{def.warning_threshold}% &middot; crit &gt;{def.critical_threshold}% &middot;
                  clear &lt;{def.clear_threshold}% &middot; {def.duration_samples} samples
                </div>
              </div>
            </div>
            <button onClick={() => handleDelete(def.id)} className="text-xs text-red-500 hover:text-red-700">Delete</button>
          </div>
        ))}
      </div>
    </div>
  );
}

// ─── Notifications Config (interactive) ─────────────────────────────────────

function NotificationsConfig() {
  const [channels, setChannels] = useState<NotificationChannel[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [channelName, setChannelName] = useState('');
  const [webhookUrl, setWebhookUrl] = useState('');
  const [creating, setCreating] = useState(false);
  const [testing, setTesting] = useState<string | null>(null);

  const load = () => {
    api.getAlarmChannels().then(c => setChannels(c || [])).catch(e => setError(String(e))).finally(() => setLoading(false));
  };
  useEffect(load, []);

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault();
    setCreating(true);
    try {
      await api.createAlarmChannel({ name: channelName, type: 'webhook', config: JSON.stringify({ url: webhookUrl }) });
      setShowCreate(false); setChannelName(''); setWebhookUrl(''); load();
    } catch (e) { setError(e instanceof Error ? e.message : 'Failed'); }
    finally { setCreating(false); }
  };

  const handleTest = async (id: string) => {
    setTesting(id); setError(null);
    try { await api.testAlarmChannel(id); } catch (e) { setError(e instanceof Error ? e.message : 'Test failed'); }
    finally { setTesting(null); }
  };

  if (loading) return <div className="text-gray-500">Loading...</div>;

  return (
    <div className="space-y-4">
      <div className="flex justify-between items-center">
        <div>
          <h3 className="text-lg font-medium text-gray-900 dark:text-white">Notification Channels</h3>
          <p className="text-sm text-gray-500">Webhook endpoints for alarm notifications</p>
        </div>
        <button onClick={() => setShowCreate(!showCreate)}
          className="px-3 py-1.5 bg-blue-600 text-white text-sm rounded hover:bg-blue-700">
          {showCreate ? 'Cancel' : '+ New Channel'}
        </button>
      </div>

      {error && <div className="p-2 bg-red-100 dark:bg-red-900/30 text-red-600 dark:text-red-400 text-sm rounded">{error}</div>}

      {showCreate && (
        <form onSubmit={handleCreate} className="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-4 space-y-3">
          <Input label="Channel Name" value={channelName} onChange={setChannelName} placeholder="Discord Alerts" required />
          <Input label="Webhook URL" value={webhookUrl} onChange={setWebhookUrl} placeholder="https://hooks.slack.com/..." type="url" required />
          <button type="submit" disabled={creating}
            className="px-4 py-2 bg-blue-600 text-white text-sm rounded hover:bg-blue-700 disabled:opacity-50">
            {creating ? 'Creating...' : 'Create'}
          </button>
        </form>
      )}

      <div className="space-y-2">
        {channels.length === 0 ? (
          <p className="text-gray-500 text-sm text-center py-4">No notification channels. Alarms fire but no notifications are sent.</p>
        ) : channels.map(ch => {
          let url = '';
          try { url = JSON.parse(ch.config)?.url || ''; } catch {}
          return (
            <div key={ch.id} className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-3 flex items-center justify-between">
              <div>
                <span className="font-medium text-sm text-gray-900 dark:text-white">{ch.name}</span>
                <div className="text-xs text-gray-500">{ch.type} &middot; {url.length > 60 ? url.slice(0, 60) + '...' : url}</div>
              </div>
              <div className="flex gap-2">
                <button onClick={() => handleTest(ch.id)} disabled={testing === ch.id}
                  className="text-xs px-2 py-1 bg-gray-100 dark:bg-gray-700 rounded hover:bg-gray-200 dark:hover:bg-gray-600 disabled:opacity-50">
                  {testing === ch.id ? 'Testing...' : 'Test'}
                </button>
                <button onClick={() => api.deleteAlarmChannel(ch.id).then(load)} className="text-xs text-red-500 hover:text-red-700">Delete</button>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

// ─── Shared Components ──────────────────────────────────────────────────────

function StatusCard({ title, value, subtitle, color }: { title: string; value: string; subtitle: string; color: string }) {
  const colors: Record<string, string> = {
    green: 'border-green-500', yellow: 'border-yellow-500', red: 'border-red-500', blue: 'border-blue-500', gray: 'border-gray-400',
  };
  return (
    <div className={`bg-white dark:bg-gray-800 rounded-lg shadow p-4 border-l-4 ${colors[color] || colors.gray}`}>
      <div className="text-sm text-gray-500">{title}</div>
      <div className="text-2xl font-bold text-gray-900 dark:text-white">{value}</div>
      <div className="text-xs text-gray-400">{subtitle}</div>
    </div>
  );
}

function ResourceBar({ label, value, detail }: { label: string; value: number; detail: string }) {
  const barColor = value > 90 ? 'bg-red-500' : value > 70 ? 'bg-yellow-500' : 'bg-blue-500';
  return (
    <div>
      <div className="flex justify-between text-sm mb-1">
        <span className="text-gray-500">{label}</span>
        <span className="text-gray-900 dark:text-white">{value.toFixed(1)}% ({detail})</span>
      </div>
      <div className="w-full h-2 bg-gray-200 dark:bg-gray-700 rounded">
        <div className={`h-full rounded ${barColor}`} style={{ width: `${Math.min(value, 100)}%` }} />
      </div>
    </div>
  );
}

function ConfigSection({ title, description, children }: { title: string; description?: string; children: React.ReactNode }) {
  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
      <h3 className="font-medium text-gray-900 dark:text-white">{title}</h3>
      {description && <p className="text-xs text-gray-500 mb-4">{description}</p>}
      <div className="space-y-1 mt-3">{children}</div>
    </div>
  );
}

function ConfigRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between py-1.5 text-sm">
      <span className="text-gray-500">{label}</span>
      <span className="text-gray-900 dark:text-white font-mono text-xs">{value}</span>
    </div>
  );
}

function Input({ label, value, onChange, placeholder, type, required }: {
  label: string; value: string; onChange: (v: string) => void;
  placeholder?: string; type?: string; required?: boolean;
}) {
  return (
    <div>
      <label className="block text-xs text-gray-500 mb-1">{label}</label>
      <input type={type || 'text'} value={value} onChange={e => onChange(e.target.value)}
        placeholder={placeholder} required={required}
        className="w-full px-3 py-1.5 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-white" />
    </div>
  );
}

function Select({ label, value, onChange, options }: {
  label: string; value: string; onChange: (v: string) => void;
  options: [string, string][];
}) {
  return (
    <div>
      <label className="block text-xs text-gray-500 mb-1">{label}</label>
      <select value={value} onChange={e => onChange(e.target.value)}
        className="w-full px-3 py-1.5 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-white">
        {options.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
      </select>
    </div>
  );
}
