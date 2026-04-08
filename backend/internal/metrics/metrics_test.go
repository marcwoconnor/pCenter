package metrics

import (
	"context"
	"testing"
	"time"
)

// openTestDB opens an in-memory metrics database and registers cleanup.
func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:) failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpenCreatesTablesAndMetricTypes(t *testing.T) {
	db := openTestDB(t)

	// Verify tables exist by querying them
	tables := []string{"metric_types", "metrics_raw", "metrics_hourly", "metrics_daily", "metrics_weekly", "metrics_monthly"}
	for _, table := range tables {
		var count int64
		err := db.Conn().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
		if err != nil {
			t.Errorf("table %s does not exist: %v", table, err)
		}
	}

	// Verify metric_types were seeded
	var count int
	err := db.Conn().QueryRow("SELECT COUNT(*) FROM metric_types").Scan(&count)
	if err != nil {
		t.Fatalf("query metric_types: %v", err)
	}
	if count != len(MetricTypes) {
		t.Errorf("expected %d metric types, got %d", len(MetricTypes), count)
	}

	// Spot-check a known metric type
	var name, unit string
	err = db.Conn().QueryRow("SELECT name, unit FROM metric_types WHERE id = 1").Scan(&name, &unit)
	if err != nil {
		t.Fatalf("query metric type id=1: %v", err)
	}
	if name != "cpu" || unit != "percent" {
		t.Errorf("metric type 1: got name=%q unit=%q, want cpu/percent", name, unit)
	}
}

func TestInsertRawMetricsBatch(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	now := time.Now().Unix()
	batch := []RawMetric{
		{Timestamp: now, Cluster: "c1", ResourceType: "node", ResourceID: "pve01", MetricTypeID: 1, Value: 45.5},
		{Timestamp: now, Cluster: "c1", ResourceType: "node", ResourceID: "pve01", MetricTypeID: 2, Value: 8000000},
		{Timestamp: now, Cluster: "c1", ResourceType: "vm", ResourceID: "100", MetricTypeID: 1, Value: 12.3},
	}

	err := db.InsertRawMetricsBatch(ctx, batch)
	if err != nil {
		t.Fatalf("InsertRawMetricsBatch: %v", err)
	}

	var count int
	err = db.Conn().QueryRow("SELECT COUNT(*) FROM metrics_raw").Scan(&count)
	if err != nil {
		t.Fatalf("count raw: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 rows in metrics_raw, got %d", count)
	}
}

func TestInsertRawMetricsBatchEmpty(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// Empty batch should be a no-op
	err := db.InsertRawMetricsBatch(ctx, nil)
	if err != nil {
		t.Fatalf("InsertRawMetricsBatch(nil): %v", err)
	}
}

