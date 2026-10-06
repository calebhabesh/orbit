package terminalcontract

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWANW01NetworkControlContracts(t *testing.T) {
	for _, p := range []NetworkPolicy{{Mode: "automatic", Profile: strings.Repeat("a", 64)}, {Mode: "self_hosted", Profile: strings.Repeat("a", 64)}, {Mode: "manual"}, {Mode: "local_only"}} {
		if e := p.Validate(); e != nil {
			t.Fatal(e)
		}
	}
	for _, p := range []NetworkPolicy{{Mode: "automatic"}, {Mode: "manual", Profile: strings.Repeat("a", 64)}, {Mode: "local_only", Profile: strings.Repeat("a", 64)}, {Mode: "future"}} {
		if e := p.Validate(); e == nil {
			t.Fatal("invalid policy")
		}
	}
	q := NetworkQuery{Purpose: "peer_data", Limit: MaxPage, Cursor: strings.Repeat("x", 256)}
	if e := q.Validate(); e != nil {
		t.Fatal(e)
	}
	q.Limit++
	if e := q.Validate(); e == nil {
		t.Fatal("query over limit")
	}
	q.Limit = MaxPage
	q.Cursor += "x"
	if e := q.Validate(); e == nil {
		t.Fatal("cursor over limit")
	}
	a := NetworkApply{Operation: strings.Repeat("a", 64), Policy: NetworkPolicy{Mode: "local_only"}, Review: Review{Token: strings.Repeat("b", 64), Generation: "review-1", ExpiresAt: "2027-01-15T00:00:00Z"}}
	if e := a.Validate(); e != nil {
		t.Fatal(e)
	}
	a.Operation = ""
	if e := a.Validate(); e == nil {
		t.Fatal("unreviewed mutation")
	}
}

// Legacy desired policy is also embedded in durable mutation fingerprints.
// An absent zero timing object must not rewrite those canonical bytes.
func TestWANW11ZeroTimingPreservesLegacyPolicyBytes(t *testing.T) {
	p := NetworkPolicy{Mode: "manual", Generation: 1}
	data, err := json.Marshal(p)
	if err != nil || string(data) != `{"mode":"manual","profile":"","lan_advertising":false,"generation":"1"}` {
		t.Fatal(string(data), err)
	}
	p.Timing.PollMS = 500
	data, err = json.Marshal(p)
	if err != nil || !bytes.Contains(data, []byte(`"timing":{"poll_ms":"500"}`)) {
		t.Fatal(string(data), err)
	}
}
