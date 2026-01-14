import { useState, useCallback, useMemo } from 'react';
import { api } from '../api/client';
import type { VMConfig, ContainerConfig } from '../types';

export type ConfigType = VMConfig | ContainerConfig;

export interface PendingChange {
  key: string;
  oldValue: unknown;
  newValue: unknown;
  label: string;
}

export interface UseConfigEditorOptions {
  vmid: number;
  type: 'vm' | 'ct';
  cluster: string;
  initialConfig: ConfigType;
  initialDigest: string;
}

export interface UseConfigEditorReturn {
  // Current values (original + pending changes applied)
  getValue: (key: string) => unknown;
  // Set a pending change
  setValue: (key: string, value: unknown, label?: string) => void;
  // Pending changes list
  pendingChanges: PendingChange[];
  // Has any changes
  isDirty: boolean;
  // Discard all changes
  discard: () => void;
  // Apply changes to server
  apply: () => Promise<void>;
  // Loading/error state
  applying: boolean;
  error: string | null;
  // Conflict detected
  conflict: boolean;
  // Current digest
  digest: string;
}

export function useConfigEditor({
  vmid,
  type,
  cluster,
  initialConfig,
  initialDigest,
}: UseConfigEditorOptions): UseConfigEditorReturn {
  const [originalConfig] = useState<ConfigType>(initialConfig);
  const [digest, setDigest] = useState(initialDigest);
  const [changes, setChanges] = useState<Map<string, { value: unknown; label: string }>>(new Map());
  const [applying, setApplying] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [conflict, setConflict] = useState(false);

  // Get value (original or pending)
  const getValue = useCallback((key: string): unknown => {
    if (changes.has(key)) {
      return changes.get(key)!.value;
    }
    // Check raw_config first, then typed fields
    const raw = originalConfig.raw_config || {};
    if (key in raw) {
      return raw[key];
    }
    // Access typed field using key
    return (originalConfig as unknown as Record<string, unknown>)[key];
  }, [originalConfig, changes]);

  // Get original value
  const getOriginalValue = useCallback((key: string): unknown => {
    const raw = originalConfig.raw_config || {};
    if (key in raw) {
      return raw[key];
    }
    // Access typed field using key
    return (originalConfig as unknown as Record<string, unknown>)[key];
  }, [originalConfig]);

  // Set a pending change
  const setValue = useCallback((key: string, value: unknown, label?: string) => {
    setChanges(prev => {
      const next = new Map(prev);
      const original = getOriginalValue(key);

      // If value matches original, remove from changes
      if (value === original || String(value) === String(original)) {
        next.delete(key);
      } else {
        next.set(key, { value, label: label || key });
      }
      return next;
    });
    setError(null);
    setConflict(false);
  }, [getOriginalValue]);

  // Build pending changes list
  const pendingChanges = useMemo((): PendingChange[] => {
    return Array.from(changes.entries()).map(([key, { value, label }]) => ({
      key,
      oldValue: getOriginalValue(key),
      newValue: value,
      label,
    }));
  }, [changes, getOriginalValue]);

  // Discard all changes
  const discard = useCallback(() => {
    setChanges(new Map());
    setError(null);
    setConflict(false);
  }, []);

  // Apply changes to server
  const apply = useCallback(async () => {
    if (changes.size === 0) return;

    setApplying(true);
    setError(null);
    setConflict(false);

    try {
      // Build changes object
      const changesObj: Record<string, unknown> = {};
      changes.forEach(({ value }, key) => {
        changesObj[key] = value;
      });

      if (type === 'vm') {
        await api.updateVMConfig(cluster, vmid, digest, changesObj);
      } else {
        await api.updateContainerConfig(cluster, vmid, digest, changesObj);
      }

      // Success - clear changes and fetch new config to get new digest
      setChanges(new Map());

      // Fetch updated config for new digest
      const newConfig = type === 'vm'
        ? await api.getVMConfig(cluster, vmid)
        : await api.getContainerConfig(cluster, vmid);
      setDigest(newConfig.digest);

    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to update config';
      if (message.includes('conflict') || message.includes('changed')) {
        setConflict(true);
      }
      setError(message);
    } finally {
      setApplying(false);
    }
  }, [changes, digest, type, cluster, vmid]);

  return {
    getValue,
    setValue,
    pendingChanges,
    isDirty: changes.size > 0,
    discard,
    apply,
    applying,
    error,
    conflict,
    digest,
  };
}
