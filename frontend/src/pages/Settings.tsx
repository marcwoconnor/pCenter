import { useState, useEffect, type FormEvent } from 'react';
import { useAuth } from '../context/AuthContext';
import { authApi } from '../api/auth';
import { api } from '../api/client';
import { TOTPSetupWizard } from '../components/TOTPSetupWizard';
import { Layout } from '../components/Layout';
import { WebhooksPanel } from '../components/WebhooksPanel';
import type { Session } from '../types/auth';
import type { AlarmDefinition, NotificationChannel, RBACRole, RBACRoleAssignment, ScheduledTask, TaskRun } from '../types';

type SettingsTab = 'security' | 'sessions' | 'rbac' | 'scheduler' | 'alarms' | 'notifications' | 'webhooks';

export function Settings() {
  const { user, refreshUser } = useAuth();
  const [activeTab, setActiveTab] = useState<SettingsTab>('security');

  return (
    <Layout>
      <div className="flex-1 overflow-auto p-6">
        <div className="max-w-3xl mx-auto">
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white mb-6">
            Account Settings
          </h1>

          {/* Tabs */}
          <div className="flex gap-4 border-b border-gray-200 dark:border-gray-700 mb-6">
            <button
              onClick={() => setActiveTab('security')}
              className={`pb-3 px-1 text-sm font-medium border-b-2 transition-colors ${
                activeTab === 'security'
                  ? 'border-blue-500 text-blue-600 dark:text-blue-400'
                  : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400'
              }`}
            >
              Security
            </button>
            <button
              onClick={() => setActiveTab('sessions')}
              className={`pb-3 px-1 text-sm font-medium border-b-2 transition-colors ${
                activeTab === 'sessions'
                  ? 'border-blue-500 text-blue-600 dark:text-blue-400'
                  : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400'
              }`}
            >
              Sessions
            </button>
            {user?.role === 'admin' && (
              <button
                onClick={() => setActiveTab('rbac')}
                className={`pb-3 px-1 text-sm font-medium border-b-2 transition-colors ${
                  activeTab === 'rbac'
                    ? 'border-blue-500 text-blue-600 dark:text-blue-400'
                    : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400'
                }`}
              >
                Roles & Permissions
              </button>
            )}
            <button
              onClick={() => setActiveTab('scheduler')}
              className={`pb-3 px-1 text-sm font-medium border-b-2 transition-colors ${
                activeTab === 'scheduler'
                  ? 'border-blue-500 text-blue-600 dark:text-blue-400'
                  : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400'
              }`}
            >
              Scheduled Tasks
            </button>
            <button
              onClick={() => setActiveTab('alarms')}
              className={`pb-3 px-1 text-sm font-medium border-b-2 transition-colors ${
                activeTab === 'alarms'
                  ? 'border-blue-500 text-blue-600 dark:text-blue-400'
                  : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400'
              }`}
            >
              Alarms
            </button>
            <button
              onClick={() => setActiveTab('notifications')}
              className={`pb-3 px-1 text-sm font-medium border-b-2 transition-colors ${
                activeTab === 'notifications'
                  ? 'border-blue-500 text-blue-600 dark:text-blue-400'
                  : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400'
              }`}
            >
              Notifications
            </button>
            {user?.role === 'admin' && (
              <button
                onClick={() => setActiveTab('webhooks')}
                className={`pb-3 px-1 text-sm font-medium border-b-2 transition-colors ${
                  activeTab === 'webhooks'
                    ? 'border-blue-500 text-blue-600 dark:text-blue-400'
                    : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400'
                }`}
              >
                Webhooks
              </button>
            )}
          </div>

          {activeTab === 'security' && <SecurityTab user={user} onUpdate={refreshUser} />}
          {activeTab === 'sessions' && <SessionsTab />}
          {activeTab === 'rbac' && <RBACTab />}
          {activeTab === 'scheduler' && <SchedulerTab />}
          {activeTab === 'alarms' && <AlarmsTab />}
          {activeTab === 'notifications' && <NotificationsTab />}
          {activeTab === 'webhooks' && <WebhooksPanel />}
        </div>
      </div>
    </Layout>
  );
}

// Security Tab - Password & 2FA
function SecurityTab({ user, onUpdate }: { user: any; onUpdate: () => void }) {
  return (
    <div className="space-y-8">
      <ChangePasswordSection />
      <TwoFactorSection user={user} onUpdate={onUpdate} />
    </div>
  );
}

