//go:build !orbit_hosted

package network

// HostedBuild reports that this build embeds the owner's hosted profile.
const HostedBuild = false

// Public builds carry no packaged service profile.
var releaseProfileJSON []byte
