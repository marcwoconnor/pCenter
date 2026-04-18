import { useEffect, useState, type FormEvent } from 'react';
import { api } from '../api/client';
import type { WebhookEndpoint, CreateWebhookRequest, UpdateWebhookRequest } from '../types';

// Suggested event names. Free-form — users can type arbitrary names if they
// want to subscribe to events we don't list here yet.
const SUGGESTED_EVENTS = [
  'vm.create', 'vm.delete', 'vm.start', 'vm.stop', 'vm.shutdown', 'vm.migrate',
  'ct.create', 'ct.delete', 'ct.start', 'ct.stop', 'ct.shutdown', 'ct.migrate',
  'ha.enable', 'ha.disable',
  'drs.apply', 'drs.dismiss',
  'folder.create', 'folder.rename', 'folder.delete', 'folder.move',
  'activity.config_update',
];

export function WebhooksPanel() {
  const [items, setItems] = useState<WebhookEndpoint[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState<WebhookEndpoint | null>(null);
  const [creating, setCreating] = useState(false);
  const [newSecret, setNewSecret] = useState<{ name: string; secret: string } | null>(null);

  const reload = async () => {
    try {
      setItems(await api.listWebhooks());
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'failed to load webhooks');
    }
  };

  useEffect(() => {
    reload();
  }, []);

  const onDelete = async (w: WebhookEndpoint) => {
    if (!confirm(`Delete webhook "${w.name}"? Receivers will stop getting events immediately.`)) return;
    await api.deleteWebhook(w.id);
    reload();
  };

  const onTest = async (w: WebhookEndpoint) => {
    try {
      await api.testWebhook(w.id);
      // Give the dispatcher a moment, then refresh to show last_status.
      setTimeout(reload, 1500);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'test failed');
    }
  };

  const onToggleEnabled = async (w: WebhookEndpoint) => {
    await api.updateWebhook(w.id, {
      name: w.name, url: w.url, events: w.events, enabled: !w.enabled,
    });
    reload();
  };

  return (
    <div className="space-y-4">
      <div className="flex justify-between items-center">
        <div>
          <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Outbound Webhooks</h2>
          <p className="text-sm text-gray-500 dark:text-gray-400">
            Fire HTTP POST requests to external systems when activity events occur.
            Requests are signed with HMAC-SHA256 (see <code className="text-xs">X-pCenter-Signature</code> header).
          </p>
        </div>
        <button
          onClick={() => setCreating(true)}
          className="bg-blue-600 hover:bg-blue-700 text-white px-4 py-2 rounded text-sm font-medium"
        >
          Add Endpoint
        </button>
      </div>

      {error && <div className="text-red-600 dark:text-red-400 text-sm">{error}</div>}

      {items === null ? (
        <div className="text-sm text-gray-500">Loading…</div>
      ) : items.length === 0 ? (
        <div className="text-sm text-gray-500 border border-dashed border-gray-300 dark:border-gray-600 rounded p-6 text-center">
          No webhooks configured yet.
        </div>
      ) : (
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left border-b border-gray-200 dark:border-gray-700">
              <th className="py-2">Name</th>
              <th className="py-2">URL</th>
              <th className="py-2">Events</th>
              <th className="py-2">Last delivery</th>
              <th className="py-2">Status</th>
              <th className="py-2 text-right">Actions</th>
            </tr>
          </thead>
          <tbody>
            {items.map((w) => (
              <tr key={w.id} className="border-b border-gray-100 dark:border-gray-800">
                <td className="py-2 font-medium">{w.name}</td>
                <td className="py-2 font-mono text-xs truncate max-w-xs" title={w.url}>{w.url}</td>
                <td className="py-2 text-xs">
                  {w.events.length === 0 ? <span className="text-gray-500">all events</span> : w.events.join(', ')}
                </td>
                <td className="py-2 text-xs text-gray-500">
                  {w.last_fired_at ? new Date(w.last_fired_at).toLocaleString() : '—'}
                </td>
                <td className="py-2">
                  <StatusBadge enabled={w.enabled} lastStatus={w.last_status} />
                </td>
                <td className="py-2 text-right space-x-2">
                  <button
                    onClick={() => onToggleEnabled(w)}
                    className="text-xs text-gray-600 dark:text-gray-400 hover:underline"
                  >
                    {w.enabled ? 'Disable' : 'Enable'}
                  </button>
                  <button onClick={() => onTest(w)} className="text-xs text-blue-600 hover:underline">Test</button>
                  <button onClick={() => setEditing(w)} className="text-xs text-gray-600 dark:text-gray-400 hover:underline">Edit</button>
                  <button onClick={() => onDelete(w)} className="text-xs text-red-600 hover:underline">Delete</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {creating && (
        <WebhookDialog
          onClose={() => setCreating(false)}
          onSaved={(secret, name) => {
            setCreating(false);
            if (secret) setNewSecret({ name, secret });
            reload();
          }}
        />
      )}
      {editing && (
        <WebhookDialog
          endpoint={editing}
          onClose={() => setEditing(null)}
          onSaved={() => { setEditing(null); reload(); }}
        />
      )}
      {newSecret && <SecretRevealDialog name={newSecret.name} secret={newSecret.secret} onClose={() => setNewSecret(null)} />}
    </div>
  );
}

function StatusBadge({ enabled, lastStatus }: { enabled: boolean; lastStatus?: string }) {
  if (!enabled) {
    return <span className="text-xs px-2 py-0.5 bg-gray-200 dark:bg-gray-700 rounded">disabled</span>;
  }
  if (lastStatus === 'success') {
    return <span className="text-xs px-2 py-0.5 bg-green-100 dark:bg-green-900 text-green-800 dark:text-green-200 rounded">ok</span>;
  }
  if (lastStatus === 'failure') {
    return <span className="text-xs px-2 py-0.5 bg-red-100 dark:bg-red-900 text-red-800 dark:text-red-200 rounded">failing</span>;
  }
  return <span className="text-xs px-2 py-0.5 bg-blue-100 dark:bg-blue-900 text-blue-800 dark:text-blue-200 rounded">enabled</span>;
}

function WebhookDialog({
  endpoint, onClose, onSaved,
}: {
  endpoint?: WebhookEndpoint;
  onClose: () => void;
  onSaved: (secret: string | null, name: string) => void;
}) {
  const [name, setName] = useState(endpoint?.name ?? '');
  const [url, setUrl] = useState(endpoint?.url ?? '');
  const [events, setEvents] = useState<string[]>(endpoint?.events ?? []);
  const [eventInput, setEventInput] = useState('');
  const [enabled, setEnabled] = useState(endpoint?.enabled ?? true);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const addEvent = (e: string) => {
    const v = e.trim();
    if (!v || events.includes(v)) return;
    setEvents([...events, v]);
    setEventInput('');
  };

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setErr(null);
    try {
      if (endpoint) {
        const req: UpdateWebhookRequest = { name, url, events, enabled };
        await api.updateWebhook(endpoint.id, req);
        onSaved(null, name);
      } else {
        const req: CreateWebhookRequest = { name, url, events, enabled };
        const resp = await api.createWebhook(req);
        onSaved(resp.secret, resp.endpoint.name);
      }
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'save failed');
      setBusy(false);
    }
  };

  return (
    <DialogShell title={endpoint ? 'Edit webhook' : 'New webhook'} onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        <Field label="Name">
          <input
            type="text" required value={name} onChange={(e) => setName(e.target.value)}
            className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800"
          />
        </Field>
        <Field label="URL" hint="Must start with http:// or https://">
          <input
            type="url" required value={url} onChange={(e) => setUrl(e.target.value)}
            className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 font-mono text-sm"
            placeholder="https://example.com/hooks/pcenter"
          />
        </Field>
        <Field label="Event filter" hint="Leave empty to receive all events. Common names shown below.">
          <div className="flex gap-2">
            <input
              type="text" value={eventInput}
              onChange={(e) => setEventInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  addEvent(eventInput);
                }
              }}
              className="flex-1 px-3 py-2 border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 font-mono text-sm"
              placeholder="e.g. vm.create"
            />
            <button
              type="button" onClick={() => addEvent(eventInput)}
              className="px-3 py-2 bg-gray-200 dark:bg-gray-700 rounded text-sm"
            >
              Add
            </button>
          </div>
          {events.length > 0 && (
            <div className="mt-2 flex flex-wrap gap-1">
              {events.map((ev) => (
                <span key={ev} className="inline-flex items-center gap-1 px-2 py-0.5 bg-blue-100 dark:bg-blue-900 text-blue-800 dark:text-blue-200 rounded text-xs">
                  {ev}
                  <button type="button" onClick={() => setEvents(events.filter((x) => x !== ev))} className="hover:text-red-600">×</button>
                </span>
              ))}
            </div>
          )}
          <div className="mt-2 flex flex-wrap gap-1">
            {SUGGESTED_EVENTS.filter((e) => !events.includes(e)).slice(0, 8).map((ev) => (
              <button
                key={ev} type="button" onClick={() => addEvent(ev)}
                className="text-xs px-1.5 py-0.5 text-gray-500 hover:text-blue-600 border border-gray-200 dark:border-gray-700 rounded"
              >
                + {ev}
              </button>
            ))}
          </div>
        </Field>
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
          <span>Enabled</span>
        </label>

        {err && <div className="text-red-600 text-sm">{err}</div>}

        <div className="flex justify-end gap-2 pt-2 border-t border-gray-200 dark:border-gray-700">
          <button type="button" onClick={onClose} className="px-4 py-2 text-sm">Cancel</button>
          <button type="submit" disabled={busy} className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded text-sm disabled:opacity-50">
            {busy ? 'Saving…' : endpoint ? 'Save' : 'Create'}
          </button>
        </div>
      </form>
    </DialogShell>
  );
}

