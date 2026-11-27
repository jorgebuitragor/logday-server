package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/jorgebuitragor/logday-server/internal/absence"
	"github.com/jorgebuitragor/logday-server/internal/auth"
	"github.com/jorgebuitragor/logday-server/internal/calendar"
	"github.com/jorgebuitragor/logday-server/internal/compat"
	"github.com/jorgebuitragor/logday-server/internal/dailyentry"
	"github.com/jorgebuitragor/logday-server/internal/db"
	"github.com/jorgebuitragor/logday-server/internal/note"
	"github.com/jorgebuitragor/logday-server/internal/overtime"
	"github.com/jorgebuitragor/logday-server/internal/realtime"
	"github.com/jorgebuitragor/logday-server/internal/security"
	"github.com/jorgebuitragor/logday-server/internal/sync"
	"github.com/jorgebuitragor/logday-server/internal/task"
	logdayweb "github.com/jorgebuitragor/logday-server/web"
)

func main() {
	addr := ":" + envOr("PORT", "8080")
	dbPath := envOr("DATABASE_PATH", "./data/logday.db")

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET must be set")
	}

	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("closing database: %v", err)
		}
	}()

	ctx := context.Background()

	if err := db.Migrate(ctx, database); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	authStore := auth.NewStore(database)
	if err := auth.Bootstrap(ctx, authStore); err != nil {
		log.Fatalf("bootstrap: %v", err)
	}
	authHandler := auth.NewHandler(authStore, []byte(jwtSecret))

	hub := realtime.NewHub()
	realtimeHandler := realtime.NewHandler(hub, authHandler)

	taskStore := task.NewStore(database)
	taskHandler := task.NewHandler(taskStore, authHandler, hub)

	noteStore := note.NewStore(database)
	noteHandler := note.NewHandler(noteStore, authHandler, hub)

	overtimeStore := overtime.NewStore(database)
	overtimeHandler := overtime.NewHandler(overtimeStore, authHandler, hub)

	calendarStore := calendar.NewStore(database)
	calendarHandler := calendar.NewHandler(calendarStore, authHandler, hub)

	absenceStore := absence.NewStore(database)
	absenceHandler := absence.NewHandler(absenceStore, authHandler, hub)

	dailyEntryStore := dailyentry.NewStore(database)
	dailyEntryHandler := dailyentry.NewHandler(dailyEntryStore, authHandler, hub)

	syncStore := sync.NewStore(database)
	syncHandler := sync.NewHandler(syncStore, authHandler)

	clientIPConfig, err := security.ParseClientIPConfig(os.Getenv("TRUSTED_PROXIES"), os.Getenv("CLIENT_IP_HEADER"))
	if err != nil {
		log.Fatal(err)
	}
	bodyLimit, err := parseBodyLimit(os.Getenv("MAX_BODY_MB"))
	if err != nil {
		log.Fatal(err)
	}

	go runTombstonePurge(database)

	r := chi.NewRouter()
	// Before the logger, so the access log shows the real client IP.
	r.Use(security.ClientIPMiddleware(clientIPConfig))
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(security.HeadersMiddleware)
	r.Use(security.BodyLimitMiddleware(bodyLimit))
	if corsOrigins := parseCORSOrigins(os.Getenv("CORS_ALLOWED_ORIGINS")); len(corsOrigins) > 0 {
		r.Use(security.CORSMiddleware(corsOrigins))
	}

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := database.PingContext(r.Context()); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ok")); err != nil {
			log.Printf("writing health response: %v", err)
		}
	})

	r.Get("/version", compat.VersionHandler)

	// La API de los clientes exige un protocolo mínimo (specs/protocolo-
	// version). Quedan fuera /health y /version, el panel y la web
	// embebida (páginas de navegador) y /ws, que solo avisa: un cliente
	// viejo recibe el 426 en su siguiente pull.
	r.Group(func(api chi.Router) {
		api.Use(compat.RequireClientProtocol(compat.MinClientProtocol, "/auth/logout"))
		authHandler.Routes(api)
		taskHandler.Routes(api)
		noteHandler.Routes(api)
		overtimeHandler.Routes(api)
		calendarHandler.Routes(api)
		absenceHandler.Routes(api)
		dailyEntryHandler.Routes(api)
		syncHandler.Routes(api)
	})
	authHandler.PanelRoutes(r)
	realtimeHandler.Routes(r)
	logdayweb.Routes(r)

	server := newHTTPServer(addr, r)

	log.Printf("listening on %s", addr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

// newHTTPServer sets the timeouts of specs/limites-auth: a client that
// opens connections and sends nothing, or reads the response byte by
// byte, can't hold them forever. They don't cut /ws: net/http clears a
// connection's deadlines when the WebSocket upgrade hijacks it
// (TestServeWSOutlivesServerTimeouts in internal/realtime).
func newHTTPServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       time.Minute,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseBodyLimit reads MAX_BODY_MB, the body limit for authenticated
// requests (notes, sync). Empty means security.DefaultBodyLimitBytes.
func parseBodyLimit(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return security.DefaultBodyLimitBytes, nil
	}
	mb, err := strconv.Atoi(raw)
	if err != nil || mb < 1 || mb > 1024 {
		return 0, fmt.Errorf("MAX_BODY_MB must be a whole number of megabytes between 1 and 1024, got %q", raw)
	}
	return int64(mb) << 20, nil
}

// parseCORSOrigins splits a comma-separated CORS_ALLOWED_ORIGINS value
// into trimmed, non-empty origins. An empty/unset value yields nil,
// which keeps CORSMiddleware unregistered entirely (see main()).
func parseCORSOrigins(raw string) []string {
	var origins []string
	for _, o := range strings.Split(raw, ",") {
		if o = strings.TrimSpace(o); o != "" {
			origins = append(origins, o)
		}
	}
	return origins
}

// runTombstonePurge runs db.PurgeTombstones once at startup and then
// once a day, for the lifetime of the process. Self-hosted single
// binary, single container — no external cron dependency.
func runTombstonePurge(database *sql.DB) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		if err := db.PurgeTombstones(context.Background(), database); err != nil {
			log.Printf("purging tombstones: %v", err)
		}
		<-ticker.C
	}
}
