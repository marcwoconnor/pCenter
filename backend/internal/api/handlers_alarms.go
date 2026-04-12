package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/moconnor/pcenter/internal/alarms"
)

func (h *Handler) SetAlarmsService(s *alarms.Service) {
	h.alarms = s
}

func (h *Handler) GetActiveAlarms(w http.ResponseWriter, r *http.Request) {
	if h.alarms == nil {
		writeJSON(w, []alarms.AlarmInstance{})
		return
	}
	active, err := h.alarms.GetActiveAlarms(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if active == nil {
		active = []alarms.AlarmInstance{}
	}
	writeJSON(w, active)
}

func (h *Handler) GetAlarmDefinitions(w http.ResponseWriter, r *http.Request) {
	if h.alarms == nil {
		writeJSON(w, []alarms.AlarmDefinition{})
		return
	}
	defs, err := h.alarms.DB.ListDefinitions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if defs == nil {
		defs = []alarms.AlarmDefinition{}
	}
	writeJSON(w, defs)
}

func (h *Handler) CreateAlarmDefinition(w http.ResponseWriter, r *http.Request) {
	if h.alarms == nil {
		writeError(w, http.StatusServiceUnavailable, "alarms not enabled")
		return
	}
	var req alarms.CreateDefinitionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	def, err := h.alarms.CreateDefinition(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, def)
}

func (h *Handler) UpdateAlarmDefinition(w http.ResponseWriter, r *http.Request) {
	if h.alarms == nil {
		writeError(w, http.StatusServiceUnavailable, "alarms not enabled")
		return
	}
	id := r.PathValue("id")
	var def alarms.AlarmDefinition
	if err := json.NewDecoder(r.Body).Decode(&def); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	def.ID = id
	if err := h.alarms.DB.UpdateDefinition(r.Context(), &def); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{"message": "updated"})
}

func (h *Handler) DeleteAlarmDefinition(w http.ResponseWriter, r *http.Request) {
	if h.alarms == nil {
		writeError(w, http.StatusServiceUnavailable, "alarms not enabled")
		return
	}
	id := r.PathValue("id")
	if err := h.alarms.DB.DeleteDefinition(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) AcknowledgeAlarm(w http.ResponseWriter, r *http.Request) {
	if h.alarms == nil {
		writeError(w, http.StatusServiceUnavailable, "alarms not enabled")
		return
	}
	id := r.PathValue("id")
	var req alarms.AcknowledgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.alarms.Acknowledge(r.Context(), id, req.User); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"message": "acknowledged"})
}

func (h *Handler) GetAlarmHistory(w http.ResponseWriter, r *http.Request) {
	if h.alarms == nil {
		writeJSON(w, []interface{}{})
		return
	}
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	history, err := h.alarms.DB.GetHistory(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if history == nil {
		history = []map[string]interface{}{}
	}
	writeJSON(w, history)
}

func (h *Handler) GetAlarmChannels(w http.ResponseWriter, r *http.Request) {
	if h.alarms == nil {
		writeJSON(w, []alarms.NotificationChannel{})
		return
	}
	channels, err := h.alarms.DB.ListChannels(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if channels == nil {
		channels = []alarms.NotificationChannel{}
	}
	writeJSON(w, channels)
}

func (h *Handler) CreateAlarmChannel(w http.ResponseWriter, r *http.Request) {
	if h.alarms == nil {
		writeError(w, http.StatusServiceUnavailable, "alarms not enabled")
		return
	}
	var req alarms.CreateChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ch := &alarms.NotificationChannel{
		Name:    req.Name,
		Type:    req.Type,
		Config:  req.Config,
		Enabled: true,
	}
	if err := h.alarms.DB.CreateChannel(r.Context(), ch); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, ch)
}

func (h *Handler) DeleteAlarmChannel(w http.ResponseWriter, r *http.Request) {
	if h.alarms == nil {
		writeError(w, http.StatusServiceUnavailable, "alarms not enabled")
		return
	}
	id := r.PathValue("id")
	if err := h.alarms.DB.DeleteChannel(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) TestAlarmChannel(w http.ResponseWriter, r *http.Request) {
	if h.alarms == nil {
		writeError(w, http.StatusServiceUnavailable, "alarms not enabled")
		return
	}
	id := r.PathValue("id")
	ch, err := h.alarms.DB.GetChannel(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}

	payload := alarms.NotificationPayload{
		AlarmName:    "Test Alarm",
		State:        "warning",
		PrevState:    "normal",
		Cluster:      "test",
		ResourceType: "node",
		ResourceID:   "test-node",
		ResourceName: "test-node",
		Value:        91.5,
		Threshold:    90.0,
	}

	var cfg alarms.WebhookConfig
	json.Unmarshal([]byte(ch.Config), &cfg)

	webhook := alarms.NewWebhookNotifier()
	if err := webhook.Send(r.Context(), cfg.URL, cfg.Headers, payload); err != nil {
		writeError(w, http.StatusBadGateway, "test failed: "+err.Error())
		return
	}
	writeJSON(w, map[string]string{"message": "test notification sent"})
}
