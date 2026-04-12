package alarms

import (
	"context"
	"fmt"
	"strings"

	"github.com/moconnor/pcenter/internal/state"
)

// Service orchestrates alarm operations
type Service struct {
	DB        *DB
	Evaluator *Evaluator
	webhook   *WebhookNotifier
}

// NewService creates a new alarm service
func NewService(db *DB, store *state.Store, evalInterval int, onChange func()) *Service {
	evaluator := NewEvaluator(db, store, evalInterval, onChange)
	return &Service{
		DB:        db,
		Evaluator: evaluator,
		webhook:   NewWebhookNotifier(),
	}
}

// CreateDefinition validates and creates an alarm definition
func (s *Service) CreateDefinition(ctx context.Context, req CreateDefinitionRequest) (*AlarmDefinition, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	if !ValidMetricTypes[req.MetricType] {
		return nil, fmt.Errorf("invalid metric_type: %s", req.MetricType)
	}
	if !ValidResourceTypes[req.ResourceType] {
		return nil, fmt.Errorf("invalid resource_type: %s", req.ResourceType)
	}
	if req.WarningThreshold <= 0 {
		return nil, fmt.Errorf("warning_threshold must be positive")
	}
	if req.CriticalThreshold <= req.WarningThreshold {
		return nil, fmt.Errorf("critical_threshold must be greater than warning_threshold")
	}
	if req.ClearThreshold >= req.WarningThreshold {
		return nil, fmt.Errorf("clear_threshold must be less than warning_threshold")
	}
	if req.DurationSamples <= 0 {
		req.DurationSamples = 3
	}

	def := &AlarmDefinition{
		Name:              req.Name,
		Enabled:           true,
		MetricType:        req.MetricType,
		ResourceType:      req.ResourceType,
		Scope:             req.Scope,
		ScopeTarget:       req.ScopeTarget,
		Condition:         Condition(req.Condition),
		WarningThreshold:  req.WarningThreshold,
		CriticalThreshold: req.CriticalThreshold,
		ClearThreshold:    req.ClearThreshold,
		DurationSamples:   req.DurationSamples,
		NotifyChannels:    req.NotifyChannels,
	}
	if def.Scope == "" {
		def.Scope = "global"
	}
	if def.Condition == "" {
		def.Condition = CondAbove
	}
	if def.NotifyChannels == nil {
		def.NotifyChannels = []string{}
	}

	if err := s.DB.CreateDefinition(ctx, def); err != nil {
		return nil, err
	}
	return def, nil
}

// GetActiveAlarms returns all non-normal alarm instances
func (s *Service) GetActiveAlarms(ctx context.Context) ([]AlarmInstance, error) {
	return s.DB.GetActiveAlarms(ctx)
}

// Acknowledge marks an alarm as acknowledged
func (s *Service) Acknowledge(ctx context.Context, id, user string) error {
	return s.DB.AcknowledgeAlarm(ctx, id, user)
}
