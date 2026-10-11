package terminal

import (
	"context"
	"testing"

	"github.com/calebhabesh/orbit/internal/config"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
)

// R04: a public build (no packaged service) proposes Local network on a fresh
// install, and Automatic can still be chosen deliberately.
func TestFinalR04FreshSetupDefaults(t *testing.T) {
	m, w := workflowModel()
	m.opts.FreshInstall = true
	w.query = func(_ context.Context, q tc.Query) (tc.Result, error) {
		if q.Kind == "settings" {
			s := config.DefaultRuntimeSettings()
			return tc.Result{Settings: &s}, nil
		}
		return tc.Result{Network: &tc.NetworkStatus{Policy: tc.NetworkPolicy{Mode: "manual", Generation: 1}}}, nil
	}
	runReply(m, m.setupForm("setup"))
	if got := m.flow.fields[13].input.Value(); got != "local_only" {
		t.Fatalf("fresh public build proposed %q", got)
	}
}
