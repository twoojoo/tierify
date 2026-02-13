package handlers

import (
	"net/http"

	"tierify/internal/errors"
	"tierify/internal/models"
	"tierify/internal/services"

	"github.com/go-chi/chi/v5"
)

type TenantHandler struct {
	svc services.TenantService
}

func NewTenantHandler(svc services.TenantService) *TenantHandler {
	return &TenantHandler{svc: svc}
}

func (h *TenantHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.Create)
	r.Get("/{tenantId}", h.GetByID)
	r.Get("/{tenantId}/children", h.GetChildren)
	r.Get("/{tenantId}/ancestors", h.GetAncestors)
	r.Put("/{tenantId}/tier", h.AssignTier)
	r.Put("/{tenantId}/status", h.UpdateStatus)
	r.Get("/{tenantId}/history", h.GetTierHistory)
	r.Delete("/{tenantId}", h.Delete)
	return r
}

type createTenantRequest struct {
	ExternalID  string          `json:"external_id"`
	ParentID    *string         `json:"parent_id,omitempty"`
	TierKey     *string         `json:"tier_key,omitempty"`
	TierVersion *int            `json:"tier_version,omitempty"`
	Metadata    map[string]any  `json:"metadata,omitempty"`
}

func (h *TenantHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createTenantRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, errors.BadRequest("invalid request body: "+err.Error()))
		return
	}

	if req.ExternalID == "" {
		writeError(w, errors.BadRequest("external_id is required"))
		return
	}

	tenant, err := h.svc.Create(r.Context(), req.ExternalID, req.ParentID, req.TierKey, req.TierVersion, req.Metadata)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, tenant)
}

func (h *TenantHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantId")
	tenant, err := h.svc.GetByID(r.Context(), tenantID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tenant)
}

func (h *TenantHandler) GetChildren(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantId")
	offset, limit := parseOffsetLimit(r)

	result, err := h.svc.GetChildren(r.Context(), tenantID, offset, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *TenantHandler) GetAncestors(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantId")
	ancestors, err := h.svc.GetAncestors(r.Context(), tenantID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ancestors)
}

type assignTierRequest struct {
	TierKey     string `json:"tier_key"`
	TierVersion *int   `json:"tier_version,omitempty"`
	ResetUsage  bool   `json:"reset_usage"`
}

func (h *TenantHandler) AssignTier(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantId")

	var req assignTierRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, errors.BadRequest("invalid request body: "+err.Error()))
		return
	}

	if req.TierKey == "" {
		writeError(w, errors.BadRequest("tier_key is required"))
		return
	}

	if err := h.svc.AssignTier(r.Context(), tenantID, req.TierKey, req.TierVersion, req.ResetUsage); err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type updateStatusRequest struct {
	Status models.TenantStatus `json:"status"`
}

func (h *TenantHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantId")

	var req updateStatusRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, errors.BadRequest("invalid request body: "+err.Error()))
		return
	}

	switch req.Status {
	case models.TenantStatusActive, models.TenantStatusSuspended, models.TenantStatusBlocked:
	default:
		writeError(w, errors.BadRequest("status must be one of: active, suspended, blocked"))
		return
	}

	if err := h.svc.UpdateStatus(r.Context(), tenantID, req.Status); err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *TenantHandler) GetTierHistory(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantId")
	offset, limit := parseOffsetLimit(r)

	result, err := h.svc.GetTierHistory(r.Context(), tenantID, offset, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *TenantHandler) Delete(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantId")

	mode := r.URL.Query().Get("mode")
	switch models.DeleteMode(mode) {
	case models.DeleteModeCascade, models.DeleteModeOrphan:
	default:
		writeError(w, errors.BadRequest("mode query parameter is required and must be 'cascade' or 'orphan'"))
		return
	}

	if err := h.svc.Delete(r.Context(), tenantID, models.DeleteMode(mode)); err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
