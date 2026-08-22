package task

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

// Handler exposes the task HTTP endpoints, protected by authHandler's
// RequireAuth middleware.
type Handler struct {
	store    *store
	auth     *auth.Handler
	realtime *realtime.Hub
}

// NewHandler builds a Handler backed by s, notifying hub of every
// successful write (see specs/sync-incremental, "Eventos WebSocket").
func NewHandler(s *store, authHandler *auth.Handler, hub *realtime.Hub) *Handler {
	return &Handler{store: s, auth: authHandler, realtime: hub}
}

// Routes registers the task-related endpoints on r.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.auth.RequireAuth)

		r.Post("/tasks", h.create)
		r.Patch("/tasks/{id}", h.patch)
		r.Delete("/tasks/{id}", h.delete)
		r.Get("/tasks", h.list)
	})
}

type taskRequest struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	TaskCode    *string   `json:"task_code"`
	Status      string    `json:"status"`
	Tags        []string  `json:"tags"`
	Project     string    `json:"project"`
	Created     string    `json:"created"`
	CompletedAt *string   `json:"completed_at"`
	Due         *string   `json:"due"`
	Content     string    `json:"content"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (req taskRequest) toTask(userID string) Task {
	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}
	return Task{
		ID: req.ID, UserID: userID, Title: req.Title, TaskCode: req.TaskCode,
		Status: req.Status, Tags: tags, Project: req.Project, Created: req.Created,
		CompletedAt: req.CompletedAt, Due: req.Due, Content: req.Content, UpdatedAt: req.UpdatedAt,
	}
}

func validateTaskRequest(req taskRequest) error {
	if req.ID == "" {
		return errors.New("id is required")
	}
	if req.Title == "" {
		return errors.New("title is required")
	}
	if !validStatuses[req.Status] {
		return errors.New("status must be one of: todo, in-progress, done")
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

	var req taskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := validateTaskRequest(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	t := req.toTask(userID)
	h.upsert(w, r, &t)
}

// parsePatch reads a PATCH body's raw fields into a Patch,
// distinguishing "absent" from "explicit null" for nullable fields
// (see db.Field).
func parsePatch(raw map[string]json.RawMessage) (Patch, error) {
	var patch Patch
	var err error
	if patch.Title, err = db.PatchField[string](raw, "title"); err != nil {
		return Patch{}, err
	}
	if patch.TaskCode, err = db.PatchField[*string](raw, "task_code"); err != nil {
		return Patch{}, err
	}
	if patch.Status, err = db.PatchField[string](raw, "status"); err != nil {
		return Patch{}, err
	}
	if patch.Tags, err = db.PatchField[[]string](raw, "tags"); err != nil {
		return Patch{}, err
	}
	if patch.Project, err = db.PatchField[string](raw, "project"); err != nil {
		return Patch{}, err
	}
	if patch.Created, err = db.PatchField[string](raw, "created"); err != nil {
		return Patch{}, err
	}
	if patch.CompletedAt, err = db.PatchField[*string](raw, "completed_at"); err != nil {
		return Patch{}, err
	}
	if patch.Due, err = db.PatchField[*string](raw, "due"); err != nil {
		return Patch{}, err
	}
	if patch.Content, err = db.PatchField[string](raw, "content"); err != nil {
		return Patch{}, err
	}
	return patch, nil
}

func validatePatch(patch Patch) error {
	if patch.Title.Set && patch.Title.Value == "" {
		return errors.New("title cannot be empty")
	}
	if patch.Status.Set && !validStatuses[patch.Status.Value] {
		return errors.New("status must be one of: todo, in-progress, done")
	}
	if patch.Created.Set && patch.Created.Value == "" {
		return errors.New("created cannot be empty")
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

	patch, err := parsePatch(raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validatePatch(patch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	stored, changed, err := h.store.patchTask(r.Context(), id, userID, patch, updatedAt.Value)
	if err != nil {
		switch {
		case errors.Is(err, errNotFound):
			http.Error(w, "task not found", http.StatusNotFound)
		case errors.Is(err, errForbidden):
			http.Error(w, "task belongs to another user", http.StatusForbidden)
		default:
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	if changed {
		h.realtime.Notify(stored.UserID, "task", stored.ID, stored.Seq)
	}
	writeJSON(w, http.StatusOK, stored)
}

func (h *Handler) upsert(w http.ResponseWriter, r *http.Request, t *Task) {
	stored, err := h.store.upsertTask(r.Context(), t)
	if err != nil {
		switch {
		case errors.Is(err, errForbidden):
			http.Error(w, "task belongs to another user", http.StatusForbidden)
		case errors.Is(err, errConflict):
			http.Error(w, "a newer version of this task already exists", http.StatusConflict)
		default:
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	h.realtime.Notify(stored.UserID, "task", stored.ID, stored.Seq)
	writeJSON(w, http.StatusOK, stored)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())
	id := chi.URLParam(r, "id")

	seq, err := h.store.softDelete(r.Context(), id, userID)
	if err != nil {
		switch {
		case errors.Is(err, errNotFound):
			http.Error(w, "task not found", http.StatusNotFound)
		case errors.Is(err, errForbidden):
			http.Error(w, "task belongs to another user", http.StatusForbidden)
		default:
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	h.realtime.Notify(userID, "task", id, seq)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())

	tasks, err := h.store.listTasks(r.Context(), userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if tasks == nil {
		tasks = []Task{}
	}
	writeJSON(w, http.StatusOK, tasks)
}
