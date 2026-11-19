package pathsafe

import (
	"strings"
	"testing"
)

func TestSegment(t *testing.T) {
	cases := []struct {
		name string
		v    string
		ok   bool
	}{
		{"uuid", "3f2b8c1e-9d4a-4f6b-8a2e-1c5d7e9f0a3b", true},
		{"id legado", "task-1", true},
		{"fecha", "2026-11-19", true},
		{"instante legado", "2026-10-28T19:30:00.000Z", true},
		{"proyecto con espacios y tildes", "Fábrica de software", true},
		{"tres puntos", "...", true},
		{"punto adentro", "v1.2", true},
		{"vacío", "", false},
		{"punto", ".", false},
		{"dos puntos", "..", false},
		{"barra", "a/b", false},
		{"traversal", "../../etc", false},
		{"barra invertida", `..\..\x`, false},
		{"fecha con traversal", "../.X..", false},
		{"nul", "a\x00b", false},
		{"salto de línea", "a\nb", false},
		{"UTF-8 inválido", "a\xffb", false},
		{"demasiado largo", strings.Repeat("a", MaxIDLen+1), false},
		{"justo en el límite", strings.Repeat("a", MaxIDLen), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Segment("id", tc.v, MaxIDLen)
			if (err == nil) != tc.ok {
				t.Fatalf("Segment(%q): ok=%v, err=%v", tc.v, tc.ok, err)
			}
		})
	}
}

func TestFolder(t *testing.T) {
	cases := []struct {
		name string
		v    string
		ok   bool
	}{
		{"raíz", "", true},
		{"una carpeta", "trabajo", true},
		{"subcarpetas", "trabajo/clientes/2026", true},
		{"con espacios", "mis notas/ideas sueltas", true},
		{"traversal", "../../x", false},
		{"traversal en medio", "a/../../b", false},
		{"punto como segmento", "a/./b", false},
		{"absoluta", "/etc", false},
		{"barra al final", "a/", false},
		{"doble barra", "a//b", false},
		{"barra invertida", `a\..\b`, false},
		{"control", "a/\tb", false},
		{"segmento largo", strings.Repeat("a", MaxSegmentLen+1), false},
		{"total largo", strings.Repeat(strings.Repeat("a", 100)+"/", 4) + "a", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Folder(tc.v)
			if (err == nil) != tc.ok {
				t.Fatalf("Folder(%q): ok=%v, err=%v", tc.v, tc.ok, err)
			}
		})
	}
}

func TestURLParam(t *testing.T) {
	for _, v := range []string{"..%2F..", "a%5Cb", "%2E%2E", "a%00b", "%zz"} {
		if err := URLParam("date", v, MaxDateLen); err == nil {
			t.Errorf("URLParam(%q): expected an error", v)
		}
	}
	for _, v := range []string{"2026-11-19", "2026-11", "task-1"} {
		if err := URLParam("date", v, MaxDateLen); err != nil {
			t.Errorf("URLParam(%q): %v", v, err)
		}
	}
}
