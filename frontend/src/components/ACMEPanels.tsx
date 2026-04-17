import { useState, useEffect, useCallback } from 'react';
import { api } from '../api/client';
import type { NodeCertificate, ACMEAccount, ACMEPlugin } from '../types';

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

  if (certs === null) return <div className="text-gray-500 p-4">Loading certificates…</div>;
  if (err) return <div className="bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-400 rounded p-4">{err}</div>;

  const acmeEligible = certs.some(c => c.filename === 'pveproxy-ssl.pem' || c.filename === 'pve-ssl.pem');

  return (
    <div className="space-y-4">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4 flex items-center justify-between">
        <div>
          <h3 className="font-medium text-gray-900 dark:text-white">Certificates on {node}</h3>
          <p className="text-sm text-gray-500">ACME-issued certs are installed at <code>pveproxy-ssl.pem</code>. Renewal works only if the cluster has an ACME account and challenge plugin configured.</p>
        </div>
        <button
          onClick={onRenew}
          disabled={renewing || !acmeEligible}
          className="px-3 py-1.5 rounded bg-blue-600 hover:bg-blue-700 text-white text-sm disabled:bg-gray-400 disabled:cursor-not-allowed"
          title={acmeEligible ? 'Request/renew ACME cert' : 'No renewable cert found'}
        >
          {renewing ? 'Renewing…' : 'Renew ACME Cert'}
        </button>
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
    </div>
  );
}

export function ClusterACMETab({ clusterName }: { clusterName: string }) {
  const [accounts, setAccounts] = useState<ACMEAccount[] | null>(null);
  const [plugins, setPlugins] = useState<ACMEPlugin[] | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    (async () => {
      try {
        const [a, p] = await Promise.all([
          api.listACMEAccounts(clusterName),
          api.listACMEPlugins(clusterName),
        ]);
        setAccounts(a);
        setPlugins(p);
      } catch (e) {
        setErr(e instanceof Error ? e.message : 'load failed');
        setAccounts([]);
        setPlugins([]);
      }
    })();
  }, [clusterName]);

  if (accounts === null || plugins === null) return <div className="text-gray-500 p-4">Loading ACME config…</div>;
  if (err) return <div className="bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-400 rounded p-4">{err}</div>;

  return (
    <div className="space-y-4">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-3 text-gray-900 dark:text-white">ACME Accounts ({accounts.length})</h3>
        {accounts.length === 0 ? (
          <p className="text-sm text-gray-500">No ACME accounts configured. Register one in the Proxmox UI under <em>Datacenter → ACME → Accounts</em>.</p>
        ) : (
          <table className="w-full text-sm">
            <thead><tr className="text-left text-gray-500 border-b border-gray-200 dark:border-gray-700">
              <th className="pb-2 font-medium">Name</th>
              <th className="pb-2 font-medium">Directory</th>
            </tr></thead>
            <tbody>
              {accounts.map(a => (
                <tr key={a.name} className="border-b border-gray-100 dark:border-gray-700/50">
                  <td className="py-2 text-gray-900 dark:text-white">{a.name}</td>
                  <td className="py-2 text-gray-700 dark:text-gray-300 break-all">{a.directory || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-4">
        <h3 className="font-medium mb-3 text-gray-900 dark:text-white">Challenge Plugins ({plugins.length})</h3>
        {plugins.length === 0 ? (
          <p className="text-sm text-gray-500">No DNS challenge plugins configured. Set one up in the Proxmox UI under <em>Datacenter → ACME → Challenge Plugins</em> (required for DNS-01 validation).</p>
        ) : (
          <table className="w-full text-sm">
            <thead><tr className="text-left text-gray-500 border-b border-gray-200 dark:border-gray-700">
              <th className="pb-2 font-medium">ID</th>
              <th className="pb-2 font-medium">Type</th>
              <th className="pb-2 font-medium">Provider</th>
              <th className="pb-2 font-medium">Status</th>
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
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
