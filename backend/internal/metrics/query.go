package metrics

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// QueryService handles metric queries
type QueryService struct {
	db *DB
}

// NewQueryService creates a new query service
func NewQueryService(db *DB) *QueryService {
	return &QueryService{db: db}
}

// Query retrieves metrics based on query parameters
func (q *QueryService) Query(ctx context.Context, params MetricQuery) (*MetricsResponse, error) {
	// Determine which table to use based on time range
	duration := params.EndTime.Sub(params.StartTime)
	table, resolution := q.selectTable(params.Resolution, duration)

	// Build metric type IDs
	var metricTypeIDs []int
	for _, name := range params.MetricTypes {
		if id := MetricTypeByName(name); id > 0 {
			metricTypeIDs = append(metricTypeIDs, id)
		}
	}
	if len(metricTypeIDs) == 0 {
		return nil, fmt.Errorf("no valid metric types specified")
	}

	// Query data
	series, err := q.queryTable(ctx, table, params, metricTypeIDs)
	if err != nil {
		return nil, err
	}

	// Count total points
	totalPoints := 0
	for _, s := range series {
		totalPoints += len(s.Data)
	}

	return &MetricsResponse{
		Series: series,
		Meta: MetricsMeta{
			Start:      params.StartTime.Unix(),
			End:        params.EndTime.Unix(),
			Resolution: resolution,
			PointCount: totalPoints,
		},
	}, nil
}

// selectTable chooses the appropriate table based on resolution and time range
func (q *QueryService) selectTable(resolution string, duration time.Duration) (string, string) {
	if resolution != "" && resolution != "auto" {
		switch resolution {
		case "raw":
			return "metrics_raw", "raw"
		case "hourly":
			return "metrics_hourly", "hourly"
		case "daily":
			return "metrics_daily", "daily"
		case "weekly":
			return "metrics_weekly", "weekly"
		case "monthly":
			return "metrics_monthly", "monthly"
		}
	}

	// Auto-select based on duration
	switch {
	case duration <= 6*time.Hour:
		return "metrics_raw", "raw"
	case duration <= 3*24*time.Hour:
		return "metrics_hourly", "hourly"
	case duration <= 30*24*time.Hour:
		return "metrics_daily", "daily"
	case duration <= 365*24*time.Hour:
		return "metrics_weekly", "weekly"
	default:
		return "metrics_monthly", "monthly"
	}
}

// queryTable queries a specific table for metrics
func (q *QueryService) queryTable(ctx context.Context, table string, params MetricQuery, metricTypeIDs []int) ([]MetricSeries, error) {
	// Build the query
	var query string
	var args []interface{}

	// Determine columns based on table type
	isRaw := table == "metrics_raw"

	if isRaw {
		query = `
			SELECT timestamp, cluster, resource_type, resource_id, metric_type_id, value
			FROM metrics_raw
			WHERE timestamp >= ? AND timestamp < ?
		`
	} else {
		query = fmt.Sprintf(`
			SELECT bucket_timestamp, cluster, resource_type, resource_id, metric_type_id,
			       min_value, max_value, avg_value
			FROM %s
			WHERE bucket_timestamp >= ? AND bucket_timestamp < ?
		`, table)
	}

	args = append(args, params.StartTime.Unix(), params.EndTime.Unix())

	// Add cluster filter if specified
	if params.Cluster != "" {
		query += " AND cluster = ?"
		args = append(args, params.Cluster)
	}

	// Add resource type filter if specified
	if params.ResourceType != "" {
		query += " AND resource_type = ?"
		args = append(args, params.ResourceType)
	}

	// Add resource ID filter if specified
	if params.ResourceID != "" && params.ResourceID != "all" {
		query += " AND resource_id = ?"
		args = append(args, params.ResourceID)
	}

	// Add metric type filter
	if len(metricTypeIDs) > 0 {
		placeholders := ""
		for i, id := range metricTypeIDs {
			if i > 0 {
				placeholders += ","
			}
			placeholders += "?"
			args = append(args, id)
		}
		query += fmt.Sprintf(" AND metric_type_id IN (%s)", placeholders)
	}

	// Order by timestamp
	if isRaw {
		query += " ORDER BY resource_id, metric_type_id, timestamp"
	} else {
		query += " ORDER BY resource_id, metric_type_id, bucket_timestamp"
	}

	// Execute query
	rows, err := q.db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query metrics: %w", err)
	}
	defer rows.Close()

	// Group results into series
	seriesMap := make(map[string]*MetricSeries)

	for rows.Next() {
		var ts int64
		var cluster, resourceType, resourceID string
		var metricTypeID int
		var value, minVal, maxVal float64

		if isRaw {
			err = rows.Scan(&ts, &cluster, &resourceType, &resourceID, &metricTypeID, &value)
			minVal, maxVal = value, value
		} else {
			err = rows.Scan(&ts, &cluster, &resourceType, &resourceID, &metricTypeID, &minVal, &maxVal, &value)
		}
		if err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		// Get metric type info
		mt := MetricTypeByID(metricTypeID)
		if mt == nil {
			continue
		}

		// Create series key
		key := fmt.Sprintf("%s:%s:%s", resourceID, resourceType, mt.Name)

		series, ok := seriesMap[key]
		if !ok {
			series = &MetricSeries{
				Metric:     mt.Name,
				ResourceID: resourceID,
				Unit:       mt.Unit,
				Data:       make([]MetricDataPoint, 0),
			}
			seriesMap[key] = series
		}

		// Add data point
		point := MetricDataPoint{
			Timestamp: ts,
			Value:     value,
		}
		if !isRaw {
			point.Min = minVal
			point.Max = maxVal
		}
		series.Data = append(series.Data, point)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	// Convert map to slice
	result := make([]MetricSeries, 0, len(seriesMap))
	for _, s := range seriesMap {
		result = append(result, *s)
	}

	return result, nil
}

// GetResourceMetrics is a convenience method for getting metrics for a single resource
func (q *QueryService) GetResourceMetrics(ctx context.Context, cluster, resourceType, resourceID string, metricNames []string, startTime, endTime time.Time) (*MetricsResponse, error) {
	return q.Query(ctx, MetricQuery{
		Cluster:      cluster,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		MetricTypes:  metricNames,
		StartTime:    startTime,
		EndTime:      endTime,
		Resolution:   "auto",
	})
}

// GetClusterOverview returns aggregate metrics for all nodes in a cluster
func (q *QueryService) GetClusterOverview(ctx context.Context, cluster string, metricNames []string, startTime, endTime time.Time) (*MetricsResponse, error) {
	return q.Query(ctx, MetricQuery{
		Cluster:      cluster,
		ResourceType: "node",
		ResourceID:   "all",
		MetricTypes:  metricNames,
		StartTime:    startTime,
		EndTime:      endTime,
		Resolution:   "auto",
	})
}

// GetLatestValue returns the most recent value for a metric
func (q *QueryService) GetLatestValue(ctx context.Context, cluster, resourceType, resourceID, metricName string) (float64, error) {
	metricTypeID := MetricTypeByName(metricName)
	if metricTypeID == 0 {
		return 0, fmt.Errorf("unknown metric: %s", metricName)
	}

	var value float64
	err := q.db.conn.QueryRowContext(ctx, `
		SELECT value FROM metrics_raw
		WHERE cluster = ? AND resource_type = ? AND resource_id = ? AND metric_type_id = ?
		ORDER BY timestamp DESC
		LIMIT 1
	`, cluster, resourceType, resourceID, metricTypeID).Scan(&value)

	if err == sql.ErrNoRows {
		return 0, nil
	}
	return value, err
}
