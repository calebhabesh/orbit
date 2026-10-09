package control

import (
	"strings"
	"testing"
)

func TestServiceUnitOverride(t *testing.T) {
	for value, want := range map[string]string{
		"":                       "orbit.service",
		"orbit-trial.service":    "orbit-trial.service",
		"orbit.service":          "orbit.service",
		"other.service":          "orbit.service",
		"orbit-../x.service":     "orbit.service",
		"orbit-trial.service\nX": "orbit.service",
	} {
		t.Setenv(ServiceUnitEnv, value)
		if got := serviceUnit(); got != want {
			t.Errorf("%q: got %q, want %q", value, got, want)
		}
	}
}

func TestUserUnitContentNamesOverride(t *testing.T) {
	t.Setenv(ServiceUnitEnv, "")
	plain := userUnitContent("/bin/orbit", "/s", "127.0.0.1:0")
	if strings.Contains(plain, "Environment=") || !strings.Contains(plain, "SyslogIdentifier=orbit\n") {
		t.Fatalf("default unit changed:\n%s", plain)
	}
	t.Setenv(ServiceUnitEnv, "orbit-trial.service")
	trial := userUnitContent("/bin/orbit", "/s", "127.0.0.1:0")
	if !strings.Contains(trial, "[Service]\nEnvironment=ORBIT_SERVICE_UNIT=orbit-trial.service\n") || !strings.Contains(trial, "SyslogIdentifier=orbit-trial\n") {
		t.Fatalf("trial unit missing override:\n%s", trial)
	}
}
