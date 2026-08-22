package absence

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jorgebuitragor/logday-server/internal/auth"
	"github.com/jorgebuitragor/logday-server/internal/db"
	"github.com/jorgebuitragor/logday-server/internal/realtime"
)

// Handler exposes the absence day HTTP endpoints, protected by
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

// Routes registers the absence-day-related endpoints on r.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.auth.RequireAuth)

		r.Post("/absence-days", h.create)
		r.Patch("/absence-days/{id}", h.patch)
		r.Delete("/absence-days/{id}", h.delete)
		r.Get("/absence-days", h.list)
	})
}

type dayRequest struct {
	ID        string    `json:"id"`
	Date      string    `json:"date"`
	Type      string    `json:"type"`
	Note      *string   `json:"note"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (req dayRequest) toDay(userID string) Day {
	return Day{
		ID: req.ID, UserID: userID, Date: req.Date, Type: req.Type,
		Note: req.Note, UpdatedAt: req.UpdatedAt,
	}
}

func validateDayRequest(req dayRequest) error {
	if req.ID == "" {
		return errors.New("id is required")
	}
	if req.Date == "" {
		return errors.New("date is required")
	}
	if !validTypes[req.Type] {
		return errors.New("type must be one of: incapacidad, vacaciones, otro")
	}
	if req.UpdatedAt.IsZero() {
		return errors.New("updated_at is required")
	}
	return nil
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())

	var req dayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := validateDayRequest(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	d := req.toDay(userID)
	h.upsert(w, r, &d)
}

// parseDayPatch reads a PATCH body's raw fields into a Patch.
func parseDayPatch(raw map[string]json.RawMessage) (Patch, error) {
	var patch Patch
	var err error
	if patch.Date, err = db.PatchField[string](raw, "date"); err != nil {
		return Patch{}, err
	}
	if patch.Type, err = db.PatchField[string](raw, "type"); err != nil {
		return Patch{}, err
	}
	if patch.Note, err = db.PatchField[*string](raw, "note"); err != nil {
		return Patch{}, err
	}
	return patch, nil
}

func validateDayPatch(patch Patch) error {
	if patch.Date.Set && patch.Date.Value == "" {
		return errors.New("date cannot be empty")
	}
	if patch.Type.Set && !validTypes[patch.Type.Value] {
		return errors.New("type must be one of: incapacidad, vacaciones, otro")
	}
	return nil
}

func (h *Handler) patch(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())
	id := chi.URLParam(r, "id")

	raw, err := db.ParsePatch(r.Body)
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	updatedAt, err := db.PatchField[time.Time](raw, "updated_at")
	if err != nil || !updatedAt.Set || updatedAt.Value.IsZero() {
		http.Error(w, "updated_at is required", http.StatusBadRequest)
		return
	}

	patch, err := parseDayPatch(raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateDayPatch(patch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	stored, changed, err := h.store.patchDay(r.Context(), id, userID, patch, updatedAt.Value)
	if err != nil {
		switch {
		case errors.Is(err, errNotFound):
			http.Error(w, "absence day not found", http.StatusNotFound)
		case errors.Is(err, errForbidden):
			http.Error(w, "absence day belongs to another user", http.StatusForbidden)
		default:
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	if changed {
		h.realtime.Notify(stored.UserID, "absence_day", stored.ID, stored.Seq)
	}
	writeJSON(w, http.StatusOK, stored)
}

func (h *Handler) upsert(w http.ResponseWriter, r *http.Request, d *Day) {
	stored, err := h.store.upsertDay(r.Context(), d)
	if err != nil {
		switch {
		case errors.Is(err, errForbidden):
			http.Error(w, "absence day belongs to another user", http.StatusForbidden)
		case errors.Is(err, errConflict):
			http.Error(w, "a newer version of this absence day already exists", http.StatusConflict)
		default:
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	h.realtime.Notify(stored.UserID, "absence_day", stored.ID, stored.Seq)
	writeJSON(w, http.StatusOK, stored)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())
	id := chi.URLParam(r, "id")

	seq, err := h.store.softDelete(r.Context(), id, userID)
	if err != nil {
		switch {
		case errors.Is(err, errNotFound):
			http.Error(w, "absence day not found", http.StatusNotFound)
		case errors.Is(err, errForbidden):
			http.Error(w, "absence day belongs to another user", http.StatusForbidden)
		default:
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	h.realtime.Notify(userID, "absence_day", id, seq)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())

	days, err := h.store.listDays(r.Context(), userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if days == nil {
		days = []Day{}
	}
	writeJSON(w, http.StatusOK, days)
}