function SecretRevealDialog({ name, secret, onClose }: { name: string; secret: string; onClose: () => void }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    await navigator.clipboard.writeText(secret);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };
  return (
    <DialogShell title={`Secret for "${name}"`} onClose={onClose}>
      <div className="space-y-4 text-sm">
        <div className="p-3 bg-yellow-50 dark:bg-yellow-900/30 border border-yellow-200 dark:border-yellow-800 rounded">
          <strong>Store this secret now.</strong> It will not be shown again.
          You'll need it on the receiving side to verify signatures.
        </div>
        <div className="font-mono text-xs bg-gray-100 dark:bg-gray-800 p-3 rounded break-all select-all">
          {secret}
        </div>
        <div className="text-xs text-gray-600 dark:text-gray-400">
          Verify incoming requests by recomputing: <code>hmac_sha256(secret, "&lt;unix_ts&gt;.&lt;body&gt;")</code>,
          where <code>&lt;unix_ts&gt;</code> comes from the <code>t=</code> field of the
          <code> X-pCenter-Signature</code> header and the hex digest is the <code>v1=</code> field.
        </div>
        <div className="flex justify-end gap-2">
          <button onClick={copy} className="px-4 py-2 bg-gray-200 dark:bg-gray-700 rounded text-sm">
            {copied ? 'Copied' : 'Copy'}
          </button>
          <button onClick={onClose} className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded text-sm">Done</button>
        </div>
      </div>
    </DialogShell>
  );
}

function DialogShell({ title, children, onClose }: { title: string; children: React.ReactNode; onClose: () => void }) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={onClose}>
      <div
        className="bg-white dark:bg-gray-900 rounded-lg shadow-xl w-full max-w-lg p-6 max-h-[90vh] overflow-auto"
        onClick={(e) => e.stopPropagation()}
      >
        <h3 className="text-lg font-semibold mb-4 text-gray-900 dark:text-white">{title}</h3>
        {children}
      </div>
    </div>
  );
}

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div>
      <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">{label}</label>
      {children}
      {hint && <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">{hint}</p>}
    </div>
  );
}
