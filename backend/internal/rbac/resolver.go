package rbac

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/moconnor/pcenter/internal/inventory"
	"github.com/moconnor/pcenter/internal/state"
)

// StateResolver resolves object ancestry using the runtime state store
// and the inventory database (for datacenter lookups).
type StateResolver struct {
	store     *state.Store
	inventory *inventory.Service
}

// NewStateResolver creates an ancestry resolver backed by state + inventory
func NewStateResolver(store *state.Store, inv *inventory.Service) *StateResolver {
	return &StateResolver{store: store, inventory: inv}
}

// GetAncestors returns the ancestor chain for an object (excluding the object itself).
// The chain goes from nearest parent to root.
//
// Examples:
//
//	VM 102 on pve04 in cluster "default" under datacenter "dc1":
//	  → [{node, pve04}, {cluster, default}, {datacenter, dc1-id}, {root, ""}]
//
//	Node pve04 in cluster "default":
//	  → [{cluster, default}, {datacenter, dc1-id}, {root, ""}]
//
//	Cluster "default" in datacenter "dc1":
//	  → [{datacenter, dc1-id}, {root, ""}]
func (r *StateResolver) GetAncestors(objectType, objectID string) []ObjectRef {
	var ancestors []ObjectRef

	switch objectType {
	case ObjectVM, ObjectCT:
		// Find which node and cluster this guest belongs to
		nodeName, clusterName := r.findGuestLocation(objectType, objectID)
		if nodeName != "" {
			ancestors = append(ancestors, ObjectRef{Type: ObjectNode, ID: nodeName})
		}
		if clusterName != "" {
			ancestors = append(ancestors, ObjectRef{Type: ObjectCluster, ID: clusterName})
			if dcID := r.findDatacenterForCluster(clusterName); dcID != "" {
				ancestors = append(ancestors, ObjectRef{Type: ObjectDatacenter, ID: dcID})
			}
		}

	case ObjectNode:
		// Find which cluster this node belongs to
		clusterName := r.findNodeCluster(objectID)
		if clusterName != "" {
			ancestors = append(ancestors, ObjectRef{Type: ObjectCluster, ID: clusterName})
			if dcID := r.findDatacenterForCluster(clusterName); dcID != "" {
				ancestors = append(ancestors, ObjectRef{Type: ObjectDatacenter, ID: dcID})
			}
		}

	case ObjectStorage:
		// Storage belongs to a node+cluster (objectID is "node-storageName")
		// For simplicity, resolve via the cluster it's in
		clusterName := r.findStorageCluster(objectID)
		if clusterName != "" {
			ancestors = append(ancestors, ObjectRef{Type: ObjectCluster, ID: clusterName})
			if dcID := r.findDatacenterForCluster(clusterName); dcID != "" {
				ancestors = append(ancestors, ObjectRef{Type: ObjectDatacenter, ID: dcID})
			}
		}

	case ObjectCluster:
		if dcID := r.findDatacenterForCluster(objectID); dcID != "" {
			ancestors = append(ancestors, ObjectRef{Type: ObjectDatacenter, ID: dcID})
		}

	case ObjectDatacenter:
		// Datacenter's parent is root (added by caller)

	case ObjectRoot:
		// Root has no ancestors
	}

	ancestors = append(ancestors, ObjectRef{Type: ObjectRoot, ID: ""})
	return ancestors
}

// findGuestLocation returns (nodeName, clusterName) for a VM or container
func (r *StateResolver) findGuestLocation(guestType, guestID string) (string, string) {
	if r.store == nil {
		return "", ""
	}

	vmid, err := strconv.Atoi(guestID)
	if err != nil {
		// guestID might be "cluster/vmid" format
		return "", ""
	}

	for _, clusterName := range r.store.GetClusterNames() {
		cs, ok := r.store.GetCluster(clusterName)
		if !ok {
			continue
		}

		if guestType == ObjectVM {
			if vm, ok := cs.GetVM(vmid); ok {
				return vm.Node, vm.Cluster
			}
		} else {
			if ct, ok := cs.GetContainer(vmid); ok {
				return ct.Node, ct.Cluster
			}
		}
	}
	return "", ""
}

// findNodeCluster returns the cluster name for a node
func (r *StateResolver) findNodeCluster(nodeName string) string {
	if r.store == nil {
		return ""
	}

	for _, clusterName := range r.store.GetClusterNames() {
		cs, ok := r.store.GetCluster(clusterName)
		if !ok {
			continue
		}
		for _, n := range cs.GetNodes() {
			if n.Node == nodeName {
				return clusterName
			}
		}
	}
	return ""
}

// findStorageCluster returns the cluster name for a storage pool
func (r *StateResolver) findStorageCluster(storageID string) string {
	if r.store == nil {
		return ""
	}

	for _, clusterName := range r.store.GetClusterNames() {
		cs, ok := r.store.GetCluster(clusterName)
		if !ok {
			continue
		}
		for _, s := range cs.GetStorage("") {
			id := fmt.Sprintf("%s-%s", s.Node, s.Storage)
			if id == storageID {
				return clusterName
			}
		}
	}
	return ""
}

// findDatacenterForCluster returns the datacenter ID for a cluster name
func (r *StateResolver) findDatacenterForCluster(clusterName string) string {
	if r.inventory == nil {
		return ""
	}

	ctx := context.Background()
	cluster, err := r.inventory.GetClusterByName(ctx, clusterName)
	if err != nil || cluster == nil {
		slog.Debug("rbac: cluster not found in inventory", "cluster", clusterName)
		return ""
	}
	if cluster.DatacenterID == nil {
		return ""
	}
	return *cluster.DatacenterID
}
