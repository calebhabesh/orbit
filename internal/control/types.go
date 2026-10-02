package control

import (
	"io"
	"strconv"

	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/replication"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

type ResolveSelectRequest struct {
	Folder            history.ID          `json:"folder"`
	Path              string              `json:"path"`
	Reviewed          []history.VersionID `json:"reviewed"`
	ExpectedHeadToken history.Digest      `json:"expected_head_token"`
	Selected          history.VersionID   `json:"selected"`
	IdempotencyKey    string              `json:"idempotency_key"`
}

type ResolveMergeRequest struct {
	Folder            history.ID          `json:"folder"`
	Path              string              `json:"path"`
	Reviewed          []history.VersionID `json:"reviewed"`
	ExpectedHeadToken history.Digest      `json:"expected_head_token"`
	Executable        bool                `json:"executable"`
	Content           []byte              `json:"-"`
	ContentReader     io.Reader           `json:"-"`
	ContentDigest     *history.Digest     `json:"content_digest,omitempty"`
	IdempotencyKey    string              `json:"idempotency_key"`
}

type CopyTarget struct {
	HeadID          history.VersionID `json:"head_id"`
	DestinationPath string            `json:"destination_path"`
}

type KeepCopiesRequest struct {
	Folder            history.ID          `json:"folder"`
	Path              string              `json:"path"`
	Reviewed          []history.VersionID `json:"reviewed"`
	ExpectedHeadToken history.Digest      `json:"expected_head_token"`
	Copies            []CopyTarget        `json:"copies"`
	OriginalSelected  *history.VersionID  `json:"original_selected,omitempty"`
	IdempotencyKey    string              `json:"idempotency_key"`
}

type RestoreRequest struct {
	Folder            history.ID          `json:"folder"`
	Path              string              `json:"path"`
	SourceVersion     history.VersionID   `json:"source_version"`
	Reviewed          []history.VersionID `json:"reviewed"`
	ExpectedHeadToken history.Digest      `json:"expected_head_token"`
	IdempotencyKey    string              `json:"idempotency_key"`
}

type RestorePreviewRequest struct {
	Folder        history.ID        `json:"folder"`
	Path          string            `json:"path"`
	SourceVersion history.VersionID `json:"source_version"`
}

type RestorePreview struct {
	Path              string                         `json:"path"`
	SourceVersion     history.VersionID              `json:"source_version"`
	SourceKind        history.Kind                   `json:"source_kind"`
	SourceSize        uint64                         `json:"source_size"`
	SourceDigest      history.Digest                 `json:"source_digest"`
	SourceExecutable  bool                           `json:"source_executable"`
	ContentState      repository.ContentAvailability `json:"content_state"`
	CurrentHeads      []history.VersionID            `json:"current_heads"`
	ExpectedHeadToken history.Digest                 `json:"expected_head_token"`
}

type ResolveResult struct {
	Folder     history.ID        `json:"folder"`
	Path       string            `json:"path"`
	Action     string            `json:"action"`
	ResolvedID history.VersionID `json:"resolved_id"`
	Envelope   history.Envelope  `json:"envelope"`
	Applied    bool              `json:"applied"`
	Replay     bool              `json:"replay,omitempty"`
}

type KeepCopiesResult struct {
	Folder     history.ID                  `json:"folder"`
	Path       string                      `json:"path"`
	ResolvedID history.VersionID           `json:"resolved_id"`
	Envelope   history.Envelope            `json:"envelope"`
	Copies     []repository.CopyStepResult `json:"copies"`
	Completed  bool                        `json:"completed"`
	Replay     bool                        `json:"replay,omitempty"`
}

type FileItem struct {
	Path        string       `json:"path"`
	Kind        history.Kind `json:"kind"`
	Size        uint64       `json:"size"`
	MtimeNS     int64        `json:"mtime_ns"`
	Inode       uint64       `json:"inode"`
	Executable  bool         `json:"executable"`
	BlockReason string       `json:"block_reason,omitempty"`
}

type HistoryItem struct {
	ID           history.VersionID              `json:"id"`
	Path         string                         `json:"path"`
	Kind         history.Kind                   `json:"kind"`
	Parents      []history.VersionID            `json:"parents"`
	Vector       []history.ClockEntry           `json:"vector"`
	Manifest     *history.Manifest              `json:"manifest,omitempty"`
	DisplayTime  string                         `json:"display_time"`
	ContentState repository.ContentAvailability `json:"content_state"`
	IsHead       bool                           `json:"is_head"`
	Applied      bool                           `json:"applied"`
}

type MembershipExportResult struct {
	Membership protocol.Membership           `json:"membership"`
	Digest     history.Digest                `json:"digest"`
	Snapshots  []protocol.RetirementSnapshot `json:"snapshots,omitempty"`
}

type MembershipPreviewResult struct {
	Folder             history.ID     `json:"folder"`
	CurrentRevision    uint64         `json:"current_revision"`
	CurrentDigest      history.Digest `json:"current_digest"`
	NextRevision       uint64         `json:"next_revision"`
	NextDigest         history.Digest `json:"next_digest"`
	PriorDigestMatches bool           `json:"prior_digest_matches"`
	AddedActive        []history.ID   `json:"added_active,omitempty"`
	RemovedActive      []history.ID   `json:"removed_active,omitempty"`
	NewlyRetired       []history.ID   `json:"newly_retired,omitempty"`
	ValidTransition    bool           `json:"valid_transition"`
	Reason             string         `json:"reason,omitempty"`
}

type RetireMemberRequest struct {
	Folder         history.ID `json:"folder"`
	TargetDevice   history.ID `json:"target_device"`
	IdempotencyKey string     `json:"idempotency_key,omitempty"`
}

type RetireMemberPreview struct {
	Folder                history.ID     `json:"folder"`
	TargetDevice          history.ID     `json:"target_device"`
	CurrentRevision       uint64         `json:"current_revision"`
	NextRevision          uint64         `json:"next_revision"`
	AcceptedVersionsCount int            `json:"accepted_versions_count"`
	SnapshotDigest        history.Digest `json:"snapshot_digest"`
	NextMembershipDigest  history.Digest `json:"next_membership_digest"`
	SurvivingMembers      []history.ID   `json:"surviving_members"`
}

type RetireMemberResult struct {
	Folder           history.ID                  `json:"folder"`
	TargetDevice     history.ID                  `json:"target_device"`
	ApprovedRevision uint64                      `json:"approved_revision"`
	ApprovedDigest   history.Digest              `json:"approved_digest"`
	Snapshot         protocol.RetirementSnapshot `json:"snapshot"`
	NextMembership   protocol.Membership         `json:"next_membership"`
	Replay           bool                        `json:"replay,omitempty"`
}

type PeerListResult struct {
	Folder   history.ID               `json:"folder"`
	Revision uint64                   `json:"revision"`
	Digest   history.Digest           `json:"digest"`
	Active   []protocol.ActiveMember  `json:"active"`
	Retired  []protocol.RetiredMember `json:"retired"`
	Aliases  map[string]string        `json:"aliases,omitempty"`
}

type EnrollPreviewResult struct {
	Folder           history.ID `json:"folder"`
	RootPath         string     `json:"root_path"`
	LocalFilesCount  int        `json:"local_files_count"`
	ExistingPaths    []string   `json:"existing_paths"`
	ConflictingPaths []string   `json:"conflicting_paths"`
	IdenticalPaths   []string   `json:"identical_paths"`
	RemoteOnlyPaths  []string   `json:"remote_only_paths"`
}

type EnrollBootstrapResult struct {
	Folder        history.ID         `json:"folder"`
	RootPath      string             `json:"root_path"`
	CapturedCount int                `json:"captured_count"`
	Envelopes     []history.Envelope `json:"envelopes"`
}

type StorageUsageResult struct {
	Usage repository.DetailedStorageUsage `json:"usage"`
}

type RetentionPreviewRequest struct {
	Folder        history.ID `json:"folder"`
	RetentionDays *int       `json:"retention_days,omitempty"`
	MinSuperseded *int       `json:"min_superseded,omitempty"`
}

type RetentionPreviewResult struct {
	Preview repository.RetentionPreview `json:"preview"`
}

type RetentionChangeRequest struct {
	Folder        history.ID `json:"folder"`
	RetentionDays int        `json:"retention_days"`
	MinSuperseded int        `json:"min_superseded"`
}

type RetentionChangeResult struct {
	Folder history.ID                 `json:"folder"`
	Policy repository.RetentionPolicy `json:"policy"`
}

type GCPreviewRequest struct {
	Folder        history.ID `json:"folder"`
	RetentionDays *int       `json:"retention_days,omitempty"`
	MinSuperseded *int       `json:"min_superseded,omitempty"`
}

type GCPreviewResult struct {
	Report repository.GCReport `json:"report"`
}

type GCRunRequest struct {
	Folder         history.ID `json:"folder"`
	RetentionDays  *int       `json:"retention_days,omitempty"`
	MinSuperseded  *int       `json:"min_superseded,omitempty"`
	IdempotencyKey string     `json:"idempotency_key,omitempty"`
}

type GCRunResult struct {
	Report repository.GCReport `json:"report"`
	Replay bool                `json:"replay,omitempty"`
}

type ReclaimRecoveryRequest struct {
	Folder history.ID `json:"folder"`
}

type ReclaimRecoveryResult struct {
	Folder         history.ID `json:"folder"`
	ReclaimedCount int        `json:"reclaimed_count"`
	ReclaimedBytes uint64     `json:"reclaimed_bytes"`
}

type PruneRecordsRequest struct {
	CutoffSeconds int64 `json:"cutoff_seconds,omitempty"`
}

type PruneRecordsResult struct {
	Report repository.LifecyclePruneReport `json:"report"`
}

type StorageCheckRequest struct {
	Folder         history.ID         `json:"folder,omitempty"`
	Path           string             `json:"path,omitempty"`
	VersionID      *history.VersionID `json:"version_id,omitempty"`
	AutoQuarantine bool               `json:"auto_quarantine"`
	Limit          int                `json:"limit,omitempty"`
}

type StorageCheckResult struct {
	Folder             history.ID                           `json:"folder,omitempty"`
	TotalChunksChecked int                                  `json:"total_chunks_checked"`
	CleanChunks        int                                  `json:"clean_chunks"`
	CorruptChunks      []repository.CorruptChunkDetail      `json:"corrupt_chunks,omitempty"`
	MissingProtected   []repository.MissingProtectedDetail  `json:"missing_protected,omitempty"`
	ExpiredHistorical  []repository.ExpiredHistoricalDetail `json:"expired_historical,omitempty"`
	DurationNS         int64                                `json:"duration_ns"`
}

type StorageRepairRequest struct {
	Folder         history.ID               `json:"folder"`
	VersionID      history.VersionID        `json:"version_id"`
	PreferredPeer  *history.ID              `json:"preferred_peer,omitempty"`
	Peers          []replication.RepairPeer `json:"-"`
	IdempotencyKey string                   `json:"idempotency_key,omitempty"`
}

type StorageRepairResult struct {
	VersionID      history.VersionID `json:"version_id"`
	Path           string            `json:"path"`
	Status         string            `json:"status"` // "ready", "repaired", "already_ready", "unavailable", "peer_unavailable"
	RepairedChunks int               `json:"repaired_chunks"`
	TotalChunks    int               `json:"total_chunks"`
	Message        string            `json:"message,omitempty"`
	Replay         bool              `json:"replay,omitempty"`
}

type WorkScanRequest struct {
	Folder         *history.ID `json:"folder,omitempty"`
	FullContent    bool        `json:"full_content"`
	IdempotencyKey string      `json:"idempotency_key,omitempty"`
}

type WorkScanResult struct {
	Folder            history.ID                 `json:"folder"`
	FullContent       bool                       `json:"full_content"`
	CapturedCount     int                        `json:"captured_count"`
	CapturedEnvelopes []history.Envelope         `json:"captured_envelopes,omitempty"`
	Issues            []workspace.ScanIssue      `json:"issues,omitempty"`
	Deletion          *workspace.DeletionPreview `json:"deletion,omitempty"`
	DurationNS        int64                      `json:"duration_ns"`
	Replay            bool                       `json:"replay,omitempty"`
}

type WorkSyncRequest struct {
	Folder         history.ID  `json:"folder"`
	PeerDevice     *history.ID `json:"peer_device,omitempty"`
	IdempotencyKey string      `json:"idempotency_key,omitempty"`
}

type WorkSyncResult struct {
	Folder          history.ID `json:"folder"`
	PeerDevice      history.ID `json:"peer_device"`
	Inventoried     int        `json:"inventoried"`
	MetadataAdded   int        `json:"metadata_added"`
	ChunksFetched   int        `json:"chunks_fetched"`
	ChunksReused    int        `json:"chunks_reused"`
	VersionsStored  int        `json:"versions_stored"`
	ReceiptsSent    int        `json:"receipts_sent"`
	VersionsApplied int        `json:"versions_applied"`
	Replay          bool       `json:"replay,omitempty"`
}

type WorkStatusRequest struct {
	Folder *history.ID `json:"folder,omitempty"`
}

type FolderWorkStatus struct {
	Folder         history.ID `json:"folder"`
	Paused         bool       `json:"paused"`
	PauseReason    string     `json:"pause_reason,omitempty"`
	QueuedTasks    int        `json:"queued_tasks"`
	RunningTasks   int        `json:"running_tasks"`
	RetryTasks     int        `json:"retry_tasks"`
	ExhaustedTasks int        `json:"exhausted_tasks"`
}

type WorkStatusResult struct {
	Folders []FolderWorkStatus `json:"folders"`
}

type WorkRetryRequest struct {
	Folder history.ID `json:"folder,omitempty"`
	TaskID string     `json:"task_id,omitempty"`
	All    bool       `json:"all"`
}

type WorkRetryResult struct {
	RetriedCount int    `json:"retried_count"`
	Message      string `json:"message"`
}

type WorkCancelRequest struct {
	TaskID string `json:"task_id"`
}

type WorkCancelResult struct {
	TaskID  string `json:"task_id"`
	Status  string `json:"status"` // "canceled"
	Message string `json:"message"`
}

type WorkListRequest struct {
	Folder history.ID `json:"folder,omitempty"`
	State  string     `json:"state,omitempty"`
	Limit  int        `json:"limit,omitempty"`
}

type WorkListResult struct {
	Tasks []repository.DurableTask `json:"tasks"`
}

type FolderAddRequest struct {
	Folder history.ID `json:"folder"`
	Root   string     `json:"root"`
}

type FolderPauseRequest struct {
	Folder history.ID `json:"folder"`
	Reason string     `json:"reason,omitempty"`
}

type FolderResumeRequest struct {
	Folder history.ID `json:"folder"`
}

type FolderRemoveRequest struct {
	Folder history.ID `json:"folder"`
}

type FolderRevalidateRequest struct {
	Folder history.ID `json:"folder"`
}

type BackupRequest struct {
	Target string `json:"target"`
}

type BackupResult struct {
	BackupPath string `json:"backup_path"`
	SizeBytes  int64  `json:"size_bytes"`
	CreatedAt  string `json:"created_at"`
}

type MigrationCheckResult struct {
	CurrentDatabaseSchema int    `json:"current_database_schema"`
	BinarySchema          int    `json:"binary_schema"`
	Status                string `json:"status"`
	Action                string `json:"action"`
}

type RecoveryInspectionResult struct {
	PendingPublications int      `json:"pending_publications"`
	QuarantinedChunks   int      `json:"quarantined_chunks"`
	ExhaustedTasks      int      `json:"exhausted_tasks"`
	ReclaimableRecovery int      `json:"reclaimable_recovery"`
	Consistent          bool     `json:"consistent"`
	ConsistencyError    string   `json:"consistency_error,omitempty"`
	Details             []string `json:"details,omitempty"`
}

type ResetIdentityResult struct {
	OldDeviceID history.ID `json:"old_device_id"`
	NewDeviceID history.ID `json:"new_device_id"`
	NewKeyPin   [32]byte   `json:"new_key_pin"`
	Message     string     `json:"message"`
	Action      string     `json:"action"`
}

type PreflightResult struct {
	Status         string   `json:"status"` // "ready", "warning", "blocked"
	StateDir       string   `json:"state_dir"`
	AgentRunning   bool     `json:"agent_running"`
	DatabaseSchema int      `json:"database_schema"`
	BinarySchema   int      `json:"binary_schema"`
	IntegrityClean bool     `json:"integrity_clean"`
	FreeSpaceBytes uint64   `json:"free_space_bytes"`
	Issues         []string `json:"issues,omitempty"`
	NextSteps      []string `json:"next_steps,omitempty"`
}

type RestoreBackupRequest struct {
	BackupPath string `json:"backup_path"`
}

type RestoreBackupResult struct {
	Status      string     `json:"status"`
	BackupPath  string     `json:"backup_path"`
	OldDeviceID history.ID `json:"old_device_id"`
	NewDeviceID history.ID `json:"new_device_id"`
	NewKeyPin   [32]byte   `json:"new_key_pin"`
	Message     string     `json:"message"`
	Action      string     `json:"action"`
}

type OperationalMetrics struct {
	CapturedFiles             uint64            `json:"captured_files"`
	CapturedBytes             uint64            `json:"captured_bytes"`
	HashDurationNS            int64             `json:"hash_duration_ns"`
	ScanDurationNS            int64             `json:"scan_duration_ns"`
	MetadataBytesSent         uint64            `json:"metadata_bytes_sent"`
	MetadataBytesReceived     uint64            `json:"metadata_bytes_received"`
	ContentBytesSent          uint64            `json:"content_bytes_sent"`
	ContentBytesReceived      uint64            `json:"content_bytes_received"`
	VerifiedChunksReused      uint64            `json:"verified_chunks_reused"`
	VerifiedChunksFetched     uint64            `json:"verified_chunks_fetched"`
	QueueDepth                int               `json:"queue_depth"`
	MaxQueueAgeNS             int64             `json:"max_queue_age_ns"`
	RetryCauses               map[string]uint64 `json:"retry_causes"`
	ActiveConflicts           int               `json:"active_conflicts"`
	StagingBytes              uint64            `json:"staging_bytes"`
	QuarantineBytes           uint64            `json:"quarantine_bytes"`
	ObjectBytes               uint64            `json:"object_bytes"`
	MetadataBytes             uint64            `json:"metadata_bytes"`
	PublicationRecoveryTimeNS int64             `json:"publication_recovery_time_ns"`
	HighWaterMarkBytes        uint64            `json:"high_water_mark_bytes"`
}

type BootstrapRequest struct {
	Token string `json:"token"`
}

type BootstrapResult struct {
	SessionID string `json:"session_id"`
	CSRFToken string `json:"csrf_token"`
	ExpiresAt string `json:"expires_at"`
}

// DecimalString converts a 64-bit integer into a canonical decimal string,
// preventing precision loss in JavaScript numbers exceeding 2^53 - 1.
func DecimalString(v uint64) string {
	return strconv.FormatUint(v, 10)
}

// --- Orbit Product Settings Contracts ---

type GetSettingsResult struct {
	Settings config.ProductSettings `json:"settings"`
}

type UpdateSettingsRequest struct {
	DeviceLabel      *string           `json:"device_label,omitempty"`
	DefaultWorkspace *string           `json:"default_workspace,omitempty"`
	WorkspaceNames   map[string]string `json:"workspace_names,omitempty"`
	Theme            *string           `json:"theme,omitempty"`
	DarkTheme        *bool             `json:"dark_theme,omitempty"`
}

type UpdateSettingsResult struct {
	Settings config.ProductSettings `json:"settings"`
	Message  string                 `json:"message"`
}

// --- Orbit Peer Endpoint Contracts ---

type PeerEndpointsListResult struct {
	Peers []config.PeerEndpoint `json:"peers"`
}

type SetPeerEndpointRequest struct {
	Folder      string `json:"folder"`
	Device      string `json:"device"`
	URL         string `json:"url"`
	Certificate string `json:"certificate"`
}

type RemovePeerEndpointRequest struct {
	Folder string `json:"folder"`
	Device string `json:"device"`
}

// --- Orbit Setup Contracts (O03) ---

type InspectSetupResult struct {
	Initialized     bool                   `json:"initialized"`
	DeviceID        string                 `json:"device_id,omitempty"`
	SuggestedRoot   string                 `json:"suggested_root"`
	CurrentPhase    string                 `json:"current_phase,omitempty"`
	SetupCompleted  bool                   `json:"setup_completed"`
	Settings        config.ProductSettings `json:"settings"`
	RegisteredCount int                    `json:"registered_count"`
}

type PreviewCreateRootRequest struct {
	Path string `json:"path"`
}

type PreviewCreateRootResult struct {
	Path            string   `json:"path"`
	Exists          bool     `json:"exists"`
	IsEmpty         bool     `json:"is_empty"`
	PreexistingRows int      `json:"preexisting_rows"`
	Writable        bool     `json:"writable"`
	Disallowed      bool     `json:"disallowed"`
	Reason          string   `json:"reason,omitempty"`
	ExistingSamples []string `json:"existing_samples,omitempty"`
}

type PreviewJoinRootRequest struct {
	Path     string     `json:"path"`
	FolderID history.ID `json:"folder_id"`
}

type PreviewJoinRootResult struct {
	Path            string     `json:"path"`
	FolderID        history.ID `json:"folder_id"`
	Exists          bool       `json:"exists"`
	IsEmpty         bool       `json:"is_empty"`
	PreexistingRows int        `json:"preexisting_rows"`
	Writable        bool       `json:"writable"`
	Disallowed      bool       `json:"disallowed"`
	Reason          string     `json:"reason,omitempty"`
	ExistingSamples []string   `json:"existing_samples,omitempty"`
}

type StartSetupRequest struct {
	RootPath       string `json:"root_path"`
	DeviceLabel    string `json:"device_label"`
	WorkspaceName  string `json:"workspace_name"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type StartSetupResult struct {
	OperationID string     `json:"operation_id"`
	DeviceID    string     `json:"device_id"`
	FolderID    history.ID `json:"folder_id"`
	RootPath    string     `json:"root_path"`
	Phase       string     `json:"phase"`
	Message     string     `json:"message"`
}

type ResumeSetupRequest struct {
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type ResumeSetupResult struct {
	OperationID string     `json:"operation_id"`
	DeviceID    string     `json:"device_id"`
	FolderID    history.ID `json:"folder_id,omitempty"`
	RootPath    string     `json:"root_path,omitempty"`
	Phase       string     `json:"phase"`
	Completed   bool       `json:"completed"`
	Message     string     `json:"message"`
}

type SetupStatusResult struct {
	Phase     string `json:"phase"`
	Completed bool   `json:"completed"`
	RootPath  string `json:"root_path,omitempty"`
	FolderID  string `json:"folder_id,omitempty"`
}

// --- Orbit Directory Picker Contracts (O03) ---

type DirectoryEntry struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	IsDir        bool   `json:"is_dir"`
	Accessible   bool   `json:"accessible"`
	DeniedReason string `json:"denied_reason,omitempty"`
}

type DirectoryPickerRequest struct {
	Path   string `json:"path,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

type DirectoryPickerResult struct {
	CurrentPath  string           `json:"current_path"`
	ParentPath   string           `json:"parent_path,omitempty"`
	Writable     bool             `json:"writable"`
	Entries      []DirectoryEntry `json:"entries"`
	TotalEntries int              `json:"total_entries"`
	Offset       int              `json:"offset"`
	Limit        int              `json:"limit"`
	HasMore      bool             `json:"has_more"`
}

// --- Orbit Desktop Helper Contracts (O03) ---

type OpenFolderRequest struct {
	Folder string `json:"folder,omitempty"`
	Path   string `json:"path,omitempty"`
}

type OpenFolderResult struct {
	Status  string `json:"status"` // "opened"
	Path    string `json:"path"`
	Message string `json:"message,omitempty"`
}

// --- Orbit User Service Lifecycle Contracts (O03) ---

type ServiceStatusResult struct {
	SystemdAvailable     bool   `json:"systemd_available"`
	UnitInstalled        bool   `json:"unit_installed"`
	EnabledOnLogin       bool   `json:"enabled_on_login"`
	CurrentlyRunning     bool   `json:"currently_running"`
	RootVerified         bool   `json:"root_verified"`
	CaptureSuccessful    bool   `json:"capture_successful"`
	LingeringEnabled     bool   `json:"lingering_enabled"`
	LingeringInstruction string `json:"lingering_instruction,omitempty"`
	ManualCommand        string `json:"manual_command"`
	StatusDetail         string `json:"status_detail,omitempty"`
}

type ServiceActionRequest struct {
	Action string `json:"action"` // "enable", "start", "stop"
}

type ServiceActionResult struct {
	Action  string              `json:"action"`
	Success bool                `json:"success"`
	Status  ServiceStatusResult `json:"status"`
	Message string              `json:"message,omitempty"`
}

// --- Orbit Invitation Contracts (G02/O05) ---

type CreateInvitationRequest struct {
	Folder   history.ID `json:"folder"`
	TTLSecs  int64      `json:"ttl_secs,omitempty"` // default 86400 (24h)
	MaxUses  int        `json:"max_uses,omitempty"` // default 1
	Endpoint string     `json:"endpoint,omitempty"`
}

type CreateInvitationResult struct {
	Token          string         `json:"token"`
	Digest         history.Digest `json:"digest"`
	Folder         history.ID     `json:"folder"`
	ExpiresAt      string         `json:"expires_at"`
	MaxUses        int            `json:"max_uses"`
	InvitationCode string         `json:"invitation_code,omitempty"`
}

type RevokeInvitationRequest struct {
	Digest history.Digest `json:"digest"`
}

type RevokeInvitationResult struct {
	Digest  history.Digest `json:"digest"`
	Revoked bool           `json:"revoked"`
}

type ListInvitationsResult struct {
	Invitations []repository.InvitationRecord `json:"invitations"`
}

// --- Orbit Enrollment Contracts (G02/O05) ---

type SubmitJoinRequestPayload struct {
	Token          string     `json:"token"`
	JoiningDevice  history.ID `json:"joining_device"`
	PublicKey      string     `json:"public_key"` // hex encoded 32 bytes
	Signature      string     `json:"signature"`  // hex encoded Ed25519 signature
	Challenge      string     `json:"challenge"`  // hex encoded challenge nonce
	SuggestedLabel string     `json:"suggested_label"`
	TargetFolder   history.ID `json:"target_folder"`
	Endpoint       string     `json:"endpoint,omitempty"`
}

type SubmitJoinRequestResult struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"` // "pending"
	Message   string `json:"message"`
}

type ListEnrollmentRequestsResult struct {
	Requests []repository.EnrollmentRequestRecord `json:"requests"`
}

type ApproveEnrollmentRequest struct {
	RequestID      string     `json:"request_id"`
	Folder         history.ID `json:"folder"`
	Endpoint       string     `json:"endpoint,omitempty"`
	Certificate    string     `json:"certificate,omitempty"`
	SuggestedLabel string     `json:"suggested_label,omitempty"`
}

type ApproveEnrollmentResult struct {
	RequestID string         `json:"request_id"`
	Status    string         `json:"status"` // "approved"
	Message   string         `json:"message"`
	Revision  uint64         `json:"revision,omitempty"`
	Digest    history.Digest `json:"digest,omitempty"`
	Replay    bool           `json:"replay,omitempty"`
}

type DeclineEnrollmentRequest struct {
	RequestID string `json:"request_id"`
}

type EnrollmentStatusResult struct {
	RequestID      string               `json:"request_id"`
	Folder         history.ID           `json:"folder"`
	DeviceID       history.ID           `json:"device_id"`
	SuggestedLabel string               `json:"suggested_label"`
	Status         string               `json:"status"` // "pending", "approved", "declined"
	Revision       uint64               `json:"revision,omitempty"`
	Membership     *protocol.Membership `json:"membership,omitempty"`
	Message        string               `json:"message,omitempty"`
}

type JoinFlowSubmitRequest struct {
	InvitationToken string `json:"invitation_token"`
	TargetFolder    string `json:"target_folder"`
	RemoteEndpoint  string `json:"remote_endpoint"`
	DeviceLabel     string `json:"device_label"`
	RootPath        string `json:"root_path"`
}

type JoinFlowSubmitResult struct {
	RequestID      string `json:"request_id"`
	Status         string `json:"status"` // "pending"
	TargetFolder   string `json:"target_folder"`
	RemoteEndpoint string `json:"remote_endpoint"`
	RootPath       string `json:"root_path"`
	DeviceID       string `json:"device_id"`
	KeyPin         string `json:"key_pin"`
	Message        string `json:"message"`
}

type JoinFlowCompleteRequest struct {
	RequestID      string `json:"request_id"`
	RemoteEndpoint string `json:"remote_endpoint"`
	TargetFolder   string `json:"target_folder"`
	RootPath       string `json:"root_path"`
	DeviceLabel    string `json:"device_label,omitempty"`
}

type JoinFlowCompleteResult struct {
	Completed bool   `json:"completed"`
	FolderID  string `json:"folder_id"`
	RootPath  string `json:"root_path"`
	Status    string `json:"status"` // "ready"
	Message   string `json:"message"`
}

type PeerTestRequest struct {
	URL string `json:"url"`
}

type PeerTestResult struct {
	Reachable bool   `json:"reachable"`
	Status    string `json:"status"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

type RenameDeviceRequest struct {
	DeviceID history.ID `json:"device_id"`
	Alias    string     `json:"alias"`
}

type RenameDeviceResult struct {
	DeviceID history.ID `json:"device_id"`
	Alias    string     `json:"alias"`
	Status   string     `json:"status"`
}

type RetireDevicePreviewRequest struct {
	Folder   history.ID `json:"folder"`
	DeviceID history.ID `json:"device_id"`
}

type RetireDevicePreviewResult struct {
	Folder          history.ID `json:"folder"`
	DeviceID        history.ID `json:"device_id"`
	DeviceName      string     `json:"device_name"`
	CurrentRevision uint64     `json:"current_revision"`
	NextRevision    uint64     `json:"next_revision"`
	RemainingCount  int        `json:"remaining_count"`
	SurvivingPeers  []string   `json:"surviving_peers"`
	Warning         string     `json:"warning"`
	Disclaimer      string     `json:"disclaimer"`
}

type DetectForkRequest struct {
	Folder    history.ID          `json:"folder"`
	Candidate protocol.Membership `json:"candidate"`
}

type DetectForkResult struct {
	Folder  history.ID `json:"folder"`
	Forked  bool       `json:"forked"`
	Message string     `json:"message,omitempty"`
}

type ReconcileMembershipRequest struct {
	Folder history.ID   `json:"folder"`
	Active []history.ID `json:"active"`
}

type ReconcileMembershipResult struct {
	Folder           history.ID          `json:"folder"`
	ApprovedRevision uint64              `json:"approved_revision"`
	ApprovedDigest   history.Digest      `json:"approved_digest"`
	Membership       protocol.Membership `json:"membership"`
}

// --- Orbit Read Lease Contracts (G03/O07 - Invariant I25) ---

type AcquireReadLeaseRequest struct {
	Folder            history.ID       `json:"folder"`
	VersionAuthor     history.ID       `json:"version_author"`
	VersionCounter    uint64           `json:"version_counter"`
	VersionCounterStr string           `json:"version_counter_str,omitempty"`
	TTLSeconds        int              `json:"ttl_seconds,omitempty"` // default 300 (5m)
	ChunkDigests      []history.Digest `json:"chunk_digests,omitempty"`
}

type AcquireReadLeaseResult struct {
	LeaseID       string `json:"lease_id"`
	ExpiresAt     string `json:"expires_at"`
	ChunkCount    int    `json:"chunk_count"`
	TotalBytes    uint64 `json:"total_bytes"`
	TotalBytesStr string `json:"total_bytes_str"`
}

type ReleaseReadLeaseRequest struct {
	LeaseID string `json:"lease_id"`
}

// --- Orbit Operation Progress Contracts (O02/O09/O10) ---

type OperationProgressResult struct {
	Progress repository.OperationProgressRecord `json:"progress"`
}

type CancelOperationRequest struct {
	OperationID string `json:"operation_id"`
}

type CancelOperationResult struct {
	OperationID string `json:"operation_id"`
	Canceled    bool   `json:"canceled"`
	Message     string `json:"message"`
}

// --- Orbit File Mutation Contracts (O09) ---

type ImportFileRequest struct {
	Folder         history.ID `json:"folder"`
	Path           string     `json:"path"`
	Executable     bool       `json:"executable"`
	Overwrite      bool       `json:"overwrite"`
	ReviewedToken  string     `json:"reviewed_token"`
	OperationID    string     `json:"operation_id"`
	IdempotencyKey string     `json:"idempotency_key"`
}

type ImportFileResult struct {
	OperationID string            `json:"operation_id"`
	VersionID   history.VersionID `json:"version_id"`
	Path        string            `json:"path"`
	Size        uint64            `json:"size"`
	Digest      string            `json:"digest"`
	Completed   bool              `json:"completed"`
}

type CreateDirRequest struct {
	Folder         history.ID `json:"folder"`
	Path           string     `json:"path"`
	OperationID    string     `json:"operation_id"`
	IdempotencyKey string     `json:"idempotency_key"`
}

type CreateDirResult struct {
	OperationID   string            `json:"operation_id"`
	VersionID     history.VersionID `json:"version_id"`
	Path          string            `json:"path"`
	AlreadyExists bool              `json:"already_exists"`
	Completed     bool              `json:"completed"`
}

type MoveFileRequest struct {
	Folder         history.ID `json:"folder"`
	SourcePath     string     `json:"source_path"`
	DestPath       string     `json:"dest_path"`
	Overwrite      bool       `json:"overwrite"`
	ReviewedToken  string     `json:"reviewed_token"`
	OperationID    string     `json:"operation_id"`
	IdempotencyKey string     `json:"idempotency_key"`
}

type MoveFileResult struct {
	OperationID    string `json:"operation_id"`
	SourcePath     string `json:"source_path"`
	DestPath       string `json:"dest_path"`
	SourceRetained bool   `json:"source_retained"`
	Completed      bool   `json:"completed"`
}

type DeleteFileRequest struct {
	Folder         history.ID `json:"folder"`
	Path           string     `json:"path"`
	Recursive      bool       `json:"recursive"`
	ReviewedToken  string     `json:"reviewed_token"`
	OperationID    string     `json:"operation_id"`
	IdempotencyKey string     `json:"idempotency_key"`
}

type DeleteFileResult struct {
	OperationID    string   `json:"operation_id"`
	Path           string   `json:"path"`
	DeletedCount   int      `json:"deleted_count"`
	AlreadyDeleted bool     `json:"already_deleted"`
	Completed      bool     `json:"completed"`
	DeletedPaths   []string `json:"deleted_paths"`
}
