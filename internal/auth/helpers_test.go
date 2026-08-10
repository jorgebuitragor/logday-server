package auth

import "testing"

func TestValidEmail(t *testing.T) {
	cases := []struct {
		email string
		want  bool
	}{
		{"someone@example.com", true},
		{"a@b", true}, // syntax-only check, not a deliverability check — see validEmail's doc comment
		{"not-an-email", false},
		{"", false},
		{"@example.com", false},
		{"someone@", false},
		{"someone with spaces@example.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.email, func(t *testing.T) {
			if got := validEmail(tc.email); got != tc.want {
				t.Fatalf("validEmail(%q) = %v, want %v", tc.email, got, tc.want)
			}
		})
	}
}
