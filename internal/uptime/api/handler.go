package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"apiservices/uptime-monitoring/internal/uptime/monitor"
)

type Handler struct {
	store   *monitor.Store
	service *monitor.Service
}

func NewHandler(store *monitor.Store, service *monitor.Service) *Handler {
	return &Handler{
		store:   store,
		service: service,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/v1/uptime/") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1/uptime/")
	path = strings.Trim(path, "/")

	switch {
	case path == "monitors":
		h.handleMonitorsCollection(w, r)
		return
	case path == "summary":
		h.handleSummary(w, r)
		return
	case strings.HasPrefix(path, "monitors/"):
		h.handleMonitorResource(w, r, strings.TrimPrefix(path, "monitors/"))
		return
	case path == "checks/run":
		h.handleRunChecks(w, r)
		return
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

func (h *Handler) handleSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": h.store.Summary()})
}

func (h *Handler) handleMonitorsCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req struct {
			Name            string `json:"name"`
			Type            string `json:"type"`
			Target          string `json:"target"`
			IntervalSeconds int    `json:"interval_seconds"`
			TimeoutSeconds  int    `json:"timeout_seconds"`
			ExpectedStatus  int    `json:"expected_status"`
			WebhookURL      string `json:"webhook_url"`
			CheckSSL        *bool  `json:"check_ssl"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}

		created, err := h.store.CreateMonitor(monitor.CreateMonitorInput{
			Name:            req.Name,
			Type:            monitor.MonitorType(strings.ToLower(strings.TrimSpace(req.Type))),
			Target:          req.Target,
			IntervalSeconds: req.IntervalSeconds,
			TimeoutSeconds:  req.TimeoutSeconds,
			ExpectedStatus:  req.ExpectedStatus,
			WebhookURL:      req.WebhookURL,
			CheckSSL:        req.CheckSSL,
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{"data": created})
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"data": h.store.ListMonitors()})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) handleMonitorResource(w http.ResponseWriter, r *http.Request, path string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	monitorID := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			m, ok := h.store.GetMonitor(monitorID)
			if !ok {
				writeError(w, http.StatusNotFound, "monitor not found")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"data": m})
		case http.MethodDelete:
			if ok := h.store.DeleteMonitor(monitorID); !ok {
				writeError(w, http.StatusNotFound, "monitor not found")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true}})
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	switch parts[1] {
	case "check":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		result, err := h.service.RunCheckForMonitor(r.Context(), monitorID)
		if err != nil {
			if errors.Is(err, monitor.ErrMonitorNotFound) {
				writeError(w, http.StatusNotFound, "monitor not found")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": result})
	case "results":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		limit := 50
		if raw := r.URL.Query().Get("limit"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil {
				writeError(w, http.StatusBadRequest, "limit must be an integer")
				return
			}
			limit = parsed
		}

		results, err := h.store.GetResults(monitorID, limit)
		if err != nil {
			if errors.Is(err, monitor.ErrMonitorNotFound) {
				writeError(w, http.StatusNotFound, "monitor not found")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": results})
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

func (h *Handler) handleRunChecks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	if mode == "" {
		mode = "due"
	}

	var count int
	switch mode {
	case "all":
		count = h.service.RunAllChecks(context.Background())
	case "due":
		count = h.service.RunDueChecks(context.Background())
	default:
		writeError(w, http.StatusBadRequest, "mode must be one of: due, all")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"mode":              mode,
			"executed_monitors": count,
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to marshal response")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
}
