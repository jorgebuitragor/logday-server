package overtime

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

// Handler exposes the overtime HTTP endpoints (entries and per-month
// metadata), protected by authHandler's RequireAuth middleware.
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

// Routes registers the overtime-related endpoints on r.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.auth.RequireAuth)

		r.Post("/overtime-entries", h.createEntry)
		r.Patch("/overtime-entries/{id}", h.patchEntry)
		r.Delete("/overtime-entries/{id}", h.deleteEntry)
		r.Get("/overtime-entries", h.listEntries)

		r.Patch("/overtime-month-meta/{yearMonth}", h.patchMonthMeta)
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

// parseEntryPatch reads a PATCH body's raw fields into an EntryPatch.
func parseEntryPatch(raw map[string]json.RawMessage) (EntryPatch, error) {
	var patch EntryPatch
	var err error
	if patch.Fecha, err = db.PatchField[string](raw, "fecha"); err != nil {
		return EntryPatch{}, err
	}
	if patch.SolicitadaPor, err = db.PatchField[string](raw, "solicitada_por"); err != nil {
		return EntryPatch{}, err
	}
	if patch.Actividad, err = db.PatchField[string](raw, "actividad"); err != nil {
		return EntryPatch{}, err
	}
	if patch.Observaciones, err = db.PatchField[string](raw, "observaciones"); err != nil {
		return EntryPatch{}, err
	}
	if patch.HoraInicio, err = db.PatchField[string](raw, "hora_inicio"); err != nil {
		return EntryPatch{}, err
	}
	if patch.HoraFinal, err = db.PatchField[string](raw, "hora_final"); err != nil {
		return EntryPatch{}, err
	}
	if patch.TotalHoras, err = db.PatchField[float64](raw, "total_horas"); err != nil {
		return EntryPatch{}, err
	}
	if patch.ExtrasDiurnas, err = db.PatchField[float64](raw, "extras_diurnas"); err != nil {
		return EntryPatch{}, err
	}
	if patch.ExtrasNocturnas, err = db.PatchField[float64](raw, "extras_nocturnas"); err != nil {
		return EntryPatch{}, err
	}
	if patch.ExtrasDiurnasFestivas, err = db.PatchField[float64](raw, "extras_diurnas_festivas"); err != nil {
		return EntryPatch{}, err
	}
	if patch.ExtrasNocturnasFestivas, err = db.PatchField[float64](raw, "extras_nocturnas_festivas"); err != nil {
		return EntryPatch{}, err
	}
	return patch, nil
}

func (h *Handler) patchEntry(w http.ResponseWriter, r *http.Request) {
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

	patch, err := parseEntryPatch(raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	stored, changed, err := h.store.patchEntry(r.Context(), id, userID, patch, updatedAt.Value)
	if err != nil {
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
	if changed {
		h.realtime.Notify(stored.UserID, "overtime_entry", stored.ID, stored.Seq)
	}
	writeJSON(w, http.StatusOK, stored)
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
	h.realtime.Notify(stored.UserID, "overtime_entry", stored.ID, stored.Seq)
	writeJSON(w, http.StatusOK, stored)
}

func (h *Handler) deleteEntry(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())
	id := chi.URLParam(r, "id")

	seq, err := h.store.softDeleteEntry(r.Context(), id, userID)
	if err != nil {
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
	h.realtime.Notify(userID, "overtime_entry", id, seq)
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

func (h *Handler) patchMonthMeta(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())
	yearMonth := chi.URLParam(r, "yearMonth")

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

	var patch MonthMetaPatch
	if patch.Colaborador, err = db.PatchField[string](raw, "colaborador"); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if patch.Cedula, err = db.PatchField[string](raw, "cedula"); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	stored, changed, err := h.store.patchMonthMeta(r.Context(), userID, yearMonth, patch, updatedAt.Value)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if changed {
		h.realtime.Notify(stored.UserID, "overtime_month_meta", stored.YearMonth, stored.Seq)
	}
	writeJSON(w, http.StatusOK, stored)
}

func (h *Handler) deleteMonthMeta(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())
	yearMonth := chi.URLParam(r, "yearMonth")

	seq, err := h.store.softDeleteMonthMeta(r.Context(), userID, yearMonth)
	if err != nil {
		if errors.Is(err, errNotFound) {
			http.Error(w, "month metadata not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.realtime.Notify(userID, "overtime_month_meta", yearMonth, seq)
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
