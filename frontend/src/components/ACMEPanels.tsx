import { useState, useEffect, useCallback } from 'react';
import { api } from '../api/client';
import { useCluster } from '../context/ClusterContext';
import type {
  NodeCertificate,
  ACMEAccount,
  ACMEPlugin,
  ACMEDirectory,
  ACMEChallengeSchema,
  NodeACMEDomain,
} from '../types';

function formatUnixDate(secs?: number): string {
  if (!secs) return '—';
  return new Date(secs * 1000).toLocaleString();
}

function daysUntil(secs?: number): number | null {
  if (!secs) return null;
  const ms = secs * 1000 - Date.now();
  return Math.floor(ms / (24 * 60 * 60 * 1000));
}

function expiryBadge(notafter?: number) {
  const days = daysUntil(notafter);
  if (days === null) return null;
  if (days < 0) {
    return <span className="px-2 py-0.5 rounded text-xs bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400">Expired {-days}d ago</span>;
  }
  if (days < 7) {
    return <span className="px-2 py-0.5 rounded text-xs bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400">Expires in {days}d</span>;
  }
  if (days < 30) {
    return <span className="px-2 py-0.5 rounded text-xs bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400">Expires in {days}d</span>;
  }
  return <span className="px-2 py-0.5 rounded text-xs bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400">Valid · {days}d left</span>;
}

interface ExpiringCert { cluster: string; node: string; filename: string; daysLeft: number; notafter?: number }

