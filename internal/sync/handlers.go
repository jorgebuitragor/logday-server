package sync

import (
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

	changes, err := h.store.changesSince(r.Context(), userID, since)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, changes)
}
