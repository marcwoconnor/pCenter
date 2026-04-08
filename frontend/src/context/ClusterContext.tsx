import { createContext, useContext, useState, useEffect, useCallback, useRef, type ReactNode } from 'react';
import { useWebSocket } from '../hooks/useWebSocket';
import { api } from '../api/client';
import type { Summary, Node, Guest, Storage, ClusterInfo, MigrationProgress, DRSRecommendation, ActivityEntry } from '../types';

interface CephHealthCheck {
  severity: string;
  summary: string;
  detail?: string;
}

interface CephStatus {
  health: string;
  checks?: Record<string, CephHealthCheck>;
  bytes_used: number;
  bytes_avail: number;
  bytes_total: number;
}

interface Task {
  id: string;
  type: string;
  status: 'running' | 'completed' | 'failed';
  target: string;
  startTime: number;
  message?: string;
}

export interface ConsoleWindow {
  id: string;
  type: 'vm' | 'ct';
  vmid: number;
  name: string;
  cluster: string;
  x: number;
  y: number;
  width: number;
  height: number;
  zIndex: number;
}

interface ClusterState {
  // Multi-cluster data
  clusters: ClusterInfo[];
  summary: Summary | null;
  nodes: Node[];
  guests: Guest[];
  storage: Storage[];
  ceph: CephStatus | null;
  migrations: MigrationProgress[];
  drsRecommendations: DRSRecommendation[];
  activityEntries: ActivityEntry[];
  // UI state
  tasks: Task[];
  isConnected: boolean;
  isLoading: boolean;
  error: string | null;
  selectedObject: SelectedObject | null;
  setSelectedObject: (obj: SelectedObject | null) => void;
  addTask: (task: Task) => void;
  updateTask: (id: string, updates: Partial<Task>) => void;
  performAction: (type: 'vm' | 'ct', vmid: number, action: 'start' | 'stop' | 'shutdown', cluster?: string) => Promise<void>;
  startClone: (guest: Guest, newId: number, name: string, opts?: { targetNode?: string; full?: boolean; storage?: string }) => void;
  consoles: ConsoleWindow[];
  openConsole: (type: 'vm' | 'ct', vmid: number, name: string, cluster: string) => void;
  closeConsole: (id: string) => void;
  focusConsole: (id: string) => void;
  updateConsole: (id: string, updates: Partial<ConsoleWindow>) => void;
  // Helpers
  getCluster: (name: string) => ClusterInfo | undefined;
  getGuestsByCluster: (cluster: string) => Guest[];
  getNodesByCluster: (cluster: string) => Node[];
}

export interface SelectedObject {
  type: 'node' | 'vm' | 'ct' | 'storage' | 'cluster' | 'datacenter' | 'network';
  id: string | number;
  name: string;
  node?: string;
  cluster?: string;
  defaultTab?: string;
}

const ClusterContext = createContext<ClusterState | null>(null);

export function useCluster() {
  const ctx = useContext(ClusterContext);
  if (!ctx) throw new Error('useCluster must be used within ClusterProvider');
  return ctx;
}

interface StatePayload {
  clusters: ClusterInfo[];
  summary: Summary;
  nodes: Node[];
  guests: Guest[];
  storage: Storage[];
  ceph?: CephStatus;
  migrations?: MigrationProgress[];
  drs_recommendations?: DRSRecommendation[];
}

