package auth

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/jorgebuitragor/logday-server/internal/security"
)

// PanelRoutes registers the HTML admin panel — kept separate from
// Routes (the JSON API) so the split between the two surfaces is
// unambiguous when reading route registrations. Like Routes, it
// registers directly on r (no sub-router / chi.Mount).
func (h *Handler) PanelRoutes(r chi.Router) {
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
	})
}

type formPageData struct {
	CSRFToken string
	Error     string
	Email     string
}

type usersPageData struct {
	CSRFToken string
	Error     string
	Users     []user
}

type devicesPageData struct {
	CSRFToken string
	Error     string
	Devices   []deviceWithOwner
}

func (h *Handler) redirectWithError(w http.ResponseWriter, r *http.Request, path, msg string) {
	http.Redirect(w, r, path+"?error="+url.QueryEscape(msg), http.StatusFound)
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
	renderTemplate(w, h.tmpl, http.StatusOK, "setup.html", formPageData{CSRFToken: csrf})
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
			formPageData{CSRFToken: csrf, Error: msg, Email: email})
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
	renderTemplate(w, h.tmpl, http.StatusOK, "login.html", formPageData{CSRFToken: csrf})
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
		renderTemplate(w, h.tmpl, status, "login.html", formPageData{CSRFToken: csrf, Error: msg, Email: email})
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
		usersPageData{CSRFToken: csrf, Users: users, Error: r.URL.Query().Get("error")})
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
		devicesPageData{CSRFToken: csrf, Devices: devices, Error: r.URL.Query().Get("error")})
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
