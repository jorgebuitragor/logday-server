package note

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jorgebuitragor/logday-server/internal/auth"
	"github.com/jorgebuitragor/logday-server/internal/realtime"
)

// Handler exposes the note HTTP endpoints, protected by authHandler's
// RequireAuth middleware.
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

// Routes registers the note-related endpoints on r.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.auth.RequireAuth)

		r.Post("/notes", h.create)
		r.Put("/notes/{id}", h.update)
		r.Delete("/notes/{id}", h.delete)
		r.Get("/notes", h.list)
	})
}

type noteRequest struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Folder    string    `json:"folder"`
	Tags      []string  `json:"tags"`
	Created   string    `json:"created"`
	Updated   string    `json:"updated"`
	Pinned    bool      `json:"pinned"`
	Content   string    `json:"content"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (req noteRequest) toNote(userID string) Note {
	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}
	return Note{
		ID: req.ID, UserID: userID, Title: req.Title, Folder: req.Folder, Tags: tags,
		Created: req.Created, Updated: req.Updated, Pinned: req.Pinned,
		Content: req.Content, UpdatedAt: req.UpdatedAt,
	}
}

func validateNoteRequest(req noteRequest) error {
	if req.ID == "" {
		return errors.New("id is required")
	}
	if req.Title == "" {
		return errors.New("title is required")
	}
	if req.Created == "" {
		return errors.New("created is required")
	}
	if req.UpdatedAt.IsZero() {
		return errors.New("updated_at is required")
	}
	return nil
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())

	var req noteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := validateNoteRequest(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	n := req.toNote(userID)
	h.upsert(w, r, &n)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())

	var req noteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.ID = chi.URLParam(r, "id")
	if err := validateNoteRequest(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	n := req.toNote(userID)
	h.upsert(w, r, &n)
}

func (h *Handler) upsert(w http.ResponseWriter, r *http.Request, n *Note) {
	stored, err := h.store.upsertNote(r.Context(), n)
	if err != nil {
		switch {
		case errors.Is(err, errForbidden):
			http.Error(w, "note belongs to another user", http.StatusForbidden)
		case errors.Is(err, errConflict):
			http.Error(w, "a newer version of this note already exists", http.StatusConflict)
		default:
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	h.realtime.Notify(stored.UserID, "note", stored.ID, stored.Seq)
	writeJSON(w, http.StatusOK, stored)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())
	id := chi.URLParam(r, "id")

	seq, err := h.store.softDelete(r.Context(), id, userID)
	if err != nil {
		switch {
		case errors.Is(err, errNotFound):
			http.Error(w, "note not found", http.StatusNotFound)
		case errors.Is(err, errForbidden):
			http.Error(w, "note belongs to another user", http.StatusForbidden)
		default:
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	h.realtime.Notify(userID, "note", id, seq)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())

	notes, err := h.store.listNotes(r.Context(), userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if notes == nil {
		notes = []Note{}
	}
	writeJSON(w, http.StatusOK, notes)
}
