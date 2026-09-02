package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jorgebuitragor/logday-server/internal/security"
	"github.com/jorgebuitragor/logday-server/internal/settings"
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
	return &Handler{store: s, jwtSecret: jwtSecret, limiter: newLoginLimiter(s.db), tmpl: parseTemplates()}
}

// Routes registers the auth-related endpoints on r.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/auth/login", h.login)
	r.Post("/auth/refresh", h.refresh)
	r.Get("/policy", h.policy)

	r.Group(func(r chi.Router) {
		r.Use(h.RequireAuth)

		r.Get("/devices", h.listDevices)
		r.Delete("/devices/{id}", h.revokeDevice)

		r.Post("/policy/accept", h.acceptPolicy)
		r.Post("/policy/accept-sensitive", h.acceptSensitiveData)

		r.Get("/account/export", h.exportAccount)
		r.Delete("/account", h.deleteAccount)

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
	// PolicyVersion/PolicyAcceptedVersion: el cliente los compara para
	// saber si mostrar el gate de consentimiento sin pegarle a GET
	// /policy aparte en el camino crítico de login (ver
	// specs/cumplimiento-datos-personales/). AcceptedVersion es nil si
	// el usuario nunca aceptó nada todavía.
	PolicyVersion         int  `json:"policy_version"`
	PolicyAcceptedVersion *int `json:"policy_accepted_version"`
	// SensitiveDataAccepted: evita que el cliente tenga que pegarle a
	// otro endpoint solo para saber si ya mostró el aviso de dato
	// sensible alguna vez.
	SensitiveDataAccepted bool `json:"sensitive_data_accepted"`
}

func tokenResponseFor(access, refresh, deviceID string, cfg *settings.Settings, u *user) tokenResponse {
	return tokenResponse{
		AccessToken:           access,
		RefreshToken:          refresh,
		DeviceID:              deviceID,
		PolicyVersion:         cfg.PrivacyPolicyVersion,
		PolicyAcceptedVersion: u.PrivacyAcceptedVersion,
		SensitiveDataAccepted: u.SensitiveDataAcceptedAt != nil,
	}
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

	cfg, err := settings.Get(r.Context(), h.store.db)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if cfg.MaxDevicesPerUser > 0 {
		count, err := h.store.countDevices(r.Context(), u.ID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if count >= cfg.MaxDevicesPerUser {
			http.Error(w, "device limit reached, revoke a device first", http.StatusForbidden)
			return
		}
	}

	deviceName := req.DeviceName
	if deviceName == "" {
		deviceName = "unknown device"
	}

	rawRefresh, refreshHash, err := security.GenerateOpaqueToken()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	d, err := h.store.createDevice(r.Context(), u.ID, deviceName, refreshHash, time.Now().Add(cfg.RefreshTokenTTL()), clientIP(r), r.UserAgent())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	access, err := issueAccessToken(h.jwtSecret, u.ID, d.ID, u.IsAdmin, cfg.AccessTokenTTL())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, tokenResponseFor(access, rawRefresh, d.ID, cfg, u))
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

	cfg, err := settings.Get(r.Context(), h.store.db)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	newRaw, newHash, err := security.GenerateOpaqueToken()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if err := h.store.rotateRefreshToken(r.Context(), d.ID, hash, newHash, time.Now().Add(cfg.RefreshTokenTTL()), cfg.RefreshTokenTTL(), clientIP(r), r.UserAgent()); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	access, err := issueAccessToken(h.jwtSecret, u.ID, d.ID, u.IsAdmin, cfg.AccessTokenTTL())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, tokenResponseFor(access, newRaw, d.ID, cfg, u))
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

// ── Política de tratamiento de datos ────────────────────────────────
// Ver specs/cumplimiento-datos-personales/. GET /policy es público a
// propósito — un cliente necesita poder mostrar el texto antes de que
// el usuario decida si quiere usar ese servidor, no solo después de
// loguearse.

type policyResponse struct {
	Text    string `json:"text"`
	Version int    `json:"version"`
}

func (h *Handler) policy(w http.ResponseWriter, r *http.Request) {
	cfg, err := settings.Get(r.Context(), h.store.db)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, policyResponse{Text: cfg.PrivacyPolicyText, Version: cfg.PrivacyPolicyVersion})
}

type acceptPolicyRequest struct {
	Version int `json:"version"`
}

func (h *Handler) acceptPolicy(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserIDFromContext(r.Context())

	var req acceptPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.store.acceptPolicy(r.Context(), userID, req.Version); err != nil {
		if errors.Is(err, errStalePolicyVersion) {
			http.Error(w, "policy version is out of date, fetch it again", http.StatusConflict)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) acceptSensitiveData(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserIDFromContext(r.Context())

	if err := h.store.acceptSensitiveData(r.Context(), userID); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Derechos del titular ─────────────────────────────────────────────

func (h *Handler) exportAccount(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserIDFromContext(r.Context())

	data, err := h.store.exportUserData(r.Context(), userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

type deleteAccountRequest struct {
	Password string `json:"password"`
}

// deleteAccount exige la contraseña actual en el body (no solo el
// access token) — mismo criterio que cualquier otra acción
// irreversible ya en la app (ver specs/cumplimiento-datos-personales/
// design.md): un access token robado/filtrado no alcanza para borrar
// una cuenta entera.
func (h *Handler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserIDFromContext(r.Context())

	var req deleteAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	u, err := h.store.getUserByID(r.Context(), userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	match, err := security.VerifyPassword(req.Password, u.PasswordHash)
	if err != nil || !match {
		http.Error(w, "invalid password", http.StatusUnauthorized)
		return
	}

	if err := h.store.deleteUserAccount(r.Context(), userID); err != nil {
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
	if !validEmail(req.Email) {
		http.Error(w, "invalid email address", http.StatusBadRequest)
		return
	}

	cfg, err := settings.Get(r.Context(), h.store.db)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if len(req.Password) < cfg.MinPasswordLength {
		http.Error(w, fmt.Sprintf("password must be at least %d characters", cfg.MinPasswordLength), http.StatusBadRequest)
		return
	}
	if !cfg.EmailDomainAllowed(req.Email) {
		http.Error(w, "email domain not allowed", http.StatusBadRequest)
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
