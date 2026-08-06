package calendar

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jorgebuitragor/logday-server/internal/auth"
	"github.com/jorgebuitragor/logday-server/internal/realtime"
)

// Handler exposes the calendar event HTTP endpoints, protected by
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

// Routes registers the calendar-related endpoints on r.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.auth.RequireAuth)

		r.Post("/calendar-events", h.create)
		r.Put("/calendar-events/{id}", h.update)
		r.Delete("/calendar-events/{id}", h.delete)
		r.Get("/calendar-events", h.list)
	})
}

type eventRequest struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	Date            string    `json:"date"`
	Time            string    `json:"time"`
	Description     string    `json:"description"`
	Color           string    `json:"color"`
	ReminderMinutes int       `json:"reminder_minutes"`
	Repeat          string    `json:"repeat"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (req eventRequest) toEvent(userID string) Event {
	return Event{
		ID: req.ID, UserID: userID, Title: req.Title, Date: req.Date, Time: req.Time,
		Description: req.Description, Color: req.Color, ReminderMinutes: req.ReminderMinutes,
		Repeat: req.Repeat, UpdatedAt: req.UpdatedAt,
	}
}

func validateEventRequest(req eventRequest) error {
	if req.ID == "" {
		return errors.New("id is required")
	}
	if req.Title == "" {
		return errors.New("title is required")
	}
	if req.Date == "" {
		return errors.New("date is required")
	}
	if !validColors[req.Color] {
		return errors.New("color must be one of: indigo, amber, emerald, rose, sky, violet")
	}
	if !validRepeats[req.Repeat] {
		return errors.New("repeat must be one of: none, daily, weekly, biweekly, monthly, yearly")
	}
	if req.UpdatedAt.IsZero() {
		return errors.New("updated_at is required")
	}
	return nil
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())

	var req eventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := validateEventRequest(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	e := req.toEvent(userID)
	h.upsert(w, r, &e)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())

	var req eventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.ID = chi.URLParam(r, "id")
	if err := validateEventRequest(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	e := req.toEvent(userID)
	h.upsert(w, r, &e)
}

func (h *Handler) upsert(w http.ResponseWriter, r *http.Request, e *Event) {
	stored, err := h.store.upsertEvent(r.Context(), e)
	if err != nil {
		switch {
		case errors.Is(err, errForbidden):
			http.Error(w, "calendar event belongs to another user", http.StatusForbidden)
		case errors.Is(err, errConflict):
			http.Error(w, "a newer version of this calendar event already exists", http.StatusConflict)
		default:
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	h.realtime.Notify(stored.UserID, "calendar_event", stored.ID, stored.Seq)
	writeJSON(w, http.StatusOK, stored)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())
	id := chi.URLParam(r, "id")

	seq, err := h.store.softDelete(r.Context(), id, userID)
	if err != nil {
		switch {
		case errors.Is(err, errNotFound):
			http.Error(w, "calendar event not found", http.StatusNotFound)
		case errors.Is(err, errForbidden):
			http.Error(w, "calendar event belongs to another user", http.StatusForbidden)
		default:
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	h.realtime.Notify(userID, "calendar_event", id, seq)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())

	events, err := h.store.listEvents(r.Context(), userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if events == nil {
		events = []Event{}
	}
	writeJSON(w, http.StatusOK, events)
}
