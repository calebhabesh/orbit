package control

import (
	"testing"
	"time"
)

// F11: a remote device's chosen name is shown without control characters.
func TestOnboardingE04PrintableLabel(t *testing.T) {
	for in, want := range map[string]string{"Pi": "Pi", " Laptop\x1b[31m ": "Laptop[31m", "\x07\n": "", "Büro-PC": "Büro-PC"} {
		if got := printableLabel(in); got != want {
			t.Errorf("printableLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

// F10: approval status checks are 30–36 s apart (two requests each against
// the inviter's 5-per-minute source bucket).
func TestOnboardingE07ApprovalPollInterval(t *testing.T) {
	for range 200 {
		if d := approvalPollInterval(); d < 30*time.Second || d >= 36*time.Second {
			t.Fatalf("interval %s outside [30s, 36s)", d)
		}
	}
}
