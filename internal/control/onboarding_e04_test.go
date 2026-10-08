package control

import "testing"

// F11: a remote device's chosen name is shown without control characters.
func TestOnboardingE04PrintableLabel(t *testing.T) {
	for in, want := range map[string]string{"Pi": "Pi", " Laptop\x1b[31m ": "Laptop[31m", "\x07\n": "", "Büro-PC": "Büro-PC"} {
		if got := printableLabel(in); got != want {
			t.Errorf("printableLabel(%q) = %q, want %q", in, got, want)
		}
	}
}
