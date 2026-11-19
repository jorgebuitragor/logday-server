package calendar

import (
	"strings"
	"testing"
	"time"
)

// U-06: el id de toda entidad sigue la regla de internal/pathsafe.
func TestValidateEventRequestID(t *testing.T) {
	req := eventRequest{
		Title: "t", Date: "2026-11-19", Color: "indigo", Repeat: "none",
		UpdatedAt: time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC),
	}
	for _, id := range []string{"", "..", "../e1", `a\b`, "a\nb"} {
		req.ID = id
		if err := validateEventRequest(req); err == nil || !strings.HasPrefix(err.Error(), "id ") {
			t.Errorf("id %q: expected an id error, got %v", id, err)
		}
	}
	req.ID = "evento-legado-1"
	if err := validateEventRequest(req); err != nil {
		t.Errorf("legacy id: %v", err)
	}
}
