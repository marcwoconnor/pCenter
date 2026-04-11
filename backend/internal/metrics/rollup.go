package metrics

import (
	"context"
	"log/slog"
	"time"
)

// RollupService handles metric aggregation into rollup tables
type RollupService struct {
	db        *DB
	retention RetentionConfig
}

// NewRollupService creates a new rollup service
func NewRollupService(db *DB, retention RetentionConfig) *RollupService {
	return &RollupService{
		db:        db,
		retention: retention,
	}
}

// Start begins the rollup loop
func (r *RollupService) Start(ctx context.Context) {
	// Run rollup jobs on different schedules
	go r.runHourlyLoop(ctx)
	go r.runDailyLoop(ctx)
	go r.runWeeklyLoop(ctx)
	go r.runMonthlyLoop(ctx)
	go r.runCleanupLoop(ctx)

	slog.Info("rollup service started")
}

// runHourlyLoop runs hourly rollups at the start of each hour
func (r *RollupService) runHourlyLoop(ctx context.Context) {
	// Wait until the next hour boundary + 5 minutes (to ensure data is collected)
	now := time.Now()
	nextHour := now.Truncate(time.Hour).Add(time.Hour).Add(5 * time.Minute)
	time.Sleep(time.Until(nextHour))

	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	// Initial run for the previous hour
	r.rollupHourly()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.rollupHourly()
		}
	}
}

// runDailyLoop runs daily rollups at 00:15 UTC each day
func (r *RollupService) runDailyLoop(ctx context.Context) {
	// Wait until next day at 00:15 UTC
	now := time.Now().UTC()
	nextRun := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 15, 0, 0, time.UTC)
	time.Sleep(time.Until(nextRun))

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// Initial run
	r.rollupDaily()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.rollupDaily()
		}
	}
}

// runWeeklyLoop runs weekly rollups on Mondays at 01:00 UTC
func (r *RollupService) runWeeklyLoop(ctx context.Context) {
	// Wait until next Monday at 01:00 UTC
	now := time.Now().UTC()
	daysUntilMonday := (8 - int(now.Weekday())) % 7
	if daysUntilMonday == 0 && now.Hour() >= 1 {
		daysUntilMonday = 7
	}
	nextRun := time.Date(now.Year(), now.Month(), now.Day()+daysUntilMonday, 1, 0, 0, 0, time.UTC)
	time.Sleep(time.Until(nextRun))

	ticker := time.NewTicker(7 * 24 * time.Hour)
	defer ticker.Stop()

	// Initial run
	r.rollupWeekly()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.rollupWeekly()
		}
	}
}

// runMonthlyLoop runs monthly rollups on the 1st at 02:00 UTC
func (r *RollupService) runMonthlyLoop(ctx context.Context) {
	// Wait until next month 1st at 02:00 UTC
	now := time.Now().UTC()
	nextMonth := time.Date(now.Year(), now.Month()+1, 1, 2, 0, 0, 0, time.UTC)
	time.Sleep(time.Until(nextMonth))

	// Run initial
	r.rollupMonthly()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Wait until next month
		now = time.Now().UTC()
		nextMonth = time.Date(now.Year(), now.Month()+1, 1, 2, 0, 0, 0, time.UTC)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(nextMonth)):
			r.rollupMonthly()
		}
	}
}

// runCleanupLoop runs retention cleanup every hour
func (r *RollupService) runCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	// Initial cleanup
	r.cleanup()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.cleanup()
		}
	}
}

// rollupHourly aggregates raw metrics into hourly buckets
func (r *RollupService) rollupHourly() {
	// Process the previous completed hour
	hourEnd := time.Now().Truncate(time.Hour).Unix()
	hourStart := hourEnd - 3600

	query := `
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
	`

	result, err := r.db.conn.Exec(query, hourStart, hourStart, hourEnd)
	if err != nil {
		slog.Error("hourly rollup failed", "error", err)
		return
	}

	rows, _ := result.RowsAffected()
	slog.Info("hourly rollup completed", "rows", rows, "hour", time.Unix(hourStart, 0).Format("2006-01-02 15:04"))
}

// rollupDaily aggregates hourly metrics into daily buckets
func (r *RollupService) rollupDaily() {
	// Process the previous completed day (UTC)
	now := time.Now().UTC()
	dayEnd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Unix()
	dayStart := dayEnd - 86400

	query := `
		INSERT OR REPLACE INTO metrics_daily
		(bucket_timestamp, cluster, resource_type, resource_id, metric_type_id,
		 min_value, max_value, avg_value, sample_count)
		SELECT
			?,
			cluster,
			resource_type,
			resource_id,
			metric_type_id,
			MIN(min_value),
			MAX(max_value),
			SUM(avg_value * sample_count) / SUM(sample_count),
			SUM(sample_count)
		FROM metrics_hourly
		WHERE bucket_timestamp >= ? AND bucket_timestamp < ?
		GROUP BY cluster, resource_type, resource_id, metric_type_id
	`

	result, err := r.db.conn.Exec(query, dayStart, dayStart, dayEnd)
	if err != nil {
		slog.Error("daily rollup failed", "error", err)
		return
	}

	rows, _ := result.RowsAffected()
	slog.Info("daily rollup completed", "rows", rows, "day", time.Unix(dayStart, 0).Format("2006-01-02"))
}

