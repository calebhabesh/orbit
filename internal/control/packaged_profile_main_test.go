package control

import (
	"os"
	"testing"

	"github.com/calebhabesh/file-sync/internal/network"
)

// Hermetic suites never select the packaged hosted profile, so daemons they
// start cannot contact the operated service. W14 tests opt back in explicitly.
func TestMain(m *testing.M) {
	os.Setenv(network.DisablePackagedProfileEnv, "1")
	os.Exit(m.Run())
}
