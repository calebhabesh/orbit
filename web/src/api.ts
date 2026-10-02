import {
  FolderRecord,
  PeerListResult,
  WorkStatusResult,
  WorkListResult,
  DoctorReport,
  StorageUsageResult,
  ConflictsResult,
  FileItem,
  HistoryItem,
  RestorePreview,
  ResolveResult,
  KeepCopiesResult,
  SessionInfo,
  ControlError,
  RetentionPreviewResult,
  RetentionChangeResult,
  GCPreviewResult,
  ReclaimRecoveryResult,
  PruneRecordsResult,
  BackupResult,
  RecoveryInspectionResult,
  VersionID,
  ProductSettings,
  InspectSetupResult,
  PreviewCreateRootResult,
  PreviewJoinRootResult,
  StartSetupRequest,
  StartSetupResult,
  ResumeSetupResult,
  SetupStatusResult,
  DirectoryPickerResult,
  OpenFolderResult,
  ServiceStatusResult,
  ServiceActionResult,
  SupportExportPreview,
  InvitationRecord,
  CreateInvitationResult,
  EnrollmentRequestRecord,
  EnrollmentStatusResult,
  JoinFlowSubmitResult,
  JoinFlowCompleteResult,
  PeerTestResult,
  RenameDeviceResult,
  RetireDevicePreviewResult,
  PeerEndpointConfig,
  BrowseResult,
  SearchResult,
  FileDetails,
  FileHistoryResult,
  DeletedFilesResult,
  ImportFileResult,
  CreateDirResult,
  MoveFileResult,
  DeleteFileResult,
} from './types';

let currentCSRFToken = '';

export function setCSRFToken(token: string) {
  currentCSRFToken = token;
}

export function getCSRFToken(): string {
  return currentCSRFToken;
}

export class APIError extends Error {
  code: string;
  retryable: boolean;
  action: string;
  status: number;

  constructor(status: number, err: ControlError) {
    super(err.message || 'API request failed');
    this.name = 'APIError';
    this.status = status;
    this.code = err.code || 'UNKNOWN';
    this.retryable = !!err.retryable;
    this.action = err.action || '';
  }
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
    ...(options.headers as Record<string, string>),
  };

  const method = options.method?.toUpperCase() || 'GET';
  if (['POST', 'PUT', 'DELETE', 'PATCH'].includes(method)) {
    if (!headers['Content-Type'] && !(options.body instanceof FormData)) {
      headers['Content-Type'] = 'application/json';
    }
    if (currentCSRFToken) {
      headers['X-CSRF-Token'] = currentCSRFToken;
    }
  }

  const response = await fetch(path, {
    ...options,
    headers,
    credentials: 'same-origin',
  });

  if (!response.ok) {
    let errBody: ControlError;
    try {
      errBody = await response.json();
    } catch {
      errBody = {
        code: 'HTTP_' + response.status,
        message: response.statusText || 'Request failed',
        retryable: response.status >= 500,
        action: 'Retry operation or check agent logs',
      };
    }
    throw new APIError(response.status, errBody);
  }

  return response.json();
}