export function CertExpiryBanner({ nodes }: { nodes: Array<{ cluster: string; node: string }> }) {
  const [expiring, setExpiring] = useState<ExpiringCert[]>([]);
  const key = nodes.map(n => `${n.cluster}/${n.node}`).sort().join(',');

  useEffect(() => {
    if (!key) return;
    let cancelled = false;
    (async () => {
      const results = await Promise.all(nodes.map(async n => {
        try {
          const certs = await api.getNodeCertificates(n.cluster, n.node);
          return certs.map(c => ({ cluster: n.cluster, node: n.node, filename: c.filename, notafter: c.notafter, daysLeft: daysUntil(c.notafter) ?? 99999 }));
        } catch {
          return [];
        }
      }));
      if (cancelled) return;
      const flat = results.flat().filter(c => c.daysLeft < 30);
      // keep only the earliest-expiring cert per node
      const byNode = new Map<string, ExpiringCert>();
      for (const c of flat) {
        const k = `${c.cluster}/${c.node}`;
        const prev = byNode.get(k);
        if (!prev || c.daysLeft < prev.daysLeft) byNode.set(k, c);
      }
      setExpiring(Array.from(byNode.values()).sort((a, b) => a.daysLeft - b.daysLeft));
    })();
    return () => { cancelled = true };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);

  if (expiring.length === 0) return null;

  const worst = expiring[0].daysLeft;
  const critical = worst < 7;
  return (
    <div className={`mb-4 p-3 rounded-lg border ${critical
      ? 'bg-red-50 dark:bg-red-900/20 border-red-200 dark:border-red-800'
      : 'bg-yellow-50 dark:bg-yellow-900/20 border-yellow-200 dark:border-yellow-800'}`}>
      <div className="flex items-center gap-3">
        <span className="text-xl">🔒</span>
        <div className="flex-1">
          <div className={`font-medium ${critical ? 'text-red-700 dark:text-red-400' : 'text-yellow-700 dark:text-yellow-400'}`}>
            {expiring.length === 1
              ? `${expiring[0].node}: certificate ${expiring[0].daysLeft < 0 ? 'expired' : `expires in ${expiring[0].daysLeft}d`}`
              : `${expiring.length} nodes have certificates expiring within 30 days`}
          </div>
          <div className="text-xs text-gray-600 dark:text-gray-400">
            {expiring.slice(0, 3).map(c => `${c.node} (${c.daysLeft < 0 ? 'expired' : `${c.daysLeft}d`})`).join(' · ')}
            {expiring.length > 3 ? ` · and ${expiring.length - 3} more` : ''}
          </div>
        </div>
      </div>
    </div>
  );
}

export function NodeCertificatesTab({ cluster, node }: { cluster: string; node: string }) {
  const [certs, setCerts] = useState<NodeCertificate[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [renewing, setRenewing] = useState(false);
  const [editDomains, setEditDomains] = useState(false);
  const [uploadCert, setUploadCert] = useState(false);

  const load = useCallback(async () => {
    try {
      setErr(null);
      const data = await api.getNodeCertificates(cluster, node);
      setCerts(data);
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'load failed');
      setCerts([]);
    }
  }, [cluster, node]);

  useEffect(() => { load(); }, [load]);

  const onRenew = async () => {
    if (!confirm(`Trigger ACME certificate renewal on ${node}?\n\nRequires an ACME account and a working challenge plugin already configured for this cluster.`)) return;
    try {
      setRenewing(true);
      await api.renewNodeACMECertificate(cluster, node);
      alert('Renewal task started. Check the tasks bar for progress. The cert list will update after PVE reloads the cert.');
      setTimeout(load, 3000);
    } catch (e) {
      alert('Renewal failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      setRenewing(false);
    }
  };

  const onDeleteCustom = async () => {
    if (!confirm(`Remove the custom certificate on ${node} and revert to the default self-signed cert?\n\npveproxy will restart.`)) return;
    try {
      await api.deleteNodeCustomCertificate(cluster, node, true);
      setTimeout(load, 2000);
    } catch (e) {
      alert('Delete failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  };

  if (certs === null) return <div className="text-gray-500 p-4">Loading certificates…</div>;
  if (err) return <div className="bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-400 rounded p-4">{err}</div>;

  const acmeEligible = certs.some(c => c.filename === 'pveproxy-ssl.pem' || c.filename === 'pve-ssl.pem');
  // Heuristic: a non-default cert exists if pveproxy-ssl.pem's issuer doesn't say "Proxmox".
  const pveproxyCert = certs.find(c => c.filename === 'pveproxy-ssl.pem');
  const hasCustomCert = !!pveproxyCert && !!pveproxyCert.issuer && !/Proxmox/i.test(pveproxyCert.issuer);

  return (
    <div className="space-y-4">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4 flex items-center justify-between">
        <div>
          <h3 className="font-medium text-gray-900 dark:text-white">Certificates on {node}</h3>
          <p className="text-sm text-gray-500">ACME-issued certs are installed at <code>pveproxy-ssl.pem</code>. Renewal works only if the cluster has an ACME account and challenge plugin configured.</p>
        </div>
        <div className="flex gap-2">
          <button
            onClick={() => setEditDomains(true)}
            className="px-3 py-1.5 rounded border border-gray-300 dark:border-gray-600 hover:bg-gray-50 dark:hover:bg-gray-700 text-gray-700 dark:text-gray-200 text-sm"
          >
            Configure Domains
          </button>
          <button
            onClick={() => setUploadCert(true)}
            className="px-3 py-1.5 rounded border border-gray-300 dark:border-gray-600 hover:bg-gray-50 dark:hover:bg-gray-700 text-gray-700 dark:text-gray-200 text-sm"
          >
            Upload Custom Cert
          </button>
          {hasCustomCert && (
            <button
              onClick={onDeleteCustom}
              className="px-3 py-1.5 rounded border border-red-300 dark:border-red-700 hover:bg-red-50 dark:hover:bg-red-900/20 text-red-700 dark:text-red-400 text-sm"
              title="Remove custom cert and revert to default self-signed"
            >
              Revert to Self-Signed
            </button>
          )}
          <button
            onClick={onRenew}
            disabled={renewing || !acmeEligible}
            className="px-3 py-1.5 rounded bg-blue-600 hover:bg-blue-700 text-white text-sm disabled:bg-gray-400 disabled:cursor-not-allowed"
            title={acmeEligible ? 'Request/renew ACME cert' : 'No renewable cert found'}
          >
            {renewing ? 'Renewing…' : 'Renew ACME Cert'}
          </button>
        </div>
      </div>

      {certs.length === 0 && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-6 text-center text-gray-500">No certificate info returned.</div>
      )}

      {certs.map(c => (
        <div key={c.filename} className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <div className="flex items-center justify-between mb-2">
            <div className="font-mono text-sm text-gray-900 dark:text-white">{c.filename}</div>
            {expiryBadge(c.notafter)}
          </div>
          <div className="grid md:grid-cols-2 gap-x-4 gap-y-1 text-sm">
            <div><span className="text-gray-500">Subject: </span><span className="text-gray-900 dark:text-white break-all">{c.subject || '—'}</span></div>
            <div><span className="text-gray-500">Issuer: </span><span className="text-gray-900 dark:text-white break-all">{c.issuer || '—'}</span></div>
            <div><span className="text-gray-500">Not Before: </span><span className="text-gray-900 dark:text-white">{formatUnixDate(c.notbefore)}</span></div>
            <div><span className="text-gray-500">Not After: </span><span className="text-gray-900 dark:text-white">{formatUnixDate(c.notafter)}</span></div>
            <div><span className="text-gray-500">Public Key: </span><span className="text-gray-900 dark:text-white">{c.public_key_type || '—'} {c.public_key_bits ? `${c.public_key_bits}b` : ''}</span></div>
            <div className="md:col-span-2">
              <span className="text-gray-500">Fingerprint: </span>
              <span className="text-gray-900 dark:text-white font-mono text-xs break-all">{c.fingerprint || '—'}</span>
            </div>
            {c.san && c.san.length > 0 && (
              <div className="md:col-span-2">
                <span className="text-gray-500">SAN: </span>
                <span className="text-gray-900 dark:text-white">{c.san.join(', ')}</span>
              </div>
            )}
          </div>
        </div>
      ))}

      {editDomains && (
        <NodeDomainsDialog
          clusterName={cluster}
          node={node}
          onClose={() => setEditDomains(false)}
          onSuccess={() => { setEditDomains(false); load(); }}
        />
      )}
      {uploadCert && (
        <UploadCustomCertDialog
          cluster={cluster}
          node={node}
          onClose={() => setUploadCert(false)}
          onSuccess={() => { setUploadCert(false); setTimeout(load, 2000); }}
        />
      )}
    </div>
  );
}

function UploadCustomCertDialog({
  cluster, node, onClose, onSuccess,
}: {
  cluster: string;
  node: string;
  onClose: () => void;
  onSuccess: () => void;
}) {
  const [cert, setCert] = useState('');
  const [key, setKey] = useState('');
  const [force, setForce] = useState(true);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const onSave = async () => {
    setErr(null);
    if (!cert.trim()) { setErr('Certificate PEM is required'); return }
    if (!cert.includes('-----BEGIN CERTIFICATE-----')) {
      setErr('Certificate must be PEM-encoded (contain -----BEGIN CERTIFICATE-----)');
      return;
    }
    if (key && !key.includes('-----BEGIN') && !key.includes('PRIVATE KEY-----')) {
      setErr('Private key must be PEM-encoded');
      return;
    }
    if (!confirm(`Install custom certificate on ${node}?\n\nThis will restart pveproxy. Make sure the certificate chain and key are correct — a bad cert can lock you out of the Proxmox UI.`)) return;

    setLoading(true);
    try {
      await api.uploadNodeCustomCertificate(cluster, node, {
        certificates: cert,
        key: key || undefined,
        force,
        restart: true,
      });
      onSuccess();
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'upload failed');
      setLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-3xl p-6 max-h-[90vh] overflow-auto">
        <h2 className="text-lg font-semibold mb-2 text-gray-900 dark:text-white">Upload Custom Certificate — {node}</h2>
        <p className="text-xs text-gray-500 mb-4">
          Paste a PEM-encoded certificate chain (leaf + intermediates) and optionally a private key.
          If no key is provided, the node's existing private key is reused. pveproxy restarts after install.
        </p>
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Certificate Chain (PEM) <span className="text-red-500">*</span></label>
            <textarea
              value={cert}
              onChange={e => setCert(e.target.value)}
              disabled={loading}
              rows={8}
              placeholder="-----BEGIN CERTIFICATE-----&#10;...&#10;-----END CERTIFICATE-----"
              className="w-full border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded px-3 py-2 font-mono text-xs"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Private Key (PEM) — optional</label>
            <textarea
              value={key}
              onChange={e => setKey(e.target.value)}
              disabled={loading}
              rows={6}
              placeholder="-----BEGIN PRIVATE KEY-----&#10;...&#10;-----END PRIVATE KEY-----"
              className="w-full border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded px-3 py-2 font-mono text-xs"
            />
            <p className="text-xs text-gray-500 mt-1">Omit to reuse the node's existing private key (e.g. after just renewing a cert with same key).</p>
          </div>
          <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
            <input type="checkbox" checked={force} onChange={e => setForce(e.target.checked)} disabled={loading} />
            Overwrite existing custom cert (force)
          </label>
          {err && <div className="bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-400 px-3 py-2 rounded text-sm">{err}</div>}
        </div>
        <div className="flex justify-end gap-3 mt-6">
          <button onClick={onClose} disabled={loading}
            className="px-4 py-2 text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded">Cancel</button>
          <button onClick={onSave} disabled={loading}
            className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50">
            {loading ? 'Uploading…' : 'Upload'}
          </button>
        </div>
      </div>
    </div>
  );
}

export function ClusterACMETab({ clusterName }: { clusterName: string }) {
  const { nodes } = useCluster();
  const [accounts, setAccounts] = useState<ACMEAccount[] | null>(null);
  const [plugins, setPlugins] = useState<ACMEPlugin[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [showAddAccount, setShowAddAccount] = useState(false);
  const [showAddPlugin, setShowAddPlugin] = useState(false);
  const [editAccount, setEditAccount] = useState<ACMEAccount | null>(null);
  const [editPlugin, setEditPlugin] = useState<ACMEPlugin | null>(null);
  const [renewAll, setRenewAll] = useState<{ node: string; status: 'pending' | 'done' | 'error'; msg?: string }[] | null>(null);

  const clusterNodes = (nodes || []).filter(n => n.cluster === clusterName && n.status === 'online');

  const load = useCallback(async () => {
    try {
      const [a, p] = await Promise.all([
        api.listACMEAccounts(clusterName),
        api.listACMEPlugins(clusterName),
      ]);
      setAccounts(a);
      setPlugins(p);
      setErr(null);
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'load failed');
      setAccounts([]);
      setPlugins([]);
    }
  }, [clusterName]);

  useEffect(() => { load() }, [load]);

  const onDeleteAccount = async (name: string) => {
    if (!confirm(`Deregister ACME account "${name}"?\n\nThis removes the account from Proxmox. Any certificates already issued remain installed but won't renew through this account.`)) return;
    try {
      await api.deleteACMEAccount(clusterName, name);
      load();
    } catch (e) {
      alert('Delete failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  };

  const onRenewAll = async () => {
    if (clusterNodes.length === 0) { alert('No online nodes to renew on'); return }
    if (!confirm(`Trigger ACME cert renewal on all ${clusterNodes.length} online node(s) in ${clusterName}?\n\nEach node runs independently; failures on one don't block others.`)) return;

    const initial = clusterNodes.map(n => ({ node: n.node, status: 'pending' as const }));
    setRenewAll(initial);

    // Fire renewals in parallel, update status as each completes
    await Promise.all(clusterNodes.map(async (n, i) => {
      try {
        await api.renewNodeACMECertificate(clusterName, n.node);
        setRenewAll(prev => prev && prev.map((e, j) => j === i ? { ...e, status: 'done' } : e));
      } catch (e) {
        const msg = e instanceof Error ? e.message : String(e);
        setRenewAll(prev => prev && prev.map((e, j) => j === i ? { ...e, status: 'error', msg } : e));
      }
    }));
  };

  const onDeletePlugin = async (id: string) => {
    if (!confirm(`Delete challenge plugin "${id}"?\n\nNodes using this plugin will lose the ability to renew until a replacement is configured.`)) return;
    try {
      await api.deleteACMEPlugin(clusterName, id);
      load();
    } catch (e) {
      alert('Delete failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  };

  if (accounts === null || plugins === null) return <div className="text-gray-500 p-4">Loading ACME config…</div>;
  if (err) return <div className="bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-400 rounded p-4">{err}</div>;

  return (
    <div className="space-y-4">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4 flex items-center justify-between">
        <div>
          <h3 className="font-medium text-gray-900 dark:text-white">Bulk Operations</h3>
          <p className="text-xs text-gray-500">Trigger ACME renewal across all online nodes in this cluster.</p>
        </div>
        <button
          onClick={onRenewAll}
          disabled={clusterNodes.length === 0 || (!!renewAll && renewAll.some(r => r.status === 'pending'))}
          className="px-3 py-1.5 bg-blue-600 hover:bg-blue-700 text-white text-sm rounded disabled:bg-gray-400"
        >
          Renew All ({clusterNodes.length} nodes)
        </button>
      </div>

      {renewAll && (
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
          <h4 className="text-sm font-medium mb-2 text-gray-900 dark:text-white">Bulk Renewal Status</h4>
          <div className="space-y-1 text-sm">
            {renewAll.map(r => (
              <div key={r.node} className="flex items-center gap-2">
                <span className={
                  r.status === 'pending' ? 'text-gray-500' :
                  r.status === 'done' ? 'text-green-600 dark:text-green-400' :
                  'text-red-600 dark:text-red-400'
                }>
                  {r.status === 'pending' ? '⏳' : r.status === 'done' ? '✓' : '✗'}
                </span>
                <span className="text-gray-900 dark:text-white">{r.node}</span>
                {r.msg && <span className="text-xs text-red-600 dark:text-red-400">— {r.msg}</span>}
              </div>
            ))}
          </div>
        </div>
      )}

      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <div className="flex items-center justify-between mb-3">
          <h3 className="font-medium text-gray-900 dark:text-white">ACME Accounts ({accounts.length})</h3>
          <button onClick={() => setShowAddAccount(true)} className="px-3 py-1 bg-blue-600 hover:bg-blue-700 text-white text-sm rounded">+ Add Account</button>
        </div>
        {accounts.length === 0 ? (
          <p className="text-sm text-gray-500">No ACME accounts registered yet. Click <em>Add Account</em> to register one with Let's Encrypt or another ACME directory.</p>
        ) : (
          <table className="w-full text-sm">
            <thead><tr className="text-left text-gray-500 border-b border-gray-200 dark:border-gray-700">
              <th className="pb-2 font-medium">Name</th>
              <th className="pb-2 font-medium">Directory</th>
              <th className="pb-2 font-medium w-32">Actions</th>
            </tr></thead>
            <tbody>
              {accounts.map(a => (
                <tr key={a.name} className="border-b border-gray-100 dark:border-gray-700/50">
                  <td className="py-2 text-gray-900 dark:text-white">{a.name}</td>
                  <td className="py-2 text-gray-700 dark:text-gray-300 break-all">{a.directory || '—'}</td>
                  <td className="py-2 space-x-2">
                    <button onClick={() => setEditAccount(a)} className="text-blue-600 hover:text-blue-800 text-xs">Edit</button>
                    <button onClick={() => onDeleteAccount(a.name)} className="text-red-600 hover:text-red-800 text-xs">Delete</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <div className="flex items-center justify-between mb-3">
          <h3 className="font-medium text-gray-900 dark:text-white">Challenge Plugins ({plugins.length})</h3>
          <button onClick={() => setShowAddPlugin(true)} className="px-3 py-1 bg-blue-600 hover:bg-blue-700 text-white text-sm rounded">+ Add Plugin</button>
        </div>
        {plugins.length === 0 ? (
          <p className="text-sm text-gray-500">No DNS challenge plugins configured. Click <em>Add Plugin</em> to configure a DNS provider (Cloudflare, Route53, etc.) for DNS-01 validation.</p>
        ) : (
          <table className="w-full text-sm">
            <thead><tr className="text-left text-gray-500 border-b border-gray-200 dark:border-gray-700">
              <th className="pb-2 font-medium">ID</th>
              <th className="pb-2 font-medium">Type</th>
              <th className="pb-2 font-medium">Provider</th>
              <th className="pb-2 font-medium">Status</th>
              <th className="pb-2 font-medium w-32">Actions</th>
            </tr></thead>
            <tbody>
              {plugins.map(p => (
                <tr key={p.plugin} className="border-b border-gray-100 dark:border-gray-700/50">
                  <td className="py-2 text-gray-900 dark:text-white">{p.plugin}</td>
                  <td className="py-2 text-gray-700 dark:text-gray-300">{p.type}</td>
                  <td className="py-2 text-gray-700 dark:text-gray-300">{p.api || '—'}</td>
                  <td className="py-2">
                    {p.disable ? (
                      <span className="px-2 py-0.5 rounded text-xs bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400">Disabled</span>
                    ) : (
                      <span className="px-2 py-0.5 rounded text-xs bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400">Enabled</span>
                    )}
                  </td>
                  <td className="py-2 space-x-2">
                    <button onClick={() => setEditPlugin(p)} className="text-blue-600 hover:text-blue-800 text-xs">Edit</button>
                    <button onClick={() => onDeletePlugin(p.plugin)} className="text-red-600 hover:text-red-800 text-xs">Delete</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {showAddAccount && (
        <ACMEAccountDialog
          clusterName={clusterName}
          onClose={() => setShowAddAccount(false)}
          onSuccess={() => { setShowAddAccount(false); load(); }}
        />
      )}
      {editAccount && (
        <ACMEAccountDialog
          clusterName={clusterName}
          account={editAccount}
          onClose={() => setEditAccount(null)}
          onSuccess={() => { setEditAccount(null); load(); }}
        />
      )}
      {showAddPlugin && (
        <ACMEPluginDialog
          clusterName={clusterName}
          onClose={() => setShowAddPlugin(false)}
          onSuccess={() => { setShowAddPlugin(false); load(); }}
        />
      )}
      {editPlugin && (
        <ACMEPluginDialog
          clusterName={clusterName}
          plugin={editPlugin}
          onClose={() => setEditPlugin(null)}
          onSuccess={() => { setEditPlugin(null); load(); }}
        />
      )}
    </div>
  );
}

// --- Dialogs ---

function ACMEAccountDialog({
  clusterName, account, onClose, onSuccess,
}: {
  clusterName: string;
  account?: ACMEAccount;
  onClose: () => void;
  onSuccess: () => void;
}) {
  const isEdit = !!account;
  const [name, setName] = useState(account?.name || '');
  const [contact, setContact] = useState('');
  const [directories, setDirectories] = useState<ACMEDirectory[]>([]);
  const [directory, setDirectory] = useState(account?.directory || '');
  const [tos, setTos] = useState('');
  const [acceptTos, setAcceptTos] = useState(false);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (isEdit) return;
    (async () => {
      try {
        const dirs = await api.listACMEDirectories(clusterName);
        setDirectories(dirs);
        if (dirs.length > 0 && !directory) setDirectory(dirs[0].url);
      } catch (e) {
        setErr(e instanceof Error ? e.message : 'failed to load directories');
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [clusterName]);

  useEffect(() => {
    if (!directory || isEdit) return;
    (async () => {
      try {
        const { tos } = await api.getACMETOSURL(clusterName, directory);
        setTos(tos);
      } catch {
        setTos('');
      }
    })();
  }, [directory, clusterName, isEdit]);

  const onSave = async () => {
    setErr(null);
    if (!isEdit) {
      if (!name.trim() || !contact.trim()) { setErr('Name and contact required'); return }
      if (!acceptTos) { setErr('You must accept the Terms of Service'); return }
    }
    setLoading(true);
    try {
      if (isEdit && account) {
        await api.updateACMEAccount(clusterName, account.name, contact);
      } else {
        await api.createACMEAccount(clusterName, { name: name.trim(), contact: contact.trim(), directory, tos_url: tos });
      }
      onSuccess();
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'save failed');
      setLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-md p-6">
        <h2 className="text-lg font-semibold mb-4 text-gray-900 dark:text-white">
          {isEdit ? `Edit Account: ${account?.name}` : 'Register ACME Account'}
        </h2>

        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Name {!isEdit && <span className="text-red-500">*</span>}</label>
            <input type="text" value={name} onChange={e => setName(e.target.value)} disabled={isEdit || loading}
              placeholder="e.g. default"
              className="w-full border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded px-3 py-2 disabled:opacity-50" />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Contact Email <span className="text-red-500">*</span></label>
            <input type="email" value={contact} onChange={e => setContact(e.target.value)} disabled={loading}
              placeholder="admin@example.com"
              className="w-full border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded px-3 py-2" />
          </div>
          {!isEdit && (
            <>
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Directory</label>
                <select value={directory} onChange={e => setDirectory(e.target.value)} disabled={loading}
                  className="w-full border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded px-3 py-2">
                  {directories.map(d => <option key={d.url} value={d.url}>{d.name}</option>)}
                </select>
              </div>
              <div className="flex items-start gap-2">
                <input type="checkbox" id="accept-tos" checked={acceptTos} onChange={e => setAcceptTos(e.target.checked)} disabled={loading || !tos}
                  className="mt-1" />
                <label htmlFor="accept-tos" className="text-sm text-gray-700 dark:text-gray-300">
                  I accept the <a href={tos} target="_blank" rel="noreferrer" className="text-blue-600 hover:underline">Terms of Service</a>
                  {!tos && <span className="text-gray-500 italic"> (loading…)</span>}
                </label>
              </div>
            </>
          )}
          {err && <div className="bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-400 px-3 py-2 rounded text-sm">{err}</div>}
        </div>

        <div className="flex justify-end gap-3 mt-6">
          <button onClick={onClose} disabled={loading}
            className="px-4 py-2 text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded">Cancel</button>
          <button onClick={onSave} disabled={loading}
            className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50">
            {loading ? 'Saving…' : (isEdit ? 'Save' : 'Register')}
          </button>
        </div>
      </div>
    </div>
  );
}

function ACMEPluginDialog({
  clusterName, plugin, onClose, onSuccess,
}: {
  clusterName: string;
  plugin?: ACMEPlugin;
  onClose: () => void;
  onSuccess: () => void;
}) {
  const isEdit = !!plugin;
  const [id, setId] = useState(plugin?.plugin || '');
  const [schemas, setSchemas] = useState<ACMEChallengeSchema[]>([]);
  const [api_, setApi] = useState(plugin?.api || '');
  const [data, setData] = useState<Record<string, string>>(plugin?.data || {});
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    (async () => {
      try {
        const s = await api.listACMEChallengeSchemas(clusterName);
        // keep only dns-type schemas
        setSchemas(s.filter(x => x.type === 'dns'));
      } catch (e) {
        setErr(e instanceof Error ? e.message : 'failed to load schemas');
      }
    })();
  }, [clusterName]);

  const currentSchema = schemas.find(s => s.id === api_);

  const onSave = async () => {
    setErr(null);
    if (!isEdit) {
      if (!id.trim() || !api_) { setErr('ID and provider required'); return }
    }
    setLoading(true);
    try {
      if (isEdit && plugin) {
        await api.updateACMEPlugin(clusterName, plugin.plugin, data);
      } else {
        await api.createACMEPlugin(clusterName, { id: id.trim(), type: 'dns', api: api_, data });
      }
      onSuccess();
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'save failed');
      setLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-lg p-6 max-h-[90vh] overflow-auto">
        <h2 className="text-lg font-semibold mb-4 text-gray-900 dark:text-white">
          {isEdit ? `Edit Plugin: ${plugin?.plugin}` : 'Add DNS Challenge Plugin'}
        </h2>

        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Plugin ID {!isEdit && <span className="text-red-500">*</span>}</label>
            <input type="text" value={id} onChange={e => setId(e.target.value)} disabled={isEdit || loading}
              placeholder="e.g. cloudflare"
              className="w-full border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded px-3 py-2 disabled:opacity-50" />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">DNS Provider {!isEdit && <span className="text-red-500">*</span>}</label>
            <select value={api_} onChange={e => { setApi(e.target.value); setData({}) }} disabled={isEdit || loading}
              className="w-full border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded px-3 py-2 disabled:opacity-50">
              <option value="">— select provider —</option>
              {schemas.map(s => <option key={s.id} value={s.id}>{s.name} ({s.id})</option>)}
            </select>
          </div>
          {currentSchema && (
            <div className="space-y-3 bg-gray-50 dark:bg-gray-700/40 p-3 rounded">
              <div className="text-xs font-medium text-gray-600 dark:text-gray-400">Provider Configuration</div>
              {Object.keys(currentSchema.schema).length === 0 ? (
                <p className="text-sm text-gray-500">No configuration fields required.</p>
              ) : (
                Object.entries(currentSchema.schema).map(([field, meta]) => (
                  <div key={field}>
                    <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">
                      {field}
                      {meta.description && <span className="text-gray-500 font-normal"> — {meta.description}</span>}
                    </label>
                    <input
                      type={/token|key|secret|password/i.test(field) ? 'password' : 'text'}
                      value={data[field] || ''}
                      onChange={e => setData({ ...data, [field]: e.target.value })}
                      disabled={loading}
                      className="w-full border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded px-2 py-1 text-sm"
                    />
                  </div>
                ))
              )}
            </div>
          )}
          {err && <div className="bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-400 px-3 py-2 rounded text-sm">{err}</div>}
        </div>

        <div className="flex justify-end gap-3 mt-6">
          <button onClick={onClose} disabled={loading}
            className="px-4 py-2 text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded">Cancel</button>
          <button onClick={onSave} disabled={loading}
            className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50">
            {loading ? 'Saving…' : (isEdit ? 'Save' : 'Create')}
          </button>
        </div>
      </div>
    </div>
  );
}

export function NodeDomainsDialog({
  clusterName, node, onClose, onSuccess,
}: {
  clusterName: string;
  node: string;
  onClose: () => void;
  onSuccess: () => void;
}) {
  const [domains, setDomains] = useState<NodeACMEDomain[]>([]);
  const [plugins, setPlugins] = useState<ACMEPlugin[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    (async () => {
      try {
        const [d, p] = await Promise.all([
          api.getNodeACMEDomains(clusterName, node),
          api.listACMEPlugins(clusterName),
        ]);
        setDomains(d);
        setPlugins(p);
      } catch (e) {
        setErr(e instanceof Error ? e.message : 'load failed');
      } finally {
        setLoading(false);
      }
    })();
  }, [clusterName, node]);

  const addRow = () => setDomains([...domains, { domain: '', plugin: plugins[0]?.plugin || '' }]);
  const updateRow = (i: number, field: keyof NodeACMEDomain, val: string) => {
    const next = [...domains];
    next[i] = { ...next[i], [field]: val };
    setDomains(next);
  };
  const removeRow = (i: number) => setDomains(domains.filter((_, idx) => idx !== i));

  const onSave = async () => {
    setErr(null);
    for (const d of domains) {
      if (!d.domain.trim()) { setErr('All domains must be non-empty'); return }
    }
    setSaving(true);
    try {
      await api.setNodeACMEDomains(clusterName, node, domains.map(d => ({ domain: d.domain.trim(), plugin: d.plugin || undefined })));
      onSuccess();
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'save failed');
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
        <div className="bg-white dark:bg-gray-800 rounded-lg p-6 text-gray-500">Loading…</div>
      </div>
    );
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-xl p-6 max-h-[90vh] overflow-auto">
        <h2 className="text-lg font-semibold mb-4 text-gray-900 dark:text-white">ACME Domains: {node}</h2>
        <p className="text-xs text-gray-500 mb-4">
          Each domain needs a challenge plugin that can prove control of that domain. Plugins must be configured at the cluster level first.
        </p>

        {plugins.length === 0 && (
          <div className="bg-yellow-50 dark:bg-yellow-900/20 text-yellow-700 dark:text-yellow-400 px-3 py-2 rounded text-sm mb-4">
            No challenge plugins configured at the cluster level. Go to the cluster's ACME tab and add one first.
          </div>
        )}

        <div className="space-y-2">
          {domains.length === 0 && <p className="text-sm text-gray-500">No domains configured.</p>}
          {domains.map((d, i) => (
            <div key={i} className="flex gap-2 items-center">
              <input type="text" value={d.domain} onChange={e => updateRow(i, 'domain', e.target.value)} disabled={saving}
                placeholder="hostname.example.com"
                className="flex-1 border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded px-2 py-1 text-sm" />
              <select value={d.plugin || ''} onChange={e => updateRow(i, 'plugin', e.target.value)} disabled={saving}
                className="border border-gray-300 dark:border-gray-600 dark:bg-gray-700 dark:text-white rounded px-2 py-1 text-sm">
                <option value="">(no plugin)</option>
                {plugins.map(p => <option key={p.plugin} value={p.plugin}>{p.plugin}</option>)}
              </select>
              <button onClick={() => removeRow(i)} disabled={saving}
                className="text-red-600 hover:text-red-800 text-sm">Remove</button>
            </div>
          ))}
        </div>

        <button onClick={addRow} disabled={saving}
          className="mt-3 px-3 py-1 text-sm border border-gray-300 dark:border-gray-600 rounded hover:bg-gray-50 dark:hover:bg-gray-700 text-gray-700 dark:text-gray-300">+ Add Domain</button>

        {err && <div className="bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-400 px-3 py-2 rounded text-sm mt-4">{err}</div>}

        <div className="flex justify-end gap-3 mt-6">
          <button onClick={onClose} disabled={saving}
            className="px-4 py-2 text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded">Cancel</button>
          <button onClick={onSave} disabled={saving}
            className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50">
            {saving ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
    </div>
  );
}
