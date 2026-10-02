export interface VersionID {
  folder?: string;
  author?: string;
  counter?: number;
  Folder?: string;
  Author?: string;
  Counter?: number;
}

export interface ClockEntry {
  author?: string;
  counter?: number;
  Author?: string;
  Counter?: number;
}

export interface ManifestChunk {
  digest?: string;
  length?: number;
  Digest?: string;
  Length?: number;
}

export interface Manifest {
  size?: number;
  digest?: string;
  chunks?: ManifestChunk[];
  executable?: boolean;
  Size?: number;
  Digest?: string;
  Chunks?: ManifestChunk[];
  Executable?: boolean;
}

export interface FolderRecord {
  folder: string;
  folder_id?: string;
  display_name?: string;
  local_author: string;
  next_counter: number;
  membership_revision: number;
  root_path?: string;
  root_device?: number;
  root_inode?: number;
  registration_id?: string;
  scan_generation: number;
  bootstrap_complete: boolean;
  paused: boolean;
  pause_reason?: string;
}

export interface ActiveMember {
  device: string;
  key_pin: string;
}

export interface RetiredMember {
  device: string;
  retired_at: number;
  snapshot_digest: string;
}

export interface PeerListResult {
  folder: string;
  revision: number;
  digest: string;
  active: ActiveMember[];
  retired: RetiredMember[];
  aliases?: Record<string, string>;
}

export interface PeerProgress {
  peer: string;
  version: VersionID;
  receipt: boolean;
  direct: boolean;
  remote_state: string;
  last_contact: string;
}

export interface DurableTask {
  id: string;
  task_id?: string;
  folder: string;
  folder_id?: string;
  kind?: string;
  operation?: string;
  phase?: string;
  peer?: string;
  target_path?: string;
  state: string;
  attempts?: number;
  retries?: number;
  max_attempts?: number;
  last_error?: string;
  error_code?: string;
  created_ns?: number;
  updated_ns?: number;
  file_size?: number;
  scheduled_at?: string;
  created_at?: string;
}

export interface FolderWorkStatus {
  folder: string;
  paused: boolean;
  pause_reason?: string;
  queued_tasks: number;
  running_tasks: number;
  retry_tasks: number;
  exhausted_tasks: number;
}

export interface WorkStatusResult {
  folders: FolderWorkStatus[];
}

export interface WorkListResult {
  tasks: DurableTask[];
}

export interface DoctorCheck {
  category: string;
  name: string;
  status: string;
  message: string;
  remediation?: string;
  action?: string;
}

export interface DoctorReport {
  overall_status: string;
  status?: string;
  checks: DoctorCheck[];
  generated_at?: string;
  timestamp?: string;
}

export interface FilesystemUsage {
  path: string;
  total_bytes: number;
  free_bytes: number;
  available_bytes: number;
}

export interface FolderStorageUsage {
  folder_id: string;
  root_path: string;
  working_root_bytes: number;
  stage_bytes: number;
  recovery_bytes: number;
  filesystem: FilesystemUsage;
}

export interface StorageUsage {
  object_bytes: number;
  staging_bytes: number;
  recovery_bytes?: number;
  quarantine_bytes: number;
  wal_bytes: number;
  metadata_bytes: number;
  total_working_root_bytes?: number;
  total_managed_bytes: number;
  metadata_budget_bytes?: number;
  data_budget_bytes?: number;
  free_space_reserve_bytes?: number;
  state_filesystem?: FilesystemUsage;
  folders?: FolderStorageUsage[];
  warnings?: string[];
}

export interface StorageUsageResult {
  usage: StorageUsage;
}

export interface RetentionPolicy {
  retention_days: number;
  min_superseded: number;
}

export interface RetentionPreview {
  folder_id: string;
  policy: RetentionPolicy;
  total_versions: number;
  head_versions: number;
  retained_versions: number;
  expired_versions: number;
  protected_chunks: number;
  candidate_chunks: number;
  reclaimable_bytes: number;
  suspended: boolean;
  suspend_reason?: string;
}

export interface RetentionPreviewResult {
  preview: RetentionPreview;
}

export interface RetentionChangeResult {
  folder: string;
  policy: RetentionPolicy;
}

export interface GCReport {
  folder_id: string;
  candidates: number;
  unlinked_objects: number;
  reclaimed_bytes: number;
  remaining_objects: number;
  remaining_bytes: number;
  suspended: boolean;
  suspend_reason?: string;
}

export interface GCPreviewResult {
  report: GCReport;
}

export interface ReclaimRecoveryResult {
  folder: string;
  reclaimed_count: number;
  reclaimed_bytes: number;
}

export interface LifecyclePruneReport {
  tasks_pruned: number;
  invitations_pruned: number;
  enrollment_requests_pruned: number;
  read_leases_pruned: number;
  operations_pruned: number;
  control_ops_pruned: number;
  total_pruned: number;
}

export interface PruneRecordsResult {
  report: LifecyclePruneReport;
}

export interface BackupResult {
  backup_path: string;
  size_bytes: number;
  created_at: string;
}

