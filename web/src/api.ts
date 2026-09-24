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
  VersionID,
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
