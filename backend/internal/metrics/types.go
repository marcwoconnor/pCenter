package metrics

import "time"

// MetricType represents a type of metric being collected
type MetricType struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Unit string `json:"unit"`
}

// RawMetric is a single metric sample
type RawMetric struct {
	Timestamp    int64
	Cluster      string
	ResourceType string // "node", "vm", "ct", "storage", "ceph"
	ResourceID   string
	MetricTypeID int
	Value        float64
}

// RollupMetric contains aggregated data
type RollupMetric struct {
	BucketTimestamp int64
	Cluster         string
	ResourceType    string
	ResourceID      string
	MetricTypeID    int
	MinValue        float64
	MaxValue        float64
	AvgValue        float64
	SampleCount     int
}

// MetricQuery parameters for API queries
type MetricQuery struct {
	Cluster      string
	ResourceType string
	ResourceID   string
	MetricTypes  []string
	StartTime    time.Time
	EndTime      time.Time
	Resolution   string // "raw", "hourly", "daily", "weekly", "monthly", "auto"
}

// MetricSeries is the response format for charts
type MetricSeries struct {
	Metric     string            `json:"metric"`
	ResourceID string            `json:"resource_id"`
	Unit       string            `json:"unit"`
	Data       []MetricDataPoint `json:"data"`
}

// MetricDataPoint is a single point in a series
type MetricDataPoint struct {
	Timestamp int64   `json:"ts"`
	Value     float64 `json:"value"`
	Min       float64 `json:"min,omitempty"`
	Max       float64 `json:"max,omitempty"`
}

// MetricsResponse is the API response format
type MetricsResponse struct {
	Series []MetricSeries `json:"series"`
	Meta   MetricsMeta    `json:"meta"`
}

// MetricsMeta contains query metadata
type MetricsMeta struct {
	Start      int64  `json:"start"`
	End        int64  `json:"end"`
	Resolution string `json:"resolution"`
	PointCount int    `json:"point_count"`
}

// RetentionConfig defines how long each tier is kept
type RetentionConfig struct {
	RawHours     int `yaml:"raw_hours"`     // Default: 24
	HourlyDays   int `yaml:"hourly_days"`   // Default: 7
	DailyDays    int `yaml:"daily_days"`    // Default: 30
	WeeklyMonths int `yaml:"weekly_months"` // Default: 12
}

// DefaultRetention returns the default retention config
func DefaultRetention() RetentionConfig {
	return RetentionConfig{
		RawHours:     24,
		HourlyDays:   7,
		DailyDays:    30,
		WeeklyMonths: 12,
	}
}

// Predefined metric types
var MetricTypes = []MetricType{
	{1, "cpu", "percent"},
	{2, "mem", "bytes"},
	{3, "mem_percent", "percent"},
	{4, "disk", "bytes"},
	{5, "disk_percent", "percent"},
	{6, "netin", "bytes_per_sec"},
	{7, "netout", "bytes_per_sec"},
	{8, "diskread", "bytes_per_sec"},
	{9, "diskwrite", "bytes_per_sec"},
	{10, "swap", "bytes"},
	{11, "swap_percent", "percent"},
	{12, "loadavg_1m", "ratio"},
	{13, "loadavg_5m", "ratio"},
	{14, "loadavg_15m", "ratio"},
	{15, "uptime", "seconds"},
	{16, "ceph_used", "bytes"},
	{17, "ceph_avail", "bytes"},
	{18, "ceph_health", "enum"},
}

// MetricTypeByName returns the metric type ID for a given name
func MetricTypeByName(name string) int {
	for _, mt := range MetricTypes {
		if mt.Name == name {
			return mt.ID
		}
	}
	return 0
}

// MetricTypeByID returns the metric type for a given ID
func MetricTypeByID(id int) *MetricType {
	for _, mt := range MetricTypes {
		if mt.ID == id {
			return &mt
		}
	}
	return nil
}
