package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/jorgebuitragor/logday-server/internal/auth"
	"github.com/jorgebuitragor/logday-server/internal/db"
	"github.com/jorgebuitragor/logday-server/internal/sync"
	"github.com/jorgebuitragor/logday-server/internal/task"
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

	taskStore := task.NewStore(database)
	taskHandler := task.NewHandler(taskStore, authHandler)

	syncStore := sync.NewStore(database)
	syncHandler := sync.NewHandler(syncStore, authHandler)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

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

	authHandler.Routes(r)
	taskHandler.Routes(r)
	syncHandler.Routes(r)

	server := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("listening on %s", addr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
