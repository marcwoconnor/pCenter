package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/moconnor/pcenter/internal/scheduler"
)

func (h *Handler) GetScheduledTasks(w http.ResponseWriter, r *http.Request) {
	if h.scheduler == nil {
		writeJSON(w, []interface{}{})
		return
	}
	tasks, err := h.scheduler.DB().ListTasks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tasks == nil {
		tasks = []scheduler.ScheduledTask{}
	}
	writeJSON(w, tasks)
}

func (h *Handler) CreateScheduledTask(w http.ResponseWriter, r *http.Request) {
	if h.scheduler == nil {
		writeError(w, http.StatusServiceUnavailable, "scheduler not enabled")
		return
	}

	var req scheduler.CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.TaskType == "" || req.TargetType == "" || req.TargetID == 0 || req.CronExpr == "" {
		writeError(w, http.StatusBadRequest, "name, task_type, target_type, target_id, and cron_expr required")
		return
	}

	task, err := h.scheduler.DB().CreateTask(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Compute next run time
	h.scheduler.ComputeNextRun(task.ID)

	// Re-read to get next_run
	task, _ = h.scheduler.DB().GetTask(task.ID)

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, task)
}

func (h *Handler) UpdateScheduledTask(w http.ResponseWriter, r *http.Request) {
	if h.scheduler == nil {
		writeError(w, http.StatusServiceUnavailable, "scheduler not enabled")
		return
	}

	id := r.PathValue("id")
	var req scheduler.UpdateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.scheduler.DB().UpdateTask(id, req); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Recompute next run
	h.scheduler.ComputeNextRun(id)

	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *Handler) DeleteScheduledTask(w http.ResponseWriter, r *http.Request) {
	if h.scheduler == nil {
		writeError(w, http.StatusServiceUnavailable, "scheduler not enabled")
		return
	}

	id := r.PathValue("id")
	if err := h.scheduler.DB().DeleteTask(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *Handler) GetTaskRuns(w http.ResponseWriter, r *http.Request) {
	if h.scheduler == nil {
		writeJSON(w, []interface{}{})
		return
	}

	taskID := r.URL.Query().Get("task_id")
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	runs, err := h.scheduler.DB().ListRuns(taskID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if runs == nil {
		runs = []scheduler.TaskRun{}
	}
	writeJSON(w, runs)
}
