//go:build orbit_hosted

package network

import _ "embed"

// HostedBuild reports that this build embeds the owner's hosted profile.
const HostedBuild = true

// Owner builds (-tags orbit_hosted) embed the signed release selection.
// Operators replace this file (and nothing else) when a new epoch is published.
//
//go:embed release-profile.json
var releaseProfileJSON []byte