export const api = {
  // Session & Auth
  async getSession(): Promise<SessionInfo> {
    const res = await request<SessionInfo>('/api/v1/auth/session');
    if (res.csrf_token) {
      setCSRFToken(res.csrf_token);
    }
    return res;
  },

  async bootstrap(token: string): Promise<{ session_id: string; csrf_token: string; expires_at: string }> {
    const res = await request<{ session_id: string; csrf_token: string; expires_at: string }>('/api/v1/auth/bootstrap', {
      method: 'POST',
      body: JSON.stringify({ token: token.trim() }),
    });
    if (res.csrf_token) {
      setCSRFToken(res.csrf_token);
    }
    return res;
  },

  async logout(): Promise<{ status: string }> {
    const res = await request<{ status: string }>('/api/v1/auth/logout', { method: 'POST' });
    setCSRFToken('');
    return res;
  },

  // Doctor & System
  async getDoctor(): Promise<DoctorReport> {
    return request<DoctorReport>('/api/v1/doctor');
  },

  async getStorageUsage(): Promise<StorageUsageResult> {
    return request<StorageUsageResult>('/api/v1/storage/usage');
  },

  // Folders
  async getFolders(): Promise<FolderRecord[]> {
    return request<FolderRecord[]>('/api/v1/folders');
  },

  async addFolder(folder: string, root: string): Promise<any> {
    return request('/api/v1/folders', {
      method: 'POST',
      body: JSON.stringify({ folder: folder.trim(), root: root.trim() }),
    });
  },

  async pauseFolder(folder: string, reason?: string): Promise<{ status: string }> {
    return request('/api/v1/folders/pause', {
      method: 'POST',
      body: JSON.stringify({ folder, reason: reason || 'OPERATOR_PAUSED' }),
    });
  },

  async resumeFolder(folder: string): Promise<{ status: string }> {
    return request('/api/v1/folders/resume', {
      method: 'POST',
      body: JSON.stringify({ folder }),
    });
  },

  async revalidateFolder(folder: string): Promise<{ status: string }> {
    return request('/api/v1/folders/revalidate', {
      method: 'POST',
      body: JSON.stringify({ folder }),
    });
  },

  async removeFolder(folder: string): Promise<{ status: string; message: string }> {
    return request('/api/v1/folders/remove', {
      method: 'POST',
      body: JSON.stringify({ folder }),
    });
  },

  // Peers & Membership
  async getPeers(folder: string): Promise<PeerListResult> {
    return request<PeerListResult>(`/api/v1/peers?folder=${encodeURIComponent(folder)}`);
  },

  async retireMember(folder: string, targetDevice: string): Promise<any> {
    return request('/api/v1/peers/retire', {
      method: 'POST',
      body: JSON.stringify({
        folder,
        target_device: targetDevice,
        idempotency_key: `retire-${targetDevice}-${Date.now()}`,
      }),
    });
  },


  // Work & Tasks
  async getWorkStatus(folder?: string): Promise<WorkStatusResult> {
    const q = folder ? `?folder=${encodeURIComponent(folder)}` : '';
    return request<WorkStatusResult>(`/api/v1/work/status${q}`);
  },

  async getWorkList(folder?: string, state?: string): Promise<WorkListResult> {
    const params = new URLSearchParams();
    if (folder) params.set('folder', folder);
    if (state) params.set('state', state);
    params.set('limit', '50');
    return request<WorkListResult>(`/api/v1/work/list?${params.toString()}`);
  },

  async retryWork(folder?: string, taskId?: string, all?: boolean): Promise<{ retried_count: number; message: string }> {
    return request('/api/v1/work/retry', {
      method: 'POST',
      body: JSON.stringify({ folder, task_id: taskId, all: !!all }),
    });
  },

  async cancelWork(taskId: string): Promise<{ task_id: string; status: string; message: string }> {
    return request('/api/v1/work/cancel', {
      method: 'POST',
      body: JSON.stringify({ task_id: taskId }),
    });
  },

  // Files & History
  async getFiles(folder: string): Promise<FileItem[]> {
    return request<FileItem[]>(`/api/v1/files?folder=${encodeURIComponent(folder)}`);
  },

  async getDeletedFiles(folder: string): Promise<FileItem[]> {
    return request<FileItem[]>(`/api/v1/files/deleted?folder=${encodeURIComponent(folder)}`);
  },


  async getFileHistory(folder: string, path: string): Promise<HistoryItem[]> {
    return request<HistoryItem[]>(
      `/api/v1/files/history?folder=${encodeURIComponent(folder)}&path=${encodeURIComponent(path)}`
    );
  },

  // Restore
  async previewRestore(folder: string, path: string, sourceVersion: VersionID): Promise<RestorePreview> {
    return request<RestorePreview>('/api/v1/restore/preview', {
      method: 'POST',
      body: JSON.stringify({ folder, path, source_version: sourceVersion }),
    });
  },

  async restore(
    folder: string,
    path: string,
    sourceVersion: VersionID,
    reviewed: VersionID[],
    expectedHeadToken: string
  ): Promise<ResolveResult> {
    return request<ResolveResult>('/api/v1/restore', {
      method: 'POST',
      body: JSON.stringify({
        folder,
        path,
        source_version: sourceVersion,
        reviewed,
        expected_head_token: expectedHeadToken,
        idempotency_key: `restore-${path}-${Date.now()}`,
      }),
    });
  },

  // Conflicts
  async getConflicts(folder: string): Promise<ConflictsResult> {
    return request<ConflictsResult>(`/api/v1/conflicts?folder=${encodeURIComponent(folder)}`);
  },

  async resolveSelect(
    folder: string,
    path: string,
    reviewed: VersionID[],
    expectedHeadToken: string,
    selected: VersionID
  ): Promise<ResolveResult> {
    return request<ResolveResult>('/api/v1/conflicts/select', {
      method: 'POST',
      body: JSON.stringify({
        folder,
        path,
        reviewed,
        expected_head_token: expectedHeadToken,
        selected,
        idempotency_key: `select-${path}-${Date.now()}`,
      }),
    });
  },

  async resolveKeepCopies(
    folder: string,
    path: string,
    reviewed: VersionID[],
    expectedHeadToken: string
  ): Promise<KeepCopiesResult> {
    return request<KeepCopiesResult>('/api/v1/conflicts/keep-copies', {
      method: 'POST',
      body: JSON.stringify({
        folder,
        path,
        reviewed,
        expected_head_token: expectedHeadToken,
        copies: [], // Engine auto-names copies
        idempotency_key: `keep-${path}-${Date.now()}`,
      }),
    });
  },

  async resolveManualMerge(
    folder: string,
    path: string,
    reviewed: VersionID[],
    expectedHeadToken: string,
    executable: boolean,
    content: string
  ): Promise<ResolveResult> {
    return request<ResolveResult>('/api/v1/conflicts/merge', {
      method: 'POST',
      body: JSON.stringify({
        folder,
        path,
        reviewed,
        expected_head_token: expectedHeadToken,
        executable,
        content,
        idempotency_key: `merge-${path}-${Date.now()}`,
      }),
    });
  },

  async runGC(folder: string): Promise<any> {
    return request('/api/v1/storage/gc/run', {
      method: 'POST',
      body: JSON.stringify({
        folder,
        idempotency_key: `gc-${folder}-${Date.now()}`,
      }),
    });
  },

  async getRetentionPreview(folder: string, days?: number, minSuperseded?: number): Promise<RetentionPreviewResult> {
    const payload: any = { folder };
    if (typeof days === 'number') payload.retention_days = days;
    if (typeof minSuperseded === 'number') payload.min_superseded = minSuperseded;
    return request<RetentionPreviewResult>('/api/v1/storage/retention/preview', {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  },

  async changeRetention(folder: string, days: number, minSuperseded: number): Promise<RetentionChangeResult> {
    return request<RetentionChangeResult>('/api/v1/storage/retention/change', {
      method: 'POST',
      body: JSON.stringify({
        folder,
        retention_days: days,
        min_superseded: minSuperseded,
      }),
    });
  },

  async getGCPreview(folder: string, days?: number, minSuperseded?: number): Promise<GCPreviewResult> {
    const payload: any = { folder };
    if (typeof days === 'number') payload.retention_days = days;
    if (typeof minSuperseded === 'number') payload.min_superseded = minSuperseded;
    return request<GCPreviewResult>('/api/v1/storage/gc/preview', {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  },

  async reclaimRecovery(folder: string): Promise<ReclaimRecoveryResult> {
    return request<ReclaimRecoveryResult>('/api/v1/storage/recovery/reclaim', {
      method: 'POST',
      body: JSON.stringify({ folder }),
    });
  },

  async pruneLifecycleRecords(cutoffSeconds = 86400): Promise<PruneRecordsResult> {
    return request<PruneRecordsResult>('/api/v1/maintenance/prune', {
      method: 'POST',
      body: JSON.stringify({ cutoff_seconds: cutoffSeconds }),
    });
  },

  async createBackup(targetPath?: string): Promise<BackupResult> {
    return request<BackupResult>('/api/v1/maintenance/backup', {
      method: 'POST',
      body: JSON.stringify({ target: targetPath || '' }),
    });
  },

  async getRecoveryInspection(): Promise<RecoveryInspectionResult> {
    return request<RecoveryInspectionResult>('/api/v1/maintenance/recovery');
  },

  async checkStorageIntegrity(folder?: string, autoQuarantine = false): Promise<any> {
    return request('/api/v1/storage/check', {
      method: 'POST',
      body: JSON.stringify({ folder: folder || undefined, auto_quarantine: autoQuarantine }),
    });
  },

  // --- Orbit Setup & Service Operations (O03 / O04) ---

  async inspectSetup(): Promise<InspectSetupResult> {
    return request<InspectSetupResult>('/api/v1/setup/inspect');
  },

  async previewCreateRoot(path: string): Promise<PreviewCreateRootResult> {
    return request<PreviewCreateRootResult>('/api/v1/setup/preview-root', {
      method: 'POST',
      body: JSON.stringify({ path: path.trim() }),
    });
  },

  async previewJoinRoot(path: string, folderId: string): Promise<PreviewJoinRootResult> {
    return request<PreviewJoinRootResult>('/api/v1/setup/preview-join-root', {
      method: 'POST',
      body: JSON.stringify({ path: path.trim(), folder_id: folderId.trim() }),
    });
  },

  async startSetup(req: StartSetupRequest): Promise<StartSetupResult> {
    return request<StartSetupResult>('/api/v1/setup/start', {
      method: 'POST',
      body: JSON.stringify({
        root_path: req.root_path.trim(),
        device_label: req.device_label.trim(),
        workspace_name: req.workspace_name.trim(),
        idempotency_key: req.idempotency_key || `setup-${Date.now()}`,
      }),
    });
  },

  async resumeSetup(): Promise<ResumeSetupResult> {
    return request<ResumeSetupResult>('/api/v1/setup/resume', {
      method: 'POST',
      body: JSON.stringify({
        idempotency_key: `resume-${Date.now()}`,
      }),
    });
  },

  async getSetupStatus(): Promise<SetupStatusResult> {
    return request<SetupStatusResult>('/api/v1/setup/status');
  },

  async browseDirectories(path?: string, limit = 50, offset = 0): Promise<DirectoryPickerResult> {
    const params = new URLSearchParams();
    if (path) params.set('path', path);
    params.set('limit', String(limit));
    params.set('offset', String(offset));
    return request<DirectoryPickerResult>(`/api/v1/system/directories?${params.toString()}`);
  },

  async openLocalFolder(folder?: string, path?: string): Promise<OpenFolderResult> {
    return request<OpenFolderResult>('/api/v1/system/open-folder', {
      method: 'POST',
      body: JSON.stringify({ folder: folder || '', path: path || '' }),
    });
  },

  async getServiceStatus(): Promise<ServiceStatusResult> {
    return request<ServiceStatusResult>('/api/v1/system/service/status');
  },

  async serviceAction(action: 'enable' | 'start' | 'stop' | 'restart'): Promise<ServiceActionResult> {
    return request<ServiceActionResult>('/api/v1/system/service/action', {
      method: 'POST',
      body: JSON.stringify({ action }),
    });
  },

  async getSettings(): Promise<ProductSettings> {
    return request<ProductSettings>('/api/v1/settings');
  },

  async updateSettings(req: Partial<ProductSettings>): Promise<{ settings: ProductSettings; message: string }> {
    return request<{ settings: ProductSettings; message: string }>('/api/v1/settings', {
      method: 'POST',
      body: JSON.stringify(req),
    });
  },

  async getRecoveryStatus(): Promise<any> {
    return request<any>('/api/v1/recovery/status');
  },

  async previewSupport(unredacted = false): Promise<SupportExportPreview> {
    const q = unredacted ? '?unredacted=true' : '';
    return request<SupportExportPreview>(`/api/v1/support/preview${q}`);
  },

  async exportSupport(unredacted = false): Promise<any> {
    const q = unredacted ? '?unredacted=true' : '';
    return request<any>(`/api/v1/support/export${q}`, {
      method: 'POST',
      body: JSON.stringify({ redact_paths: !unredacted }),
    });
  },

  // --- Orbit Pairing & Device Management (O06) ---

  async createInvitation(folder: string, ttlSecs = 86400, maxUses = 1, endpoint?: string): Promise<CreateInvitationResult> {
    return request<CreateInvitationResult>('/api/v1/invitations', {
      method: 'POST',
      body: JSON.stringify({
        folder,
        ttl_secs: ttlSecs,
        max_uses: maxUses,
        endpoint,
      }),
    });
  },

  async listInvitations(folder: string): Promise<{ invitations: InvitationRecord[] }> {
    return request<{ invitations: InvitationRecord[] }>(`/api/v1/invitations?folder=${encodeURIComponent(folder)}`);
  },

  async revokeInvitation(digest: string): Promise<{ digest: string; revoked: boolean }> {
    return request<{ digest: string; revoked: boolean }>('/api/v1/invitations/revoke', {
      method: 'POST',
      body: JSON.stringify({ digest }),
    });
  },

  async listEnrollmentRequests(folder?: string, status = 'pending'): Promise<{ requests: EnrollmentRequestRecord[] }> {
    const params = new URLSearchParams();
    if (folder) params.set('folder', folder);
    if (status) params.set('status', status);
    return request<{ requests: EnrollmentRequestRecord[] }>(`/api/v1/enrollment/requests?${params.toString()}`);
  },

  async approveEnrollment(requestId: string, folder: string, suggestedLabel?: string, endpoint?: string, certificate?: string): Promise<any> {
    return request<any>('/api/v1/enrollment/approve', {
      method: 'POST',
      body: JSON.stringify({
        request_id: requestId,
        folder,
        suggested_label: suggestedLabel,
        endpoint,
        certificate,
      }),
    });
  },

  async declineEnrollment(requestId: string): Promise<any> {
    return request<any>('/api/v1/enrollment/decline', {
      method: 'POST',
      body: JSON.stringify({ request_id: requestId }),
    });
  },

  async getEnrollmentStatus(requestId: string, remoteEndpoint?: string): Promise<EnrollmentStatusResult> {
    if (remoteEndpoint) {
      const url = `/api/v1/orbit/setup/join/status?request_id=${encodeURIComponent(requestId)}&remote_endpoint=${encodeURIComponent(remoteEndpoint)}`;
      return request<EnrollmentStatusResult>(url);
    }
    const url = `/api/v1/enrollment/status?request_id=${encodeURIComponent(requestId)}`;
    return request<EnrollmentStatusResult>(url);
  },

  async submitJoinFlow(payload: { invitation_token: string; target_folder: string; remote_endpoint: string; device_label: string; root_path: string }): Promise<JoinFlowSubmitResult> {
    return request<JoinFlowSubmitResult>('/api/v1/orbit/setup/join/submit', {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  },

  async completeJoinFlow(payload: { request_id: string; remote_endpoint: string; target_folder: string; root_path: string; device_label?: string }): Promise<JoinFlowCompleteResult> {
    return request<JoinFlowCompleteResult>('/api/v1/orbit/setup/join/complete', {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  },

  async testPeerEndpoint(url: string): Promise<PeerTestResult> {
    return request<PeerTestResult>('/api/v1/peers/test', {
      method: 'POST',
      body: JSON.stringify({ url }),
    });
  },

  async renameDevice(deviceId: string, alias: string): Promise<RenameDeviceResult> {
    return request<RenameDeviceResult>('/api/v1/devices/alias', {
      method: 'POST',
      body: JSON.stringify({ device_id: deviceId, alias }),
    });
  },

  async previewRetireDevice(folder: string, deviceId: string): Promise<RetireDevicePreviewResult> {
    return request<RetireDevicePreviewResult>('/api/v1/peers/retire/preview', {
      method: 'POST',
      body: JSON.stringify({ folder, device_id: deviceId }),
    });
  },

  async getPeerEndpoints(): Promise<PeerEndpointConfig[]> {
    const res = await request<any>('/api/v1/settings/peers');
    if (Array.isArray(res)) return res;
    if (res && Array.isArray(res.endpoints)) return res.endpoints;
    return [];
  },

  async setPeerEndpoint(endpoint: PeerEndpointConfig): Promise<any> {
    return request<any>('/api/v1/settings/peers', {
      method: 'POST',
      body: JSON.stringify(endpoint),
    });
  },

  async removePeerEndpoint(folder: string, device: string): Promise<any> {
    return request<any>('/api/v1/settings/peers', {
      method: 'DELETE',
      body: JSON.stringify({ folder, device }),
    });
  },

  // Browse, Search, Content & Previews (O07 - O08)
  async browse(folder: string, options: { path?: string; sort?: string; direction?: string; limit?: number; cursor?: string } = {}): Promise<BrowseResult> {
    const params = new URLSearchParams();
    params.set('folder', folder);
    if (options.path !== undefined) params.set('path', options.path);
    if (options.sort) params.set('sort', options.sort);
    if (options.direction) params.set('direction', options.direction);
    if (options.limit) params.set('limit', String(options.limit));
    if (options.cursor) params.set('cursor', options.cursor);
    return request<BrowseResult>(`/api/v1/browse?${params.toString()}`);
  },

  async search(folder: string, options: { query: string; limit?: number; cursor?: string }): Promise<SearchResult> {
    const params = new URLSearchParams();
    params.set('folder', folder);
    params.set('q', options.query);
    if (options.limit) params.set('limit', String(options.limit));
    if (options.cursor) params.set('cursor', options.cursor);
    return request<SearchResult>(`/api/v1/search?${params.toString()}`);
  },

  async getFileDetails(folder: string, path: string): Promise<FileDetails> {
    return request<FileDetails>(`/api/v1/browse/details?folder=${encodeURIComponent(folder)}&path=${encodeURIComponent(path)}`);
  },

  async getBrowseHistory(folder: string, path: string, options: { limit?: number; cursor?: string } = {}): Promise<FileHistoryResult> {
    const params = new URLSearchParams();
    params.set('folder', folder);
    params.set('path', path);
    if (options.limit) params.set('limit', String(options.limit));
    if (options.cursor) params.set('cursor', options.cursor);
    return request<FileHistoryResult>(`/api/v1/browse/history?${params.toString()}`);
  },

  async getBrowseDeleted(folder: string, options: { limit?: number; cursor?: string } = {}): Promise<DeletedFilesResult> {
    const params = new URLSearchParams();
    params.set('folder', folder);
    if (options.limit) params.set('limit', String(options.limit));
    if (options.cursor) params.set('cursor', options.cursor);
    return request<DeletedFilesResult>(`/api/v1/browse/deleted?${params.toString()}`);
  },

  getContentURL(folder: string, author: string, counter: number, preview?: 'text' | 'raster'): string {
    const params = new URLSearchParams();
    params.set('folder', folder);
    params.set('author', author);
    params.set('counter', String(counter));
    if (preview) params.set('preview', preview);
    return `/api/v1/content?${params.toString()}`;
  },

  async fetchTextPreview(folder: string, author: string, counter: number): Promise<string> {
    const url = api.getContentURL(folder, author, counter, 'text');
    const response = await fetch(url, {
      headers: {
        Accept: 'text/plain',
      },
      credentials: 'same-origin',
    });
    if (!response.ok) {
      let msg = 'Failed to load text preview';
      try {
        const errJson = await response.json();
        msg = errJson.message || msg;
      } catch {}
      throw new Error(msg);
    }
    return response.text();
  },

  downloadContent(folder: string, author: string, counter: number, filename?: string): void {
    const url = api.getContentURL(folder, author, counter);
    const link = document.createElement('a');
    link.href = url;
    if (filename) {
      link.download = filename;
    }
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
  },

  // --- Orbit File Mutations (O09 - O10) ---
  async importFile(
    folder: string,
    path: string,
    file: File | Blob,
    options: {
      overwrite?: boolean;
      executable?: boolean;
      reviewedToken?: string;
      idempotencyKey?: string;
    } = {}
  ): Promise<ImportFileResult> {
    const formData = new FormData();
    formData.append('folder', folder);
    formData.append('path', path);
    formData.append('file', file);
    if (options.overwrite) formData.append('overwrite', 'true');
    if (options.executable) formData.append('executable', 'true');
    if (options.reviewedToken) formData.append('reviewed_token', options.reviewedToken);
    if (options.idempotencyKey) formData.append('idempotency_key', options.idempotencyKey);

    return request<ImportFileResult>('/api/v1/files/import', {
      method: 'POST',
      body: formData,
    });
  },

  async createDir(
    folder: string,
    path: string,
    options: { idempotencyKey?: string } = {}
  ): Promise<CreateDirResult> {
    return request<CreateDirResult>('/api/v1/files/mkdir', {
      method: 'POST',
      body: JSON.stringify({
        folder,
        path,
        idempotency_key: options.idempotencyKey || `mkdir-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
      }),
    });
  },

  async moveFile(
    folder: string,
    sourcePath: string,
    destPath: string,
    options: {
      overwrite?: boolean;
      reviewedToken?: string;
      idempotencyKey?: string;
    } = {}
  ): Promise<MoveFileResult> {
    return request<MoveFileResult>('/api/v1/files/move', {
      method: 'POST',
      body: JSON.stringify({
        folder,
        source_path: sourcePath,
        dest_path: destPath,
        overwrite: !!options.overwrite,
        reviewed_token: options.reviewedToken || '',
        idempotency_key: options.idempotencyKey || `move-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
      }),
    });
  },

  async deleteFile(
    folder: string,
    path: string,
    options: {
      recursive?: boolean;
      reviewedToken?: string;
      idempotencyKey?: string;
    } = {}
  ): Promise<DeleteFileResult> {
    return request<DeleteFileResult>('/api/v1/files/delete', {
      method: 'POST',
      body: JSON.stringify({
        folder,
        path,
        recursive: !!options.recursive,
        reviewed_token: options.reviewedToken || '',
        idempotency_key: options.idempotencyKey || `delete-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
      }),
    });
  },
};


export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(2))} ${sizes[i]}`;
}

export function formatShortID(hexID: string): string {
  if (!hexID) return '';
  if (hexID.length <= 12) return hexID;
  return `${hexID.slice(0, 8)}…${hexID.slice(-4)}`;
}

export function formatVersionID(id: VersionID): string {
  if (!id) return '';
  const author = id.author || id.Author || '';
  const counter = id.counter !== undefined ? id.counter : id.Counter !== undefined ? id.Counter : '';
  return `${formatShortID(author)}:${counter}`;
}

export function formatTimestamp(nsOrIso: number | string): string {
  if (typeof nsOrIso === 'number') {
    if (nsOrIso === 0) return 'never';
    const date = new Date(nsOrIso / 1000000);
    return date.toLocaleString();
  }
  if (!nsOrIso) return 'never';
  const d = new Date(nsOrIso);
  if (isNaN(d.getTime())) return nsOrIso;
  return d.toLocaleString();
}
