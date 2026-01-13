import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from 'react';
import { api } from '../api/client';
import type { Folder, TreeView, CreateFolderRequest, MoveResourceRequest } from '../types';

interface FolderState {
  // Folder trees
  hostsTree: Folder[];
  vmsTree: Folder[];
  isLoading: boolean;
  error: string | null;

  // CRUD operations
  createFolder: (name: string, treeView: TreeView, parentId?: string, cluster?: string) => Promise<Folder>;
  renameFolder: (id: string, name: string) => Promise<void>;
  deleteFolder: (id: string) => Promise<void>;
  moveFolder: (id: string, parentId?: string) => Promise<void>;

  // Resource membership
  addResourceToFolder: (folderId: string, resourceType: string, resourceId: string, cluster: string) => Promise<void>;
  removeResourceFromFolder: (folderId: string, resourceType: string, resourceId: string, cluster: string) => Promise<void>;
  moveResource: (req: MoveResourceRequest, tree: TreeView) => Promise<void>;

  // Helpers
  getFolderById: (id: string, tree: TreeView) => Folder | undefined;
  getResourceFolder: (resourceType: string, resourceId: string, cluster: string, tree: TreeView) => Folder | undefined;
  refreshTree: (tree: TreeView) => Promise<void>;
  refreshAll: () => Promise<void>;
}

const FolderContext = createContext<FolderState | null>(null);

export function useFolders() {
  const ctx = useContext(FolderContext);
  if (!ctx) throw new Error('useFolders must be used within FolderProvider');
  return ctx;
}

// Recursively find a folder by ID in a tree
function findFolderById(folders: Folder[], id: string): Folder | undefined {
  for (const folder of folders) {
    if (folder.id === id) return folder;
    if (folder.children) {
      const found = findFolderById(folder.children, id);
      if (found) return found;
    }
  }
  return undefined;
}

// Recursively find folder containing a resource
function findResourceFolder(folders: Folder[], resourceType: string, resourceId: string, cluster: string): Folder | undefined {
  for (const folder of folders) {
    if (folder.members?.some(m =>
      m.resource_type === resourceType &&
      m.resource_id === resourceId &&
      m.cluster === cluster
    )) {
      return folder;
    }
    if (folder.children) {
      const found = findResourceFolder(folder.children, resourceType, resourceId, cluster);
      if (found) return found;
    }
  }
  return undefined;
}

export function FolderProvider({ children }: { children: ReactNode }) {
  const [hostsTree, setHostsTree] = useState<Folder[]>([]);
  const [vmsTree, setVmsTree] = useState<Folder[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Fetch a single tree
  const refreshTree = useCallback(async (tree: TreeView) => {
    try {
      const folders = await api.getFolderTree(tree);
      if (tree === 'hosts') {
        setHostsTree(folders);
      } else {
        setVmsTree(folders);
      }
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to fetch folders');
    }
  }, []);

  // Fetch both trees
  const refreshAll = useCallback(async () => {
    setIsLoading(true);
    try {
      const [hosts, vms] = await Promise.all([
        api.getFolderTree('hosts'),
        api.getFolderTree('vms'),
      ]);
      setHostsTree(hosts);
      setVmsTree(vms);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to fetch folders');
    } finally {
      setIsLoading(false);
    }
  }, []);

  // Initial fetch
  useEffect(() => {
    refreshAll();
  }, [refreshAll]);

  // CRUD: Create folder
  const createFolder = useCallback(async (
    name: string,
    treeView: TreeView,
    parentId?: string,
    cluster?: string
  ): Promise<Folder> => {
    const req: CreateFolderRequest = {
      name,
      tree_view: treeView,
      parent_id: parentId,
      cluster,
    };
    const folder = await api.createFolder(req);
    await refreshTree(treeView);
    return folder;
  }, [refreshTree]);

  // CRUD: Rename folder
  const renameFolder = useCallback(async (id: string, name: string) => {
    await api.renameFolder(id, name);
    // Refresh both trees since we don't know which one it's in
    await refreshAll();
  }, [refreshAll]);

  // CRUD: Delete folder
  const deleteFolder = useCallback(async (id: string) => {
    await api.deleteFolder(id);
    await refreshAll();
  }, [refreshAll]);

  // CRUD: Move folder
  const moveFolder = useCallback(async (id: string, parentId?: string) => {
    await api.moveFolder(id, parentId);
    await refreshAll();
  }, [refreshAll]);

  // Resource membership: Add
  const addResourceToFolder = useCallback(async (
    folderId: string,
    resourceType: string,
    resourceId: string,
    cluster: string
  ) => {
    await api.addFolderMember(folderId, resourceType, resourceId, cluster);
    await refreshAll();
  }, [refreshAll]);

  // Resource membership: Remove
  const removeResourceFromFolder = useCallback(async (
    folderId: string,
    resourceType: string,
    resourceId: string,
    cluster: string
  ) => {
    await api.removeFolderMember(folderId, resourceType, resourceId, cluster);
    await refreshAll();
  }, [refreshAll]);

  // Move resource to folder (handles removing from old folder)
  const moveResource = useCallback(async (req: MoveResourceRequest, tree: TreeView) => {
    await api.moveResource(req, tree);
    await refreshTree(tree);
  }, [refreshTree]);

  // Helper: Find folder by ID
  const getFolderById = useCallback((id: string, tree: TreeView): Folder | undefined => {
    const folders = tree === 'hosts' ? hostsTree : vmsTree;
    return findFolderById(folders, id);
  }, [hostsTree, vmsTree]);

  // Helper: Find folder containing resource
  const getResourceFolder = useCallback((
    resourceType: string,
    resourceId: string,
    cluster: string,
    tree: TreeView
  ): Folder | undefined => {
    const folders = tree === 'hosts' ? hostsTree : vmsTree;
    return findResourceFolder(folders, resourceType, resourceId, cluster);
  }, [hostsTree, vmsTree]);

  return (
    <FolderContext.Provider value={{
      hostsTree,
      vmsTree,
      isLoading,
      error,
      createFolder,
      renameFolder,
      deleteFolder,
      moveFolder,
      addResourceToFolder,
      removeResourceFromFolder,
      moveResource,
      getFolderById,
      getResourceFolder,
      refreshTree,
      refreshAll,
    }}>
      {children}
    </FolderContext.Provider>
  );
}
