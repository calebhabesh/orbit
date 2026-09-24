package control

import (
	"io"

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
