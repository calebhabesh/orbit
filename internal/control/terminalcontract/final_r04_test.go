package terminalcontract

import (
	"strings"
	"testing"
)

func TestFinalR04FreshConnectionMode(t *testing.T) {
	if FreshConnectionMode(nil) != "local_only" || FreshConnectionMode(&BuiltinProfile{Expired: true}) != "local_only" || FreshConnectionMode(&BuiltinProfile{Digest: strings.Repeat("d", 64)}) != "automatic" {
		t.Fatal("fresh default must be Automatic only with a current packaged service")
	}
	if !strings.Contains(NoServiceAction, "orbit-net init") || !strings.Contains(NoServiceAction, "--mode self_hosted") {
		t.Fatal(NoServiceAction)
	}
}
