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

// recordingSrv returns a server that captures method/path/body of every
// request and replies with a fixed body. Used by the write-method tests.
func recordingSrv(t *testing.T, body string) (*httptest.Server, *struct {
	method string
	path   string
	body   string
	query  string
}) {
	t.Helper()
	rec := &struct {
		method string
		path   string
		body   string
		query  string
	}{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.method = r.Method
		rec.path = r.URL.Path
		rec.query = r.URL.RawQuery
		buf := make([]byte, r.ContentLength)
		if r.ContentLength > 0 {
			_, _ = r.Body.Read(buf)
		}
		rec.body = string(buf)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func TestCreateCephPool_FormsRequest(t *testing.T) {
	srv, rec := recordingSrv(t, `{"data":"UPID:pve1:0001:DEAD:CEPH_POOL_CREATE:rbd:root@pam:"}`)
	c := newTestClient(srv, "pve1")

	upid, err := c.CreateCephPool(context.Background(), CephPoolCreateOptions{
		Name:            "rbd",
		Size:            3,
		MinSize:         2,
		PGNum:           128,
		PGAutoscaleMode: "on",
		Application:     "rbd",
		AddStorages:     true,
	})
	if err != nil {
		t.Fatalf("CreateCephPool: %v", err)
	}
	if !strings.HasPrefix(upid, "UPID:") {
		t.Errorf("expected UPID, got %q", upid)
	}
	if rec.method != "POST" {
		t.Errorf("method: want POST, got %q", rec.method)
	}
	if want := "/api2/json/nodes/pve1/ceph/pool"; rec.path != want {
		t.Errorf("path: want %q, got %q", want, rec.path)
	}
	for _, want := range []string{"name=rbd", "size=3", "min_size=2", "pg_num=128", "pg_autoscale_mode=on", "application=rbd", "add_storages=1"} {
		if !strings.Contains(rec.body, want) {
			t.Errorf("body missing %q; got %q", want, rec.body)
		}
	}
}

func TestCreateCephPool_RequiresName(t *testing.T) {
	c := newTestClient(httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("server should not be hit when name is empty")
		w.WriteHeader(http.StatusInternalServerError)
	})), "pve1")
	defer c.httpClient.CloseIdleConnections()

	_, err := c.CreateCephPool(context.Background(), CephPoolCreateOptions{Size: 3})
	if err == nil {
		t.Fatal("expected error when name is empty")
	}
}

func TestCreateCephPool_OmitsZeroValues(t *testing.T) {
	srv, rec := recordingSrv(t, `{"data":"UPID:x"}`)
	c := newTestClient(srv, "pve1")

	_, err := c.CreateCephPool(context.Background(), CephPoolCreateOptions{Name: "minimal"})
	if err != nil {
		t.Fatalf("CreateCephPool: %v", err)
	}
	for _, omit := range []string{"size=", "min_size=", "pg_num=", "pg_autoscale_mode=", "application=", "crush_rule=", "add_storages="} {
		if strings.Contains(rec.body, omit) {
			t.Errorf("body should not contain %q (omit defaults); got %q", omit, rec.body)
		}
	}
	if !strings.Contains(rec.body, "name=minimal") {
		t.Errorf("body must contain name=minimal; got %q", rec.body)
	}
}

func TestUpdateCephPool_RequiresAtLeastOneField(t *testing.T) {
	c := newTestClient(httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("server should not be hit when no fields are set")
	})), "pve1")
	defer c.httpClient.CloseIdleConnections()

	if err := c.UpdateCephPool(context.Background(), "rbd", CephPoolUpdateOptions{}); err == nil {
		t.Fatal("expected error when no fields are set")
	}
}

func TestUpdateCephPool_PutsToNamedPath(t *testing.T) {
	srv, rec := recordingSrv(t, `{"data":null}`)
	c := newTestClient(srv, "pve1")

	if err := c.UpdateCephPool(context.Background(), "rbd", CephPoolUpdateOptions{Size: 2}); err != nil {
		t.Fatalf("UpdateCephPool: %v", err)
	}
	if rec.method != "PUT" {
		t.Errorf("method: want PUT, got %q", rec.method)
	}
	if want := "/api2/json/nodes/pve1/ceph/pool/rbd"; rec.path != want {
		t.Errorf("path: want %q, got %q", want, rec.path)
	}
	if !strings.Contains(rec.body, "size=2") {
		t.Errorf("body missing size=2; got %q", rec.body)
	}
}

