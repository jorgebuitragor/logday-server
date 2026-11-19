package absence

import (
	"strings"
	"testing"
	"time"
)

// U-06: el id de toda entidad sigue la regla de internal/pathsafe.
func TestValidateDayRequestID(t *testing.T) {
	req := dayRequest{
		Date: "2026-11-19", Type: "vacaciones",
		UpdatedAt: time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC),
	}
	for _, id := range []string{"", "..", "../d1", `a\b`, "a\x00b"} {
		req.ID = id
		if err := validateDayRequest(req); err == nil || !strings.HasPrefix(err.Error(), "id ") {
			t.Errorf("id %q: expected an id error, got %v", id, err)
		}
	}
	req.ID = "ausencia-legada-1"
	if err := validateDayRequest(req); err != nil {
		t.Errorf("legacy id: %v", err)
	}
}
