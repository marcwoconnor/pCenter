package alarms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Notifier sends alarm notifications
type Notifier interface {
	Send(ctx context.Context, payload NotificationPayload) error
}

// NotificationPayload is what gets sent to notification channels
type NotificationPayload struct {
	AlarmName    string  `json:"alarm_name"`
	State        string  `json:"state"`
	PrevState    string  `json:"previous_state"`
	Cluster      string  `json:"cluster"`
	ResourceType string  `json:"resource_type"`
	ResourceID   string  `json:"resource_id"`
	ResourceName string  `json:"resource_name"`
	Value        float64 `json:"value"`
	Threshold    float64 `json:"threshold"`
	Timestamp    int64   `json:"timestamp"`
}

// WebhookNotifier sends notifications via HTTP POST
type WebhookNotifier struct {
	client *http.Client
}

// NewWebhookNotifier creates a webhook notifier
func NewWebhookNotifier() *WebhookNotifier {
	return &WebhookNotifier{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (w *WebhookNotifier) Send(ctx context.Context, url string, headers map[string]string, payload NotificationPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}

// Dispatch sends notifications for alarm transitions to all configured channels
func Dispatch(ctx context.Context, db *DB, webhook *WebhookNotifier, inst *AlarmInstance, def AlarmDefinition, oldState AlarmState) {
	if len(def.NotifyChannels) == 0 {
		return
	}

	payload := NotificationPayload{
		AlarmName:    def.Name,
		State:        string(inst.State),
		PrevState:    string(oldState),
		Cluster:      inst.Cluster,
		ResourceType: inst.ResourceType,
		ResourceID:   inst.ResourceID,
		ResourceName: inst.ResourceName,
		Value:        inst.CurrentValue,
		Threshold:    inst.Threshold,
		Timestamp:    time.Now().Unix(),
	}

	for _, chID := range def.NotifyChannels {
		ch, err := db.GetChannel(ctx, chID)
		if err != nil || !ch.Enabled {
			continue
		}

		switch ch.Type {
		case "webhook":
			var cfg WebhookConfig
			json.Unmarshal([]byte(ch.Config), &cfg)
			if cfg.URL == "" {
				continue
			}
			go func(url string, headers map[string]string) {
				if err := webhook.Send(ctx, url, headers, payload); err != nil {
					slog.Error("webhook notification failed", "channel", ch.Name, "error", err)
				} else {
					slog.Info("webhook notification sent", "channel", ch.Name, "alarm", def.Name)
				}
			}(cfg.URL, cfg.Headers)
		}
	}
}
