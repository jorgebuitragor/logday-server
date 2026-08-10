package auth

import (
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jorgebuitragor/logday-server/internal/security"
)

// Handler exposes the auth HTTP endpoints and the middleware (RequireAuth,
// RequireAdmin) that other domain packages mount to protect their own
// routes.
type Handler struct {
	store     *store
	jwtSecret []byte
	limiter   *loginLimiter
	tmpl      *template.Template
}

// NewHandler builds a Handler backed by s, signing/verifying access
// tokens with jwtSecret.
func NewHandler(s *store, jwtSecret []byte) *Handler {
	return &Handler{store: s, jwtSecret: jwtSecret, limiter: newLoginLimiter(), tmpl: parseTemplates()}
}

// Routes registers the auth-related endpoints on r.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/auth/login", h.login)
	r.Post("/auth/refresh", h.refresh)

	r.Group(func(r chi.Router) {
		r.Use(h.RequireAuth)

		r.Get("/devices", h.listDevices)
		r.Delete("/devices/{id}", h.revokeDevice)

		r.Group(func(r chi.Router) {
			r.Use(h.RequireAdmin)
			r.Post("/admin/users", h.adminCreateUser)
		})
	})
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	DeviceID     string `json:"device_id"`
}

type loginRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	DeviceName string `json:"device_name"`
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	limitKey := clientIP(r) + ":" + strings.ToLower(req.Email)
	if !h.limiter.Allow(limitKey) {
		http.Error(w, "too many attempts, try again later", http.StatusTooManyRequests)
		return
	}

	u, err := h.store.getUserByEmail(r.Context(), req.Email)
	if err != nil {
		h.limiter.RecordFailure(limitKey)
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	match, err := security.VerifyPassword(req.Password, u.PasswordHash)
	if err != nil || !match {
		h.limiter.RecordFailure(limitKey)
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	h.limiter.Reset(limitKey)

	deviceName := req.DeviceName
	if deviceName == "" {
		deviceName = "unknown device"
	}

	rawRefresh, refreshHash, err := security.GenerateOpaqueToken()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	d, err := h.store.createDevice(r.Context(), u.ID, deviceName, refreshHash, time.Now().Add(refreshTokenTTL))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	access, err := issueAccessToken(h.jwtSecret, u.ID, d.ID, u.IsAdmin)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, tokenResponse{AccessToken: access, RefreshToken: rawRefresh, DeviceID: d.ID})
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.RefreshToken == "" {
		http.Error(w, "refresh_token is required", http.StatusBadRequest)
		return
	}

	hash := security.HashOpaqueToken(req.RefreshToken)

	d, err := h.store.getDeviceByRefreshTokenHash(r.Context(), hash)
	if err != nil {
		if errors.Is(err, errNotFound) {
			if reused, checkErr := h.store.wasRefreshTokenUsed(r.Context(), hash); checkErr == nil && reused {
				_ = h.store.revokeDeviceByUsedTokenHash(r.Context(), hash)
			}
		}
		http.Error(w, "invalid refresh token", http.StatusUnauthorized)
		return
	}

	if time.Now().After(d.RefreshTokenExpiresAt) {
		_ = h.store.deleteDevice(r.Context(), d.ID)
		http.Error(w, "refresh token expired", http.StatusUnauthorized)
		return
	}

	u, err := h.store.getUserByID(r.Context(), d.UserID)
	if err != nil {
		http.Error(w, "invalid refresh token", http.StatusUnauthorized)
		return
	}

	newRaw, newHash, err := security.GenerateOpaqueToken()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if err := h.store.rotateRefreshToken(r.Context(), d.ID, hash, newHash, time.Now().Add(refreshTokenTTL)); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	access, err := issueAccessToken(h.jwtSecret, u.ID, d.ID, u.IsAdmin)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, tokenResponse{AccessToken: access, RefreshToken: newRaw, DeviceID: d.ID})
}

type deviceResponse struct {
	ID         string    `json:"id"`
	DeviceName string    `json:"device_name"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
}

func toDeviceResponse(d device) deviceResponse {
	return deviceResponse{ID: d.ID, DeviceName: d.DeviceName, CreatedAt: d.CreatedAt, LastUsedAt: d.LastUsedAt}
}

func (h *Handler) listDevices(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserIDFromContext(r.Context())

	devices, err := h.store.listDevices(r.Context(), userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	out := make([]deviceResponse, 0, len(devices))
	for _, d := range devices {
		out = append(out, toDeviceResponse(d))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) revokeDevice(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserIDFromContext(r.Context())
	deviceID := chi.URLParam(r, "id")

	if err := h.store.revokeDeviceForUser(r.Context(), deviceID, userID); err != nil {
		if errors.Is(err, errNotFound) {
			http.Error(w, "device not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type createUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	IsAdmin  bool   `json:"is_admin"`
}

type userResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	IsAdmin   bool      `json:"is_admin"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *Handler) adminCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Email == "" || req.Password == "" {
		http.Error(w, "email and password are required", http.StatusBadRequest)
		return
	}

	hash, err := security.HashPassword(req.Password)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	u, err := h.store.createUser(r.Context(), req.Email, hash, req.IsAdmin)
	if err != nil {
		if errors.Is(err, errDuplicateEmail) {
			http.Error(w, "email already exists", http.StatusConflict)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, userResponse{
		ID: u.ID, Email: u.Email, IsAdmin: u.IsAdmin, CreatedAt: u.CreatedAt,
	})
}
