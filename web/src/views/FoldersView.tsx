import React, { useState } from 'react';
import { FolderRecord, PeerListResult, WorkStatusResult, WorkListResult, StorageUsage } from '../types';
import { api, formatBytes, formatShortID, formatTimestamp, APIError } from '../api';

interface FoldersViewProps {
  folders: FolderRecord[];
  peersMap: Record<string, PeerListResult>;
  workStatus: WorkStatusResult | null;
  workList: WorkListResult | null;
  storageUsage: StorageUsage | null;
  onRefresh: () => void;
}

export const FoldersView: React.FC<FoldersViewProps> = ({
  folders,
  peersMap,
  workStatus,
  workList,
  storageUsage,
  onRefresh,
}) => {
  const [showAddModal, setShowAddModal] = useState(false);
  const [newFolderID, setNewFolderID] = useState('');
  const [newRootPath, setNewRootPath] = useState('');
  const [actionError, setActionError] = useState<string | null>(null);
  const [actionSuccess, setActionSuccess] = useState<string | null>(null);
  const [loadingAction, setLoadingAction] = useState<string | null>(null);

  const handleAddFolder = async (e: React.FormEvent) => {
    e.preventDefault();
    setActionError(null);
    setActionSuccess(null);
    setLoadingAction('add');
    try {
      await api.addFolder(newFolderID, newRootPath);
      setActionSuccess(`Folder ${formatShortID(newFolderID)} registered successfully!`);
      setShowAddModal(false);
      setNewFolderID('');
      setNewRootPath('');
      onRefresh();
    } catch (err: any) {
      if (err instanceof APIError) {
        setActionError(`Failed to add folder: ${err.message} (${err.code}). ${err.action}`);
      } else {
        setActionError(err.message || 'Failed to add folder');
      }
    } finally {
      setLoadingAction(null);
    }
  };

  const handlePause = async (folder: string) => {
    const reason = window.prompt('Enter pause reason (optional):', 'OPERATOR_PAUSED');
    if (reason === null) return;
    setLoadingAction(`pause-${folder}`);
    setActionError(null);
    try {
      await api.pauseFolder(folder, reason);
      onRefresh();
    } catch (err: any) {
      setActionError(err.message || 'Failed to pause folder');
    } finally {
      setLoadingAction(null);
    }
  };

  const handleResume = async (folder: string) => {
    setLoadingAction(`resume-${folder}`);
    setActionError(null);
    try {
      await api.resumeFolder(folder);
      onRefresh();
    } catch (err: any) {
      setActionError(err.message || 'Failed to resume folder');
    } finally {
      setLoadingAction(null);
    }
  };

  const handleRevalidate = async (folder: string) => {
    setLoadingAction(`revalidate-${folder}`);
    setActionError(null);
    try {
      await api.revalidateFolder(folder);
      setActionSuccess(`Folder ${formatShortID(folder)} root revalidated successfully!`);
      onRefresh();
    } catch (err: any) {
      setActionError(err.message || 'Failed to revalidate root');
    } finally {
      setLoadingAction(null);
    }
  };

  const handleRemove = async (folder: string) => {
    const confirmed = window.confirm(
      'Are you sure you want to unregister this folder?\n\n' +
      'SAFE GUARANTEE: Unregistering clears database tracking records while leaving your working files completely intact on disk. It authors 0 deletion tombstones into the replication graph.'
    );
    if (!confirmed) return;

    setLoadingAction(`remove-${folder}`);
    setActionError(null);
    try {
      const res = await api.removeFolder(folder);
      setActionSuccess(res.message);
      onRefresh();
    } catch (err: any) {
      setActionError(err.message || 'Failed to remove folder');
    } finally {
      setLoadingAction(null);
    }
  };

  const handleCancelTask = async (taskId: string) => {
    setLoadingAction(`cancel-${taskId}`);
    try {
      await api.cancelWork(taskId);
      onRefresh();
    } catch (err: any) {
      setActionError(err.message || 'Failed to cancel task');
    } finally {
      setLoadingAction(null);
    }
  };

  const handleRetryAll = async () => {
    setLoadingAction('retry-all');
    try {
      const res = await api.retryWork(undefined, undefined, true);
      setActionSuccess(res.message || 'Retried exhausted tasks');
      onRefresh();
    } catch (err: any) {
      setActionError(err.message || 'Failed to retry tasks');
    } finally {
      setLoadingAction(null);
    }
  };

  const handleRunGC = async (folder: string) => {
    setLoadingAction(`gc-${folder}`);
    try {
      await api.runGC(folder);
      setActionSuccess('Garbage collection completed.');
      onRefresh();
    } catch (err: any) {
      setActionError(err.message || 'Failed to run GC');
    } finally {
      setLoadingAction(null);
    }
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '1.5rem' }}>
      {actionError && (
        <div className="storage-banner danger" role="alert">
          <div>{actionError}</div>
          <button type="button" className="btn btn-secondary btn-sm" onClick={() => setActionError(null)}>✕</button>
        </div>
      )}
      {actionSuccess && (
        <div className="storage-banner" style={{ backgroundColor: 'var(--status-success-bg)', border: '1px solid rgba(52, 211, 153, 0.4)', color: '#d1fae5' }} role="status">
          <div>{actionSuccess}</div>
          <button type="button" className="btn btn-secondary btn-sm" onClick={() => setActionSuccess(null)}>✕</button>
        </div>
      )}

      {/* 1. REGISTERED FOLDERS SECTION */}
      <section className="panel-card" aria-labelledby="folders-heading">
        <div className="panel-header">
          <h2 id="folders-heading" className="panel-title">
            Registered Folders ({folders.length})
          </h2>
          <button type="button" className="btn btn-primary btn-sm" onClick={() => setShowAddModal(true)}>
            + Add Folder
          </button>
        </div>

        {folders.length === 0 ? (
          <div className="empty-state">
            <p>No sync folders are currently registered on this device.</p>
            <button type="button" className="btn btn-secondary" onClick={() => setShowAddModal(true)}>
              Register Folder
            </button>
          </div>
        ) : (
          <div className="table-wrapper">
            <table className="data-table">
              <thead>
                <tr>
                  <th>Folder ID</th>
                  <th>Root Path</th>
                  <th>Status</th>
                  <th>Generation / Rev</th>
                  <th>Device &amp; Inode</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {folders.map((f) => {
                  const fid = f.folder || f.folder_id || '';
                  return (
                    <tr key={fid}>
                      <td className="code-font" style={{ fontWeight: 600 }}>
                        <span title={fid}>{formatShortID(fid)}</span>
                      </td>
                      <td className="code-font">
                        {f.root_path || <em style={{ color: 'var(--text-subtle)' }}>(Unregistered root)</em>}
                      </td>
                      <td>
                        {f.paused ? (
                          <span className="badge badge-warning" title={f.pause_reason}>
                            PAUSED ({f.pause_reason || 'OPERATOR'})
                          </span>
                        ) : (
                          <span className="badge badge-success">ACTIVE</span>
                        )}
                      </td>
                      <td style={{ fontSize: '0.8125rem' }}>
                        Gen #{f.scan_generation} (Rev {f.membership_revision})
                      </td>
                      <td className="code-font" style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                        dev={f.root_device} ino={f.root_inode}
                      </td>
                      <td>
                        <div style={{ display: 'flex', gap: '0.35rem', flexWrap: 'wrap' }}>
                          {f.paused ? (
                            <button
                              type="button"
                              className="btn btn-secondary btn-sm"
                              onClick={() => handleResume(fid)}
                              disabled={loadingAction === `resume-${fid}`}
                            >
                              Resume
                            </button>
                          ) : (
                            <button
                              type="button"
                              className="btn btn-secondary btn-sm"
                              onClick={() => handlePause(fid)}
                              disabled={loadingAction === `pause-${fid}`}
                            >
                              Pause
                            </button>
                          )}
                          <button
                            type="button"
                            className="btn btn-secondary btn-sm"
                            onClick={() => handleRevalidate(fid)}
                            title="Verify workspace root mount and registration marker"
                            disabled={loadingAction === `revalidate-${fid}`}
                          >
                            Revalidate
                          </button>
                          <button
                            type="button"
                            className="btn btn-secondary btn-sm"
                            onClick={() => handleRunGC(fid)}
                            title="Reclaim superseded historical chunk storage"
                            disabled={loadingAction === `gc-${fid}`}
                          >
                            Run GC
                          </button>
                          <button
                            type="button"
                            className="btn btn-danger btn-sm"
                            onClick={() => handleRemove(fid)}
                            title="Remove registration while keeping files on disk"
                            disabled={loadingAction === `remove-${fid}`}
                          >
                            Remove
                          </button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {/* 2. PEERS & DEVICES SECTION */}
      <section className="panel-card" aria-labelledby="peers-heading">
        <div className="panel-header">
          <h2 id="peers-heading" className="panel-title">
            Devices &amp; Qualified Progress
          </h2>
          <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
            Direct receipts only; non-transitive. Offline is not sync success.
          </span>
        </div>

        {Object.keys(peersMap).length === 0 ? (
          <div className="empty-state">
            <p>No paired peers found for registered folders.</p>
          </div>
        ) : (
          Object.entries(peersMap).map(([folderID, pList]) => (
            <div key={folderID} style={{ marginBottom: '1rem' }}>
              <div style={{ fontSize: '0.875rem', fontWeight: 600, marginBottom: '0.5rem' }}>
                Folder: <span className="code-font">{formatShortID(folderID)}</span> (Rev {pList.revision})
              </div>

              <div className="table-wrapper">
                <table className="data-table">
                  <thead>
                    <tr>
                      <th>Device ID</th>
                      <th>Key Pin</th>
                      <th>Status</th>
                      <th>Connection &amp; Receipt Guarantee</th>
                    </tr>
                  </thead>
                  <tbody>
                    {pList.active.map((m) => (
                      <tr key={m.device}>
                        <td className="code-font" style={{ fontWeight: 600 }}>
                          <span title={m.device}>{formatShortID(m.device)}</span>
                        </td>
                        <td className="code-font" style={{ fontSize: '0.75rem' }}>
                          {formatShortID(m.key_pin)}
                        </td>
                        <td>
                          <span className="badge badge-success">ACTIVE MEMBER</span>
                        </td>
                        <td style={{ fontSize: '0.8125rem' }}>
                          Direct TLS 1.3 mTLS authenticated; explicit folder membership required.
                        </td>
                      </tr>
                    ))}
                    {pList.retired.map((r) => (
                      <tr key={r.device}>
                        <td className="code-font" style={{ color: 'var(--text-muted)' }}>
                          <span title={r.device}>{formatShortID(r.device)}</span>
                        </td>
                        <td className="code-font" style={{ fontSize: '0.75rem' }}>
                          Snapshot: {formatShortID(r.snapshot_digest)}
                        </td>
                        <td>
                          <span className="badge badge-neutral">RETIRED (Rev {r.retired_at})</span>
                        </td>
                        <td style={{ fontSize: '0.8125rem', color: 'var(--text-muted)' }}>
                          Permanently frozen history snapshot. Access revoked.
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          ))
        )}
      </section>

      {/* 3. PENDING WORK QUEUE SECTION */}
      <section className="panel-card" aria-labelledby="work-heading">
        <div className="panel-header">
          <h2 id="work-heading" className="panel-title">
            Durable Work Queue
          </h2>
          <div style={{ display: 'flex', gap: '0.5rem', alignItems: 'center' }}>
            {workStatus?.folders.map((ws) => (
              <span key={ws.folder} className="badge badge-neutral">
                Q: {ws.queued_tasks} | Run: {ws.running_tasks} | Retry: {ws.retry_tasks} | Exh: {ws.exhausted_tasks}
              </span>
            ))}
            <button
              type="button"
              className="btn btn-secondary btn-sm"
              onClick={handleRetryAll}
              disabled={loadingAction === 'retry-all'}
            >
              Retry All
            </button>
          </div>
        </div>

        {!workList?.tasks || workList.tasks.length === 0 ? (
          <div className="empty-state">
            <p>Work queue is empty. System is idle.</p>
          </div>
        ) : (
          <div className="table-wrapper">
            <table className="data-table">
              <thead>
                <tr>
                  <th>Task ID</th>
                  <th>Folder</th>
                  <th>Operation</th>
                  <th>State</th>
                  <th>Retries</th>
                  <th>Scheduled / Created</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {workList.tasks.map((task) => {
                  const tid = task.id || task.task_id || '';
                  const tfolder = task.folder || task.folder_id || '';
                  const tkind = task.kind || task.operation || '';
                  const tatt = task.attempts ?? task.retries ?? 0;
                  return (
                    <tr key={tid}>
                      <td className="code-font">
                        <span title={tid}>{formatShortID(tid)}</span>
                      </td>
                      <td className="code-font">{formatShortID(tfolder)}</td>
                      <td>
                        <span className="badge badge-info">{tkind}</span>
                      </td>
                      <td>
                        <span
                          className={`badge ${
                            task.state === 'running'
                              ? 'badge-success'
                              : task.state === 'retrying' || task.state === 'retry'
                              ? 'badge-warning'
                              : task.state === 'exhausted'
                              ? 'badge-danger'
                              : 'badge-neutral'
                          }`}
                        >
                          {task.state}
                        </span>
                        {task.error_code && (
                          <div style={{ fontSize: '0.75rem', color: 'var(--status-danger)', marginTop: '0.2rem' }}>
                            {task.error_code}: {task.last_error}
                          </div>
                        )}
                      </td>
                      <td>{tatt}</td>
                      <td style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                        {formatTimestamp(task.scheduled_at || task.created_at || (task.created_ns ? task.created_ns / 1e6 : ''))}
                      </td>
                      <td>
                        {task.state !== 'completed' && task.state !== 'canceled' && (
                          <button
                            type="button"
                            className="btn btn-danger btn-sm"
                            onClick={() => handleCancelTask(tid)}
                            disabled={loadingAction === `cancel-${tid}`}
                          >
                            Cancel
                          </button>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {/* 4. STORAGE USAGE BREAKDOWN */}
      {storageUsage && (
        <section className="panel-card" aria-labelledby="storage-heading">
          <div className="panel-header">
            <h2 id="storage-heading" className="panel-title">
              Storage Reserve &amp; Utilization
            </h2>
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: '1rem' }}>
            <div style={{ backgroundColor: 'var(--bg-main)', padding: '1rem', borderRadius: 'var(--radius-sm)' }}>
              <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>Object Store</div>
              <div style={{ fontSize: '1.25rem', fontWeight: 700, marginTop: '0.25rem' }}>
                {formatBytes(storageUsage.object_bytes)}
              </div>
            </div>
            <div style={{ backgroundColor: 'var(--bg-main)', padding: '1rem', borderRadius: 'var(--radius-sm)' }}>
              <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>Staging Area</div>
              <div style={{ fontSize: '1.25rem', fontWeight: 700, marginTop: '0.25rem' }}>
                {formatBytes(storageUsage.staging_bytes)}
              </div>
            </div>
            <div style={{ backgroundColor: 'var(--bg-main)', padding: '1rem', borderRadius: 'var(--radius-sm)' }}>
              <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>SQLite WAL File</div>
              <div style={{ fontSize: '1.25rem', fontWeight: 700, marginTop: '0.25rem' }}>
                {formatBytes(storageUsage.wal_bytes)}
              </div>
              <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: '0.25rem' }}>Cap: 256 MiB</div>
            </div>
            <div style={{ backgroundColor: 'var(--bg-main)', padding: '1rem', borderRadius: 'var(--radius-sm)' }}>
              <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>Metadata DB</div>
              <div style={{ fontSize: '1.25rem', fontWeight: 700, marginTop: '0.25rem' }}>
                {formatBytes(storageUsage.metadata_bytes)}
              </div>
            </div>
          </div>
        </section>
      )}

      {/* ADD FOLDER MODAL */}
      {showAddModal && (
        <div className="modal-backdrop" onClick={() => setShowAddModal(false)} role="dialog" aria-modal="true" aria-labelledby="add-folder-title">
          <div className="modal-content" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <h3 id="add-folder-title" className="panel-title">Register Local Folder</h3>
              <button type="button" className="btn btn-secondary btn-sm" onClick={() => setShowAddModal(false)}>✕</button>
            </div>
            <form onSubmit={handleAddFolder}>
              <div className="modal-body">
                <div>
                  <label htmlFor="new-folder-id" style={{ display: 'block', fontSize: '0.875rem', marginBottom: '0.25rem' }}>
                    Folder ID (64-character hex)
                  </label>
                  <input
                    id="new-folder-id"
                    type="text"
                    className="form-input code-font"
                    placeholder="0123456789abcdef..."
                    value={newFolderID}
                    onChange={(e) => setNewFolderID(e.target.value)}
                    required
                  />
                </div>
                <div>
                  <label htmlFor="new-root-path" style={{ display: 'block', fontSize: '0.875rem', marginBottom: '0.25rem' }}>
                    Absolute Root Directory Path
                  </label>
                  <input
                    id="new-root-path"
                    type="text"
                    className="form-input code-font"
                    placeholder="/home/user/my-synced-folder"
                    value={newRootPath}
                    onChange={(e) => setNewRootPath(e.target.value)}
                    required
                  />
                </div>
              </div>
              <div className="modal-footer">
                <button type="button" className="btn btn-secondary" onClick={() => setShowAddModal(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary" disabled={loadingAction === 'add'}>
                  {loadingAction === 'add' ? 'Registering…' : 'Register Folder'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};
