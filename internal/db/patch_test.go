package db

import (
	"strings"
	"testing"
)

func TestPatchFieldAbsentVsNull(t *testing.T) {
	raw, err := ParsePatch(strings.NewReader(`{"title":"hola","note":null}`))
	if err != nil {
		t.Fatalf("ParsePatch: %v", err)
	}

	title, err := PatchField[string](raw, "title")
	if err != nil {
		t.Fatalf("PatchField title: %v", err)
	}
	if !title.Set || title.Value != "hola" {
		t.Fatalf("title: got %+v, want Set=true Value=hola", title)
	}

	note, err := PatchField[*string](raw, "note")
	if err != nil {
		t.Fatalf("PatchField note: %v", err)
	}
	if !note.Set {
		t.Fatalf("note: got Set=false, want Set=true (explicit null)")
	}
	if note.Value != nil {
		t.Fatalf("note: got Value=%v, want nil (explicit null)", *note.Value)
	}

	missing, err := PatchField[string](raw, "status")
	if err != nil {
		t.Fatalf("PatchField status: %v", err)
	}
	if missing.Set {
		t.Fatalf("status: got Set=true, want Set=false (absent from payload)")
	}
}

func TestPatchFieldDecodeError(t *testing.T) {
	raw, err := ParsePatch(strings.NewReader(`{"count":"not-a-number"}`))
	if err != nil {
		t.Fatalf("ParsePatch: %v", err)
	}
	if _, err := PatchField[int](raw, "count"); err == nil {
		t.Fatal("PatchField count: want error decoding string into int, got nil")
	}
}
