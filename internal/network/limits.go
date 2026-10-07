package network

import "time"

// Initial admission/policy defaults are finite engineering bounds, not measured
// operator capacity. W11/W13/W15 own tuning and native resource evidence.
const (
	DirectHeadStart                = 750 * time.Millisecond
	DirectTransportStagger         = 200 * time.Millisecond
	ConnectionCycle                = 10 * time.Second
	DirectProbeInterval            = 60 * time.Second
	ServiceRetryQuietPeriod        = 5 * time.Second
	DirectProbeJitter              = 15 * time.Second
	MaxDirectCooldown              = 4 * time.Minute
	MaxDirectAttempts              = 2
	MaxRelayAttempts               = 1
	MaxPoolsPerTarget              = 2
	MaxActivePeerSlots             = 32
	MaxRequestsPerTarget           = 8 // includes queued HTTP requests, not relay tunnels
	MaxDataTunnels                 = 8
	MaxTunnelsPerPeer              = 2
	MaxUnknownEnrollmentTunnels    = 2
	MaxServiceDataTunnels          = 64
	MaxServiceControls             = 128
	MaxServiceReservations         = 128
	MaxReplayEntries               = 1024
	ReplayWindow                   = 5 * time.Minute
	ChallengeLifetime              = 60 * time.Second
	CandidateLifetime              = 10 * time.Minute
	ReannounceInterval             = 2 * time.Minute
	AttachmentLifetime             = 30 * time.Second
	InnerHandshakeTimeout          = 10 * time.Second
	TunnelLifetime                 = 60 * time.Minute
	TunnelDrainTimeout             = 30 * time.Second
	TunnelIdleTimeout              = 60 * time.Second
	HeartbeatInterval              = 25 * time.Second
	ControlStaleTimeout            = 75 * time.Second
	MaxMetadataOperationsPerMinute = 60
	MetadataBurst                  = 10
	// MaxOutstandingChallenges is the service's per-device bound on issued,
	// unconsumed challenges. Each signed operation holds one until it posts.
	MaxOutstandingChallenges = 2
	// ClientMetadataBurst stays below MetadataBurst: a restarted daemon cannot
	// know how much of the service's bucket its previous instance spent.
	ClientMetadataBurst = 6
	// AnnouncementRenewalMargin withdraws readiness this long before an
	// accepted announcement expires when renewals keep failing transiently.
	AnnouncementRenewalMargin = time.Minute
	MaxNetworkHeaderBytes     = 16 << 10
	MaxDiscoveryDatagramBytes = 1200
)
