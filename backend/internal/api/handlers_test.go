package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moconnor/pcenter/internal/pve"
	"github.com/moconnor/pcenter/internal/state"
)

// newTestHandler creates a Handler with a fresh store and no poller/origins.
func newTestHandler(t *testing.T) (*Handler, *state.Store) {
	t.Helper()
	store := state.New()
	h := NewHandler(store, nil, nil)
	return h, store
}

// populateStore adds a cluster with nodes, VMs, and containers to the store.
func populateStore(t *testing.T, store *state.Store) {
	t.Helper()
	cs := store.GetOrCreateCluster("test-cluster")
	cs.UpdateNode("pve01",
		pve.Node{Node: "pve01", Status: "online", CPU: 0.25, MaxCPU: 8, Mem: 4 << 30, MaxMem: 16 << 30, Uptime: 3600},
		[]pve.VM{
			{VMID: 100, Name: "web-server", Node: "pve01", Status: "running", CPU: 0.5, CPUs: 4, Mem: 2 << 30, MaxMem: 4 << 30},
			{VMID: 101, Name: "db-server", Node: "pve01", Status: "stopped", CPU: 0, CPUs: 2, Mem: 0, MaxMem: 2 << 30},
		},
		[]pve.Container{
			{VMID: 200, Name: "nginx-proxy", Node: "pve01", Status: "running", CPU: 0.1, CPUs: 2, Mem: 512 << 20, MaxMem: 1 << 30},
		},
		nil, nil,
	)
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, v interface{}) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
	if err := json.NewDecoder(rec.Body).Decode(v); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
}

func TestGetSummary(t *testing.T) {
	h, store := newTestHandler(t)
	populateStore(t, store)

	req := httptest.NewRequest("GET", "/api/summary", nil)
	rec := httptest.NewRecorder()
	h.GetSummary(rec, req)

	var summary state.Summary
	decodeJSON(t, rec, &summary)

	if summary.TotalNodes != 1 {
		t.Errorf("TotalNodes: got %d, want 1", summary.TotalNodes)
	}
	if summary.OnlineNodes != 1 {
		t.Errorf("OnlineNodes: got %d, want 1", summary.OnlineNodes)
	}
	if summary.TotalVMs != 2 {
		t.Errorf("TotalVMs: got %d, want 2", summary.TotalVMs)
	}
	if summary.RunningVMs != 1 {
		t.Errorf("RunningVMs: got %d, want 1", summary.RunningVMs)
	}
	if summary.TotalContainers != 1 {
		t.Errorf("TotalContainers: got %d, want 1", summary.TotalContainers)
	}
	if summary.RunningCTs != 1 {
		t.Errorf("RunningCTs: got %d, want 1", summary.RunningCTs)
	}
}

func TestGetSummaryEmpty(t *testing.T) {
	h, _ := newTestHandler(t)

	req := httptest.NewRequest("GET", "/api/summary", nil)
	rec := httptest.NewRecorder()
	h.GetSummary(rec, req)

	var summary state.Summary
	decodeJSON(t, rec, &summary)

	if summary.TotalNodes != 0 {
		t.Errorf("expected 0 nodes on empty store, got %d", summary.TotalNodes)
	}
}

