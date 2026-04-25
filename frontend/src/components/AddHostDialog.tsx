import { useState, useEffect, useRef } from 'react';
import { api } from '../api/client';

interface AddHostDialogProps {
  datacenterId: string;
  datacenterName: string;
  onSubmit: () => Promise<void>;
  onClose: () => void;
}

export function AddHostDialog({
  datacenterId,
  datacenterName,
  onSubmit,
  onClose
}: AddHostDialogProps) {
  const [address, setAddress] = useState('');
  const [username, setUsername] = useState('root@pam');
  const [password, setPassword] = useState('');
  const [insecure, setInsecure] = useState(true);
  const [deployAgent, setDeployAgent] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [status, setStatus] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  const targetName = datacenterName;

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setStatus(null);

    let addr = address.trim();
    if (!addr) {
      setError('Address is required');
      return;
    }
    // Default to Proxmox's API port if the user omits one. Detection is
    // intentionally narrow: only append if there's no `:` after the last
    // `/` (avoids stomping on IPv6 literals or full URLs the user pasted).
    if (!/[:][0-9]+$/.test(addr) && !addr.includes('://')) {
      addr = addr + ':8006';
    }
    if (!username.trim()) {
      setError('Username is required');
      return;
    }
    if (!password) {
      setError('Password is required');
      return;
    }

    setSaving(true);
    try {
      // Step 1: Add host to inventory (auto-creates API token).
      // Probe is server-side: if the address belongs to a real PVE cluster,
      // the host is filed under a cluster; otherwise it lands as standalone
      // under the datacenter.
      setStatus('Authenticating and creating API token...');
      setStatus('Probing cluster membership...');
      const result = await api.addDatacenterHost(datacenterId, {
        address: addr,
        username: username.trim(),
        password,
        insecure,
      });
      const host = result.host;
      if (!result.standalone && result.cluster) {
        setStatus(`Detected PVE cluster "${result.detected_pve_cluster}" — filed under cluster "${result.cluster.name}"`);
      } else {
        setStatus('Standalone host added under datacenter');
      }

      if (deployAgent) {
        // Step 2: Setup SSH key (use same password)
        setStatus('Setting up SSH key...');
        try {
          await api.setupHostSSH(host.id, password);
        } catch (sshErr) {
          setError(`Host added, but SSH setup failed: ${sshErr instanceof Error ? sshErr.message : 'Unknown error'}`);
          setSaving(false);
          await onSubmit(); // Still refresh the tree
          return;
        }

        // Step 3: Deploy agent
        setStatus('Deploying agent...');
        try {
          await api.deployAgent(host.id);
        } catch (deployErr) {
          setError(`Host added and SSH configured, but agent deployment failed: ${deployErr instanceof Error ? deployErr.message : 'Unknown error'}`);
          setSaving(false);
          await onSubmit();
          return;
        }
      }

      await onSubmit();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add host');
      setSaving(false);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape' && !saving) {
      onClose();
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={saving ? undefined : onClose}>
      <div
        className="bg-white dark:bg-gray-800 rounded-lg shadow-xl p-4 min-w-[400px] max-w-lg"
        onClick={(e) => e.stopPropagation()}
      >
        <h3 className="text-lg font-medium text-gray-900 dark:text-gray-100 mb-4">
          Add Host to {targetName}
        </h3>
        <form onSubmit={handleSubmit} onKeyDown={handleKeyDown}>
          <div className="mb-3">
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Address
            </label>
            <input
              ref={inputRef}
              type="text"
              value={address}
              onChange={(e) => setAddress(e.target.value)}
              placeholder="10.0.0.1:8006 or pve01.example.com:8006"
              disabled={saving}
              className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50"
            />
            <p className="text-xs text-gray-500 mt-1">
              {mode === 'datacenter'
                ? 'Any Proxmox host. Cluster membership is auto-detected: real PVE clusters become pcenter clusters; single-node hosts are filed as standalone.'
                : 'Any node in the Proxmox cluster'}
            </p>
          </div>
          <div className="mb-3">
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Username
            </label>
            <input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="root@pam"
              disabled={saving}
              className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50"
            />
            <p className="text-xs text-gray-500 mt-1">
              Proxmox user (e.g., root@pam). API token will be created automatically.
            </p>
          </div>
          <div className="mb-3">
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Password
            </label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Proxmox password"
              disabled={saving}
              className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50"
            />
            <p className="text-xs text-gray-500 mt-1">
              Used to authenticate and create API token. {deployAgent && 'Also used for SSH key setup.'}
            </p>
          </div>
          <div className="mb-3">
            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={insecure}
                onChange={(e) => setInsecure(e.target.checked)}
                disabled={saving}
                className="rounded border-gray-300 dark:border-gray-600"
              />
              <span className="text-sm text-gray-700 dark:text-gray-300">
                Skip TLS certificate verification
              </span>
            </label>
          </div>

          <hr className="my-4 border-gray-300 dark:border-gray-600" />

          <div className="mb-3">
            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={deployAgent}
                onChange={(e) => setDeployAgent(e.target.checked)}
                disabled={saving}
                className="rounded border-gray-300 dark:border-gray-600"
              />
              <span className="text-sm font-medium text-gray-700 dark:text-gray-300">
                Deploy pve-agent automatically
              </span>
            </label>
            <p className="text-xs text-gray-500 mt-1 ml-6">
              Sets up SSH key and deploys the monitoring agent
            </p>
          </div>

          {status && (
            <div className="flex items-center gap-2 text-blue-600 dark:text-blue-400 text-sm mb-3">
              <svg className="animate-spin h-4 w-4" viewBox="0 0 24 24">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" fill="none" />
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
              </svg>
              {status}
            </div>
          )}

          {error && (
            <p className="text-red-500 text-sm mb-3">{error}</p>
          )}

          <div className="flex justify-end gap-2 mt-4">
            <button
              type="button"
              onClick={onClose}
              disabled={saving}
              className="px-3 py-1.5 text-sm text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 rounded disabled:opacity-50"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={saving}
              className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
            >
              {saving ? 'Working...' : deployAgent ? 'Add & Deploy' : 'Add Host'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
