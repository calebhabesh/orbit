package control

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// F03/F09: AttentionCodes matches every literal attention code this package
// emits, so the terminal's per-code routing cannot miss one, and no attention
// action names an engine command or a subcommand that does not exist.
func TestOnboardingE02AttentionCodesAndActions(t *testing.T) {
	var found []string
	for _, file := range []string{"terminal_status.go", "terminal_sessions.go"} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		for _, block := range regexp.MustCompile(`(?s)tc\.Attention\{.*?\}\)`).FindAllString(src, -1) {
			if m := regexp.MustCompile(`Code:\s*"([A-Z_]+)"`).FindStringSubmatch(block); m != nil && !slices.Contains(found, m[1]) {
				found = append(found, m[1])
			}
			for _, bad := range []string{"orbit engine", "conflicts resolve"} {
				if strings.Contains(block, bad) {
					t.Errorf("%s: attention action names %q:\n%s", file, bad, block)
				}
			}
		}
	}
	slices.Sort(found)
	want := slices.Clone(AttentionCodes)
	slices.Sort(want)
	if !slices.Equal(found, want) {
		t.Fatalf("AttentionCodes %v, emitted %v", want, found)
	}
}
