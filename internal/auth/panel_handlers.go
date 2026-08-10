package auth

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jorgebuitragor/logday-server/internal/security"
	"github.com/jorgebuitragor/logday-server/internal/settings"
)

// PanelRoutes registers the HTML admin panel — kept separate from
// Routes (the JSON API) so the split between the two surfaces is
// unambiguous when reading route registrations. Like Routes, it
// registers directly on r (no sub-router / chi.Mount).
func (h *Handler) PanelRoutes(r chi.Router) {
	r.Get("/admin/static/logo.png", h.serveLogo)

	r.Get("/setup", h.setupForm)
	r.Post("/setup", h.setupSubmit)

	r.Get("/admin/panel/login", h.panelLoginForm)
	r.Post("/admin/panel/login", h.panelLoginSubmit)

	r.Group(func(r chi.Router) {
		r.Use(h.requireAdminSession)

		r.Post("/admin/panel/logout", h.panelLogout)
		r.Get("/admin/panel", h.panelUsers)
		r.Post("/admin/panel/users", h.panelCreateUser)
		r.Post("/admin/panel/users/{id}/promote", h.panelPromote)
		r.Post("/admin/panel/users/{id}/demote", h.panelDemote)
		r.Post("/admin/panel/users/{id}/delete", h.panelDeleteUser)
		r.Post("/admin/panel/users/{id}/restore", h.panelRestoreUser)
		r.Post("/admin/panel/users/{id}/reset-password", h.panelResetPassword)
		r.Get("/admin/panel/devices", h.panelDevices)
		r.Post("/admin/panel/devices/{id}/revoke", h.panelRevokeDevice)
		r.Get("/admin/panel/settings", h.panelSettings)
		r.Post("/admin/panel/settings", h.panelUpdateSettings)
		r.Post("/admin/panel/settings/generate-secret", h.panelGenerateSecret)
	})
}

// serveLogo serves the embedded Logday brand mark — used as both the
// panel's favicon and its header logo. Cached aggressively: it's
// embedded at compile time, so it only ever changes across a binary
// upgrade, never at runtime.
func (h *Handler) serveLogo(w http.ResponseWriter, r *http.Request) {
	data, err := staticFS.ReadFile("static/logo.png")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeContent(w, r, "logo.png", time.Time{}, bytes.NewReader(data))
}

type formPageData struct {
	CSRFToken    string
	Error        string
	Email        string
	InstanceName string
}

type usersPageData struct {
	CSRFToken    string
	Error        string
	Users        []user
	Active       string
	InstanceName string
}

type devicesPageData struct {
	CSRFToken    string
	Error        string
	Devices      []deviceWithOwner
	Active       string
	InstanceName string
}

type settingsPageData struct {
	CSRFToken       string
	Error           string
	Active          string
	Settings        settings.Settings
	GeneratedSecret string
	InstanceName    string
}

func (h *Handler) redirectWithError(w http.ResponseWriter, r *http.Request, path, msg string) {
	http.Redirect(w, r, path+"?error="+url.QueryEscape(msg), http.StatusFound)
}

// instanceName reads the configurable instance name (Configuración >
// General) for the <title>/header brand — falls back to the default
// rather than failing the whole page render if settings can't be read.
func (h *Handler) instanceName(ctx context.Context) string {
	cfg, err := settings.Get(ctx, h.store.db)
	if err != nil {
		return "Logday Server"
	}
	return cfg.InstanceName
}

// setupForm serves the one-time "create the first admin" screen. Public
// by necessity — no admin exists yet to require. The redirect here is
// only for UX (skip straight to login once an admin exists); the real
// guarantee against creating a second "first admin" is the transaction
// in store.createFirstAdmin, checked again in setupSubmit.
func (h *Handler) setupForm(w http.ResponseWriter, r *http.Request) {
	if count, err := h.store.countUsers(r.Context()); err == nil && count > 0 {
		http.Redirect(w, r, "/admin/panel/login", http.StatusFound)
		return
	}
	csrf, err := ensureCSRFCookie(w, r)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	renderTemplate(w, h.tmpl, http.StatusOK, "setup.html", formPageData{CSRFToken: csrf, InstanceName: h.instanceName(r.Context())})
}

