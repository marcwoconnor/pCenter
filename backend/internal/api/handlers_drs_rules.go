package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/moconnor/pcenter/internal/drs"
)

func (h *Handler) SetDRSRulesDB(db *drs.RulesDB) {
	h.drsRulesDB = db
}

func (h *Handler) GetDRSRules(w http.ResponseWriter, r *http.Request) {
	if h.drsRulesDB == nil {
		writeJSON(w, []drs.Rule{})
		return
	}
	cluster := r.PathValue("cluster")
	rules, err := h.drsRulesDB.ListRules(r.Context(), cluster)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rules == nil {
		rules = []drs.Rule{}
	}
	writeJSON(w, rules)
}

func (h *Handler) CreateDRSRule(w http.ResponseWriter, r *http.Request) {
	if h.drsRulesDB == nil {
		writeError(w, http.StatusServiceUnavailable, "DRS rules not enabled")
		return
	}
	cluster := r.PathValue("cluster")
	var req drs.CreateRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if !drs.ValidRuleTypes[req.Type] {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid type: %s (must be affinity, anti-affinity, or host-pin)", req.Type))
		return
	}
	if len(req.Members) == 0 {
		writeError(w, http.StatusBadRequest, "members is required (at least 1 VMID)")
		return
	}
	if req.Type == "host-pin" && req.HostNode == "" {
		writeError(w, http.StatusBadRequest, "host_node is required for host-pin rules")
		return
	}

	rule := &drs.Rule{
		Cluster:  cluster,
		Name:     req.Name,
		Type:     drs.RuleType(req.Type),
		Enabled:  true,
		Members:  req.Members,
		HostNode: req.HostNode,
	}

	if err := h.drsRulesDB.CreateRule(r.Context(), rule); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, rule)
}

func (h *Handler) UpdateDRSRule(w http.ResponseWriter, r *http.Request) {
	if h.drsRulesDB == nil {
		writeError(w, http.StatusServiceUnavailable, "DRS rules not enabled")
		return
	}
	id := r.PathValue("id")
	var rule drs.Rule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rule.ID = id
	if err := h.drsRulesDB.UpdateRule(r.Context(), &rule); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"message": "updated"})
}

func (h *Handler) DeleteDRSRule(w http.ResponseWriter, r *http.Request) {
	if h.drsRulesDB == nil {
		writeError(w, http.StatusServiceUnavailable, "DRS rules not enabled")
		return
	}
	id := r.PathValue("id")
	if err := h.drsRulesDB.DeleteRule(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetDRSViolations(w http.ResponseWriter, r *http.Request) {
	cluster := r.PathValue("cluster")
	if h.poller == nil || h.poller.GetDRSScheduler() == nil {
		writeJSON(w, []drs.RuleViolation{})
		return
	}
	violations := h.poller.GetDRSScheduler().CheckViolations(cluster)
	if violations == nil {
		violations = []drs.RuleViolation{}
	}
	writeJSON(w, violations)
}
