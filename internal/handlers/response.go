package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tierify/internal/errors"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	if appErr, ok := err.(*errors.AppError); ok {
		writeJSON(w, appErr.StatusCode, appErr)
		return
	}
	writeJSON(w, http.StatusInternalServerError, errors.Internal(err.Error()))
}

func parseOffsetLimit(r *http.Request) (offset, limit int) {
	offset = 0
	limit = 20

	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 100 {
		limit = 100
	}
	return
}

func decodeJSON(r *http.Request, v any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(v)
}
