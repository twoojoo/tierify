package handlers

import (
	"net/http"
	"strconv"
	"time"

	"tierify/internal/errors"
	"tierify/internal/services"

	"github.com/go-chi/chi/v5"
)

type UsageHandler struct {
	svc services.UsageService
}

func NewUsageHandler(svc services.UsageService) *UsageHandler {
	return &UsageHandler{svc: svc}
}

// Routes registers usage-related routes on a tenant sub-router.
// These are mounted under /tenants/{tenantId}/...
func (h *UsageHandler) RegisterRoutes(r chi.Router) {
	r.Post("/{tenantId}/check", h.Check)
	r.Post("/{tenantId}/increment", h.Increment)
	r.Post("/{tenantId}/decrement", h.Decrement)
	r.Post("/{tenantId}/reset", h.Reset)
	r.Get("/{tenantId}/usage", h.GetUsage)
	r.Get("/{tenantId}/usage/{limitKey}", h.GetUsageByKey)
}

type operationsRequest struct {
	Operations []services.UsageOperation `json:"operations"`
}

type resetRequest struct {
	LimitKeys []string `json:"limit_keys"`
}

// parseTxContext extracts optional transaction context from request headers.
func parseTxContext(r *http.Request) *services.TxContext {
	txID := r.Header.Get("X-Transaction-ID")
	if txID == "" {
		return nil
	}

	tx := &services.TxContext{TransactionID: txID}

	if ttlStr := r.Header.Get("X-Transaction-TTL"); ttlStr != "" {
		if secs, err := strconv.Atoi(ttlStr); err == nil && secs > 0 {
			d := time.Duration(secs) * time.Second
			tx.TTL = &d
		}
	}

	return tx
}

func (h *UsageHandler) Check(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantId")

	var req operationsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, errors.BadRequest("invalid request body: "+err.Error()))
		return
	}

	if len(req.Operations) == 0 {
		writeError(w, errors.BadRequest("at least one operation is required"))
		return
	}

	txCtx := parseTxContext(r)
	results, err := h.svc.Check(r.Context(), tenantID, req.Operations, txCtx)
	if err != nil {
		writeError(w, err)
		return
	}

	resp := map[string]any{"results": results}
	if txCtx != nil {
		resp["transaction"] = map[string]any{
			"id":     txCtx.TransactionID,
			"status": "pending",
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *UsageHandler) Increment(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantId")

	var req operationsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, errors.BadRequest("invalid request body: "+err.Error()))
		return
	}

	if len(req.Operations) == 0 {
		writeError(w, errors.BadRequest("at least one operation is required"))
		return
	}

	txCtx := parseTxContext(r)
	results, err := h.svc.Increment(r.Context(), tenantID, req.Operations, txCtx)
	if err != nil {
		writeError(w, err)
		return
	}

	resp := map[string]any{"results": results}
	if txCtx != nil {
		resp["transaction"] = map[string]any{
			"id":     txCtx.TransactionID,
			"status": "pending",
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *UsageHandler) Decrement(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantId")

	var req operationsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, errors.BadRequest("invalid request body: "+err.Error()))
		return
	}

	if len(req.Operations) == 0 {
		writeError(w, errors.BadRequest("at least one operation is required"))
		return
	}

	txCtx := parseTxContext(r)
	results, err := h.svc.Decrement(r.Context(), tenantID, req.Operations, txCtx)
	if err != nil {
		writeError(w, err)
		return
	}

	resp := map[string]any{"results": results}
	if txCtx != nil {
		resp["transaction"] = map[string]any{
			"id":     txCtx.TransactionID,
			"status": "pending",
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *UsageHandler) Reset(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantId")

	var req resetRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, errors.BadRequest("invalid request body: "+err.Error()))
		return
	}

	if err := h.svc.Reset(r.Context(), tenantID, req.LimitKeys); err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *UsageHandler) GetUsage(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantId")

	usage, err := h.svc.GetUsage(r.Context(), tenantID)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, usage)
}

func (h *UsageHandler) GetUsageByKey(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantId")
	limitKey := chi.URLParam(r, "limitKey")

	usage, err := h.svc.GetUsageByKey(r.Context(), tenantID, limitKey)
	if err != nil {
		writeError(w, err)
		return
	}

	if usage == nil {
		writeError(w, errors.NotFound("usage", limitKey))
		return
	}

	writeJSON(w, http.StatusOK, usage)
}