func TestDeleteCephPool_PassesOptionsAsQuery(t *testing.T) {
	srv, rec := recordingSrv(t, `{"data":"UPID:pve1:0002:BEEF:CEPH_POOL_DELETE:rbd:root@pam:"}`)
	c := newTestClient(srv, "pve1")

	upid, err := c.DeleteCephPool(context.Background(), "rbd", CephPoolDeleteOptions{Force: true, RemoveStorages: true})
	if err != nil {
		t.Fatalf("DeleteCephPool: %v", err)
	}
	if !strings.HasPrefix(upid, "UPID:") {
		t.Errorf("expected UPID, got %q", upid)
	}
	if rec.method != "DELETE" {
		t.Errorf("method: want DELETE, got %q", rec.method)
	}
	if want := "/api2/json/nodes/pve1/ceph/pool/rbd"; rec.path != want {
		t.Errorf("path: want %q, got %q", want, rec.path)
	}
	for _, want := range []string{"force=1", "remove_storages=1"} {
		if !strings.Contains(rec.query, want) {
			t.Errorf("query missing %q; got %q", want, rec.query)
		}
	}
}

func TestSetCephFlag_TogglesViaCluster(t *testing.T) {
	srv, rec := recordingSrv(t, `{"data":null}`)
	c := newTestClient(srv, "pve1")

	if err := c.SetCephFlag(context.Background(), "noout", true); err != nil {
		t.Fatalf("SetCephFlag enable: %v", err)
	}
	if want := "/api2/json/cluster/ceph/flags/noout"; rec.path != want {
		t.Errorf("path: want %q, got %q", want, rec.path)
	}
	if rec.method != "PUT" {
		t.Errorf("method: want PUT, got %q", rec.method)
	}
	if !strings.Contains(rec.body, "value=1") {
		t.Errorf("body missing value=1; got %q", rec.body)
	}

	if err := c.SetCephFlag(context.Background(), "noout", false); err != nil {
		t.Fatalf("SetCephFlag disable: %v", err)
	}
	if !strings.Contains(rec.body, "value=0") {
		t.Errorf("body missing value=0 (disable); got %q", rec.body)
	}
}

func TestSetCephFlag_RequiresFlag(t *testing.T) {
	c := newTestClient(httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("server should not be hit when flag is empty")
	})), "pve1")
	defer c.httpClient.CloseIdleConnections()

	if err := c.SetCephFlag(context.Background(), "", true); err == nil {
		t.Fatal("expected error when flag is empty")
	}
}

func TestCreateCephOSD_FormsRequest(t *testing.T) {
	srv, rec := recordingSrv(t, `{"data":"UPID:pve1:0003:CAFE:CEPH_OSD_CREATE:/dev/sdb:root@pam:"}`)
	c := newTestClient(srv, "pve1")

	upid, err := c.CreateCephOSD(context.Background(), CephOSDCreateOptions{
		Dev:              "/dev/sdb",
		DBDev:            "/dev/nvme0n1",
		WALDev:           "/dev/nvme0n1",
		DBDevSize:        50,
		Encrypted:        true,
		CrushDeviceClass: "ssd",
		OSDsPerDevice:    2,
	})
	if err != nil {
		t.Fatalf("CreateCephOSD: %v", err)
	}
	if !strings.HasPrefix(upid, "UPID:") {
		t.Errorf("expected UPID, got %q", upid)
	}
	if rec.method != "POST" {
		t.Errorf("method: want POST, got %q", rec.method)
	}
	if want := "/api2/json/nodes/pve1/ceph/osd"; rec.path != want {
		t.Errorf("path: want %q, got %q", want, rec.path)
	}
	for _, want := range []string{"dev=%2Fdev%2Fsdb", "db_dev=%2Fdev%2Fnvme0n1", "wal_dev=%2Fdev%2Fnvme0n1", "db_dev_size=50", "encrypted=1", "crush_device_class=ssd", "osds_per_device=2"} {
		if !strings.Contains(rec.body, want) {
			t.Errorf("body missing %q; got %q", want, rec.body)
		}
	}
}

