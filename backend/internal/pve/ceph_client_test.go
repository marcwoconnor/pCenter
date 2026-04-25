package pve

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// cephSrv returns a single-route test server that serves a fixed JSON body
// when the requested path ends with the given suffix, and 404s otherwise.
// The handler also records the path it served so tests can assert it.
func cephSrv(t *testing.T, suffix, body string) (*httptest.Server, *string) {
	t.Helper()
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if !strings.HasSuffix(r.URL.Path, suffix) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &gotPath
}

func TestListCephOSDs_FlattensTree(t *testing.T) {
	body := `{"data":{
		"root":{
			"id":-1,"name":"default","type":"root",
			"children":[
				{"id":-2,"name":"pve-test-1","type":"host","children":[
					{"id":0,"name":"osd.0","type":"osd","status":"up","device_class":"ssd","crush_weight":1.0,"reweight":1.0},
					{"id":1,"name":"osd.1","type":"osd","status":"down","device_class":"ssd","crush_weight":1.0,"reweight":0.0}
				]},
				{"id":-3,"name":"pve-test-2","type":"host","children":[
					{"id":2,"name":"osd.2","type":"osd","status":"up","device_class":"hdd","crush_weight":2.0,"reweight":1.0}
				]}
			]
		},
		"flags":""
	}}`
	srv, gotPath := cephSrv(t, "/ceph/osd", body)
	c := newTestClient(srv, "pve-test-1")

	osds, err := c.ListCephOSDs(context.Background())
	if err != nil {
		t.Fatalf("ListCephOSDs: %v", err)
	}
	if want := "/api2/json/nodes/pve-test-1/ceph/osd"; *gotPath != want {
		t.Errorf("path: want %q, got %q", want, *gotPath)
	}
	if got := len(osds); got != 3 {
		t.Fatalf("expected 3 OSDs, got %d", got)
	}

	// Spot-check the host attribution + in-flag derivation.
	wantHost := map[int]string{0: "pve-test-1", 1: "pve-test-1", 2: "pve-test-2"}
	wantIn := map[int]bool{0: true, 1: false, 2: true}
	for _, o := range osds {
		if o.Host != wantHost[o.ID] {
			t.Errorf("osd.%d host: want %q, got %q", o.ID, wantHost[o.ID], o.Host)
		}
		if o.In != wantIn[o.ID] {
			t.Errorf("osd.%d in: want %v, got %v", o.ID, wantIn[o.ID], o.In)
		}
	}
}

func TestListCephMONs(t *testing.T) {
	body := `{"data":[
		{"name":"pve1","addr":"10.0.0.1:6789","host":"pve1","rank":0,"quorum":true,"state":"leader"},
		{"name":"pve2","addr":"10.0.0.2:6789","host":"pve2","rank":1,"quorum":true,"state":"peon"}
	]}`
	srv, gotPath := cephSrv(t, "/ceph/mon", body)
	c := newTestClient(srv, "pve1")

	mons, err := c.ListCephMONs(context.Background())
	if err != nil {
		t.Fatalf("ListCephMONs: %v", err)
	}
	if want := "/api2/json/nodes/pve1/ceph/mon"; *gotPath != want {
		t.Errorf("path: want %q, got %q", want, *gotPath)
	}
	if len(mons) != 2 || mons[0].State != "leader" || !mons[1].Quorum {
		t.Errorf("unexpected mons: %+v", mons)
	}
}

func TestListCephMGRs(t *testing.T) {
	body := `{"data":[{"name":"pve1","host":"pve1","state":"active"},{"name":"pve2","host":"pve2","state":"standby"}]}`
	srv, _ := cephSrv(t, "/ceph/mgr", body)
	c := newTestClient(srv, "pve1")

	mgrs, err := c.ListCephMGRs(context.Background())
	if err != nil {
		t.Fatalf("ListCephMGRs: %v", err)
	}
	if len(mgrs) != 2 || mgrs[0].State != "active" {
		t.Errorf("unexpected mgrs: %+v", mgrs)
	}
}

func TestListCephPools(t *testing.T) {
	body := `{"data":[
		{"id":1,"pool_name":"rbd","size":3,"min_size":2,"pg_num":128,"crush_rule":0,"application":"rbd","bytes_used":1024,"max_avail":2048},
		{"id":2,"pool_name":"cephfs_data","size":3,"min_size":2,"pg_num":64,"crush_rule":0,"application":"cephfs"}
	]}`
	srv, _ := cephSrv(t, "/ceph/pool", body)
	c := newTestClient(srv, "pve1")

	pools, err := c.ListCephPools(context.Background())
	if err != nil {
		t.Fatalf("ListCephPools: %v", err)
	}
	if len(pools) != 2 {
		t.Fatalf("expected 2 pools, got %d", len(pools))
	}
	if pools[0].Name != "rbd" || pools[0].Size != 3 || pools[0].Application != "rbd" {
		t.Errorf("unexpected pool[0]: %+v", pools[0])
	}
}

func TestGetCephRules(t *testing.T) {
	body := `{"data":[{"rule_id":0,"rule_name":"replicated_rule","ruleset":0,"type":1}]}`
	srv, _ := cephSrv(t, "/ceph/rules", body)
	c := newTestClient(srv, "pve1")

	rules, err := c.GetCephRules(context.Background())
	if err != nil {
		t.Fatalf("GetCephRules: %v", err)
	}
	if len(rules) != 1 || rules[0].Name != "replicated_rule" {
		t.Errorf("unexpected rules: %+v", rules)
	}
}

func TestGetCephFlags_Translates(t *testing.T) {
	// PVE returns array of {name, value, description}; client must translate
	// to the typed struct.
	body := `{"data":[
		{"name":"noout","value":true,"description":"OSDs will not be marked out"},
		{"name":"noscrub","value":false,"description":"..."},
		{"name":"nodeep-scrub","value":true,"description":"..."},
		{"name":"unrelated-flag","value":true,"description":"should be ignored"}
	]}`
	srv, gotPath := cephSrv(t, "/cluster/ceph/flags", body)
	c := newTestClient(srv, "pve1")

	flags, err := c.GetCephFlags(context.Background())
	if err != nil {
		t.Fatalf("GetCephFlags: %v", err)
	}
	if want := "/api2/json/cluster/ceph/flags"; *gotPath != want {
		t.Errorf("path: want %q, got %q", want, *gotPath)
	}
	if !flags.NoOut {
		t.Error("noout should be true")
	}
	if flags.NoScrub {
		t.Error("noscrub should be false")
	}
	if !flags.NoDeepScrub {
		t.Error("nodeep-scrub should be true")
	}
	if flags.NoIn || flags.NoUp {
		t.Errorf("unset flags must default false, got %+v", flags)
	}
}

func TestListCephFS(t *testing.T) {
	body := `{"data":[{"name":"cephfs","metadata_pool":"cephfs_metadata","data_pools":["cephfs_data"]}]}`
	srv, _ := cephSrv(t, "/ceph/fs", body)
	c := newTestClient(srv, "pve1")

	fs, err := c.ListCephFS(context.Background())
	if err != nil {
		t.Fatalf("ListCephFS: %v", err)
	}
	if len(fs) != 1 || fs[0].Name != "cephfs" || len(fs[0].DataPools) != 1 {
		t.Errorf("unexpected fs: %+v", fs)
	}
}