func TestGetNodes(t *testing.T) {
	h, store := newTestHandler(t)
	populateStore(t, store)

	req := httptest.NewRequest("GET", "/api/nodes", nil)
	rec := httptest.NewRecorder()
	h.GetNodes(rec, req)

	var nodes []json.RawMessage
	decodeJSON(t, rec, &nodes)

	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}

	// Decode the node to check fields
	var node struct {
		Node   string `json:"node"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(nodes[0], &node); err != nil {
		t.Fatalf("unmarshal node: %v", err)
	}
	if node.Node != "pve01" {
		t.Errorf("node name: got %q, want pve01", node.Node)
	}
	if node.Status != "online" {
		t.Errorf("node status: got %q, want online", node.Status)
	}
}

func TestGetVMs(t *testing.T) {
	h, store := newTestHandler(t)
	populateStore(t, store)

	req := httptest.NewRequest("GET", "/api/vms", nil)
	rec := httptest.NewRecorder()
	h.GetVMs(rec, req)

	var vms []pve.VM
	decodeJSON(t, rec, &vms)

	if len(vms) != 2 {
		t.Fatalf("expected 2 VMs, got %d", len(vms))
	}

	found := map[int]bool{}
	for _, vm := range vms {
		found[vm.VMID] = true
	}
	if !found[100] || !found[101] {
		t.Errorf("expected VMIDs 100 and 101, got %v", found)
	}
}

func TestGetContainers(t *testing.T) {
	h, store := newTestHandler(t)
	populateStore(t, store)

	req := httptest.NewRequest("GET", "/api/containers", nil)
	rec := httptest.NewRecorder()
	h.GetContainers(rec, req)

	var cts []pve.Container
	decodeJSON(t, rec, &cts)

	if len(cts) != 1 {
		t.Fatalf("expected 1 container, got %d", len(cts))
	}
	if cts[0].VMID != 200 {
		t.Errorf("container VMID: got %d, want 200", cts[0].VMID)
	}
	if cts[0].Name != "nginx-proxy" {
		t.Errorf("container name: got %q, want nginx-proxy", cts[0].Name)
	}
}

func TestGetAllGuests(t *testing.T) {
	h, store := newTestHandler(t)
	populateStore(t, store)

	req := httptest.NewRequest("GET", "/api/guests", nil)
	rec := httptest.NewRecorder()
	h.GetAllGuests(rec, req)

	var guests []struct {
		VMID   int    `json:"vmid"`
		Name   string `json:"name"`
		Type   string `json:"type"`
		Status string `json:"status"`
	}
	decodeJSON(t, rec, &guests)

	if len(guests) != 3 {
		t.Fatalf("expected 3 guests (2 VMs + 1 CT), got %d", len(guests))
	}

	// Should be sorted by VMID
	for i := 1; i < len(guests); i++ {
		if guests[i].VMID < guests[i-1].VMID {
			t.Errorf("guests not sorted by VMID: %d after %d", guests[i].VMID, guests[i-1].VMID)
		}
	}

	// Check types
	typeCount := map[string]int{}
	for _, g := range guests {
		typeCount[g.Type]++
	}
	if typeCount["qemu"] != 2 {
		t.Errorf("expected 2 qemu guests, got %d", typeCount["qemu"])
	}
	if typeCount["lxc"] != 1 {
		t.Errorf("expected 1 lxc guest, got %d", typeCount["lxc"])
	}
}

func TestGetClusters(t *testing.T) {
	h, store := newTestHandler(t)
	populateStore(t, store)

	// Add a second cluster
	cs2 := store.GetOrCreateCluster("prod-cluster")
	cs2.UpdateNode("pve02",
		pve.Node{Node: "pve02", Status: "online", MaxCPU: 4},
		nil, nil, nil, nil,
	)

	req := httptest.NewRequest("GET", "/api/clusters", nil)
	rec := httptest.NewRecorder()
	h.GetClusters(rec, req)

	var gs state.GlobalSummary
	decodeJSON(t, rec, &gs)

	if len(gs.Clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(gs.Clusters))
	}

	names := map[string]bool{}
	for _, c := range gs.Clusters {
		names[c.Name] = true
	}
	if !names["test-cluster"] || !names["prod-cluster"] {
		t.Errorf("expected test-cluster and prod-cluster, got %v", names)
	}

	// Total should aggregate
	if gs.Total.TotalNodes != 2 {
		t.Errorf("total nodes: got %d, want 2", gs.Total.TotalNodes)
	}
}

func TestGetVMsEmpty(t *testing.T) {
	h, _ := newTestHandler(t)

	req := httptest.NewRequest("GET", "/api/vms", nil)
	rec := httptest.NewRecorder()
	h.GetVMs(rec, req)

	// Should return null or empty array, not error
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestGetContainersEmpty(t *testing.T) {
	h, _ := newTestHandler(t)

	req := httptest.NewRequest("GET", "/api/containers", nil)
	rec := httptest.NewRecorder()
	h.GetContainers(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestGetAllGuestsEmpty(t *testing.T) {
	h, _ := newTestHandler(t)

	req := httptest.NewRequest("GET", "/api/guests", nil)
	rec := httptest.NewRecorder()
	h.GetAllGuests(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// TestGetClusterCeph_404s covers the two distinct nil paths: the cluster
// itself doesn't exist, and the cluster exists but has no Ceph topology
// (not installed or not yet polled). Both surface as 404.
func TestGetClusterCeph_404s(t *testing.T) {
	h, _ := newTestHandler(t)

	t.Run("unknown cluster", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/clusters/nope/ceph", nil)
		req.SetPathValue("cluster", "nope")
		rec := httptest.NewRecorder()
		h.GetClusterCeph(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("cluster exists but no Ceph", func(t *testing.T) {
		_, store := newTestHandler(t)
		store.GetOrCreateCluster("test-cluster")
		h := NewHandler(store, nil, nil)

		req := httptest.NewRequest("GET", "/api/clusters/test-cluster/ceph", nil)
		req.SetPathValue("cluster", "test-cluster")
		rec := httptest.NewRecorder()
		h.GetClusterCeph(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

// TestCreateClusterCephPool_Validation covers the request-validation paths
// that don't require a live PVE backend: bad JSON, missing name, unknown
// cluster, no poller (agent-only mode). The happy path is exercised via
// the pve client's TestCreateCephPool_FormsRequest test.
func TestCreateClusterCephPool_Validation(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*state.Store)
		body     string
		wantCode int
	}{
		{
			name:     "invalid JSON",
			body:     `{not json`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "missing name",
			body:     `{"size":3}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "unknown cluster",
			body:     `{"name":"rbd"}`,
			wantCode: http.StatusNotFound,
		},
		{
			name: "agent-only mode (no poller)",
			setup: func(s *state.Store) {
				s.GetOrCreateCluster("test-cluster")
			},
			body:     `{"name":"rbd"}`,
			wantCode: http.StatusServiceUnavailable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := state.New()
			if tt.setup != nil {
				tt.setup(store)
			}
			h := NewHandler(store, nil, nil)

			req := httptest.NewRequest("POST", "/api/clusters/test-cluster/ceph/pool", strings.NewReader(tt.body))
			req.SetPathValue("cluster", "test-cluster")
			rec := httptest.NewRecorder()
			h.CreateClusterCephPool(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("got status %d, want %d (body: %s)", rec.Code, tt.wantCode, rec.Body.String())
			}
		})
	}
}

