package terminal_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/testkit"
)

// F15: a review file in an ordinary 0750 home directory reports the file's
// directory and the fix, not a "state directory" error.
func TestOnboardingE03F15ReviewFileInGroupReadableHome(t *testing.T) {
	base := testkit.NewDisposable(t)
	_ = os.Chmod(base, 0700)
	binary := buildOrbitBinary(t, base)
	home := filepath.Join(base, "home")
	if err := os.Mkdir(home, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(home, 0750); err != nil {
		t.Fatal(err)
	}
	state, root := filepath.Join(base, "state"), filepath.Join(base, "Orbit")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	env := []string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}
	e00StopOnCleanup(t, base, state)
	review := filepath.Join(home, "r.json")
	out, err := e00Run(t, env, 60*time.Second, binary, "setup", "--state", state, "--root", root, "--label", "PC", "--name", "Orbit", "--preview", "--review-file", review, "--json")
	t.Logf("setup --review-file ~/r.json (0750 home) => %v: %s", err, strings.TrimSpace(out))
	if err == nil {
		if fi, e := os.Stat(review); e == nil && fi.Mode().Perm() == 0600 {
			t.Log("review file written privately")
			return
		}
	}
	if strings.Contains(out, "state directory") {
		t.Error("review-file failure is described as a state directory problem")
	}
	if !strings.Contains(out, home) || !strings.Contains(out, "mkdir -m 700") {
		t.Error("review-file failure does not name the file's directory and the fix")
	}
}