// Change Password Section
function ChangePasswordSection() {
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    setSuccess(false);

    if (newPassword !== confirmPassword) {
      setError('New passwords do not match');
      return;
    }

    if (newPassword.length < 8) {
      setError('Password must be at least 8 characters');
      return;
    }

    setIsLoading(true);
    try {
      await authApi.changePassword({
        current_password: currentPassword,
        new_password: newPassword,
      });
      setSuccess(true);
      setCurrentPassword('');
      setNewPassword('');
      setConfirmPassword('');
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to change password');
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg p-6 shadow">
      <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
        Change Password
      </h2>

      <form onSubmit={handleSubmit} className="space-y-4 max-w-md">
        {error && (
          <div className="bg-red-50 dark:bg-red-900/20 text-red-600 dark:text-red-400 p-3 rounded-lg text-sm">
            {error}
          </div>
        )}
        {success && (
          <div className="bg-green-50 dark:bg-green-900/20 text-green-600 dark:text-green-400 p-3 rounded-lg text-sm">
            Password changed successfully
          </div>
        )}

        <div>
          <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
            Current Password
          </label>
          <input
            type="password"
            value={currentPassword}
            onChange={(e) => setCurrentPassword(e.target.value)}
            className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white focus:ring-2 focus:ring-blue-500 focus:border-transparent"
            required
          />
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
            New Password
          </label>
          <input
            type="password"
            value={newPassword}
            onChange={(e) => setNewPassword(e.target.value)}
            className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white focus:ring-2 focus:ring-blue-500 focus:border-transparent"
            required
          />
          <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
            Min 8 chars with uppercase, lowercase, and digit
          </p>
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
            Confirm New Password
          </label>
          <input
            type="password"
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
            className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-white focus:ring-2 focus:ring-blue-500 focus:border-transparent"
            required
          />
        </div>

        <button
          type="submit"
          disabled={isLoading}
          className="px-4 py-2 bg-blue-600 hover:bg-blue-700 disabled:bg-blue-400 text-white rounded-lg transition-colors"
        >
          {isLoading ? 'Changing...' : 'Change Password'}
        </button>
      </form>
    </div>
  );
}

// Two-Factor Authentication Section
function TwoFactorSection({ user, onUpdate }: { user: any; onUpdate: () => void }) {
  const [showSetup, setShowSetup] = useState(false);
  const [isDisabling, setIsDisabling] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleDisable = async () => {
    if (!confirm('Are you sure you want to disable two-factor authentication?')) {
      return;
    }

    setIsDisabling(true);
    setError(null);
    try {
      await authApi.disableTOTP();
      onUpdate();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to disable 2FA');
    } finally {
      setIsDisabling(false);
    }
  };

  const handleSetupComplete = () => {
    setShowSetup(false);
    onUpdate();
  };

  return (
    <>
      <div className="bg-white dark:bg-gray-800 rounded-lg p-6 shadow">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-2">
          Two-Factor Authentication
        </h2>
        <p className="text-sm text-gray-500 dark:text-gray-400 mb-4">
          Add an extra layer of security using an authenticator app.
        </p>

        {error && (
          <div className="bg-red-50 dark:bg-red-900/20 text-red-600 dark:text-red-400 p-3 rounded-lg text-sm mb-4">
            {error}
          </div>
        )}

        {user?.totp_enabled ? (
          <div className="flex items-center justify-between p-4 bg-green-50 dark:bg-green-900/20 rounded-lg">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 bg-green-100 dark:bg-green-800 rounded-full flex items-center justify-center">
                <svg className="w-5 h-5 text-green-600 dark:text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
                </svg>
              </div>
              <div>
                <p className="font-medium text-green-800 dark:text-green-200">Enabled</p>
                <p className="text-sm text-green-600 dark:text-green-400">Your account is protected with 2FA</p>
              </div>
            </div>
            <button
              onClick={handleDisable}
              disabled={isDisabling}
              className="px-4 py-2 text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20 rounded-lg transition-colors"
            >
              {isDisabling ? 'Disabling...' : 'Disable'}
            </button>
          </div>
        ) : (
          <div className="flex items-center justify-between p-4 bg-yellow-50 dark:bg-yellow-900/20 rounded-lg">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 bg-yellow-100 dark:bg-yellow-800 rounded-full flex items-center justify-center">
                <svg className="w-5 h-5 text-yellow-600 dark:text-yellow-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                </svg>
              </div>
              <div>
                <p className="font-medium text-yellow-800 dark:text-yellow-200">Not Enabled</p>
                <p className="text-sm text-yellow-600 dark:text-yellow-400">Add 2FA for better security</p>
              </div>
            </div>
            <button
              onClick={() => setShowSetup(true)}
              className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg transition-colors"
            >
              Enable 2FA
            </button>
          </div>
        )}
      </div>

      {showSetup && (
        <TOTPSetupWizard
          onComplete={handleSetupComplete}
          onCancel={() => setShowSetup(false)}
        />
      )}
    </>
  );
}

// Sessions Tab
function SessionsTab() {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadSessions();
  }, []);

  const loadSessions = async () => {
    try {
      const data = await authApi.listSessions();
      setSessions(data);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load sessions');
    } finally {
      setIsLoading(false);
    }
  };

  const handleRevoke = async (id: string) => {
    if (!confirm('Revoke this session? The device will be signed out.')) {
      return;
    }

    try {
      await authApi.revokeSession(id);
      setSessions(sessions.filter(s => s.id !== id));
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to revoke session');
    }
  };

  const formatDate = (dateStr: string) => {
    return new Date(dateStr).toLocaleString();
  };

  if (isLoading) {
    return (
      <div className="bg-white dark:bg-gray-800 rounded-lg p-6 shadow">
        <p className="text-gray-500 dark:text-gray-400">Loading sessions...</p>
      </div>
    );
  }

  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg p-6 shadow">
      <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
        Active Sessions
      </h2>
      <p className="text-sm text-gray-500 dark:text-gray-400 mb-4">
        Devices where you're currently signed in.
      </p>

      {error && (
        <div className="bg-red-50 dark:bg-red-900/20 text-red-600 dark:text-red-400 p-3 rounded-lg text-sm mb-4">
          {error}
        </div>
      )}

      <div className="space-y-3">
        {sessions.map((session) => (
          <div
            key={session.id}
            className="flex items-center justify-between p-4 border border-gray-200 dark:border-gray-700 rounded-lg"
          >
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 bg-gray-100 dark:bg-gray-700 rounded-full flex items-center justify-center">
                <svg className="w-5 h-5 text-gray-500 dark:text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                </svg>
              </div>
              <div>
                <div className="flex items-center gap-2">
                  <p className="font-medium text-gray-900 dark:text-white">
                    {session.ip_address || 'Unknown'}
                  </p>
                  {session.current && (
                    <span className="px-2 py-0.5 text-xs bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400 rounded">
                      Current
                    </span>
                  )}
                </div>
                <p className="text-sm text-gray-500 dark:text-gray-400">
                  Last active: {formatDate(session.last_seen_at)}
                </p>
              </div>
            </div>
            {!session.current && (
              <button
                onClick={() => handleRevoke(session.id)}
                className="px-3 py-1.5 text-sm text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20 rounded-lg transition-colors"
              >
                Revoke
              </button>
            )}
          </div>
        ))}

        {sessions.length === 0 && (
          <p className="text-gray-500 dark:text-gray-400 text-center py-4">
            No active sessions found
          </p>
        )}
      </div>
    </div>
  );
}