// TestToggleClusterCephFlag_RejectsUnsupportedFlag verifies the flag
// allowlist gates obvious typos before any backend call.
func TestToggleClusterCephFlag_RejectsUnsupportedFlag(t *testing.T) {
	h, _ := newTestHandler(t)

	req := httptest.NewRequest("POST", "/api/clusters/test/ceph/flags/sortbitwise", strings.NewReader(`{"enable":true}`))
	req.SetPathValue("cluster", "test")
	req.SetPathValue("flag", "sortbitwise")
	rec := httptest.NewRecorder()
	h.ToggleClusterCephFlag(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unsupported flag, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestUpdateClusterCephPool_RejectsEmptyPool covers the path-param check.
func TestUpdateClusterCephPool_RejectsEmptyPool(t *testing.T) {
	h, _ := newTestHandler(t)

	req := httptest.NewRequest("PUT", "/api/clusters/test/ceph/pool/", strings.NewReader(`{"size":2}`))
	req.SetPathValue("cluster", "test")
	req.SetPathValue("pool", "")
	rec := httptest.NewRecorder()
	h.UpdateClusterCephPool(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty pool name, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestCreateClusterCephOSD_Validation covers the validation paths reachable
// without a live PVE backend.
func TestCreateClusterCephOSD_Validation(t *testing.T) {
	tests := []struct {
		name     string
		node     string
		body     string
		wantCode int
	}{
		{name: "missing node path-param", node: "", body: `{"dev":"/dev/sdb"}`, wantCode: http.StatusBadRequest},
		{name: "invalid JSON", node: "pve1", body: `{nope`, wantCode: http.StatusBadRequest},
		{name: "missing dev", node: "pve1", body: `{"encrypted":true}`, wantCode: http.StatusBadRequest},
		{name: "unknown cluster", node: "pve1", body: `{"dev":"/dev/sdb"}`, wantCode: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newTestHandler(t)

			req := httptest.NewRequest("POST", "/api/clusters/x/nodes/x/ceph/osd", strings.NewReader(tt.body))
			req.SetPathValue("cluster", "test-cluster")
			req.SetPathValue("node", tt.node)
			rec := httptest.NewRecorder()
			h.CreateClusterCephOSD(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("got status %d, want %d (body: %s)", rec.Code, tt.wantCode, rec.Body.String())
			}
		})
	}
}

// TestOSDActionHandlers_RejectInvalidOSDID covers the parseOSDID guard
// shared by DELETE / in / out / scrub.
func TestOSDActionHandlers_RejectInvalidOSDID(t *testing.T) {
	h, _ := newTestHandler(t)

	handlers := map[string]func(http.ResponseWriter, *http.Request){
		"delete": h.DeleteClusterCephOSD,
		"in":     h.SetClusterCephOSDIn,
		"out":    h.SetClusterCephOSDOut,
		"scrub":  h.ScrubClusterCephOSD,
	}
	for name, fn := range handlers {
		t.Run(name+"_non_numeric", func(t *testing.T) {
			req := httptest.NewRequest("POST", "/", nil)
			req.SetPathValue("cluster", "c")
			req.SetPathValue("node", "n")
			req.SetPathValue("osdid", "abc")
			rec := httptest.NewRecorder()
			fn(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s: expected 400 for non-numeric osdid, got %d", name, rec.Code)
			}
		})
		t.Run(name+"_negative", func(t *testing.T) {
			req := httptest.NewRequest("POST", "/", nil)
			req.SetPathValue("cluster", "c")
			req.SetPathValue("node", "n")
			req.SetPathValue("osdid", "-1")
			rec := httptest.NewRecorder()
			fn(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s: expected 400 for negative osdid, got %d", name, rec.Code)
			}
		})
	}
}

// TestCephMONHandlers_Validation covers create/delete validation for
// monitors and managers (same shape — pickNodeClient gates the rest).
func TestCephMONHandlers_Validation(t *testing.T) {
	h, _ := newTestHandler(t)

	t.Run("create requires node path-param", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader(`{}`))
		req.SetPathValue("cluster", "c")
		req.SetPathValue("node", "")
		rec := httptest.NewRecorder()
		h.CreateClusterCephMON(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rec.Code)
		}
	})
	t.Run("create rejects bad JSON when body present", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", strings.NewReader(`{not json`))
		req.SetPathValue("cluster", "c")
		req.SetPathValue("node", "pve1")
		rec := httptest.NewRecorder()
		h.CreateClusterCephMON(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rec.Code)
		}
	})
	t.Run("create allows empty body", func(t *testing.T) {
		// Empty body means "use defaults" — should pass JSON parsing and
		// fall through to pickNodeClient, which returns 404 (cluster not found)
		// for our empty test handler. Net result: 404, not 400.
		req := httptest.NewRequest("POST", "/", nil)
		req.SetPathValue("cluster", "missing-cluster")
		req.SetPathValue("node", "pve1")
		rec := httptest.NewRecorder()
		h.CreateClusterCephMON(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404 (passed body validation), got %d: %s", rec.Code, rec.Body.String())
		}
	})
	t.Run("delete requires monid", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/", nil)
		req.SetPathValue("cluster", "c")
		req.SetPathValue("node", "pve1")
		req.SetPathValue("monid", "")
		rec := httptest.NewRecorder()
		h.DeleteClusterCephMON(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rec.Code)
		}
	})
}