func TestQueryRawFiltersByTimeRange(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	qs := NewQueryService(db)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// Insert metrics at different timestamps
	batch := []RawMetric{
		{Timestamp: base.Unix(), Cluster: "c1", ResourceType: "node", ResourceID: "pve01", MetricTypeID: 1, Value: 10},
		{Timestamp: base.Add(1 * time.Hour).Unix(), Cluster: "c1", ResourceType: "node", ResourceID: "pve01", MetricTypeID: 1, Value: 20},
		{Timestamp: base.Add(2 * time.Hour).Unix(), Cluster: "c1", ResourceType: "node", ResourceID: "pve01", MetricTypeID: 1, Value: 30},
		{Timestamp: base.Add(5 * time.Hour).Unix(), Cluster: "c1", ResourceType: "node", ResourceID: "pve01", MetricTypeID: 1, Value: 50},
	}
	if err := db.InsertRawMetricsBatch(ctx, batch); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Query a sub-range (should get first 3 points, not the 5h one)
	resp, err := qs.Query(ctx, MetricQuery{
		Cluster:      "c1",
		ResourceType: "node",
		ResourceID:   "pve01",
		MetricTypes:  []string{"cpu"},
		StartTime:    base,
		EndTime:      base.Add(3 * time.Hour),
		Resolution:   "raw",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(resp.Series) != 1 {
		t.Fatalf("expected 1 series, got %d", len(resp.Series))
	}
	if len(resp.Series[0].Data) != 3 {
		t.Errorf("expected 3 data points, got %d", len(resp.Series[0].Data))
	}
}

func TestQueryRawFiltersByClusterAndResourceType(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	qs := NewQueryService(db)

	now := time.Now().Truncate(time.Hour)
	batch := []RawMetric{
		{Timestamp: now.Unix(), Cluster: "c1", ResourceType: "node", ResourceID: "pve01", MetricTypeID: 1, Value: 10},
		{Timestamp: now.Unix(), Cluster: "c2", ResourceType: "node", ResourceID: "pve02", MetricTypeID: 1, Value: 20},
		{Timestamp: now.Unix(), Cluster: "c1", ResourceType: "vm", ResourceID: "100", MetricTypeID: 1, Value: 30},
	}
	if err := db.InsertRawMetricsBatch(ctx, batch); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Filter by cluster c1 only
	resp, err := qs.Query(ctx, MetricQuery{
		Cluster:     "c1",
		MetricTypes: []string{"cpu"},
		StartTime:   now.Add(-time.Minute),
		EndTime:     now.Add(time.Minute),
		Resolution:  "raw",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	totalPoints := 0
	for _, s := range resp.Series {
		totalPoints += len(s.Data)
	}
	if totalPoints != 2 {
		t.Errorf("expected 2 points for cluster c1, got %d", totalPoints)
	}

	// Filter by cluster + resource_type
	resp, err = qs.Query(ctx, MetricQuery{
		Cluster:      "c1",
		ResourceType: "node",
		MetricTypes:  []string{"cpu"},
		StartTime:    now.Add(-time.Minute),
		EndTime:      now.Add(time.Minute),
		Resolution:   "raw",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	totalPoints = 0
	for _, s := range resp.Series {
		totalPoints += len(s.Data)
	}
	if totalPoints != 1 {
		t.Errorf("expected 1 point for c1/node, got %d", totalPoints)
	}
}

func TestQueryRawFiltersByResourceID(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	qs := NewQueryService(db)

	now := time.Now().Truncate(time.Hour)
	batch := []RawMetric{
		{Timestamp: now.Unix(), Cluster: "c1", ResourceType: "vm", ResourceID: "100", MetricTypeID: 1, Value: 10},
		{Timestamp: now.Unix(), Cluster: "c1", ResourceType: "vm", ResourceID: "101", MetricTypeID: 1, Value: 20},
	}
	if err := db.InsertRawMetricsBatch(ctx, batch); err != nil {
		t.Fatalf("insert: %v", err)
	}

	resp, err := qs.Query(ctx, MetricQuery{
		Cluster:      "c1",
		ResourceType: "vm",
		ResourceID:   "100",
		MetricTypes:  []string{"cpu"},
		StartTime:    now.Add(-time.Minute),
		EndTime:      now.Add(time.Minute),
		Resolution:   "raw",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(resp.Series) != 1 {
		t.Fatalf("expected 1 series, got %d", len(resp.Series))
	}
	if resp.Series[0].ResourceID != "100" {
		t.Errorf("expected resource_id 100, got %s", resp.Series[0].ResourceID)
	}
}

func TestQueryRawFiltersByMetricNames(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	qs := NewQueryService(db)

	now := time.Now().Truncate(time.Hour)
	cpuID := MetricTypeByName("cpu")
	memID := MetricTypeByName("mem")
	diskID := MetricTypeByName("disk")

	batch := []RawMetric{
		{Timestamp: now.Unix(), Cluster: "c1", ResourceType: "node", ResourceID: "pve01", MetricTypeID: cpuID, Value: 50},
		{Timestamp: now.Unix(), Cluster: "c1", ResourceType: "node", ResourceID: "pve01", MetricTypeID: memID, Value: 4000},
		{Timestamp: now.Unix(), Cluster: "c1", ResourceType: "node", ResourceID: "pve01", MetricTypeID: diskID, Value: 9000},
	}
	if err := db.InsertRawMetricsBatch(ctx, batch); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Request only cpu and mem
	resp, err := qs.Query(ctx, MetricQuery{
		Cluster:      "c1",
		ResourceType: "node",
		ResourceID:   "pve01",
		MetricTypes:  []string{"cpu", "mem"},
		StartTime:    now.Add(-time.Minute),
		EndTime:      now.Add(time.Minute),
		Resolution:   "raw",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(resp.Series) != 2 {
		t.Errorf("expected 2 series (cpu, mem), got %d", len(resp.Series))
	}
	for _, s := range resp.Series {
		if s.Metric != "cpu" && s.Metric != "mem" {
			t.Errorf("unexpected metric %q in results", s.Metric)
		}
	}
}

func TestHourlyRollupProducesCorrectAggregation(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// Insert raw metrics within a single hour bucket
	hourStart := time.Now().Truncate(time.Hour)
	cpuID := MetricTypeByName("cpu")

	batch := []RawMetric{
		{Timestamp: hourStart.Unix(), Cluster: "c1", ResourceType: "node", ResourceID: "pve01", MetricTypeID: cpuID, Value: 10},
		{Timestamp: hourStart.Add(10 * time.Second).Unix(), Cluster: "c1", ResourceType: "node", ResourceID: "pve01", MetricTypeID: cpuID, Value: 30},
		{Timestamp: hourStart.Add(20 * time.Second).Unix(), Cluster: "c1", ResourceType: "node", ResourceID: "pve01", MetricTypeID: cpuID, Value: 50},
	}
	if err := db.InsertRawMetricsBatch(ctx, batch); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Manually run the rollup SQL (same as rollupHourly but with our hour)
	hourEnd := hourStart.Add(time.Hour)
	_, err := db.Conn().Exec(`
		INSERT OR REPLACE INTO metrics_hourly
		(bucket_timestamp, cluster, resource_type, resource_id, metric_type_id,
		 min_value, max_value, avg_value, sample_count)
		SELECT
			?,
			cluster,
			resource_type,
			resource_id,
			metric_type_id,
			MIN(value),
			MAX(value),
			AVG(value),
			COUNT(*)
		FROM metrics_raw
		WHERE timestamp >= ? AND timestamp < ?
		GROUP BY cluster, resource_type, resource_id, metric_type_id
	`, hourStart.Unix(), hourStart.Unix(), hourEnd.Unix())
	if err != nil {
		t.Fatalf("rollup exec: %v", err)
	}

	// Query the hourly table
	qs := NewQueryService(db)
	resp, err := qs.Query(ctx, MetricQuery{
		Cluster:      "c1",
		ResourceType: "node",
		ResourceID:   "pve01",
		MetricTypes:  []string{"cpu"},
		StartTime:    hourStart,
		EndTime:      hourEnd,
		Resolution:   "hourly",
	})
	if err != nil {
		t.Fatalf("Query hourly: %v", err)
	}
	if len(resp.Series) != 1 {
		t.Fatalf("expected 1 series, got %d", len(resp.Series))
	}
	s := resp.Series[0]
	if len(s.Data) != 1 {
		t.Fatalf("expected 1 rollup point, got %d", len(s.Data))
	}

	point := s.Data[0]
	// min=10, max=50, avg=30
	if point.Min != 10 {
		t.Errorf("min: got %v, want 10", point.Min)
	}
	if point.Max != 50 {
		t.Errorf("max: got %v, want 50", point.Max)
	}
	if point.Value != 30 {
		t.Errorf("avg: got %v, want 30", point.Value)
	}
}

func TestRetentionCleanupRemovesOldRawData(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	retention := RetentionConfig{
		RawHours:     1,
		HourlyDays:   7,
		DailyDays:    30,
		WeeklyMonths: 12,
	}
	rollupSvc := NewRollupService(db, retention)

	now := time.Now()
	cpuID := MetricTypeByName("cpu")

	// Insert old metric (2 hours ago) and recent metric (now)
	batch := []RawMetric{
		{Timestamp: now.Add(-2 * time.Hour).Unix(), Cluster: "c1", ResourceType: "node", ResourceID: "pve01", MetricTypeID: cpuID, Value: 10},
		{Timestamp: now.Unix(), Cluster: "c1", ResourceType: "node", ResourceID: "pve01", MetricTypeID: cpuID, Value: 50},
	}
	if err := db.InsertRawMetricsBatch(ctx, batch); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Verify both are present
	var count int
	db.Conn().QueryRow("SELECT COUNT(*) FROM metrics_raw").Scan(&count)
	if count != 2 {
		t.Fatalf("expected 2 raw rows before cleanup, got %d", count)
	}

	// Run cleanup
	rollupSvc.ForceCleanup()

	// Old metric should be deleted, recent should remain
	db.Conn().QueryRow("SELECT COUNT(*) FROM metrics_raw").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 raw row after cleanup, got %d", count)
	}

	// Verify the remaining row is the recent one
	var val float64
	db.Conn().QueryRow("SELECT value FROM metrics_raw").Scan(&val)
	if val != 50 {
		t.Errorf("expected remaining value 50, got %v", val)
	}
}

func TestGetStats(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	batch := []RawMetric{
		{Timestamp: time.Now().Unix(), Cluster: "c1", ResourceType: "node", ResourceID: "pve01", MetricTypeID: 1, Value: 10},
		{Timestamp: time.Now().Unix(), Cluster: "c1", ResourceType: "node", ResourceID: "pve01", MetricTypeID: 2, Value: 20},
	}
	if err := db.InsertRawMetricsBatch(ctx, batch); err != nil {
		t.Fatalf("insert: %v", err)
	}

	stats, err := db.GetStats()
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats["metrics_raw"] != 2 {
		t.Errorf("expected metrics_raw=2, got %d", stats["metrics_raw"])
	}
	if stats["metrics_hourly"] != 0 {
		t.Errorf("expected metrics_hourly=0, got %d", stats["metrics_hourly"])
	}
}

func TestMetricTypeByNameAndByID(t *testing.T) {
	id := MetricTypeByName("cpu")
	if id != 1 {
		t.Errorf("MetricTypeByName(cpu): got %d, want 1", id)
	}

	mt := MetricTypeByID(1)
	if mt == nil || mt.Name != "cpu" {
		t.Errorf("MetricTypeByID(1): got %v, want cpu", mt)
	}

	// Unknown
	if MetricTypeByName("nonexistent") != 0 {
		t.Error("MetricTypeByName(nonexistent) should return 0")
	}
	if MetricTypeByID(9999) != nil {
		t.Error("MetricTypeByID(9999) should return nil")
	}
}

func TestQueryResponseMeta(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	qs := NewQueryService(db)

	now := time.Now().Truncate(time.Hour)
	batch := []RawMetric{
		{Timestamp: now.Unix(), Cluster: "c1", ResourceType: "node", ResourceID: "pve01", MetricTypeID: 1, Value: 42},
	}
	if err := db.InsertRawMetricsBatch(ctx, batch); err != nil {
		t.Fatalf("insert: %v", err)
	}

	start := now.Add(-time.Minute)
	end := now.Add(time.Minute)
	resp, err := qs.Query(ctx, MetricQuery{
		Cluster:     "c1",
		MetricTypes: []string{"cpu"},
		StartTime:   start,
		EndTime:     end,
		Resolution:  "raw",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	if resp.Meta.Resolution != "raw" {
		t.Errorf("expected resolution=raw, got %s", resp.Meta.Resolution)
	}
	if resp.Meta.Start != start.Unix() {
		t.Errorf("expected start=%d, got %d", start.Unix(), resp.Meta.Start)
	}
	if resp.Meta.End != end.Unix() {
		t.Errorf("expected end=%d, got %d", end.Unix(), resp.Meta.End)
	}
	if resp.Meta.PointCount != 1 {
		t.Errorf("expected point_count=1, got %d", resp.Meta.PointCount)
	}
}
