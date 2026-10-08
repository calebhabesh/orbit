package terminal_test

// E00 onboarding baseline reproductions for the 2026-10-08 owner trial
// findings that need real processes. Each test asserts approved behavior and
// deliberately fails on the current implementation; ordinary runs skip them.
// Opt in with ORBIT_ONBOARDING_BASELINE=1. E01 promoted F01 and F04/F14 into
// ordinary regressions in onboarding_e01_test.go; E02 promoted F03; E03 promoted F15; E04 promoted F08, F11 and F16 (onboarding_e04_test.go). All state, HOME and service-manager
// stand-ins live in marked disposable roots; no real user unit, systemd
// manager, personal folder or deployed service is touched.

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/app"
	"github.com/calebhabesh/orbit/internal/testkit"
)

func onboardingBaseline(t *testing.T) {
	t.Helper()
	if os.Getenv("ORBIT_ONBOARDING_BASELINE") != "1" {
		t.Skip("deliberately failing E00 baseline; opt in with ORBIT_ONBOARDING_BASELINE=1")
	}
}

// e00Run runs the built CLI with an explicit environment and returns combined
// output. Outputs here never contain invitation capabilities.
func e00Run(t *testing.T, env []string, timeout time.Duration, binary string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

func e00StopOnCleanup(t *testing.T, base, dir string) {
	t.Cleanup(func() {
		if err := testkit.ValidateDestructiveTarget(base, dir); err != nil {
			t.Error(err)
			return
		}
		_ = app.StopAgent(dir, 10*time.Second)
	})
}
