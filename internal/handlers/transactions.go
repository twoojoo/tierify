package handlers

import (
	"net/http"

	"tierify/internal/services"

	"github.com/go-chi/chi/v5"
)

type TransactionHandler struct {
	svc services.TransactionService
}

func NewTransactionHandler(svc services.TransactionService) *TransactionHandler {
	return &TransactionHandler{svc: svc}
}

func (h *TransactionHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/{txId}/commit", h.Commit)
	r.Post("/{txId}/rollback", h.Rollback)
	r.Get("/{txId}", h.Get)
	return r
}

func (h *TransactionHandler) Get(w http.ResponseWriter, r *http.Request) {
	txID := chi.URLParam(r, "txId")

	tx, err := h.svc.Get(r.Context(), txID)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, tx)
}

func (h *TransactionHandler) Commit(w http.ResponseWriter, r *http.Request) {
	txID := chi.URLParam(r, "txId")

	// Get transaction details before commit for the response.
	tx, err := h.svc.Get(r.Context(), txID)
	if err != nil {
		writeError(w, err)
		return
	}

	opCount := len(tx.Operations)

	if err := h.svc.Commit(r.Context(), txID); err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":             "committed",
		"operations_applied": opCount,
	})
}

func (h *TransactionHandler) Rollback(w http.ResponseWriter, r *http.Request) {
	txID := chi.URLParam(r, "txId")

	if err := h.svc.Rollback(r.Context(), txID); err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "rolled_back",
	})
}