func TestCreateCephOSD_RequiresDev(t *testing.T) {
	c := newTestClient(httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("server should not be hit when dev is empty")
	})), "pve1")
	defer c.httpClient.CloseIdleConnections()

	if _, err := c.CreateCephOSD(context.Background(), CephOSDCreateOptions{}); err == nil {
		t.Fatal("expected error when dev is empty")
	}
}

func TestDeleteCephOSD_PassesCleanupAsQuery(t *testing.T) {
	srv, rec := recordingSrv(t, `{"data":"UPID:pve1:0004:DEAD:CEPH_OSD_DESTROY:0:root@pam:"}`)
	c := newTestClient(srv, "pve1")

	upid, err := c.DeleteCephOSD(context.Background(), 0, true)
	if err != nil {
		t.Fatalf("DeleteCephOSD: %v", err)
	}
	if !strings.HasPrefix(upid, "UPID:") {
		t.Errorf("expected UPID, got %q", upid)
	}
	if rec.method != "DELETE" {
		t.Errorf("method: want DELETE, got %q", rec.method)
	}
	if want := "/api2/json/nodes/pve1/ceph/osd/0"; rec.path != want {
		t.Errorf("path: want %q, got %q", want, rec.path)
	}
	if rec.query != "cleanup=1" {
		t.Errorf("query: want cleanup=1, got %q", rec.query)
	}
}

func TestDeleteCephOSD_NoCleanupOmitsQuery(t *testing.T) {
	srv, rec := recordingSrv(t, `{"data":"UPID:x"}`)
	c := newTestClient(srv, "pve1")

	if _, err := c.DeleteCephOSD(context.Background(), 1, false); err != nil {
		t.Fatalf("DeleteCephOSD: %v", err)
	}
	if rec.query != "" {
		t.Errorf("query should be empty when cleanup=false, got %q", rec.query)
	}
}

func TestSetCephOSDIn(t *testing.T) {
	srv, rec := recordingSrv(t, `{"data":null}`)
	c := newTestClient(srv, "pve1")

	if err := c.SetCephOSDIn(context.Background(), 3); err != nil {
		t.Fatalf("SetCephOSDIn: %v", err)
	}
	if rec.method != "POST" {
		t.Errorf("method: want POST, got %q", rec.method)
	}
	if want := "/api2/json/nodes/pve1/ceph/osd/3/in"; rec.path != want {
		t.Errorf("path: want %q, got %q", want, rec.path)
	}
}

func TestSetCephOSDOut(t *testing.T) {
	srv, rec := recordingSrv(t, `{"data":null}`)
	c := newTestClient(srv, "pve1")

	if err := c.SetCephOSDOut(context.Background(), 5); err != nil {
		t.Fatalf("SetCephOSDOut: %v", err)
	}
	if want := "/api2/json/nodes/pve1/ceph/osd/5/out"; rec.path != want {
		t.Errorf("path: want %q, got %q", want, rec.path)
	}
}

func TestScrubCephOSD(t *testing.T) {
	srv, rec := recordingSrv(t, `{"data":null}`)
	c := newTestClient(srv, "pve1")

	if err := c.ScrubCephOSD(context.Background(), 7, false); err != nil {
		t.Fatalf("ScrubCephOSD shallow: %v", err)
	}
	if rec.body != "" {
		t.Errorf("shallow scrub: body should be empty, got %q", rec.body)
	}
	if want := "/api2/json/nodes/pve1/ceph/osd/7/scrub"; rec.path != want {
		t.Errorf("path: want %q, got %q", want, rec.path)
	}

	if err := c.ScrubCephOSD(context.Background(), 7, true); err != nil {
		t.Fatalf("ScrubCephOSD deep: %v", err)
	}
	if !strings.Contains(rec.body, "deep=1") {
		t.Errorf("deep scrub: body must contain deep=1, got %q", rec.body)
	}
}

