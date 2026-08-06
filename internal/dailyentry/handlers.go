package dailyentry

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jorgebuitragor/logday-server/internal/auth"
	"github.com/jorgebuitragor/logday-server/internal/realtime"
)

// Handler exposes the daily entry HTTP endpoints, protected by
// authHandler's RequireAuth middleware.
type Handler struct {
	store    *store
	auth     *auth.Handler
	realtime *realtime.Hub
}

// NewHandler builds a Handler backed by s, notifying hub of every
// successful write.
func NewHandler(s *store, authHandler *auth.Handler, hub *realtime.Hub) *Handler {
	return &Handler{store: s, auth: authHandler, realtime: hub}
}

// Routes registers the daily-entry-related endpoints on r. Unlike
// task/note/etc., there's no POST: the resource is identified by its
// own natural key (date) from the start, so PUT (upsert-by-URL) is
// the only write verb needed.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.auth.RequireAuth)

		r.Put("/daily-entries/{date}", h.put)
		r.Delete("/daily-entries/{date}", h.delete)
		r.Get("/daily-entries", h.list)
	})
}

type entryRequest struct {
	Content   string    `json:"content"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (h *Handler) put(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())
	date := chi.URLParam(r, "date")

	var req entryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.UpdatedAt.IsZero() {
		http.Error(w, "updated_at is required", http.StatusBadRequest)
		return
	}

	e := Entry{UserID: userID, Date: date, Content: req.Content, UpdatedAt: req.UpdatedAt}

	stored, err := h.store.upsertEntry(r.Context(), &e)
	if err != nil {
		if errors.Is(err, errConflict) {
			http.Error(w, "a newer version of this daily entry already exists", http.StatusConflict)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.realtime.Notify(stored.UserID, "daily_entry", stored.Date, stored.Seq)
	writeJSON(w, http.StatusOK, stored)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())
	date := chi.URLParam(r, "date")

	seq, err := h.store.softDelete(r.Context(), userID, date)
	if err != nil {
		if errors.Is(err, errNotFound) {
			http.Error(w, "daily entry not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.realtime.Notify(userID, "daily_entry", date, seq)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())

	entries, err := h.store.listEntries(r.Context(), userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if entries == nil {
		entries = []Entry{}
	}
	writeJSON(w, http.StatusOK, entries)
}