export interface RecoveryInspectionResult {
  pending_publications: number;
  quarantined_chunks: number;
  exhausted_tasks: number;
  reclaimable_recovery: number;
  consistent: boolean;
  consistency_error?: string;
  details?: string[];
}

export interface ConflictHead {
  id: VersionID;
  kind: number; // 1: file, 2: directory, 3: tombstone
  manifest?: Manifest;
  applied: boolean;
  content_state: 'ready' | 'pending' | 'unavailable' | 'expired';
  display_time?: string;
}

export interface ConflictSet {
  path: string;
  conflict_kind: string;
  heads: ConflictHead[];
  applied?: VersionID;
  head_token: string;
}

export interface StructuralConflict {
  ancestor_path: string;
  ancestor: VersionID;
  descendant_path: string;
  descendant: VersionID;
}

export interface ConflictsResult {
  content_conflicts: ConflictSet[];
  structural_conflicts: StructuralConflict[];
}

export interface FileItem {
  path: string;
  kind: number;
  size: number;
  mtime_ns: number;
  inode: number;
  executable: boolean;
  block_reason?: string;
}

export interface HistoryItem {
  id: VersionID;
  path: string;
  kind: number;
  parents: VersionID[];
  vector: ClockEntry[];
  manifest?: Manifest;
  display_time: string;
  content_state: 'ready' | 'pending' | 'unavailable' | 'expired';
  is_head: boolean;
  applied: boolean;
}

export interface RestorePreview {
  path: string;
  source_version: VersionID;
  source_kind: number;
  source_size: number;
  source_digest: string;
  source_executable: boolean;
  content_state: 'ready' | 'pending' | 'unavailable' | 'expired';
  current_heads: VersionID[];
  expected_head_token: string;
}

export interface ResolveResult {
  folder: string;
  path: string;
  action: string;
  resolved_id: VersionID;
  applied: boolean;
  replay?: boolean;
}

export interface KeepCopiesResult {
  folder: string;
  path: string;
  resolved_id: VersionID;
  completed: boolean;
  replay?: boolean;
}

export interface SessionInfo {
  authenticated: boolean;
  csrf_token?: string;
  expires_at?: string;
}

export interface ControlError {
  code: string;
  message: string;
  retryable: boolean;
  action: string;
}

// --- Orbit Types (O02 - O04) ---

export interface ProductSettings {
  device_label: string;
  default_workspace: string;
  workspace_names: Record<string, string>;
  theme: string;
  dark_theme: boolean;
}

export interface InspectSetupResult {
  initialized: boolean;
  device_id?: string;
  suggested_root: string;
  current_phase?: string;
  setup_completed: boolean;
  settings: ProductSettings;
  registered_count: number;
}

export interface PreviewCreateRootResult {
  path: string;
  exists: boolean;
  is_empty: boolean;
  preexisting_rows: number;
  writable: boolean;
  disallowed: boolean;
  reason?: string;
  existing_samples?: string[];
}

export interface PreviewJoinRootResult {
  path: string;
  folder_id: string;
  exists: boolean;
  is_empty: boolean;
  preexisting_rows: number;
  writable: boolean;
  disallowed: boolean;
  reason?: string;
  existing_samples?: string[];
}

export interface StartSetupRequest {
  root_path: string;
  device_label: string;
  workspace_name: string;
  idempotency_key?: string;
}

export interface StartSetupResult {
  operation_id: string;
  device_id: string;
  folder_id: string;
  root_path: string;
  phase: string;
  message: string;
}

export interface ResumeSetupResult {
  operation_id: string;
  device_id: string;
  folder_id?: string;
  root_path?: string;
  phase: string;
  completed: boolean;
  message: string;
}

export interface SetupStatusResult {
  phase: string;
  completed: boolean;
  root_path?: string;
  folder_id?: string;
}

export interface DirectoryEntry {
  name: string;
  path: string;
  is_dir: boolean;
  accessible: boolean;
  denied_reason?: string;
}

export interface DirectoryPickerResult {
  current_path: string;
  parent_path?: string;
  writable: boolean;
  entries: DirectoryEntry[];
  total_entries: number;
  offset: number;
  limit: number;
  has_more: boolean;
}

export interface OpenFolderResult {
  status: string; // "opened"
  path: string;
  message?: string;
}

export interface ServiceStatusResult {
  systemd_available: boolean;
  unit_installed: boolean;
  enabled_on_login: boolean;
  currently_running: boolean;
  root_verified: boolean;
  capture_successful: boolean;
  lingering_enabled: boolean;
  lingering_instruction?: string;
  manual_command: string;
  status_detail?: string;
}

export interface ServiceActionResult {
  action: string;
  success: boolean;
  status: ServiceStatusResult;
  message?: string;
}

export interface SupportExportPreview {
  redacted_paths: boolean;
  file_count: number;
  total_size_bytes: number;
  entries: string[];
}

// --- Orbit Pairing & Device Management Contracts (O06) ---

