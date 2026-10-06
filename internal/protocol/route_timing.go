package protocol

import "errors"

// RouteTiming is private reviewed intent, not a signed peer or service message.
// Milliseconds use decimal strings like the rest of the network control contract.
// Zero selects a finite default; it never disables a deadline or quiet period.
type RouteTiming struct {
	HeadStartMS NetworkUint `json:"head_start_ms,omitempty"`
	CycleMS     NetworkUint `json:"cycle_ms,omitempty"`
	ProbeMS     NetworkUint `json:"probe_ms,omitempty"`
	CooldownMS  NetworkUint `json:"cooldown_ms,omitempty"`
	PollMS      NetworkUint `json:"poll_ms,omitempty"`
	QuietMS     NetworkUint `json:"quiet_ms,omitempty"`
}

func (t RouteTiming) Effective() RouteTiming {
	fields := []*NetworkUint{&t.HeadStartMS, &t.CycleMS, &t.ProbeMS, &t.CooldownMS, &t.PollMS, &t.QuietMS}
	defaults := []NetworkUint{750, 10000, 60000, 240000, 2000, 5000}
	for i, field := range fields {
		if *field == 0 {
			*field = defaults[i]
		}
	}
	return t
}

func (t RouteTiming) Validate() error {
	t = t.Effective()
	if t.HeadStartMS < 250 || t.HeadStartMS > 3000 || t.CycleMS < 5000 || t.CycleMS > 30000 || t.ProbeMS < 10000 || t.ProbeMS > 300000 || t.CooldownMS < t.ProbeMS || t.CooldownMS > 900000 || t.PollMS < 500 || t.PollMS > 10000 || t.QuietMS < 2000 || t.QuietMS > 30000 || t.CycleMS < 2*t.HeadStartMS || t.QuietMS < t.PollMS {
		return errors.New("INVALID_NETWORK_TIMING")
	}
	return nil
}