export function ClusterProvider({ children }: { children: ReactNode }) {
  const [clusters, setClusters] = useState<ClusterInfo[]>([]);
  const [summary, setSummary] = useState<Summary | null>(null);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [guests, setGuests] = useState<Guest[]>([]);
  const [storage, setStorage] = useState<Storage[]>([]);
  const [ceph, setCeph] = useState<CephStatus | null>(null);
  const [migrations, setMigrations] = useState<MigrationProgress[]>([]);
  const [drsRecommendations, setDRSRecommendations] = useState<DRSRecommendation[]>([]);
  const [activityEntries, setActivityEntries] = useState<ActivityEntry[]>([]);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedObject, setSelectedObject] = useState<SelectedObject | null>(null);
  const [consoles, setConsoles] = useState<ConsoleWindow[]>([]);
  const [nextZIndex, setNextZIndex] = useState(100);

  // Track active clone poll intervals so we can clean them up on unmount.
  // Without this, intervals leak if the provider unmounts mid-clone.
  const clonePollIntervals = useRef<Set<ReturnType<typeof setInterval>>>(new Set());
  const clonePollTimeouts = useRef<Set<ReturnType<typeof setTimeout>>>(new Set());

  // Cleanup all active clone polling on unmount
  useEffect(() => {
    return () => {
      clonePollIntervals.current.forEach(clearInterval);
      clonePollTimeouts.current.forEach(clearTimeout);
    };
  }, []);

  const wsUrl = `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws`;

  const handleMessage = useCallback((msg: { type: string; payload: unknown }) => {
    if (msg.type === 'state') {
      const state = msg.payload as StatePayload;
      setClusters(state.clusters || []);
      setSummary(state.summary);
      setNodes(state.nodes || []);
      setGuests(state.guests || []);
      setStorage(state.storage || []);
      setCeph(state.ceph || null);
      setMigrations(state.migrations || []);
      setDRSRecommendations(state.drs_recommendations || []);
      setError(null);
      setIsLoading(false);
    } else if (msg.type === 'activity') {
      const entry = msg.payload as ActivityEntry;
      setActivityEntries(prev => [entry, ...prev].slice(0, 100));
    }
  }, []);

  const { isConnected } = useWebSocket(wsUrl, {
    onMessage: handleMessage,
    onConnect: () => setError(null),
    onDisconnect: () => setError('Connection lost - reconnecting...'),
  });

  // Fallback fetch
  useEffect(() => {
    if (!isConnected && isLoading) {
      const fetchData = async () => {
        try {
          const [s, n, g, st] = await Promise.all([
            api.getSummary(),
            api.getNodes(),
            api.getGuests(),
            api.getStorage(),
          ]);
          setSummary(s);
          setNodes(n);
          setGuests(g);
          setStorage(st);
          setError(null);
        } catch (e) {
          setError(e instanceof Error ? e.message : 'Failed to fetch');
        } finally {
          setIsLoading(false);
        }
      };
      fetchData();
    }
  }, [isConnected, isLoading]);

  // Fetch initial activity entries
  useEffect(() => {
    api.getActivity({ limit: 50 }).then(setActivityEntries).catch(console.error);
  }, []);

  const addTask = useCallback((task: Task) => {
    setTasks((prev) => [task, ...prev].slice(0, 50)); // Keep last 50 tasks
  }, []);

  const updateTask = useCallback((id: string, updates: Partial<Task>) => {
    setTasks((prev) => prev.map((t) => (t.id === id ? { ...t, ...updates } : t)));
  }, []);

  const performAction = useCallback(async (
    type: 'vm' | 'ct',
    vmid: number,
    action: 'start' | 'stop' | 'shutdown',
    cluster?: string
  ) => {
    const guest = guests.find((g) => g.vmid === vmid && (!cluster || g.cluster === cluster));
    const taskId = `${Date.now()}-${vmid}-${action}`;

    addTask({
      id: taskId,
      type: `${action} ${type === 'vm' ? 'VM' : 'Container'}`,
      status: 'running',
      target: guest?.name || `${vmid}`,
      startTime: Date.now(),
    });

    try {
      // Use cluster-specific endpoint if cluster is provided
      if (cluster) {
        if (type === 'vm') {
          await api.clusterVMAction(cluster, vmid, action);
        } else {
          await api.clusterContainerAction(cluster, vmid, action);
        }
      } else {
        // Legacy endpoint - searches all clusters
        if (type === 'vm') {
          await api.vmAction(vmid, action);
        } else {
          await api.containerAction(vmid, action);
        }
      }
      updateTask(taskId, { status: 'completed', message: 'Success' });
    } catch (e) {
      updateTask(taskId, {
        status: 'failed',
        message: e instanceof Error ? e.message : 'Failed'
      });
      throw e;
    }
  }, [guests, addTask, updateTask]);

  // Start a clone operation and track it in the tasks bar
  const startClone = useCallback((
    guest: Guest,
    newId: number,
    name: string,
    opts?: { targetNode?: string; full?: boolean; storage?: string }
  ) => {
    const taskId = `clone-${Date.now()}-${guest.vmid}`;
    const isVM = guest.type === 'qemu';

    addTask({
      id: taskId,
      type: `Clone ${isVM ? 'VM' : 'Container'}`,
      status: 'running',
      target: `${guest.name} → ${name}`,
      startTime: Date.now(),
    });

    // Start the clone
    const clonePromise = isVM
      ? api.cloneVM(guest.cluster, guest.vmid, {
          new_id: newId,
          name,
          target_node: opts?.targetNode,
          full: opts?.full,
          storage: opts?.storage,
        })
      : api.cloneContainer(guest.cluster, guest.vmid, {
          new_id: newId,
          name,
          target_node: opts?.targetNode,
          full: opts?.full,
          storage: opts?.storage,
        });

    clonePromise
      .then((result) => {
        // Poll for task completion, tracked in refs for cleanup on unmount
        const pollInterval = setInterval(async () => {
          try {
            const task = await api.getTaskStatus(guest.cluster, result.upid);
            if (task.status === 'stopped') {
              clearInterval(pollInterval);
              clonePollIntervals.current.delete(pollInterval);
              if (task.exitstatus === 'OK') {
                updateTask(taskId, { status: 'completed', message: 'Clone completed' });
              } else {
                let errorMsg = task.exitstatus || 'Unknown error';
                if (errorMsg.includes('cannot clone TPM state while VM is running')) {
                  errorMsg = 'VM has TPM and must be stopped first';
                }
                updateTask(taskId, { status: 'failed', message: errorMsg });
              }
            }
          } catch (err) {
            console.error('Clone poll error:', err);
          }
        }, 1000);
        clonePollIntervals.current.add(pollInterval);

        // Safety timeout - stop polling after 10 minutes
        const safetyTimeout = setTimeout(() => {
          clearInterval(pollInterval);
          clonePollIntervals.current.delete(pollInterval);
          clonePollTimeouts.current.delete(safetyTimeout);
        }, 600000);
        clonePollTimeouts.current.add(safetyTimeout);
      })
      .catch((err) => {
        let errorMsg = err instanceof Error ? err.message : 'Clone failed';
        if (errorMsg.includes('cannot clone TPM state while VM is running')) {
          errorMsg = 'VM has TPM and must be stopped first';
        }
        updateTask(taskId, { status: 'failed', message: errorMsg });
      });
  }, [addTask, updateTask]);

  const openConsole = useCallback((type: 'vm' | 'ct', vmid: number, name: string, cluster: string) => {
    // Check if console for this vmid is already open
    setConsoles(prev => {
      const existing = prev.find(c => c.vmid === vmid && c.cluster === cluster);
      if (existing) {
        // Just bring it to front
        return prev.map(c => c.id === existing.id ? { ...c, zIndex: nextZIndex } : c);
      }
      // Create new console window with cascading position
      const offset = prev.length * 30;
      const newConsole: ConsoleWindow = {
        id: `${cluster}-${vmid}-${Date.now()}`,
        type,
        vmid,
        name,
        cluster,
        x: 100 + offset,
        y: 50 + offset,
        width: 900,
        height: 600,
        zIndex: nextZIndex,
      };
      return [...prev, newConsole];
    });
    setNextZIndex(z => z + 1);
  }, [nextZIndex]);

  const closeConsole = useCallback((id: string) => {
    setConsoles(prev => prev.filter(c => c.id !== id));
  }, []);

  const focusConsole = useCallback((id: string) => {
    setConsoles(prev => prev.map(c => c.id === id ? { ...c, zIndex: nextZIndex } : c));
    setNextZIndex(z => z + 1);
  }, [nextZIndex]);

  const updateConsole = useCallback((id: string, updates: Partial<ConsoleWindow>) => {
    setConsoles(prev => prev.map(c => c.id === id ? { ...c, ...updates } : c));
  }, []);

  // Helper: get cluster by name
  const getCluster = useCallback((name: string) => {
    return clusters.find(c => c.name === name);
  }, [clusters]);

  // Helper: get guests by cluster
  const getGuestsByCluster = useCallback((cluster: string) => {
    return guests.filter(g => g.cluster === cluster);
  }, [guests]);

  // Helper: get nodes by cluster
  const getNodesByCluster = useCallback((cluster: string) => {
    return nodes.filter(n => n.cluster === cluster);
  }, [nodes]);

  return (
    <ClusterContext.Provider value={{
      clusters,
      summary,
      nodes,
      guests,
      storage,
      ceph,
      migrations,
      drsRecommendations,
      activityEntries,
      tasks,
      isConnected,
      isLoading,
      error,
      selectedObject,
      setSelectedObject,
      addTask,
      updateTask,
      performAction,
      startClone,
      consoles,
      openConsole,
      closeConsole,
      focusConsole,
      updateConsole,
      getCluster,
      getGuestsByCluster,
      getNodesByCluster,
    }}>
      {children}
    </ClusterContext.Provider>
  );
}
