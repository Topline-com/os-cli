package output

import "testing"

func TestMaskPIIRedactsPhonesEmailsAndBearerTokens(t *testing.T) {
	in := `Email jane@example.com or call +1 (843) 555-1212. Authorization: Bearer secret-token TOPLINE_PIT=pit-secret`
	got := MaskPII(in)
	for _, forbidden := range []string{"jane@example.com", "555-1212", "secret-token", "pit-secret"} {
		if contains(got, forbidden) {
			t.Fatalf("MaskPII leaked %q in %q", forbidden, got)
		}
	}
	for _, expected := range []string{"[EMAIL]", "[PHONE]", "Bearer [REDACTED]", "TOPLINE_PIT=[REDACTED]"} {
		if !contains(got, expected) {
			t.Fatalf("MaskPII missing %q in %q", expected, got)
		}
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && index(s, sub) >= 0)
}

func index(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