// rollupWeekly aggregates daily metrics into weekly buckets
func (r *RollupService) rollupWeekly() {
	// Process the previous completed week (Monday-Sunday)
	now := time.Now().UTC()
	// Find last Monday
	daysFromMonday := int(now.Weekday())
	if daysFromMonday == 0 {
		daysFromMonday = 7
	}
	weekEnd := time.Date(now.Year(), now.Month(), now.Day()-daysFromMonday+1, 0, 0, 0, 0, time.UTC).Unix()
	weekStart := weekEnd - 7*86400

	query := `
		INSERT OR REPLACE INTO metrics_weekly
		(bucket_timestamp, cluster, resource_type, resource_id, metric_type_id,
		 min_value, max_value, avg_value, sample_count)
		SELECT
			?,
			cluster,
			resource_type,
			resource_id,
			metric_type_id,
			MIN(min_value),
			MAX(max_value),
			SUM(avg_value * sample_count) / SUM(sample_count),
			SUM(sample_count)
		FROM metrics_daily
		WHERE bucket_timestamp >= ? AND bucket_timestamp < ?
		GROUP BY cluster, resource_type, resource_id, metric_type_id
	`

	result, err := r.db.conn.Exec(query, weekStart, weekStart, weekEnd)
	if err != nil {
		slog.Error("weekly rollup failed", "error", err)
		return
	}

	rows, _ := result.RowsAffected()
	slog.Info("weekly rollup completed", "rows", rows, "week_start", time.Unix(weekStart, 0).Format("2006-01-02"))
}

// rollupMonthly aggregates daily metrics into monthly buckets
func (r *RollupService) rollupMonthly() {
	// Process the previous completed month
	now := time.Now().UTC()
	monthEnd := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Unix()
	prevMonth := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, time.UTC)
	monthStart := prevMonth.Unix()

	query := `
		INSERT OR REPLACE INTO metrics_monthly
		(bucket_timestamp, cluster, resource_type, resource_id, metric_type_id,
		 min_value, max_value, avg_value, sample_count)
		SELECT
			?,
			cluster,
			resource_type,
			resource_id,
			metric_type_id,
			MIN(min_value),
			MAX(max_value),
			SUM(avg_value * sample_count) / SUM(sample_count),
			SUM(sample_count)
		FROM metrics_daily
		WHERE bucket_timestamp >= ? AND bucket_timestamp < ?
		GROUP BY cluster, resource_type, resource_id, metric_type_id
	`

	result, err := r.db.conn.Exec(query, monthStart, monthStart, monthEnd)
	if err != nil {
		slog.Error("monthly rollup failed", "error", err)
		return
	}

	rows, _ := result.RowsAffected()
	slog.Info("monthly rollup completed", "rows", rows, "month", prevMonth.Format("2006-01"))
}

// cleanup removes expired data based on retention policy
func (r *RollupService) cleanup() {
	now := time.Now().Unix()

	// Clean raw metrics (keep retention.RawHours)
	rawCutoff := now - int64(r.retention.RawHours*3600)
	result, err := r.db.conn.Exec("DELETE FROM metrics_raw WHERE timestamp < ?", rawCutoff)
	if err != nil {
		slog.Error("raw cleanup failed", "error", err)
	} else if rows, _ := result.RowsAffected(); rows > 0 {
		slog.Debug("cleaned raw metrics", "deleted", rows)
	}

	// Clean hourly metrics (keep retention.HourlyDays)
	hourlyCutoff := now - int64(r.retention.HourlyDays*86400)
	result, err = r.db.conn.Exec("DELETE FROM metrics_hourly WHERE bucket_timestamp < ?", hourlyCutoff)
	if err != nil {
		slog.Error("hourly cleanup failed", "error", err)
	} else if rows, _ := result.RowsAffected(); rows > 0 {
		slog.Debug("cleaned hourly metrics", "deleted", rows)
	}

	// Clean daily metrics (keep retention.DailyDays)
	dailyCutoff := now - int64(r.retention.DailyDays*86400)
	result, err = r.db.conn.Exec("DELETE FROM metrics_daily WHERE bucket_timestamp < ?", dailyCutoff)
	if err != nil {
		slog.Error("daily cleanup failed", "error", err)
	} else if rows, _ := result.RowsAffected(); rows > 0 {
		slog.Debug("cleaned daily metrics", "deleted", rows)
	}

	// Clean weekly metrics (keep retention.WeeklyMonths)
	weeklyCutoff := now - int64(r.retention.WeeklyMonths*30*86400)
	result, err = r.db.conn.Exec("DELETE FROM metrics_weekly WHERE bucket_timestamp < ?", weeklyCutoff)
	if err != nil {
		slog.Error("weekly cleanup failed", "error", err)
	} else if rows, _ := result.RowsAffected(); rows > 0 {
		slog.Debug("cleaned weekly metrics", "deleted", rows)
	}

	// Monthly metrics are kept indefinitely (or could add a yearly retention)
}

// ForceRollup runs all rollups immediately (useful for testing)
func (r *RollupService) ForceRollup() {
	r.rollupHourly()
	r.rollupDaily()
	r.rollupWeekly()
	r.rollupMonthly()
}

// ForceCleanup runs cleanup immediately
func (r *RollupService) ForceCleanup() {
	r.cleanup()
}