// Alarms Tab - Manage alarm definitions
function AlarmsTab() {
  const [definitions, setDefinitions] = useState<AlarmDefinition[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);

  // Create form state
  const [name, setName] = useState('');
  const [metricType, setMetricType] = useState('cpu');
  const [resourceType, setResourceType] = useState('node');
  const [warningThreshold, setWarningThreshold] = useState(90);
  const [criticalThreshold, setCriticalThreshold] = useState(95);
  const [clearThreshold, setClearThreshold] = useState(85);
  const [durationSamples, setDurationSamples] = useState(3);
  const [creating, setCreating] = useState(false);

  const loadDefinitions = async () => {
    try {
      const defs = await api.getAlarmDefinitions();
      setDefinitions(defs || []);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { loadDefinitions(); }, []);

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault();
    setCreating(true);
    setError(null);
    try {
      await api.createAlarmDefinition({
        name, metric_type: metricType, resource_type: resourceType,
        scope: 'global', condition: 'above',
        warning_threshold: warningThreshold,
        critical_threshold: criticalThreshold,
        clear_threshold: clearThreshold,
        duration_samples: durationSamples,
        notify_channels: [],
      } as Partial<AlarmDefinition>);
      setShowCreate(false);
      setName('');
      loadDefinitions();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Create failed');
    } finally {
      setCreating(false);
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await api.deleteAlarmDefinition(id);
      loadDefinitions();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Delete failed');
    }
  };

  const handleToggle = async (def: AlarmDefinition) => {
    try {
      await api.updateAlarmDefinition(def.id, { ...def, enabled: !def.enabled });
      loadDefinitions();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Update failed');
    }
  };

  if (loading) return <div className="text-gray-500">Loading...</div>;

  return (
    <div className="space-y-6">
      {error && (
        <div className="p-3 bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400 rounded text-sm">
          {error}
        </div>
      )}

      <div className="flex justify-between items-center">
        <h2 className="text-lg font-medium text-gray-900 dark:text-white">Alarm Definitions</h2>
        <button
          onClick={() => setShowCreate(!showCreate)}
          className="px-3 py-1.5 bg-blue-600 text-white text-sm rounded hover:bg-blue-700"
        >
          {showCreate ? 'Cancel' : '+ New Alarm'}
        </button>
      </div>

      {showCreate && (
        <form onSubmit={handleCreate} className="bg-gray-50 dark:bg-gray-800 rounded-lg p-4 space-y-4">
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm text-gray-500 mb-1">Name</label>
              <input
                type="text" value={name} onChange={e => setName(e.target.value)} required
                className="w-full px-3 py-2 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-white"
                placeholder="e.g. Node CPU High"
              />
            </div>
            <div>
              <label className="block text-sm text-gray-500 mb-1">Metric</label>
              <select value={metricType} onChange={e => setMetricType(e.target.value)}
                className="w-full px-3 py-2 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-white">
                <option value="cpu">CPU %</option>
                <option value="mem_percent">Memory %</option>
                <option value="disk_percent">Disk %</option>
                <option value="cert_days_left">Cert Days Left (node)</option>
              </select>
            </div>
            <div>
              <label className="block text-sm text-gray-500 mb-1">Resource Type</label>
              <select value={resourceType} onChange={e => setResourceType(e.target.value)}
                className="w-full px-3 py-2 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-white">
                <option value="node">Node</option>
                <option value="vm">VM</option>
                <option value="ct">Container</option>
              </select>
            </div>
            <div>
              <label className="block text-sm text-gray-500 mb-1">Duration (samples)</label>
              <input
                type="number" value={durationSamples} onChange={e => setDurationSamples(parseInt(e.target.value))}
                min={1} max={20}
                className="w-full px-3 py-2 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-white"
              />
              <span className="text-xs text-gray-400">{durationSamples * 30}s at 30s interval</span>
            </div>
            <div>
              <label className="block text-sm text-gray-500 mb-1">Warning Threshold (%)</label>
              <input
                type="number" value={warningThreshold} onChange={e => setWarningThreshold(parseFloat(e.target.value))}
                className="w-full px-3 py-2 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-white"
              />
            </div>
            <div>
              <label className="block text-sm text-gray-500 mb-1">Critical Threshold (%)</label>
              <input
                type="number" value={criticalThreshold} onChange={e => setCriticalThreshold(parseFloat(e.target.value))}
                className="w-full px-3 py-2 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-white"
              />
            </div>
            <div>
              <label className="block text-sm text-gray-500 mb-1">Clear Threshold (%)</label>
              <input
                type="number" value={clearThreshold} onChange={e => setClearThreshold(parseFloat(e.target.value))}
                className="w-full px-3 py-2 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-white"
              />
              <span className="text-xs text-gray-400">Hysteresis — alarm clears below this</span>
            </div>
          </div>
          <button type="submit" disabled={creating}
            className="px-4 py-2 bg-blue-600 text-white text-sm rounded hover:bg-blue-700 disabled:opacity-50">
            {creating ? 'Creating...' : 'Create Alarm'}
          </button>
        </form>
      )}

      {/* Definitions list */}
      <div className="space-y-2">
        {definitions.length === 0 ? (
          <p className="text-gray-500 text-sm text-center py-4">No alarm definitions configured</p>
        ) : definitions.map(def => (
          <div key={def.id} className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-4">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <button
                  onClick={() => handleToggle(def)}
                  className={`w-10 h-5 rounded-full relative transition-colors ${
                    def.enabled ? 'bg-blue-600' : 'bg-gray-400'
                  }`}
                >
                  <span className={`absolute top-0.5 w-4 h-4 rounded-full bg-white transition-transform ${
                    def.enabled ? 'left-5' : 'left-0.5'
                  }`} />
                </button>
                <div>
                  <span className="font-medium text-gray-900 dark:text-white">{def.name}</span>
                  <div className="text-xs text-gray-500">
                    {def.resource_type} &middot; {def.metric_type} &middot;
                    warn &gt; {def.warning_threshold}% &middot;
                    crit &gt; {def.critical_threshold}% &middot;
                    clear &lt; {def.clear_threshold}% &middot;
                    {def.duration_samples} samples
                  </div>
                </div>
              </div>
              <button
                onClick={() => handleDelete(def.id)}
                className="text-xs text-red-500 hover:text-red-700"
              >
                Delete
              </button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

// Notifications Tab - Manage notification channels
function NotificationsTab() {
  const [channels, setChannels] = useState<NotificationChannel[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [testing, setTesting] = useState<string | null>(null);

  // Create form
  const [channelName, setChannelName] = useState('');
  const [webhookUrl, setWebhookUrl] = useState('');
  const [creating, setCreating] = useState(false);

  const loadChannels = async () => {
    try {
      const chs = await api.getAlarmChannels();
      setChannels(chs || []);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { loadChannels(); }, []);

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault();
    setCreating(true);
    setError(null);
    try {
      await api.createAlarmChannel({
        name: channelName,
        type: 'webhook',
        config: JSON.stringify({ url: webhookUrl }),
      });
      setShowCreate(false);
      setChannelName('');
      setWebhookUrl('');
      loadChannels();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Create failed');
    } finally {
      setCreating(false);
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await api.deleteAlarmChannel(id);
      loadChannels();
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Delete failed');
    }
  };

  const handleTest = async (id: string) => {
    setTesting(id);
    setError(null);
    try {
      await api.testAlarmChannel(id);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Test failed');
    } finally {
      setTesting(null);
    }
  };

  if (loading) return <div className="text-gray-500">Loading...</div>;

  return (
    <div className="space-y-6">
      {error && (
        <div className="p-3 bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400 rounded text-sm">
          {error}
        </div>
      )}

      <div className="flex justify-between items-center">
        <h2 className="text-lg font-medium text-gray-900 dark:text-white">Notification Channels</h2>
        <button
          onClick={() => setShowCreate(!showCreate)}
          className="px-3 py-1.5 bg-blue-600 text-white text-sm rounded hover:bg-blue-700"
        >
          {showCreate ? 'Cancel' : '+ New Channel'}
        </button>
      </div>

      {showCreate && (
        <form onSubmit={handleCreate} className="bg-gray-50 dark:bg-gray-800 rounded-lg p-4 space-y-4">
          <div>
            <label className="block text-sm text-gray-500 mb-1">Channel Name</label>
            <input
              type="text" value={channelName} onChange={e => setChannelName(e.target.value)} required
              className="w-full px-3 py-2 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-white"
              placeholder="e.g. Discord Alerts"
            />
          </div>
          <div>
            <label className="block text-sm text-gray-500 mb-1">Webhook URL</label>
            <input
              type="url" value={webhookUrl} onChange={e => setWebhookUrl(e.target.value)} required
              className="w-full px-3 py-2 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-white"
              placeholder="https://hooks.slack.com/... or https://ntfy.sh/..."
            />
          </div>
          <button type="submit" disabled={creating}
            className="px-4 py-2 bg-blue-600 text-white text-sm rounded hover:bg-blue-700 disabled:opacity-50">
            {creating ? 'Creating...' : 'Create Channel'}
          </button>
        </form>
      )}

      {/* Channels list */}
      <div className="space-y-2">
        {channels.length === 0 ? (
          <p className="text-gray-500 text-sm text-center py-4">
            No notification channels configured. Alarms will still fire but no notifications will be sent.
          </p>
        ) : channels.map(ch => {
          let url = '';
          try { url = JSON.parse(ch.config)?.url || ''; } catch {}
          return (
            <div key={ch.id} className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-4">
              <div className="flex items-center justify-between">
                <div>
                  <span className="font-medium text-gray-900 dark:text-white">{ch.name}</span>
                  <div className="text-xs text-gray-500">
                    {ch.type} &middot; {url.length > 50 ? url.slice(0, 50) + '...' : url}
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => handleTest(ch.id)}
                    disabled={testing === ch.id}
                    className="text-xs px-2 py-1 bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300 rounded hover:bg-gray-200 dark:hover:bg-gray-600 disabled:opacity-50"
                  >
                    {testing === ch.id ? 'Testing...' : 'Test'}
                  </button>
                  <button
                    onClick={() => handleDelete(ch.id)}
                    className="text-xs text-red-500 hover:text-red-700"
                  >
                    Delete
                  </button>
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

// RBAC Tab - Roles & Permission Assignments
function RBACTab() {
  const [roles, setRoles] = useState<RBACRole[]>([]);
  const [assignments, setAssignments] = useState<RBACRoleAssignment[]>([]);
  const [users, setUsers] = useState<{ id: string; username: string; role: string }[]>([]);
  const [allPerms, setAllPerms] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreateRole, setShowCreateRole] = useState(false);
  const [showAssign, setShowAssign] = useState(false);
  const [err, setErr] = useState('');

  // Create role form
  const [newRoleName, setNewRoleName] = useState('');
  const [newRoleDesc, setNewRoleDesc] = useState('');
  const [newRolePerms, setNewRolePerms] = useState<string[]>([]);

  // Assign role form
  const [assignUserId, setAssignUserId] = useState('');
  const [assignRoleId, setAssignRoleId] = useState('');
  const [assignObjType, setAssignObjType] = useState('root');
  const [assignObjId, setAssignObjId] = useState('');
  const [assignPropagate, setAssignPropagate] = useState(true);

  const reload = async () => {
    setLoading(true);
    try {
      const [r, a, p] = await Promise.all([
        api.getRoles(),
        api.getRoleAssignments(),
        api.getAllPermissions(),
      ]);
      setRoles(r);
      setAssignments(a);
      setAllPerms(p);
      // Load users for assignment dropdown
      try {
        const u = await authApi.listUsers();
        setUsers(u);
      } catch {
        // Non-admin may not be able to list users
      }
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : 'Failed to load RBAC data');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { reload(); }, []);

  const createRole = async () => {
    setErr('');
    try {
      await api.createRole({ name: newRoleName, description: newRoleDesc, permissions: newRolePerms });
      setShowCreateRole(false);
      setNewRoleName(''); setNewRoleDesc(''); setNewRolePerms([]);
      reload();
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : 'Failed to create role');
    }
  };

  const deleteRole = async (id: string) => {
    if (!confirm('Delete this custom role? All assignments using it will be removed.')) return;
    try {
      await api.deleteRole(id);
      reload();
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : 'Failed to delete role');
    }
  };

  const assignRole = async () => {
    setErr('');
    try {
      await api.createRoleAssignment({
        user_id: assignUserId,
        role_id: assignRoleId,
        object_type: assignObjType,
        object_id: assignObjId,
        propagate: assignPropagate,
      });
      setShowAssign(false);
      reload();
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : 'Failed to assign role');
    }
  };

  const deleteAssignment = async (id: string) => {
    try {
      await api.deleteRoleAssignment(id);
      reload();
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : 'Failed to remove assignment');
    }
  };

  const togglePerm = (perm: string) => {
    setNewRolePerms(prev => prev.includes(perm) ? prev.filter(p => p !== perm) : [...prev, perm]);
  };

  if (loading) return <div className="text-gray-500 py-4">Loading...</div>;

  const OBJ_TYPES = ['root', 'datacenter', 'cluster', 'node', 'vm', 'ct', 'storage'];

  return (
    <div className="space-y-6">
      {err && <div className="text-red-500 text-sm bg-red-50 dark:bg-red-900/20 p-3 rounded">{err}</div>}

      {/* Roles */}
      <div>
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Roles</h2>
          <button onClick={() => setShowCreateRole(!showCreateRole)}
            className="px-3 py-1 text-sm bg-blue-600 text-white rounded hover:bg-blue-700">
            + New Role
          </button>
        </div>

        {showCreateRole && (
          <div className="mb-4 p-4 border border-blue-200 dark:border-blue-800 rounded bg-blue-50/50 dark:bg-blue-900/20">
            <div className="space-y-3">
              <input value={newRoleName} onChange={e => setNewRoleName(e.target.value)}
                placeholder="Role name" className="block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-3 py-2 text-sm text-gray-900 dark:text-white" />
              <input value={newRoleDesc} onChange={e => setNewRoleDesc(e.target.value)}
                placeholder="Description" className="block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-3 py-2 text-sm text-gray-900 dark:text-white" />
              <div>
                <span className="text-sm text-gray-500 mb-1 block">Permissions</span>
                <div className="flex flex-wrap gap-1">
                  {allPerms.map(p => (
                    <button key={p} onClick={() => togglePerm(p)}
                      className={`px-2 py-0.5 text-xs rounded border ${
                        newRolePerms.includes(p)
                          ? 'bg-blue-100 dark:bg-blue-900 border-blue-400 text-blue-800 dark:text-blue-200'
                          : 'bg-gray-100 dark:bg-gray-800 border-gray-300 dark:border-gray-600 text-gray-600 dark:text-gray-400'
                      }`}>
                      {p}
                    </button>
                  ))}
                </div>
              </div>
              <div className="flex gap-2">
                <button onClick={createRole} disabled={!newRoleName || newRolePerms.length === 0}
                  className="px-3 py-1 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50">Create</button>
                <button onClick={() => setShowCreateRole(false)}
                  className="px-3 py-1 text-sm text-gray-600 dark:text-gray-400">Cancel</button>
              </div>
            </div>
          </div>
        )}

        <div className="space-y-2">
          {roles.map(role => (
            <div key={role.id} className="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-3">
              <div className="flex items-center justify-between">
                <div>
                  <span className="font-medium text-gray-900 dark:text-white">{role.name}</span>
                  {role.builtin && <span className="ml-2 text-xs bg-gray-200 dark:bg-gray-700 text-gray-600 dark:text-gray-400 px-1.5 py-0.5 rounded">built-in</span>}
                  <p className="text-xs text-gray-500 mt-0.5">{role.description}</p>
                </div>
                {!role.builtin && (
                  <button onClick={() => deleteRole(role.id)} className="text-xs text-red-600 hover:text-red-700">Delete</button>
                )}
              </div>
              <div className="mt-2 flex flex-wrap gap-1">
                {role.permissions.map(p => (
                  <span key={p} className="px-1.5 py-0.5 text-xs bg-blue-50 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300 rounded">
                    {p}
                  </span>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Assignments */}
      <div>
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Role Assignments</h2>
          <button onClick={() => { setShowAssign(!showAssign); if (roles.length > 0) setAssignRoleId(roles[0].id); if (users.length > 0) setAssignUserId(users[0].id); }}
            className="px-3 py-1 text-sm bg-blue-600 text-white rounded hover:bg-blue-700">
            + Assign Role
          </button>
        </div>

        {showAssign && (
          <div className="mb-4 p-4 border border-blue-200 dark:border-blue-800 rounded bg-blue-50/50 dark:bg-blue-900/20">
            <div className="grid grid-cols-2 gap-3 text-sm">
              <label className="block">
                <span className="text-gray-500 text-xs">User</span>
                <select value={assignUserId} onChange={e => setAssignUserId(e.target.value)}
                  className="mt-0.5 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm text-gray-900 dark:text-white">
                  {users.map(u => <option key={u.id} value={u.id}>{u.username}</option>)}
                </select>
              </label>
              <label className="block">
                <span className="text-gray-500 text-xs">Role</span>
                <select value={assignRoleId} onChange={e => setAssignRoleId(e.target.value)}
                  className="mt-0.5 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm text-gray-900 dark:text-white">
                  {roles.map(r => <option key={r.id} value={r.id}>{r.name}</option>)}
                </select>
              </label>
              <label className="block">
                <span className="text-gray-500 text-xs">Object Type</span>
                <select value={assignObjType} onChange={e => setAssignObjType(e.target.value)}
                  className="mt-0.5 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm text-gray-900 dark:text-white">
                  {OBJ_TYPES.map(t => <option key={t} value={t}>{t}</option>)}
                </select>
              </label>
              <label className="block">
                <span className="text-gray-500 text-xs">Object ID {assignObjType === 'root' ? '(leave empty)' : ''}</span>
                <input value={assignObjId} onChange={e => setAssignObjId(e.target.value)}
                  disabled={assignObjType === 'root'}
                  placeholder={assignObjType === 'root' ? '' : 'e.g. cluster name, vmid'}
                  className="mt-0.5 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm text-gray-900 dark:text-white disabled:opacity-50" />
              </label>
              <label className="flex items-center gap-2 col-span-2">
                <input type="checkbox" checked={assignPropagate} onChange={e => setAssignPropagate(e.target.checked)} />
                <span className="text-gray-500 text-xs">Propagate to child objects</span>
              </label>
            </div>
            <div className="flex gap-2 mt-3">
              <button onClick={assignRole} disabled={!assignUserId || !assignRoleId}
                className="px-3 py-1 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50">Assign</button>
              <button onClick={() => setShowAssign(false)}
                className="px-3 py-1 text-sm text-gray-600 dark:text-gray-400">Cancel</button>
            </div>
          </div>
        )}

        {assignments.length === 0 ? (
          <div className="text-sm text-gray-500 bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-4">
            No role assignments yet. Admin users bypass RBAC. Assign roles to non-admin users to grant access.
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-gray-500 border-b border-gray-200 dark:border-gray-700">
                  <th className="pb-2 pr-4">User</th>
                  <th className="pb-2 pr-4">Role</th>
                  <th className="pb-2 pr-4">Scope</th>
                  <th className="pb-2 pr-4">Propagate</th>
                  <th className="pb-2"></th>
                </tr>
              </thead>
              <tbody className="text-gray-900 dark:text-white">
                {assignments.map(a => (
                  <tr key={a.id} className="border-b border-gray-100 dark:border-gray-700/50">
                    <td className="py-2 pr-4">{a.username || a.user_id}</td>
                    <td className="py-2 pr-4">{a.role_name || a.role_id}</td>
                    <td className="py-2 pr-4 font-mono text-xs">
                      {a.object_type}{a.object_id ? `:${a.object_id}` : ''}
                    </td>
                    <td className="py-2 pr-4">{a.propagate ? 'Yes' : 'No'}</td>
                    <td className="py-2 text-right">
                      <button onClick={() => deleteAssignment(a.id)} className="text-xs text-red-600 hover:text-red-700">Remove</button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}

// Scheduler Tab
function SchedulerTab() {
  const [tasks, setTasks] = useState<ScheduledTask[]>([]);
  const [runs, setRuns] = useState<TaskRun[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [err, setErr] = useState('');

  // Create form
  const [name, setName] = useState('');
  const [taskType, setTaskType] = useState('power_off');
  const [targetType, setTargetType] = useState('vm');
  const [targetId, setTargetId] = useState('');
  const [cluster, setCluster] = useState('default');
  const [cronExpr, setCronExpr] = useState('0 2 * * *');
  const [enabled, setEnabled] = useState(true);
  const [retention, setRetention] = useState(7);
  const [backupStorage, setBackupStorage] = useState('');
  const [backupMode, setBackupMode] = useState('snapshot');

  const reload = async () => {
    setLoading(true);
    try {
      const [t, r] = await Promise.all([
        api.getScheduledTasks(),
        api.getTaskRuns(undefined, 20),
      ]);
      setTasks(t);
      setRuns(r);
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : 'Failed to load');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { reload(); }, []);

  const createTask = async () => {
    setErr('');
    try {
      // Build task-specific params payload
      let params: string | undefined;
      if (taskType === 'snapshot_rotate') {
        params = JSON.stringify({ retention });
      } else if (taskType === 'backup_create') {
        if (!backupStorage.trim()) {
          setErr('Storage is required for backup schedules');
          return;
        }
        params = JSON.stringify({ storage: backupStorage.trim(), mode: backupMode });
      }
      await api.createScheduledTask({
        name, task_type: taskType, target_type: targetType,
        target_id: parseInt(targetId), cluster, cron_expr: cronExpr, enabled,
        params,
      });
      setShowCreate(false);
      setName(''); setTargetId('');
      reload();
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : 'Create failed');
    }
  };

  const deleteTask = async (id: string) => {
    if (!confirm('Delete this scheduled task?')) return;
    try { await api.deleteScheduledTask(id); reload(); }
    catch (e: unknown) { setErr(e instanceof Error ? e.message : 'Delete failed'); }
  };

  const toggleTask = async (task: ScheduledTask) => {
    try {
      await api.updateScheduledTask(task.id, {
        name: task.name, cron_expr: task.cron_expr,
        params: task.params, enabled: !task.enabled,
      });
      reload();
    } catch (e: unknown) { setErr(e instanceof Error ? e.message : 'Update failed'); }
  };

  const TASK_TYPES = ['power_on', 'power_off', 'shutdown', 'snapshot_create', 'snapshot_cleanup', 'snapshot_rotate', 'backup_create', 'acme_renew', 'migrate'];
  const CRON_PRESETS = [
    { label: 'Every hour', value: '0 * * * *' },
    { label: 'Daily 2am', value: '0 2 * * *' },
    { label: 'Daily 6pm', value: '0 18 * * *' },
    { label: 'Weekdays 8am', value: '0 8 * * 1-5' },
    { label: 'Weekdays 6pm', value: '0 18 * * 1-5' },
    { label: 'Weekly Sun 3am', value: '0 3 * * 0' },
  ];

  if (loading) return <div className="text-gray-500 py-4">Loading...</div>;

  return (
    <div className="space-y-6">
      {err && <div className="text-red-500 text-sm bg-red-50 dark:bg-red-900/20 p-3 rounded">{err}</div>}

      {/* Tasks */}
      <div>
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Scheduled Tasks</h2>
          <button onClick={() => setShowCreate(!showCreate)}
            className="px-3 py-1 text-sm bg-blue-600 text-white rounded hover:bg-blue-700">
            + New Task
          </button>
        </div>

        {showCreate && (
          <div className="mb-4 p-4 border border-blue-200 dark:border-blue-800 rounded bg-blue-50/50 dark:bg-blue-900/20">
            <div className="grid grid-cols-2 gap-3 text-sm">
              <label className="block col-span-2">
                <span className="text-gray-500 text-xs">Task Name</span>
                <input value={name} onChange={e => setName(e.target.value)}
                  placeholder="e.g. Nightly shutdown dev VMs"
                  className="mt-0.5 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm text-gray-900 dark:text-white" />
              </label>
              <label className="block">
                <span className="text-gray-500 text-xs">Action</span>
                <select value={taskType} onChange={e => setTaskType(e.target.value)}
                  className="mt-0.5 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm text-gray-900 dark:text-white">
                  {TASK_TYPES.map(t => <option key={t} value={t}>{t.replace(/_/g, ' ')}</option>)}
                </select>
              </label>
              <label className="block">
                <span className="text-gray-500 text-xs">Target Type</span>
                <select value={targetType} onChange={e => setTargetType(e.target.value)}
                  className="mt-0.5 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm text-gray-900 dark:text-white">
                  <option value="vm">VM</option>
                  <option value="ct">Container</option>
                </select>
              </label>
              <label className="block">
                <span className="text-gray-500 text-xs">VMID</span>
                <input value={targetId} onChange={e => setTargetId(e.target.value)}
                  placeholder="e.g. 102" type="number"
                  className="mt-0.5 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm text-gray-900 dark:text-white" />
              </label>
              <label className="block">
                <span className="text-gray-500 text-xs">Cluster</span>
                <input value={cluster} onChange={e => setCluster(e.target.value)}
                  className="mt-0.5 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm text-gray-900 dark:text-white" />
              </label>
              <label className="block">
                <span className="text-gray-500 text-xs">Schedule (cron)</span>
                <input value={cronExpr} onChange={e => setCronExpr(e.target.value)}
                  placeholder="minute hour dom month dow"
                  className="mt-0.5 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm font-mono text-gray-900 dark:text-white" />
              </label>
              <div className="flex flex-wrap gap-1 items-end">
                {CRON_PRESETS.map(p => (
                  <button key={p.value} onClick={() => setCronExpr(p.value)}
                    className={`px-1.5 py-0.5 text-[10px] rounded border ${
                      cronExpr === p.value
                        ? 'bg-blue-100 dark:bg-blue-900 border-blue-400 text-blue-800 dark:text-blue-200'
                        : 'bg-gray-100 dark:bg-gray-800 border-gray-300 dark:border-gray-600 text-gray-600 dark:text-gray-400'
                    }`}>{p.label}</button>
                ))}
              </div>
              {taskType === 'snapshot_rotate' && (
                <label className="block col-span-2">
                  <span className="text-gray-500 text-xs">Retention (keep last N auto-* snapshots)</span>
                  <input type="number" min={1} max={100} value={retention}
                    onChange={e => setRetention(Math.max(1, parseInt(e.target.value) || 1))}
                    className="mt-0.5 block w-32 rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm text-gray-900 dark:text-white" />
                  <span className="text-[10px] text-gray-500 block mt-1">
                    Each run creates a new <code>auto-YYYYMMDD-HHMMSS</code> snapshot and prunes older auto-* beyond this count.
                  </span>
                </label>
              )}
              {taskType === 'acme_renew' && (
                <div className="col-span-2 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded px-3 py-2">
                  <span className="text-xs text-blue-800 dark:text-blue-300">
                    Cluster-wide task — renews ACME certs on every online node in the cluster. Target VM/CT fields are ignored (set to any valid value).
                  </span>
                </div>
              )}
              {taskType === 'backup_create' && (
                <>
                  <label className="block">
                    <span className="text-gray-500 text-xs">Backup Storage <span className="text-red-500">*</span></span>
                    <input type="text" value={backupStorage} onChange={e => setBackupStorage(e.target.value)}
                      placeholder="e.g. local-zfs or PBS-01"
                      className="mt-0.5 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm text-gray-900 dark:text-white" />
                  </label>
                  <label className="block">
                    <span className="text-gray-500 text-xs">Mode</span>
                    <select value={backupMode} onChange={e => setBackupMode(e.target.value)}
                      className="mt-0.5 block w-full rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm text-gray-900 dark:text-white">
                      <option value="snapshot">snapshot</option>
                      <option value="suspend">suspend</option>
                      <option value="stop">stop</option>
                    </select>
                  </label>
                </>
              )}
              <label className="flex items-center gap-2 col-span-2">
                <input type="checkbox" checked={enabled} onChange={e => setEnabled(e.target.checked)} />
                <span className="text-gray-500 text-xs">Enabled</span>
              </label>
            </div>
            <div className="flex gap-2 mt-3">
              <button onClick={createTask} disabled={!name || !targetId}
                className="px-3 py-1 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50">Create</button>
              <button onClick={() => setShowCreate(false)}
                className="px-3 py-1 text-sm text-gray-600 dark:text-gray-400">Cancel</button>
            </div>
          </div>
        )}

        {tasks.length === 0 ? (
          <div className="text-sm text-gray-500 bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-4">
            No scheduled tasks. Create one to automate power ops, snapshots, or migrations.
          </div>
        ) : (
          <div className="space-y-2">
            {tasks.map(task => (
              <div key={task.id} className="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-3">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <button onClick={() => toggleTask(task)}
                      className={`w-8 h-4 rounded-full transition-colors ${task.enabled ? 'bg-green-500' : 'bg-gray-400'}`}>
                      <div className={`w-3 h-3 bg-white rounded-full transition-transform mx-0.5 ${task.enabled ? 'translate-x-4' : ''}`} />
                    </button>
                    <div>
                      <span className="font-medium text-gray-900 dark:text-white text-sm">{task.name}</span>
                      <div className="text-xs text-gray-500">
                        {task.task_type.replace(/_/g, ' ')} &middot; {task.target_type} {task.target_id} &middot; {task.cluster}
                      </div>
                    </div>
                  </div>
                  <div className="flex items-center gap-3 text-xs text-gray-500">
                    <span className="font-mono">{task.cron_expr}</span>
                    {task.next_run && (
                      <span title="Next run">Next: {new Date(task.next_run).toLocaleString()}</span>
                    )}
                    <button onClick={() => deleteTask(task.id)} className="text-red-600 hover:text-red-700">Delete</button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Recent Runs */}
      <div>
        <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-3">Recent Runs</h2>
        {runs.length === 0 ? (
          <div className="text-sm text-gray-500 bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-4">
            No task runs yet.
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-gray-500 border-b border-gray-200 dark:border-gray-700">
                  <th className="pb-2 pr-4">Task</th>
                  <th className="pb-2 pr-4">Time</th>
                  <th className="pb-2 pr-4">Duration</th>
                  <th className="pb-2 pr-4">Result</th>
                  <th className="pb-2">Details</th>
                </tr>
              </thead>
              <tbody className="text-gray-900 dark:text-white">
                {runs.map(run => (
                  <tr key={run.id} className="border-b border-gray-100 dark:border-gray-700/50">
                    <td className="py-2 pr-4">{run.task_name || run.task_id}</td>
                    <td className="py-2 pr-4 text-xs">{new Date(run.started_at).toLocaleString()}</td>
                    <td className="py-2 pr-4 text-xs">{run.duration_ms}ms</td>
                    <td className="py-2 pr-4">
                      <span className={`text-xs px-1.5 py-0.5 rounded ${run.success ? 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400' : 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400'}`}>
                        {run.success ? 'OK' : 'FAIL'}
                      </span>
                    </td>
                    <td className="py-2 text-xs text-gray-500">{run.error || run.upid || ''}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
