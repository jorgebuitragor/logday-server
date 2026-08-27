package sync

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jorgebuitragor/logday-server/internal/auth"
)

// Handler exposes the unified sync-changes endpoint, protected by
// authHandler's RequireAuth middleware.
type Handler struct {
	store *store
	auth  *auth.Handler
}

// NewHandler builds a Handler backed by s.
func NewHandler(s *store, authHandler *auth.Handler) *Handler {
	return &Handler{store: s, auth: authHandler}
}

// Routes registers the sync-related endpoints on r.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.auth.RequireAuth)
		r.Get("/sync/changes", h.changes)
	})
}

func (h *Handler) changes(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())

	since := int64(0)
	if raw := r.URL.Query().Get("since"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			http.Error(w, "invalid since parameter", http.StatusBadRequest)
			return
		}
		since = parsed
	}

	// limit es opcional — 0 (ausente) significa "sin límite", el
	// comportamiento de siempre. Ver specs/sync-incremental/design.md
	// "Paginación" para por qué no hay un default aplicado sin que el
	// cliente lo pida.
	limit := int64(0)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			http.Error(w, "invalid limit parameter", http.StatusBadRequest)
			return
		}
		limit = parsed
	}

	changes, err := h.store.changesSince(r.Context(), userID, since, limit)
	if err != nil {
		if errors.Is(err, errCursorInvalid) {
			http.Error(w, "cursor is no longer valid, perform a full resync (retry without since)", http.StatusGone)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, changes)
}
