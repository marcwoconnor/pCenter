package activity

import (
	"fmt"
	"testing"
	"time"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB(:memory:) failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpenDB_CreatesTable(t *testing.T) {
	db := openTestDB(t)

	// Verify the activity table exists by running a query against it
	rows, err := db.conn.Query("SELECT count(*) FROM activity")
	if err != nil {
		t.Fatalf("query activity table: %v", err)
	}
	defer rows.Close()

	var count int
	rows.Next()
	if err := rows.Scan(&count); err != nil {
		t.Fatalf("scan count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 rows in fresh db, got %d", count)
	}
}

func TestActivity_LogAndCallbackFires(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db, 30)

	var callbackEntry Entry
	called := false
	svc.OnLog(func(e Entry) {
		called = true
		callbackEntry = e
	})

	svc.Log(Entry{
		Action:       ActionVMStart,
		ResourceType: "vm",
		ResourceID:   "100",
		ResourceName: "test-vm",
		Cluster:      "cluster1",
		Details:      "started vm",
	})

	if !called {
		t.Fatal("OnLog callback was not called")
	}
	if callbackEntry.ID == 0 {
		t.Fatal("callback entry should have a non-zero ID")
	}
	if callbackEntry.Action != ActionVMStart {
		t.Errorf("expected action %q, got %q", ActionVMStart, callbackEntry.Action)
	}
	if callbackEntry.Status != StatusSuccess {
		t.Errorf("expected status %q, got %q", StatusSuccess, callbackEntry.Status)
	}
}

func TestActivity_LogAndQuery(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db, 30)

	svc.Log(Entry{
		Action:       ActionVMStart,
		ResourceType: "vm",
		ResourceID:   "100",
		Cluster:      "cluster1",
	})

	entries, err := svc.Query(QueryParams{Limit: 10})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Action != ActionVMStart {
		t.Errorf("expected action %q, got %q", ActionVMStart, entries[0].Action)
	}
	if entries[0].ResourceID != "100" {
		t.Errorf("expected resource_id %q, got %q", "100", entries[0].ResourceID)
	}
}

func TestActivity_QueryFilterByCluster(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db, 30)

	svc.Log(Entry{Action: ActionVMStart, ResourceType: "vm", ResourceID: "100", Cluster: "clusterA"})
	svc.Log(Entry{Action: ActionVMStop, ResourceType: "vm", ResourceID: "101", Cluster: "clusterB"})

	entries, err := svc.Query(QueryParams{Cluster: "clusterA"})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry for clusterA, got %d", len(entries))
	}
	if entries[0].Cluster != "clusterA" {
		t.Errorf("expected cluster %q, got %q", "clusterA", entries[0].Cluster)
	}
}

func TestActivity_QueryFilterByAction(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db, 30)

	svc.Log(Entry{Action: ActionVMStart, ResourceType: "vm", ResourceID: "100", Cluster: "c1"})
	svc.Log(Entry{Action: ActionVMStop, ResourceType: "vm", ResourceID: "101", Cluster: "c1"})
	svc.Log(Entry{Action: ActionVMStart, ResourceType: "vm", ResourceID: "102", Cluster: "c1"})

	entries, err := svc.Query(QueryParams{Action: ActionVMStop})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry for vm_stop, got %d", len(entries))
	}
	if entries[0].ResourceID != "101" {
		t.Errorf("expected resource_id %q, got %q", "101", entries[0].ResourceID)
	}
}

func TestActivity_QueryFilterByResourceType(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db, 30)

	svc.Log(Entry{Action: ActionVMStart, ResourceType: "vm", ResourceID: "100", Cluster: "c1"})
	svc.Log(Entry{Action: ActionCTStart, ResourceType: "ct", ResourceID: "200", Cluster: "c1"})

	entries, err := svc.Query(QueryParams{ResourceType: "ct"})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 ct entry, got %d", len(entries))
	}
	if entries[0].ResourceType != "ct" {
		t.Errorf("expected resource_type %q, got %q", "ct", entries[0].ResourceType)
	}
}

func TestActivity_CleanupRemovesOldEntries(t *testing.T) {
	db := openTestDB(t)

	// Insert an entry with a timestamp in the past (2 days ago)
	pastTime := time.Now().Add(-48 * time.Hour)
	db.Insert(Entry{
		Timestamp:    pastTime,
		Action:       ActionVMStart,
		ResourceType: "vm",
		ResourceID:   "100",
		Cluster:      "c1",
		Status:       StatusSuccess,
	})

	// Insert a current entry
	db.Insert(Entry{
		Timestamp:    time.Now(),
		Action:       ActionVMStop,
		ResourceType: "vm",
		ResourceID:   "101",
		Cluster:      "c1",
		Status:       StatusSuccess,
	})

	// Cleanup with 0 retention days removes everything in the past
	deleted, err := db.Cleanup(0)
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}
	if deleted < 1 {
		t.Fatalf("expected at least 1 deleted row, got %d", deleted)
	}

	// The old entry should be gone, but the current one might remain
	entries, err := db.Query(QueryParams{Limit: 100})
	if err != nil {
		t.Fatalf("Query after cleanup failed: %v", err)
	}
	for _, e := range entries {
		if e.ResourceID == "100" {
			t.Error("old entry (resource_id=100) should have been cleaned up")
		}
	}
}

func TestActivity_ReverseChronologicalOrder(t *testing.T) {
	db := openTestDB(t)
	svc := NewService(db, 30)

	// Insert entries with explicit timestamps to guarantee ordering
	base := time.Now().Add(-time.Hour)
	for i := 0; i < 5; i++ {
		svc.Log(Entry{
			Timestamp:    base.Add(time.Duration(i) * time.Minute),
			Action:       ActionVMStart,
			ResourceType: "vm",
			ResourceID:   fmt.Sprintf("%d", i),
			Cluster:      "c1",
		})
	}

	entries, err := svc.Query(QueryParams{Limit: 10})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}

	// First entry should be the most recent (resource_id "4")
	if entries[0].ResourceID != "4" {
		t.Errorf("expected first entry resource_id %q, got %q", "4", entries[0].ResourceID)
	}
	// Last entry should be the oldest (resource_id "0")
	if entries[4].ResourceID != "0" {
		t.Errorf("expected last entry resource_id %q, got %q", "0", entries[4].ResourceID)
	}

	// Verify descending order
	for i := 1; i < len(entries); i++ {
		if entries[i].Timestamp.After(entries[i-1].Timestamp) {
			t.Errorf("entry %d (ts=%v) is after entry %d (ts=%v) - not reverse chronological",
				i, entries[i].Timestamp, i-1, entries[i-1].Timestamp)
		}
	}
}
