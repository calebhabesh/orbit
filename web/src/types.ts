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

export interface StorageUsage {
  object_bytes: number;
  staging_bytes: number;
  quarantine_bytes: number;
  wal_bytes: number;
  metadata_bytes: number;
  total_managed_bytes: number;
}

export interface StorageUsageResult {
  usage: StorageUsage;
}

export interface ConflictHead {
  id: VersionID;
  kind: number; // 1: file, 2: directory, 3: tombstone
  manifest?: Manifest;
  applied: boolean;
  content_state: 'ready' | 'pending' | 'unavailable' | 'expired';
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
