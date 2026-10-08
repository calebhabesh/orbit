package terminal_test

import (
	"context"
	"github.com/calebhabesh/orbit/internal/app"
	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/testkit"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWANW11BinaryReviewedTimingAndRestart(t *testing.T) {
	base := testkit.NewDisposable(t)
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(base, "state")
	cfg, err := app.Initialize(context.Background(), dir, app.SystemDependencies())
	if err != nil {
		t.Fatal(err)
	}
	cli := w06CLI{t: t, binary: buildOrbitBinary(t, base)}
	t.Cleanup(func() {
		if err := testkit.ValidateDestructiveTarget(base, dir); err != nil {
			t.Error(err)
			return
		}
		if err := app.StopAgent(dir, 10*time.Second); err != nil {
			t.Error(err)
		}
	})
	review := filepath.Join(base, "review.json")
	_, _, err = cli.call("", "network", "preview", "--state", dir, "--review-file", review, "--network-poll", "1ms", "--json")
	if err == nil {
		t.Fatal("invalid timing preview succeeded")
	}
	cli.ok("network", "preview", "--state", dir, "--review-file", review, "--direct-head-start", "1500ms", "--connection-cycle", "15s", "--direct-probe", "10s", "--direct-cooldown", "60s", "--network-poll", "500ms", "--network-quiet", "2s", "--json")
	result := cli.ok("network", "apply", "--state", dir, "--review-file", review, "--json")
	if result.State != "completed" {
		t.Fatal(result)
	}
	status := cli.ok("network", "status", "--state", dir, "--json")
	if status.Network.RestartRequired || status.Network.ActivePolicy.Timing.PollMS != 500 || status.Network.ActivePolicy.Timing.HeadStartMS != 1500 || status.Network.Policy.Mode != "manual" {
		t.Fatal(status.Network)
	}
	after, err := config.Load(dir)
	if err != nil || after.DeviceID != cfg.DeviceID {
		t.Fatal("identity changed", err)
	}
	replay := cli.ok("network", "apply", "--state", dir, "--review-file", review, "--json")
	if replay.Operation.ID != result.Operation.ID {
		t.Fatal("operation changed")
	}
	review = filepath.Join(base, "default-review.json")
	cli.ok("network", "preview", "--state", dir, "--review-file", review, "--network-poll", "0s", "--json")
	cli.ok("network", "apply", "--state", dir, "--review-file", review, "--json")
	status = cli.ok("network", "status", "--state", dir, "--json")
	if status.Network.Policy.Timing.PollMS != 0 || status.Network.RestartRequired {
		t.Fatal(status.Network)
	}
}
