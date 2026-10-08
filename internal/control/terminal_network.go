package control

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/calebhabesh/orbit/internal/config"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/network"
	"github.com/calebhabesh/orbit/internal/repository"
)

type networkReview struct {
	Intent  tc.NetworkIntent
	Current string
	Review  tc.Review
}

// resolvePackaged fills the packaged release profile into an intent whose
// policy names its digest while the stored selection differs. Naming that
// digest in a reviewed setup/network intent is the owner's review of the
// packaged operator and privacy text, which previews display.
func (c *Controller) resolvePackaged(in tc.NetworkIntent) tc.NetworkIntent {
	if in.Profile != nil || in.Policy.Profile == "" {
		return in
	}
	packaged, ok := network.BuiltinProfile()
	if !ok {
		return in
	}
	digest, err := packaged.Digest()
	if err != nil || digest != in.Policy.Profile {
		return in
	}
	if stored, err := config.LoadNetworkProfile(c.db.StateDir(), 0); err == nil {
		if sd, _ := stored.Digest(); sd == digest {
			return in
		}
	}
	p := packaged.Profile
	in.Profile, in.Authority, in.Environment = &p, packaged.Authority, packaged.Environment
	return in
}

func operatorChangeError() error {
	return &ControlError{Code: "PROFILE_OPERATOR_CHANGE", Message: "the profile names a different operator than this device uses", Action: "review the new operator and privacy text, then confirm the replacement (orbit network set --replace-operator); peers reach each other through Orbit services only while they use the same operator"}
}