func TestCreateCephMON_DefaultBody(t *testing.T) {
	srv, rec := recordingSrv(t, `{"data":"UPID:pve1:0010:AAAA:CEPH_MON_CREATE:pve1:root@pam:"}`)
	c := newTestClient(srv, "pve1")

	upid, err := c.CreateCephMON(context.Background(), "")
	if err != nil {
		t.Fatalf("CreateCephMON: %v", err)
	}
	if !strings.HasPrefix(upid, "UPID:") {
		t.Errorf("expected UPID, got %q", upid)
	}
	if rec.method != "POST" {
		t.Errorf("method: want POST, got %q", rec.method)
	}
	if want := "/api2/json/nodes/pve1/ceph/mon"; rec.path != want {
		t.Errorf("path: want %q, got %q", want, rec.path)
	}
	if rec.body != "" {
		t.Errorf("body should be empty when no monAddress, got %q", rec.body)
	}
}

func TestCreateCephMON_WithAddress(t *testing.T) {
	srv, rec := recordingSrv(t, `{"data":"UPID:x"}`)
	c := newTestClient(srv, "pve1")

	if _, err := c.CreateCephMON(context.Background(), "10.0.0.1"); err != nil {
		t.Fatalf("CreateCephMON: %v", err)
	}
	if !strings.Contains(rec.body, "mon-address=10.0.0.1") {
		t.Errorf("body missing mon-address; got %q", rec.body)
	}
}

func TestDeleteCephMON_RequiresMonID(t *testing.T) {
	c := newTestClient(httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("server should not be hit when monid is empty")
	})), "pve1")
	defer c.httpClient.CloseIdleConnections()

	if _, err := c.DeleteCephMON(context.Background(), ""); err == nil {
		t.Fatal("expected error when monid is empty")
	}
}

func TestDeleteCephMON_PathFormat(t *testing.T) {
	srv, rec := recordingSrv(t, `{"data":"UPID:pve1:0011:BBBB:CEPH_MON_DESTROY:pve2:root@pam:"}`)
	c := newTestClient(srv, "pve1")

	upid, err := c.DeleteCephMON(context.Background(), "pve2")
	if err != nil {
		t.Fatalf("DeleteCephMON: %v", err)
	}
	if !strings.HasPrefix(upid, "UPID:") {
		t.Errorf("expected UPID, got %q", upid)
	}
	if rec.method != "DELETE" {
		t.Errorf("method: want DELETE, got %q", rec.method)
	}
	if want := "/api2/json/nodes/pve1/ceph/mon/pve2"; rec.path != want {
		t.Errorf("path: want %q, got %q", want, rec.path)
	}
}

func TestCreateCephMGR(t *testing.T) {
	srv, rec := recordingSrv(t, `{"data":"UPID:pve1:0012:CCCC:CEPH_MGR_CREATE:pve1:root@pam:"}`)
	c := newTestClient(srv, "pve1")

	upid, err := c.CreateCephMGR(context.Background())
	if err != nil {
		t.Fatalf("CreateCephMGR: %v", err)
	}
	if !strings.HasPrefix(upid, "UPID:") {
		t.Errorf("expected UPID, got %q", upid)
	}
	if rec.method != "POST" || rec.path != "/api2/json/nodes/pve1/ceph/mgr" {
		t.Errorf("unexpected request: %s %s", rec.method, rec.path)
	}
}

func TestDeleteCephMGR_PathFormat(t *testing.T) {
	srv, rec := recordingSrv(t, `{"data":"UPID:x"}`)
	c := newTestClient(srv, "pve1")

	if _, err := c.DeleteCephMGR(context.Background(), "pve3"); err != nil {
		t.Fatalf("DeleteCephMGR: %v", err)
	}
	if want := "/api2/json/nodes/pve1/ceph/mgr/pve3"; rec.path != want {
		t.Errorf("path: want %q, got %q", want, rec.path)
	}
	if rec.method != "DELETE" {
		t.Errorf("method: want DELETE, got %q", rec.method)
	}
}

func TestDeleteCephMGR_RequiresMgrID(t *testing.T) {
	c := newTestClient(httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("server should not be hit when mgrid is empty")
	})), "pve1")
	defer c.httpClient.CloseIdleConnections()

	if _, err := c.DeleteCephMGR(context.Background(), ""); err == nil {
		t.Fatal("expected error when mgrid is empty")
	}
}
