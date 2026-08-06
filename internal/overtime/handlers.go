package overtime

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jorgebuitragor/logday-server/internal/auth"
)

// Handler exposes the overtime HTTP endpoints (entries and per-month
// metadata), protected by authHandler's RequireAuth middleware.
type Handler struct {
	store *store
	auth  *auth.Handler
}

// NewHandler builds a Handler backed by s.
func NewHandler(s *store, authHandler *auth.Handler) *Handler {
	return &Handler{store: s, auth: authHandler}
}

// Routes registers the overtime-related endpoints on r.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.auth.RequireAuth)

		r.Post("/overtime-entries", h.createEntry)
		r.Put("/overtime-entries/{id}", h.updateEntry)
		r.Delete("/overtime-entries/{id}", h.deleteEntry)
		r.Get("/overtime-entries", h.listEntries)

		r.Put("/overtime-month-meta/{yearMonth}", h.putMonthMeta)
		r.Delete("/overtime-month-meta/{yearMonth}", h.deleteMonthMeta)
		r.Get("/overtime-month-meta", h.listMonthMeta)
	})
}

// --- Entry ---

type entryRequest struct {
	ID                      string    `json:"id"`
	Fecha                   string    `json:"fecha"`
	SolicitadaPor           string    `json:"solicitada_por"`
	Actividad               string    `json:"actividad"`
	Observaciones           string    `json:"observaciones"`
	HoraInicio              string    `json:"hora_inicio"`
	HoraFinal               string    `json:"hora_final"`
	TotalHoras              float64   `json:"total_horas"`
	ExtrasDiurnas           float64   `json:"extras_diurnas"`
	ExtrasNocturnas         float64   `json:"extras_nocturnas"`
	ExtrasDiurnasFestivas   float64   `json:"extras_diurnas_festivas"`
	ExtrasNocturnasFestivas float64   `json:"extras_nocturnas_festivas"`
	UpdatedAt               time.Time `json:"updated_at"`
}

func (req entryRequest) toEntry(userID string) Entry {
	return Entry{
		ID: req.ID, UserID: userID, Fecha: req.Fecha, SolicitadaPor: req.SolicitadaPor,
		Actividad: req.Actividad, Observaciones: req.Observaciones, HoraInicio: req.HoraInicio,
		HoraFinal: req.HoraFinal, TotalHoras: req.TotalHoras, ExtrasDiurnas: req.ExtrasDiurnas,
		ExtrasNocturnas: req.ExtrasNocturnas, ExtrasDiurnasFestivas: req.ExtrasDiurnasFestivas,
		ExtrasNocturnasFestivas: req.ExtrasNocturnasFestivas, UpdatedAt: req.UpdatedAt,
	}
}

func validateEntryRequest(req entryRequest) error {
	if req.ID == "" {
		return errors.New("id is required")
	}
	if req.Fecha == "" {
		return errors.New("fecha is required")
	}
	if req.UpdatedAt.IsZero() {
		return errors.New("updated_at is required")
	}
	return nil
}

func (h *Handler) createEntry(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())

	var req entryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := validateEntryRequest(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	e := req.toEntry(userID)
	h.upsertEntry(w, r, &e)
}

func (h *Handler) updateEntry(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())

	var req entryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.ID = chi.URLParam(r, "id")
	if err := validateEntryRequest(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	e := req.toEntry(userID)
	h.upsertEntry(w, r, &e)
}

func (h *Handler) upsertEntry(w http.ResponseWriter, r *http.Request, e *Entry) {
	stored, err := h.store.upsertEntry(r.Context(), e)
	if err != nil {
		switch {
		case errors.Is(err, errForbidden):
			http.Error(w, "overtime entry belongs to another user", http.StatusForbidden)
		case errors.Is(err, errConflict):
			http.Error(w, "a newer version of this overtime entry already exists", http.StatusConflict)
		default:
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, http.StatusOK, stored)
}

func (h *Handler) deleteEntry(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())
	id := chi.URLParam(r, "id")

	if err := h.store.softDeleteEntry(r.Context(), id, userID); err != nil {
		switch {
		case errors.Is(err, errNotFound):
			http.Error(w, "overtime entry not found", http.StatusNotFound)
		case errors.Is(err, errForbidden):
			http.Error(w, "overtime entry belongs to another user", http.StatusForbidden)
		default:
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listEntries(w http.ResponseWriter, r *http.Request) {
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

// --- MonthMeta ---

type monthMetaRequest struct {
	Colaborador string    `json:"colaborador"`
	Cedula      string    `json:"cedula"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (h *Handler) putMonthMeta(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())
	yearMonth := chi.URLParam(r, "yearMonth")

	var req monthMetaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.UpdatedAt.IsZero() {
		http.Error(w, "updated_at is required", http.StatusBadRequest)
		return
	}

	m := MonthMeta{
		UserID: userID, YearMonth: yearMonth, Colaborador: req.Colaborador,
		Cedula: req.Cedula, UpdatedAt: req.UpdatedAt,
	}

	stored, err := h.store.upsertMonthMeta(r.Context(), &m)
	if err != nil {
		if errors.Is(err, errConflict) {
			http.Error(w, "a newer version of this month's metadata already exists", http.StatusConflict)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, stored)
}

func (h *Handler) deleteMonthMeta(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())
	yearMonth := chi.URLParam(r, "yearMonth")

	if err := h.store.softDeleteMonthMeta(r.Context(), userID, yearMonth); err != nil {
		if errors.Is(err, errNotFound) {
			http.Error(w, "month metadata not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listMonthMeta(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())

	records, err := h.store.listMonthMeta(r.Context(), userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if records == nil {
		records = []MonthMeta{}
	}
	writeJSON(w, http.StatusOK, records)
}