func (c *Controller) validateNetworkIntent(in tc.NetworkIntent) error {
	if err := in.Policy.Validate(); err != nil {
		return err
	}
	in = c.resolvePackaged(in)
	if in.Policy.LANAdvertising && in.Policy.Mode == "manual" {
		return terminalError("UNSUPPORTED_CAPABILITY")
	}
	if in.ServiceRoots != "" {
		if in.Profile == nil || in.Environment == "release" {
			return terminalError("SERVICE_TRUST_INVALID")
		}
		if _, _, err := config.ValidateServiceRoots([]byte(in.ServiceRoots), c.options.Now()); err != nil {
			return terminalError("SERVICE_TRUST_INVALID")
		}
	}
	if in.Profile != nil {
		var old *network.ProfileSelection
		prior, err := config.LoadNetworkProfile(c.db.StateDir(), 0) // retains expired rollback floor
		if err == nil {
			old = &prior
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		floors, err := config.LoadProfileFloors(c.db.StateDir())
		if err != nil {
			return err
		}
		s, err := network.ReviewProfileChange(old, floors, *in.Profile, in.Authority, in.Environment, uint64(c.options.Now().Unix()), in.ReplaceOperator)
		if errors.Is(err, network.ErrOperatorChange) {
			return operatorChangeError()
		}
		if err != nil {
			return err
		}
		digest, err := s.Digest()
		if err != nil {
			return err
		}
		if digest != in.Policy.Profile || (in.Policy.Mode == "automatic" && s.Environment != "release") {
			return terminalError("PROFILE_MISMATCH")
		}
	} else if in.Policy.Profile != "" {
		s, err := config.LoadNetworkProfile(c.db.StateDir(), uint64(c.options.Now().Unix()))
		if err != nil {
			return err
		}
		digest, err := s.Digest()
		if err != nil {
			return err
		}
		if digest != in.Policy.Profile || (in.Policy.Mode == "automatic" && s.Environment != "release") {
			return terminalError("PROFILE_MISMATCH")
		}
	}
	return nil
}
func (c *Controller) saveNetworkIntent(in tc.NetworkIntent) error {
	in = c.resolvePackaged(in)
	if in.DeclineAutomaticOffer {
		if err := config.DeclineAutomaticOffer(c.db.StateDir()); err != nil {
			return err
		}
	}
	if in.Profile != nil {
		// Each profile review states its trust: custom roots are replaced or removed.
		if err := config.SaveServiceRoots(c.db.StateDir(), []byte(in.ServiceRoots), c.options.Now()); err != nil {
			return err
		}
		s := network.ProfileSelection{Profile: *in.Profile, Authority: in.Authority, Environment: in.Environment, HighestEpoch: in.Profile.Epoch}
		if err := config.SaveNetworkProfileChange(c.db.StateDir(), s, uint64(c.options.Now().Unix()), in.ReplaceOperator); err != nil {
			return err
		}
	}
	if err := config.SaveNetworkPolicy(c.db.StateDir(), in.Policy); err != nil {
		return err
	}
	if in.Policy.Profile != "" {
		if err := config.RebindPeerRoutes(c.db.StateDir(), in.Policy.Profile); err != nil {
			return err
		}
	}
	if (in.Policy.Mode == "local_only" || in.Policy.Mode == "manual") && c.options.NetworkPolicy != nil && (c.options.NetworkPolicy.Mode == "automatic" || c.options.NetworkPolicy.Mode == "self_hosted") && c.options.StopInternet != nil {
		return c.options.StopInternet()
	}
	return nil
}
func (c *Controller) networkGeneration() (string, error) {
	policy, err := config.LoadNetworkPolicy(c.db.StateDir())
	if err != nil {
		return "", err
	}
	s, err := config.LoadNetworkProfile(c.db.StateDir(), 0)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	roots, err := os.ReadFile(filepath.Join(c.db.StateDir(), config.ServiceRootsFile))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return generation(struct {
		Policy  tc.NetworkPolicy
		Profile network.ProfileSelection
		Roots   []byte
	}{policy, s, roots}), nil
}
func (c *Controller) terminalNetworkQuery(ctx context.Context, q tc.Query) (tc.Result, error) {
	r := terminalResult()
	policy, err := config.LoadNetworkPolicy(c.db.StateDir())
	if err != nil {
		return r, err
	}
	active := policy
	if c.options.NetworkPolicy != nil {
		active = *c.options.NetworkPolicy
	}
	status := &tc.NetworkStatus{GeneratedAt: c.options.Now().UTC().Format(time.RFC3339Nano), Probes: []network.ProbeResult{}, Policy: policy, ActivePolicy: active, RestartRequired: policy != active, Code: "NOT_TESTED", Observations: []tc.NetworkObservation{}}
	r.Network = status
	if q.Kind == "network_preview" {
		if q.NetworkPlan == nil {
			return r, terminalError("INVALID_REQUEST")
		}
		in := *q.NetworkPlan
		in.Review = tc.Review{}
		if policy.Generation == ^tc.Uint(0) {
			return r, terminalError("NETWORK_GENERATION_EXHAUSTED")
		}
		in.Policy.Generation = policy.Generation + 1
		if err = c.validateNetworkIntent(in); err != nil {
			return r, err
		}
		current, err := c.networkGeneration()
		if err != nil {
			return r, err
		}
		token, err := randomTerminalID()
		if err != nil {
			return r, err
		}
		review := tc.Review{Token: token, Generation: generation(struct {
			Current string
			Intent  tc.NetworkIntent
		}{current, in}), ExpiresAt: c.options.Now().Add(15 * time.Minute).UTC().Format(time.RFC3339Nano)}
		if err = c.db.SaveTerminalRecord(ctx, "networkreview/"+token, networkReview{in, current, review}, true); err != nil {
			return r, err
		}
		status.Policy = in.Policy
		r.Review = &review
		if shown := c.resolvePackaged(in); shown.Profile != nil {
			status.Operator = shown.Profile.Operator
			status.Privacy = shown.Profile.Privacy
			status.ProfileExpires = time.Unix(int64(shown.Profile.Expires), 0).UTC().Format(time.RFC3339)
			status.ServiceTrust = "system"
			if in.ServiceRoots != "" {
				_, fingerprint, _ := config.ValidateServiceRoots([]byte(in.ServiceRoots), c.options.Now())
				status.ServiceTrust = "custom:" + fingerprint
			}
		}
		return r, nil
	}
	if c.options.NetworkError != "" {
		status.Code = c.options.NetworkError
	} else if active.Mode == "local_only" {
		status.Code = "LOCAL_ONLY"
	} else if active.Mode == "manual" {
		status.Code = "MANUAL"
	} else {
		s, e := config.LoadNetworkProfile(c.db.StateDir(), uint64(c.options.Now().Unix()))
		if e != nil {
			status.Code = "PROFILE_INVALID"
			if errors.Is(e, os.ErrNotExist) {
				// Keep the established aggregate code for existing consumers and
				// expose the qualified state alongside it.
				status.Code = "PROFILE_MISSING_OR_EXPIRED"
				status.ProfileState = "missing"
			} else if e.Error() == "PROFILE_EXPIRED" {
				status.Code = "PROFILE_EXPIRED"
				status.ProfileState = "expired"
			} else {
				status.ProfileState = "invalid"
			}
		} else {
			status.ProfileState = "verified"
			status.ServiceTrust = "system"
			if s.Environment != "release" {
				if _, fingerprint, e := config.LoadServiceRoots(c.db.StateDir(), c.options.Now()); e != nil {
					status.ServiceTrust = "invalid"
				} else if fingerprint != "" {
					status.ServiceTrust = "custom:" + fingerprint
				}
			}
			status.ProfileExpires = time.Unix(int64(s.Profile.Expires), 0).UTC().Format(time.RFC3339)
			status.Operator = s.Profile.Operator
			status.Privacy = s.Profile.Privacy
			status.Code = "SERVICE_UNAVAILABLE"
			if c.options.Relay != nil && c.options.Relay.Ready() {
				status.Ready = true
				status.Code = "SERVICE_READY"
			}
		}
	}
	if c.options.StoppedAdapter {
		status.Code = "DAEMON_STOPPED"
		status.Ready = false
	} else if status.RestartRequired {
		status.Code = "NETWORK_RESTART_REQUIRED"
		status.Ready = false
	}
	status.Action = networkAction(status.Code)
	if err = c.packagedStatus(status, policy); err != nil {
		return r, err
	}
	targets := []network.Target{}
	if c.options.Network != nil {
		targets = c.options.Network.KnownTargets()
	}
	routes, err := config.LoadPeerRoutes(c.db.StateDir())
	if err != nil {
		return r, err
	}
	for _, route := range routes {
		t := network.Target{Device: terminalID(route.Device), Pin: history.Digest(terminalID(route.Pin)), Profile: history.Digest(terminalID(route.Profile)), Purpose: network.PeerData}
		found := false
		for _, known := range targets {
			if known == t {
				found = true
				break
			}
		}
		if !found {
			targets = append(targets, t)
		}
	}
	sort.Slice(targets, func(i, j int) bool {
		return hex.EncodeToString(targets[i].Device[:])+string(targets[i].Purpose) < hex.EncodeToString(targets[j].Device[:])+string(targets[j].Purpose)
	})
	for _, target := range targets {
		if target.Purpose != network.PeerData {
			continue
		}
		device := hex.EncodeToString(target.Device[:])
		if q.ID != "" && q.ID != device {
			continue
		}
		o := network.Observation{Route: "not_tested", Code: "NOT_TESTED"}
		summary := network.CandidateSummary{}
		udp := "NOT_TESTED"
		if c.options.Network != nil {
			o = c.options.Network.Observe(target)
			summary = c.options.Network.Candidates(target)
			if failure := c.options.Network.ICEFailure(target); failure != nil {
				udp = network.ProbeCode(ctx, failure)
			}
		}
		when, freshness := "", "untested"
		if !o.ObservedAt.IsZero() {
			when = o.ObservedAt.UTC().Format(time.RFC3339Nano)
			freshness = "recent"
			if c.options.Now().Sub(o.ObservedAt) > 2*time.Minute {
				freshness = "stale"
			}
		}
		status.Observations = append(status.Observations, tc.NetworkObservation{Device: device, Pin: hex.EncodeToString(target.Pin[:]), Purpose: string(target.Purpose), Route: o.Route, Code: o.Code, ObservedAt: when, Generation: tc.Uint(o.Generation), Freshness: freshness, Action: networkAction(o.Code), LANCandidates: tc.Uint(summary.LAN), PublicCandidates: tc.Uint(summary.Public), ExpiredCandidates: tc.Uint(summary.Expired), UDPCode: udp})
	}

	return r, nil
}
func (c *Controller) terminalNetworkMutation(ctx context.Context, m tc.Mutation) (tc.Result, error) {
	c.terminalMu.Lock()
	defer c.terminalMu.Unlock()
	fp, err := m.Fingerprint()
	if err != nil {
		return tc.Result{}, err
	}
	var old repository.TerminalRecord
	if err = c.db.TerminalRecord(ctx, "operation/"+m.OperationID, &old); err == nil {
		return c.replayTerminal(old, fp)
	} else if !errors.Is(err, repository.ErrOperationNotFound) {
		return tc.Result{}, err
	}
	var saved networkReview
	in := *m.Network
	review := in.Review
	in.Review = tc.Review{}
	current, err := c.networkGeneration()
	if err != nil {
		return tc.Result{}, err
	}
	expiry, e := time.Parse(time.RFC3339Nano, review.ExpiresAt)
	if err = c.db.TerminalRecord(ctx, "networkreview/"+review.Token, &saved); err != nil || e != nil || saved.Review != review || current != saved.Current || generation(in) != generation(saved.Intent) || !c.options.Now().Before(expiry) {
		return tc.Result{}, terminalError("STALE_VIEW")
	}
	if err = c.validateNetworkIntent(in); err != nil {
		return tc.Result{}, err
	}
	cfg, err := config.Load(c.db.StateDir())
	if err != nil {
		return tc.Result{}, err
	}
	r := terminalResult()
	r.State = "running"
	r.Operation = &tc.Operation{ID: m.OperationID, Kind: m.Kind, Fingerprint: fp, State: "running", Phase: "accepted", CommittedEffects: []tc.Effect{}}
	record := repository.TerminalRecord{Mutation: m, Result: r, Owner: cfg.DeviceID, CreatedAt: c.options.Now().UTC().Format(time.RFC3339Nano)}
	owned, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if err = c.db.SaveTerminalRecord(owned, "operation/"+m.OperationID, record, true); err != nil {
		return r, err
	}
	if err = c.callHook("terminal.network.accepted"); err != nil {
		return r, err
	}
	return c.executeTerminal(owned, record)
}

// packagedStatus reports the packaged release profile, a reviewable newer epoch
// of the selected authority and the one-time Automatic offer for manual installs.
func (c *Controller) packagedStatus(status *tc.NetworkStatus, policy tc.NetworkPolicy) error {
	packaged, ok := network.BuiltinProfile()
	if !ok {
		return nil
	}
	digest, err := packaged.Digest()
	if err != nil {
		return nil
	}
	expires := time.Unix(int64(packaged.Profile.Expires), 0).UTC()
	status.Builtin = &tc.BuiltinProfile{Digest: digest, Operator: packaged.Profile.Operator, Privacy: packaged.Profile.Privacy, Epoch: tc.Uint(packaged.Profile.Epoch), Expires: expires.Format(time.RFC3339), Expired: !c.options.Now().Before(expires)}
	if status.Builtin.Expired {
		return nil
	}
	switch policy.Mode {
	case "manual":
		declined, err := config.AutomaticOfferDeclined(c.db.StateDir())
		if err != nil {
			return err
		}
		status.AutomaticOffer = !declined
		if status.AutomaticOffer && status.Code == "MANUAL" {
			status.Action = "Automatic connection with " + packaged.Profile.Operator + " is available: review it with orbit network automatic, or keep manual with orbit network automatic --decline."
		}
	case "automatic":
		stored, err := config.LoadNetworkProfile(c.db.StateDir(), 0)
		if err == nil && stored.Environment == "release" && stored.Authority == packaged.Authority && packaged.Profile.Epoch > stored.HighestEpoch {
			status.ProfileUpdate = "available"
			status.Action = "An updated service profile from " + packaged.Profile.Operator + " is packaged with this version: review it with orbit network update."
		} else if errors.Is(err, os.ErrNotExist) && policy.AwaitingProfile {
			status.Action = "Restart the daemon to use the packaged service profile, or review it with orbit network automatic."
		}
	}
	return nil
}
