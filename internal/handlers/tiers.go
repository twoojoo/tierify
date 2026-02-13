package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tierify/internal/errors"
	"tierify/internal/models"
	"tierify/internal/services"

	"github.com/go-chi/chi/v5"
)

type TierHandler struct {
	svc services.TierService
}

func NewTierHandler(svc services.TierService) *TierHandler {
	return &TierHandler{svc: svc}
}

func (h *TierHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.Create)
	r.Get("/", h.List)
	r.Get("/{tierKey}", h.GetByKey)
	r.Put("/{tierKey}", h.Update)
	r.Get("/{tierKey}/versions", h.ListVersions)
	r.Get("/{tierKey}/versions/{version}", h.GetByKeyAndVersion)
	r.Delete("/{tierKey}/versions/{version}", h.Deprecate)
	r.Get("/{tierKey}/versions/{version}/limits", h.GetLimits)
	r.Get("/{tierKey}/versions/{version}/limits/{limitKey}", h.GetLimit)
	return r
}

type createTierRequest struct {
	Key    string                    `json:"key"`
	Name   string                    `json:"name"`
	Limits []createLimitRequest      `json:"limits"`
}

type createLimitRequest struct {
	Key    string            `json:"key"`
	Kind   models.LimitKind  `json:"kind"`
	Scope  models.LimitScope `json:"scope"`
	Config json.RawMessage   `json:"config"`
}

func (h *TierHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createTierRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, errors.BadRequest("invalid request body: "+err.Error()))
		return
	}

	if req.Key == "" || req.Name == "" {
		writeError(w, errors.BadRequest("key and name are required"))
		return
	}

	limits, err := parseLimitDefinitions(req.Limits)
	if err != nil {
		writeError(w, err)
		return
	}

	tier, err := h.svc.Create(r.Context(), req.Key, req.Name, limits)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, tier)
}

func (h *TierHandler) GetByKey(w http.ResponseWriter, r *http.Request) {
	tierKey := chi.URLParam(r, "tierKey")
	tier, err := h.svc.GetByKey(r.Context(), tierKey)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tier)
}

func (h *TierHandler) GetByKeyAndVersion(w http.ResponseWriter, r *http.Request) {
	tierKey := chi.URLParam(r, "tierKey")
	version, err := strconv.Atoi(chi.URLParam(r, "version"))
	if err != nil {
		writeError(w, errors.BadRequest("invalid version"))
		return
	}

	tier, err := h.svc.GetByKeyAndVersion(r.Context(), tierKey, version)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tier)
}

func (h *TierHandler) List(w http.ResponseWriter, r *http.Request) {
	offset, limit := parseOffsetLimit(r)
	filters := models.TierFilters{Offset: offset, Limit: limit}

	if v := r.URL.Query().Get("key"); v != "" {
		filters.Key = &v
	}
	if v := r.URL.Query().Get("deprecated"); v != "" {
		b := v == "true"
		filters.Deprecated = &b
	}

	result, err := h.svc.List(r.Context(), filters)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *TierHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	tierKey := chi.URLParam(r, "tierKey")
	offset, limit := parseOffsetLimit(r)

	result, err := h.svc.ListVersions(r.Context(), tierKey, offset, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type updateTierRequest struct {
	Limits            []createLimitRequest `json:"limits"`
	MigrateExisting   bool                 `json:"migrate_existing"`
	DeprecatePrevious string               `json:"deprecate_previous"`
	ResetUsage        bool                 `json:"reset_usage"`
}

func (h *TierHandler) Update(w http.ResponseWriter, r *http.Request) {
	tierKey := chi.URLParam(r, "tierKey")

	var req updateTierRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, errors.BadRequest("invalid request body: "+err.Error()))
		return
	}

	limits, err := parseLimitDefinitions(req.Limits)
	if err != nil {
		writeError(w, err)
		return
	}

	opts := models.TierUpdateOptions{
		MigrateExisting:   req.MigrateExisting,
		DeprecatePrevious: req.DeprecatePrevious,
		ResetUsage:        req.ResetUsage,
	}
	if opts.DeprecatePrevious == "" {
		opts.DeprecatePrevious = "none"
	}

	tier, err := h.svc.Update(r.Context(), tierKey, limits, opts)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, tier)
}

func (h *TierHandler) Deprecate(w http.ResponseWriter, r *http.Request) {
	tierKey := chi.URLParam(r, "tierKey")
	version, err := strconv.Atoi(chi.URLParam(r, "version"))
	if err != nil {
		writeError(w, errors.BadRequest("invalid version"))
		return
	}

	if err := h.svc.Deprecate(r.Context(), tierKey, version); err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *TierHandler) GetLimits(w http.ResponseWriter, r *http.Request) {
	tierKey := chi.URLParam(r, "tierKey")
	version, err := strconv.Atoi(chi.URLParam(r, "version"))
	if err != nil {
		writeError(w, errors.BadRequest("invalid version"))
		return
	}

	limits, err := h.svc.GetLimits(r.Context(), tierKey, version)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, limits)
}

func (h *TierHandler) GetLimit(w http.ResponseWriter, r *http.Request) {
	tierKey := chi.URLParam(r, "tierKey")
	version, err := strconv.Atoi(chi.URLParam(r, "version"))
	if err != nil {
		writeError(w, errors.BadRequest("invalid version"))
		return
	}
	limitKey := chi.URLParam(r, "limitKey")

	limit, err := h.svc.GetLimit(r.Context(), tierKey, version, limitKey)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, limit)
}

// parseLimitDefinitions converts request limit definitions into model objects.
func parseLimitDefinitions(reqs []createLimitRequest) ([]models.LimitDefinition, error) {
	limits := make([]models.LimitDefinition, len(reqs))
	for i, req := range reqs {
		cfg, err := models.ParseLimitConfig(req.Kind, req.Config)
		if err != nil {
			return nil, errors.BadRequest("limit " + req.Key + ": " + err.Error())
		}
		scope := req.Scope
		if scope == "" {
			scope = models.LimitScopeLocal
		}
		limits[i] = models.LimitDefinition{
			Key:    req.Key,
			Kind:   req.Kind,
			Scope:  scope,
			Config: cfg,
		}
	}
	return limits, nil
}