func TestCephMGRHandlers_Validation(t *testing.T) {
	h, _ := newTestHandler(t)

	t.Run("create requires node path-param", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", nil)
		req.SetPathValue("cluster", "c")
		req.SetPathValue("node", "")
		rec := httptest.NewRecorder()
		h.CreateClusterCephMGR(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rec.Code)
		}
	})
	t.Run("delete requires mgrid", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/", nil)
		req.SetPathValue("cluster", "c")
		req.SetPathValue("node", "pve1")
		req.SetPathValue("mgrid", "")
		rec := httptest.NewRecorder()
		h.DeleteClusterCephMGR(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rec.Code)
		}
	})
}

// TestGetClusterCeph_ReturnsTopology populates the topology and verifies
// the JSON round-trip preserves the cluster-wide shape.
func TestGetClusterCeph_ReturnsTopology(t *testing.T) {
	store := state.New()
	cs := store.GetOrCreateCluster("prod")
	cs.SetCephTopology(&pve.CephCluster{
		MONs:  []pve.CephMON{{Name: "pve1", Quorum: true, State: "leader"}},
		OSDs:  []pve.CephOSD{{ID: 0, Name: "osd.0", Status: "up", In: true, Host: "pve1"}},
		Pools: []pve.CephPool{{Name: "rbd", Size: 3, MinSize: 2, PGNum: 128}},
		Flags: pve.CephFlags{NoOut: true},
	})
	h := NewHandler(store, nil, nil)

	req := httptest.NewRequest("GET", "/api/clusters/prod/ceph", nil)
	req.SetPathValue("cluster", "prod")
	rec := httptest.NewRecorder()
	h.GetClusterCeph(rec, req)

	var got pve.CephCluster
	decodeJSON(t, rec, &got)

	if len(got.MONs) != 1 || got.MONs[0].State != "leader" {
		t.Errorf("MONs lost in roundtrip: %+v", got.MONs)
	}
	if len(got.OSDs) != 1 || got.OSDs[0].Host != "pve1" {
		t.Errorf("OSDs lost in roundtrip: %+v", got.OSDs)
	}
	if len(got.Pools) != 1 || got.Pools[0].Name != "rbd" || got.Pools[0].Size != 3 {
		t.Errorf("Pools lost in roundtrip: %+v", got.Pools)
	}
	if !got.Flags.NoOut {
		t.Error("NoOut flag not preserved")
	}
}