func (h *Handler) setupSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if !verifyCSRF(r) {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	confirm := r.FormValue("password_confirm")

	renderSetupError := func(msg string) {
		csrf, _ := ensureCSRFCookie(w, r)
		renderTemplate(w, h.tmpl, http.StatusBadRequest, "setup.html",
			formPageData{CSRFToken: csrf, Error: msg, Email: email, InstanceName: h.instanceName(r.Context())})
	}

	switch {
	case email == "" || password == "":
		renderSetupError("email y contraseña son obligatorios")
		return
	case len(password) < 8:
		renderSetupError("la contraseña debe tener al menos 8 caracteres")
		return
	case password != confirm:
		renderSetupError("las contraseñas no coinciden")
		return
	}

	hash, err := security.HashPassword(password)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	u, err := h.store.createFirstAdmin(r.Context(), email, hash)
	if err != nil {
		switch {
		case errors.Is(err, errAlreadyInit):
			http.Redirect(w, r, "/admin/panel/login", http.StatusFound)
		case errors.Is(err, errDuplicateEmail):
			renderSetupError("ya existe un usuario con ese email")
		default:
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}

	token, err := issuePanelSession(h.jwtSecret, u.ID, true)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	setSessionCookie(w, r, token)
	http.Redirect(w, r, "/admin/panel", http.StatusFound)
}

func (h *Handler) panelLoginForm(w http.ResponseWriter, r *http.Request) {
	csrf, err := ensureCSRFCookie(w, r)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	renderTemplate(w, h.tmpl, http.StatusOK, "login.html", formPageData{CSRFToken: csrf, InstanceName: h.instanceName(r.Context())})
}

func (h *Handler) panelLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if !verifyCSRF(r) {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	renderLoginError := func(status int, msg string) {
		csrf, _ := ensureCSRFCookie(w, r)
		renderTemplate(w, h.tmpl, status, "login.html", formPageData{CSRFToken: csrf, Error: msg, Email: email, InstanceName: h.instanceName(r.Context())})
	}

	limitKey := clientIP(r) + ":" + strings.ToLower(email)
	if !h.limiter.Allow(limitKey) {
		renderLoginError(http.StatusTooManyRequests, "demasiados intentos, probá de nuevo más tarde")
		return
	}

	// Same generic message for "no existe", "password incorrecto" y "no
	// es admin" — no filtrar cuál de los tres pasó.
	fail := func() {
		h.limiter.RecordFailure(limitKey)
		renderLoginError(http.StatusUnauthorized, "credenciales inválidas")
	}

	u, err := h.store.getUserByEmail(r.Context(), email)
	if err != nil {
		fail()
		return
	}
	match, err := security.VerifyPassword(password, u.PasswordHash)
	if err != nil || !match {
		fail()
		return
	}
	if !u.IsAdmin {
		fail()
		return
	}
	h.limiter.Reset(limitKey)

	token, err := issuePanelSession(h.jwtSecret, u.ID, u.IsAdmin)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	setSessionCookie(w, r, token)
	http.Redirect(w, r, "/admin/panel", http.StatusFound)
}

func (h *Handler) panelLogout(w http.ResponseWriter, r *http.Request) {
	clearSessionCookie(w, r)
	http.Redirect(w, r, "/admin/panel/login", http.StatusFound)
}

func (h *Handler) panelUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.store.listUsers(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	csrf, err := ensureCSRFCookie(w, r)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	renderTemplate(w, h.tmpl, http.StatusOK, "users.html",
		usersPageData{CSRFToken: csrf, Users: users, Error: r.URL.Query().Get("error"), Active: "users", InstanceName: h.instanceName(r.Context())})
}

func (h *Handler) panelCreateUser(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if !verifyCSRF(r) {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	isAdmin := r.FormValue("is_admin") == "on"

	if email == "" || len(password) < 8 {
		h.redirectWithError(w, r, "/admin/panel", "email y contraseña (mínimo 8 caracteres) son obligatorios")
		return
	}

	hash, err := security.HashPassword(password)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if _, err := h.store.createUser(r.Context(), email, hash, isAdmin); err != nil {
		if errors.Is(err, errDuplicateEmail) {
			h.redirectWithError(w, r, "/admin/panel", "ya existe un usuario con ese email")
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/panel", http.StatusFound)
}

func (h *Handler) panelPromote(w http.ResponseWriter, r *http.Request) {
	h.panelSetAdmin(w, r, true)
}

func (h *Handler) panelDemote(w http.ResponseWriter, r *http.Request) {
	h.panelSetAdmin(w, r, false)
}

func (h *Handler) panelSetAdmin(w http.ResponseWriter, r *http.Request, isAdmin bool) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if !verifyCSRF(r) {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.store.updateUserAdmin(r.Context(), id, isAdmin); err != nil {
		switch {
		case errors.Is(err, errLastAdmin):
			h.redirectWithError(w, r, "/admin/panel", "no podés quitarle admin al único administrador activo")
		case errors.Is(err, errNotFound):
			h.redirectWithError(w, r, "/admin/panel", "usuario no encontrado")
		default:
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	http.Redirect(w, r, "/admin/panel", http.StatusFound)
}

func (h *Handler) panelDeleteUser(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if !verifyCSRF(r) {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.store.softDeleteUser(r.Context(), id); err != nil {
		switch {
		case errors.Is(err, errLastAdmin):
			h.redirectWithError(w, r, "/admin/panel", "no podés dar de baja al único administrador activo")
		case errors.Is(err, errNotFound):
			h.redirectWithError(w, r, "/admin/panel", "usuario no encontrado")
		default:
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	http.Redirect(w, r, "/admin/panel", http.StatusFound)
}

func (h *Handler) panelRestoreUser(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if !verifyCSRF(r) {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.store.restoreUser(r.Context(), id); err != nil {
		if errors.Is(err, errNotFound) {
			h.redirectWithError(w, r, "/admin/panel", "usuario no encontrado")
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/panel", http.StatusFound)
}

func (h *Handler) panelResetPassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if !verifyCSRF(r) {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}
	id := chi.URLParam(r, "id")
	password := r.FormValue("password")
	if len(password) < 8 {
		h.redirectWithError(w, r, "/admin/panel", "la contraseña debe tener al menos 8 caracteres")
		return
	}

	hash, err := security.HashPassword(password)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := h.store.updateUserPassword(r.Context(), id, hash); err != nil {
		if errors.Is(err, errNotFound) {
			h.redirectWithError(w, r, "/admin/panel", "usuario no encontrado")
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/panel", http.StatusFound)
}

func (h *Handler) panelDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := h.store.listAllDevices(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	csrf, err := ensureCSRFCookie(w, r)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	renderTemplate(w, h.tmpl, http.StatusOK, "devices.html",
		devicesPageData{CSRFToken: csrf, Devices: devices, Error: r.URL.Query().Get("error"), Active: "devices", InstanceName: h.instanceName(r.Context())})
}

func (h *Handler) panelRevokeDevice(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if !verifyCSRF(r) {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.store.deleteDevice(r.Context(), id); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/panel/devices", http.StatusFound)
}

func (h *Handler) panelSettings(w http.ResponseWriter, r *http.Request) {
	cfg, err := settings.Get(r.Context(), h.store.db)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	csrf, err := ensureCSRFCookie(w, r)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	renderTemplate(w, h.tmpl, http.StatusOK, "settings.html", settingsPageData{
		CSRFToken:       csrf,
		Active:          "settings",
		Settings:        *cfg,
		Error:           r.URL.Query().Get("error"),
		GeneratedSecret: r.URL.Query().Get("generated_secret"),
		InstanceName:    cfg.InstanceName,
	})
}

// panelUpdateSettings validates and saves the four operator-tunable
// settings. Bounds are deliberately generous but not unlimited — wide
// enough not to get in the way, tight enough to catch a fat-fingered
// value (e.g. "0" attempts, which would lock every login out including
// the admin's own).
func (h *Handler) panelUpdateSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if !verifyCSRF(r) {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}

	name := strings.TrimSpace(r.FormValue("instance_name"))
	retentionDays, retentionErr := strconv.Atoi(r.FormValue("tombstone_retention_days"))
	attempts, attemptsErr := strconv.Atoi(r.FormValue("login_rate_limit_attempts"))
	windowSecs, windowErr := strconv.Atoi(r.FormValue("login_rate_limit_window_seconds"))

	switch {
	case name == "" || len(name) > 60:
		h.redirectWithError(w, r, "/admin/panel/settings", "el nombre de la instancia debe tener entre 1 y 60 caracteres")
		return
	case retentionErr != nil || retentionDays < 1 || retentionDays > 3650:
		h.redirectWithError(w, r, "/admin/panel/settings", "la retención de tombstones debe ser un número entre 1 y 3650 días")
		return
	case attemptsErr != nil || attempts < 1 || attempts > 100:
		h.redirectWithError(w, r, "/admin/panel/settings", "el límite de intentos de login debe ser un número entre 1 y 100")
		return
	case windowErr != nil || windowSecs < 10 || windowSecs > 3600:
		h.redirectWithError(w, r, "/admin/panel/settings", "la ventana del límite de login debe ser un número entre 10 y 3600 segundos")
		return
	}

	err := settings.Update(r.Context(), h.store.db, settings.Settings{
		InstanceName:                name,
		TombstoneRetentionDays:      retentionDays,
		LoginRateLimitAttempts:      attempts,
		LoginRateLimitWindowSeconds: windowSecs,
	})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/panel/settings", http.StatusFound)
}

// panelGenerateSecret suggests a new JWT_SECRET value for the operator
// to copy into their .env — it's never persisted or applied at
// runtime. Actually rotating the active secret would invalidate every
// session instantly and without warning, and JWT_SECRET is sourced
// from the environment by design (see specs/panel-admin/design.md and
// cmd/server/main.go) — this only helps generate a strong value, using
// the same primitive already used for opaque refresh tokens.
func (h *Handler) panelGenerateSecret(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if !verifyCSRF(r) {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}

	raw, _, err := security.GenerateOpaqueToken()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/panel/settings?generated_secret="+url.QueryEscape(raw), http.StatusFound)
}
