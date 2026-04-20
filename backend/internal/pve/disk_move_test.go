package pve

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestClient returns a Client whose baseURL points at the given test server.
// It bypasses NewClientForNode (which hardcodes https://) so we can use a
// plain http httptest server.
func newTestClient(srv *httptest.Server, nodeName string) *Client {
	return &Client{
		baseURL:     srv.URL + "/api2/json",
		tokenID:     "root@pam!test",
		tokenSecret: "secret",
		nodeName:    nodeName,
		clusterName: "test",
		httpClient:  &http.Client{Timeout: 5 * time.Second},
	}
}

func TestMoveVMDisk_RequestShape(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":"UPID:pve-test-1:0001:ABCD:MOVE_DISK:scsi0:root@pam:"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv, "pve-test-1")
	ctx := context.Background()

	upid, err := c.MoveVMDisk(ctx, 101, "scsi0", "local-lvm-new", true, "qcow2")
	if err != nil {
		t.Fatalf("MoveVMDisk: %v", err)
	}
	if !strings.HasPrefix(upid, "UPID:") {
		t.Errorf("expected UPID, got %q", upid)
	}

	wantPath := "/api2/json/nodes/pve-test-1/qemu/101/move_disk"
	if gotPath != wantPath {
		t.Errorf("path: want %q, got %q", wantPath, gotPath)
	}
	// Verify body carries all params (application/x-www-form-urlencoded)
	for _, want := range []string{"disk=scsi0", "storage=local-lvm-new", "delete=1", "format=qcow2"} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("body missing %q; got %q", want, gotBody)
		}
	}
}

func TestMoveVMDisk_DefaultsOmitOptional(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Write([]byte(`{"data":"UPID:x"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv, "pve-test-1")
	_, err := c.MoveVMDisk(context.Background(), 101, "scsi0", "local", false, "")
	if err != nil {
		t.Fatalf("MoveVMDisk: %v", err)
	}
	// delete=1 and format should NOT appear when unset
	if strings.Contains(gotBody, "delete=1") {
		t.Errorf("delete=1 should be omitted when DeleteSource=false; body=%q", gotBody)
	}
	if strings.Contains(gotBody, "format=") {
		t.Errorf("format should be omitted when format=\"\"; body=%q", gotBody)
	}
}

func TestMoveContainerVolume_RequestShape(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Write([]byte(`{"data":"UPID:pve-test-1:0002:BEEF:MOVE_VOL:rootfs:root@pam:"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv, "pve-test-1")
	upid, err := c.MoveContainerVolume(context.Background(), 200, "rootfs", "ceph-pool", true)
	if err != nil {
		t.Fatalf("MoveContainerVolume: %v", err)
	}
	if !strings.HasPrefix(upid, "UPID:") {
		t.Errorf("expected UPID, got %q", upid)
	}

	wantPath := "/api2/json/nodes/pve-test-1/lxc/200/move_volume"
	if gotPath != wantPath {
		t.Errorf("path: want %q, got %q", wantPath, gotPath)
	}
	for _, want := range []string{"volume=rootfs", "storage=ceph-pool", "delete=1"} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("body missing %q; got %q", want, gotBody)
		}
	}
}
