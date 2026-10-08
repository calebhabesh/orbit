package network

import (
	_ "embed"
	"errors"
	"os"
	"sync"

	"github.com/calebhabesh/orbit/internal/protocol"
)

// ReleaseAuthority is the frozen trust anchor for packaged release profiles.
// A packaged profile signed by any other key is rejected at startup; switching
// operators always goes through a reviewed profile selection instead.
const ReleaseAuthority = "9af3cf8a979f1b635a56831259d7645a62fb7c19db2de8be51e6afb0ce423b36"

// The signed release selection shipped in every ordinary build. Operators
// replace this file (and nothing else) when a new epoch is published.
//
//go:embed release-profile.json
var releaseProfileJSON []byte

type builtinState struct {
	loaded   bool
	override bool
	s        *ProfileSelection
	err      error
}

var (
	builtinMu sync.Mutex
	builtin   builtinState
)

// DecodeBuiltinProfile checks that a packaged selection is a correctly signed
// release profile under ReleaseAuthority. Expiry is not checked here: an
// expired packaged profile is still reported so status can name it.
func DecodeBuiltinProfile(b []byte) (ProfileSelection, error) {
	var s ProfileSelection
	if err := protocol.NetworkDecode(b, &s); err != nil {
		return s, err
	}
	if s.Environment != "release" || s.Authority != ReleaseAuthority || s.Profile.Authority != ReleaseAuthority {
		return s, errors.New("PROFILE_UNTRUSTED")
	}
	return s, s.Validate(0)
}

// DisablePackagedProfileEnv set to "1" makes this process behave like a build
// without a packaged profile. It can only remove the default, never supply
// trust; hermetic test suites set it so they never contact the hosted service.
const DisablePackagedProfileEnv = "ORBIT_DISABLE_PACKAGED_PROFILE"

// BuiltinProfile returns the packaged release selection, if this build has a
// valid one. Callers still check expiry against their own clock.
func BuiltinProfile() (ProfileSelection, bool) {
	builtinMu.Lock()
	defer builtinMu.Unlock()
	if !builtin.override && os.Getenv(DisablePackagedProfileEnv) == "1" {
		return ProfileSelection{}, false
	}
	if !builtin.loaded && !builtin.override {
		builtin.loaded = true
		s, err := DecodeBuiltinProfile(releaseProfileJSON)
		if err == nil {
			builtin.s = &s
		}
		builtin.err = err
	}
	if builtin.s == nil {
		return ProfileSelection{}, false
	}
	return *builtin.s, true
}

// EmbeddedProfile decodes the selection compiled into this build, ignoring the
// disable switch and test overrides; version/provenance output uses it.
func EmbeddedProfile() (ProfileSelection, error) {
	return DecodeBuiltinProfile(releaseProfileJSON)
}

// BuiltinProfileError reports why the packaged profile is unavailable.
func BuiltinProfileError() error {
	BuiltinProfile()
	builtinMu.Lock()
	defer builtinMu.Unlock()
	return builtin.err
}

// OverrideBuiltinProfile substitutes the packaged selection for tests; nil
// models a build without a usable profile. The returned function restores it.
func OverrideBuiltinProfile(s *ProfileSelection) func() {
	builtinMu.Lock()
	old := builtin
	builtin = builtinState{loaded: true, override: true, s: s}
	builtinMu.Unlock()
	return func() {
		builtinMu.Lock()
		builtin = old
		builtinMu.Unlock()
	}
}
