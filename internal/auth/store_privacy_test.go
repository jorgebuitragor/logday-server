package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/jorgebuitragor/logday-server/internal/settings"
)

func TestAcceptPolicyRejectsStaleVersion(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	u := mustCreateUser(t, s, ctx, "member@example.com", false)

	// La instancia arranca en la versión 1 (seed de la migración) —
	// aceptar la 1 debe funcionar.
	if err := s.acceptPolicy(ctx, u.ID, 1); err != nil {
		t.Fatalf("acceptPolicy(1): %v", err)
	}
	got, err := s.getUserByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("getUserByID: %v", err)
	}
	if got.PrivacyAcceptedVersion == nil || *got.PrivacyAcceptedVersion != 1 {
		t.Fatalf("expected PrivacyAcceptedVersion=1, got %v", got.PrivacyAcceptedVersion)
	}
	if got.PrivacyAcceptedAt == nil {
		t.Fatal("expected PrivacyAcceptedAt to be set")
	}

	// Aceptar una versión que ya no es la vigente (2, sin que el admin
	// la haya subido) se rechaza — evita marcar como aceptado un texto
	// que el usuario nunca vio.
	if err := s.acceptPolicy(ctx, u.ID, 2); !errors.Is(err, errStalePolicyVersion) {
		t.Fatalf("expected errStalePolicyVersion, got %v", err)
	}

	// El admin sube la política a la versión 2 — ahora sí se puede
	// aceptar esa versión.
	cfg, err := settings.Get(ctx, s.db)
	if err != nil {
		t.Fatalf("settings.Get: %v", err)
	}
	cfg.PrivacyPolicyVersion = 2
	if err := settings.Update(ctx, s.db, *cfg); err != nil {
		t.Fatalf("settings.Update: %v", err)
	}
	if err := s.acceptPolicy(ctx, u.ID, 2); err != nil {
		t.Fatalf("acceptPolicy(2) after bump: %v", err)
	}
}

func TestAcceptSensitiveData(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	u := mustCreateUser(t, s, ctx, "member@example.com", false)

	before, err := s.getUserByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("getUserByID: %v", err)
	}
	if before.SensitiveDataAcceptedAt != nil {
		t.Fatal("expected SensitiveDataAcceptedAt to start nil")
	}

	if err := s.acceptSensitiveData(ctx, u.ID); err != nil {
		t.Fatalf("acceptSensitiveData: %v", err)
	}

	after, err := s.getUserByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("getUserByID: %v", err)
	}
	if after.SensitiveDataAcceptedAt == nil {
		t.Fatal("expected SensitiveDataAcceptedAt to be set")
	}
}

func TestExportAndDeleteUserAccount(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	u := mustCreateUser(t, s, ctx, "member@example.com", false)

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO tasks (id, user_id, title, status, created, seq, updated_at)
		 VALUES ('task-1', ?, 'Una tarea', 'todo', '2026-01-01T00:00:00Z', 1, '2026-01-01T00:00:00Z')`,
		u.ID); err != nil {
		t.Fatalf("seeding a task: %v", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO absence_days (id, user_id, date, type, seq, updated_at)
		 VALUES ('absence-1', ?, '2026-01-02', 'incapacidad', 1, '2026-01-01T00:00:00Z')`,
		u.ID); err != nil {
		t.Fatalf("seeding an absence day: %v", err)
	}

	data, err := s.exportUserData(ctx, u.ID)
	if err != nil {
		t.Fatalf("exportUserData: %v", err)
	}
	tasks, ok := data["tasks"].([]map[string]any)
	if !ok || len(tasks) != 1 || tasks[0]["id"] != "task-1" {
		t.Fatalf("expected exactly the seeded task in the export, got %v", data["tasks"])
	}
	absences, ok := data["absence_days"].([]map[string]any)
	if !ok || len(absences) != 1 || absences[0]["type"] != "incapacidad" {
		t.Fatalf("expected exactly the seeded absence day in the export, got %v", data["absence_days"])
	}

	if err := s.deleteUserAccount(ctx, u.ID); err != nil {
		t.Fatalf("deleteUserAccount: %v", err)
	}

	if _, err := s.getUserByID(ctx, u.ID); !errors.Is(err, errNotFound) {
		t.Fatalf("expected the user to be gone, got %v", err)
	}
	var remainingTasks int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE user_id = ?`, u.ID).Scan(&remainingTasks); err != nil {
		t.Fatalf("counting remaining tasks: %v", err)
	}
	if remainingTasks != 0 {
		t.Fatalf("expected no tasks left for the deleted user, got %d", remainingTasks)
	}
}