export interface InvitationRecord {
  digest: string;
  folder: string;
  created_ns: number;
  expires_ns: number;
  max_uses: number;
  uses_count: number;
  revoked: boolean;
}

export interface CreateInvitationResult {
  token: string;
  digest: string;
  folder: string;
  expires_at: string;
  max_uses: number;
  invitation_code?: string;
}

export interface EnrollmentRequestRecord {
  request_id: string;
  folder: string;
  device_id: string;
  public_key: string;
  key_pin: string;
  suggested_label: string;
  status: 'pending' | 'approved' | 'declined';
  created_ns: number;
  updated_ns: number;
}

export interface EnrollmentStatusResult {
  request_id: string;
  folder: string;
  device_id: string;
  suggested_label: string;
  status: 'pending' | 'approved' | 'declined';
  revision?: number;
  membership?: any;
  message?: string;
}

export interface JoinFlowSubmitResult {
  request_id: string;
  status: string;
  target_folder: string;
  remote_endpoint: string;
  root_path: string;
  device_id: string;
  key_pin: string;
  message: string;
}

export interface JoinFlowCompleteResult {
  completed: boolean;
  folder_id: string;
  root_path: string;
  status: string;
  message: string;
}

export interface PeerTestResult {
  reachable: boolean;
  status: string;
  latency_ms?: number;
  error?: string;
}

export interface RenameDeviceResult {
  device_id: string;
  alias: string;
  status: string;
}

export interface RetireDevicePreviewResult {
  folder: string;
  device_id: string;
  device_name: string;
  current_revision: number;
  next_revision: number;
  remaining_count: number;
  surviving_peers: string[];
  warning: string;
  disclaimer: string;
}

export interface PeerEndpointConfig {
  folder: string;
  device: string;
  url: string;
  certificate?: string;
}

export interface PeerEndpointsListResult {
  endpoints: PeerEndpointConfig[];
}

// --- Orbit Browse, Previews & Details Contracts (O07 - O08) ---

export interface BrowseItem {
  working_state: string; // "observed", "unobserved", "pending", "conflict", "blocked"
  path: string;
  name: string;
  is_dir: boolean;
  kind: number; // 1: file, 2: directory, 3: tombstone
  size: number;
  mtime_ns: number;
  inode: number;
  executable: boolean;
  block_reason?: string;
  content_state?: 'ready' | 'pending' | 'unavailable' | 'unknown';
  has_conflict?: boolean;
  structural_conflict?: string;
}

export interface BrowseResult {
  dir_path: string;
  parent_path: string;
  items: BrowseItem[];
  next_cursor?: string;
  total_items: number;
  snapshot_generation: number;
}

export interface SearchResult {
  query: string;
  items: BrowseItem[];
  total_found: number;
  has_more: boolean;
  next_cursor?: string;
  snapshot_generation: number;
}

export interface FileVersionDetail {
  author_id: string;
  author_name?: string;
  counter: number;
  counter_str: string;
  file_digest?: string;
  file_size: number;
  display_time: string;
  content_state: string;
  cas_available: boolean;
}

export interface PeerProgressSummary {
  version_author: string;
  version_counter: number;
  peer_id: string;
  peer_name?: string;
  receipt: boolean;
  remote_status?: string;
  last_contact_ns: number;
}

export interface FileDetails {
  path: string;
  name: string;
  is_dir: boolean;
  kind: number;
  size: number;
  mtime_ns: number;
  inode: number;
  executable: boolean;
  block_reason?: string;
  working_state: string;
  content_state: string;
  heads: FileVersionDetail[];
  has_conflict: boolean;
  structural_conflict?: string;
  peers?: PeerProgressSummary[];
}

export interface FileHistoryItem {
  author_id: string;
  author_name?: string;
  counter: number;
  counter_str: string;
  kind: number;
  display_time: string;
  file_size: number;
  file_digest?: string;
  content_state: string;
  cas_available: boolean;
  is_head: boolean;
}

export interface FileHistoryResult {
  items: FileHistoryItem[];
  next_cursor?: string;
  total_items: number;
  snapshot_generation: number;
}

export interface DeletedFileItem {
  path: string;
  name: string;
  deleted_at: string;
  deleted_by: string;
  deleted_counter: number;
  last_active_size: number;
  cas_available: boolean;
}

export interface DeletedFilesResult {
  items: DeletedFileItem[];
  next_cursor?: string;
  total_items: number;
}

// --- Orbit File Mutation Contracts (O09 - O10) ---

export interface ImportFileResult {
  operation_id: string;
  version_id: VersionID;
  path: string;
  size: number;
  digest: string;
  completed: boolean;
}

export interface CreateDirResult {
  operation_id: string;
  version_id: VersionID;
  path: string;
  already_exists: boolean;
  completed: boolean;
}

export interface MoveFileResult {
  operation_id: string;
  source_path: string;
  dest_path: string;
  source_retained: boolean;
  completed: boolean;
}

export interface DeleteFileResult {
  operation_id: string;
  path: string;
  deleted_count: number;
  already_deleted: boolean;
  completed: boolean;
  deleted_paths: string[];
}
