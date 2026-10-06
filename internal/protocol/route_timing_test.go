package protocol

import (
	"encoding/json"
	"testing"
)

func TestWANW11RouteTimingBoundsAndLegacyDefaults(t *testing.T) {
	if err := (RouteTiming{}).Validate(); err != nil {
		t.Fatal(err)
	}
	var legacy RouteTiming
	if err := json.Unmarshal([]byte(`{}`), &legacy); err != nil || legacy.Effective().HeadStartMS != 750 {
		t.Fatal(legacy, err)
	}
	for _, bad := range []RouteTiming{{HeadStartMS: 249}, {HeadStartMS: 3001}, {CycleMS: 4999}, {CycleMS: 30001}, {ProbeMS: 9999}, {CooldownMS: 59999}, {PollMS: 499}, {QuietMS: 1999}, {PollMS: 10000, QuietMS: 2000}, {HeadStartMS: 3000, CycleMS: 5000}, {CycleMS: ^NetworkUint(0)}} {
		if bad.Validate() == nil {
			t.Fatal("accepted invalid timing", bad)
		}
	}
	for _, raw := range []string{`{"poll_ms":500}`, `{"poll_ms":"-1"}`, `{"poll_ms":"18446744073709551616"}`} {
		var timing RouteTiming
		if json.Unmarshal([]byte(raw), &timing) == nil {
			t.Fatal("accepted invalid encoding", raw)
		}
	}
}
