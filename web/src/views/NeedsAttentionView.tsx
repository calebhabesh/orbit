import React, { useState, useEffect, useCallback } from 'react';
import {
  FolderRecord,
  ConflictsResult,
  ConflictSet,
  StructuralConflict,
  DoctorReport,
  PeerListResult,
  StorageUsage,
  DurableTask,
} from '../types';
import { api, formatBytes } from '../api';
import { ConflictResolveModal } from '../components/ConflictResolveModal';

interface NeedsAttentionViewProps {
  folders: FolderRecord[];
  conflictsMap: Record<string, ConflictsResult>;
  doctorReport: DoctorReport | null;
  peersMap?: Record<string, PeerListResult>;
  storageUsage?: StorageUsage | null;
  onRefresh: () => void;
}

export const NeedsAttentionView: React.FC<NeedsAttentionViewProps> = ({
  folders,
  conflictsMap,
  doctorReport,
  peersMap = {},
  storageUsage,
  onRefresh,
}) => {
  const [activeConflict, setActiveConflict] = useState<{ folder: string; conflict: ConflictSet } | null>(null);
  const [actionMessage, setActionMessage] = useState<string | null>(null);
  const [runningAction, setRunningAction] = useState(false);
  const [failedTasks, setFailedTasks] = useState<DurableTask[]>([]);

  // Load failed/exhausted operation tasks
  const loadFailedTasks = useCallback(async () => {
    try {
      const [failedRes, exhaustedRes] = await Promise.all([
        api.getWorkList(undefined, 'failed').catch(() => ({ tasks: [] })),
        api.getWorkList(undefined, 'exhausted').catch(() => ({ tasks: [] })),
      ]);
      const combined = [...(failedRes.tasks || []), ...(exhaustedRes.tasks || [])];
      setFailedTasks(combined);
    } catch {
      // ignore
    }
  }, []);

  useEffect(() => {
    loadFailedTasks();
  }, [loadFailedTasks]);

  // Collect all content conflicts
  const contentConflicts: { folder: string; conflict: ConflictSet }[] = [];
  const structuralConflicts: { folder: string; conflict: StructuralConflict }[] = [];

  for (const [fid, res] of Object.entries(conflictsMap)) {
    if (res?.content_conflicts) {
      for (const c of res.content_conflicts) {
        contentConflicts.push({ folder: fid, conflict: c });
      }
    }
    if (res?.structural_conflicts) {
      for (const sc of res.structural_conflicts) {
        structuralConflicts.push({ folder: fid, conflict: sc });
      }
    }
  }

  // Paused / unavailable folders
  const pausedFolders = folders.filter((f) => f.paused);

  // Doctor issues
  const failingDoctorChecks = (doctorReport?.checks || []).filter(
    (c) => c.status === 'fail' || c.status === 'warn'
  );

  const totalIssues =
    contentConflicts.length +
    structuralConflicts.length +
    pausedFolders.length +
    failingDoctorChecks.length +
    failedTasks.length;

  const handleRevalidateFolder = async (folderId: string) => {
    setRunningAction(true);
    setActionMessage(null);
    try {
      await api.revalidateFolder(folderId);
      setActionMessage('Folder root revalidated successfully.');
      onRefresh();
    } catch (err: any) {
      setActionMessage(`Revalidation failed: ${err.message}`);
    } finally {
      setRunningAction(false);
    }
  };

  const handleRetryTask = async (task: DurableTask) => {
    setRunningAction(true);
    setActionMessage(null);
    try {
      await api.retryWork(task.folder || task.folder_id, task.id || task.task_id);
      setActionMessage(`Retried task ${task.id || task.task_id}.`);
      loadFailedTasks();
      onRefresh();
    } catch (err: any) {
      setActionMessage(`Failed to retry task: ${err.message}`);
    } finally {
      setRunningAction(false);
    }
  };

  const handleCancelTask = async (task: DurableTask) => {
    setRunningAction(true);
    setActionMessage(null);
    try {
      await api.cancelWork(task.id || task.task_id || '');
      setActionMessage(`Canceled task ${task.id || task.task_id}.`);
      loadFailedTasks();
      onRefresh();
    } catch (err: any) {
      setActionMessage(`Failed to cancel task: ${err.message}`);
    } finally {
      setRunningAction(false);
    }
  };

  const handleRunGC = async (folderId: string) => {
    setRunningAction(true);
    setActionMessage(null);
    try {
      await api.runGC(folderId);
      setActionMessage('Garbage collection completed. Reclaimed unreferenced objects.');
      onRefresh();
    } catch (err: any) {
      setActionMessage(`GC failed: ${err.message}`);
    } finally {
      setRunningAction(false);
    }
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '1.25rem' }}>
      
      {/* Header Summary */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '1rem 1.25rem', backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)' }}>
        <div>
          <h2 style={{ fontSize: '1.125rem', fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>
            Needs Attention
          </h2>
          <p style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', margin: '0.25rem 0 0 0' }}>
            {totalIssues === 0
              ? 'No items require attention. All files and replicas are healthy.'
              : `${totalIssues} ${totalIssues === 1 ? 'item requires' : 'items require'} operator review.`}
          </p>
        </div>

        {totalIssues > 0 && (
          <span
            style={{
              padding: '0.25rem 0.625rem',
              borderRadius: '9999px',
              backgroundColor: 'var(--status-warning-bg)',
              color: 'var(--status-warning)',
              border: '1px solid var(--status-warning)',
              fontSize: '0.75rem',
              fontWeight: 600,
            }}
          >
            {totalIssues} Active
          </span>
        )}
      </div>

      {actionMessage && (
        <div style={{ padding: '0.75rem 1rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', fontSize: '0.8125rem', color: 'var(--text-primary)' }}>
          {actionMessage}
        </div>
      )}

      {/* ALL CLEAR STATE */}
      {totalIssues === 0 && (
        <div style={{ padding: '4rem 2rem', textAlign: 'center', backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)' }}>
          <div style={{ width: '3.5rem', height: '3.5rem', borderRadius: '50%', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', display: 'inline-flex', alignItems: 'center', justifyContent: 'center', marginBottom: '1rem' }}>
            <svg style={{ width: '1.75rem', height: '1.75rem', color: 'var(--status-success)' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
              <polyline points="22 4 12 14.01 9 11.01" />
            </svg>
          </div>
          <h3 style={{ fontSize: '1.125rem', fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>
            Everything is Up to Date
          </h3>
          <p style={{ fontSize: '0.875rem', color: 'var(--text-secondary)', marginTop: '0.5rem', maxWidth: '420px', margin: '0.5rem auto 0 auto' }}>
            No concurrent conflicts, root access issues, failed operations, or doctor warnings detected. Your synchronized files are safe and durable.
          </p>
        </div>
      )}

      {/* 1. CONTENT CONFLICTS */}
      {contentConflicts.length > 0 && (
        <div style={{ backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)', overflow: 'hidden' }}>
          <div style={{ padding: '0.875rem 1.25rem', borderBottom: '1px solid var(--border)', backgroundColor: 'var(--surface-raised)', display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
            <svg style={{ width: '1rem', height: '1rem', color: 'var(--status-warning)' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
              <line x1="12" y1="9" x2="12" y2="13" />
              <line x1="12" y1="17" x2="12.01" y2="17" />
            </svg>
            <span style={{ fontWeight: 600, fontSize: '0.875rem', color: 'var(--text-primary)' }}>
              Concurrent File Conflicts ({contentConflicts.length})
            </span>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column' }}>
            {contentConflicts.map((item, idx) => {
              const isDeleteConflict = item.conflict.heads.some((h) => h.kind === 3);

              return (
                <div
                  key={idx}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    padding: '1rem 1.25rem',
                    borderBottom: idx < contentConflicts.length - 1 ? '1px solid var(--border)' : 'none',
                    gap: '1rem',
                  }}
                >
                  <div>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                      <span style={{ fontWeight: 600, fontSize: '0.875rem', color: 'var(--text-primary)' }}>
                        {item.conflict.path}
                      </span>
                      <span
                        style={{
                          fontSize: '0.6875rem',
                          padding: '0.0625rem 0.375rem',
                          borderRadius: 'var(--radius-sm)',
                          backgroundColor: isDeleteConflict ? 'var(--status-danger-bg)' : 'var(--status-warning-bg)',
                          color: isDeleteConflict ? 'var(--status-danger)' : 'var(--status-warning)',
                          border: `1px solid ${isDeleteConflict ? 'var(--status-danger)' : 'var(--status-warning)'}`,
                        }}
                      >
                        {item.conflict.conflict_kind}
                      </span>
                    </div>
                    <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginTop: '0.25rem' }}>
                      {item.conflict.heads.length} competing versions created concurrently.{' '}
                      {isDeleteConflict ? 'Includes a deletion tombstone head. Review choices to resolve.' : 'Review differences to resolve.'}
                    </div>
                  </div>

                  <button
                    type="button"
                    id={`btn-resolve-conflict-${idx}`}
                    onClick={() => setActiveConflict(item)}
                    className="btn btn-primary"
                    style={{
                      padding: '0.5rem 1rem',
                      fontSize: '0.8125rem',
                      fontWeight: 600,
                      backgroundColor: 'var(--text-primary)',
                      color: 'var(--canvas)',
                      border: 'none',
                      borderRadius: 'var(--radius-sm)',
                      cursor: 'pointer',
                      whiteSpace: 'nowrap',
                    }}
                  >
                    Review &amp; Resolve
                  </button>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {/* 2. STRUCTURAL CONFLICTS */}
      {structuralConflicts.length > 0 && (
        <div style={{ backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)', overflow: 'hidden' }}>
          <div style={{ padding: '0.875rem 1.25rem', borderBottom: '1px solid var(--border)', backgroundColor: 'var(--surface-raised)', display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
            <span style={{ fontWeight: 600, fontSize: '0.875rem', color: 'var(--text-primary)' }}>
              Structural Namespace Conflicts ({structuralConflicts.length})
            </span>
          </div>
          <div style={{ padding: '1rem 1.25rem', display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
            {structuralConflicts.map((sc, idx) => (
              <div key={idx} style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)' }}>
                Ancestor: <code className="code-font" style={{ color: 'var(--text-primary)' }}>{sc.conflict.ancestor_path}</code> vs Descendant: <code className="code-font" style={{ color: 'var(--text-primary)' }}>{sc.conflict.descendant_path}</code>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* 3. PAUSED OR UNAVAILABLE FOLDERS */}
      {pausedFolders.length > 0 && (
        <div style={{ backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)', overflow: 'hidden' }}>
          <div style={{ padding: '0.875rem 1.25rem', borderBottom: '1px solid var(--border)', backgroundColor: 'var(--surface-raised)', display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
            <span style={{ fontWeight: 600, fontSize: '0.875rem', color: 'var(--text-primary)' }}>
              Folder Sync Paused ({pausedFolders.length})
            </span>
          </div>
          <div style={{ display: 'flex', flexDirection: 'column' }}>
            {pausedFolders.map((f, idx) => (
              <div key={idx} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '1rem 1.25rem', gap: '1rem' }}>
                <div>
                  <div style={{ fontWeight: 600, fontSize: '0.875rem', color: 'var(--text-primary)' }}>
                    {f.root_path || f.folder || f.folder_id}
                  </div>
                  <div style={{ fontSize: '0.75rem', color: 'var(--status-danger)', marginTop: '0.25rem' }}>
                    Reason: {f.pause_reason || 'Root directory unmounted or inaccessible'}
                  </div>
                </div>
                <button
                  type="button"
                  id={`btn-revalidate-root-${idx}`}
                  onClick={() => handleRevalidateFolder(f.folder || f.folder_id || '')}
                  disabled={runningAction}
                  className="btn btn-secondary"
                  style={{ padding: '0.5rem 1rem', fontSize: '0.8125rem', fontWeight: 600, backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', color: 'var(--text-primary)', borderRadius: 'var(--radius-sm)', cursor: 'pointer', whiteSpace: 'nowrap' }}
                >
                  Revalidate Root
                </button>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* 4. OPERATION ERRORS & FAILED TASKS */}
      {failedTasks.length > 0 && (
        <div style={{ backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)', overflow: 'hidden' }}>
          <div style={{ padding: '0.875rem 1.25rem', borderBottom: '1px solid var(--border)', backgroundColor: 'var(--surface-raised)', display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
            <svg style={{ width: '1rem', height: '1rem', color: 'var(--status-danger)' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="12" cy="12" r="10" />
              <line x1="12" y1="8" x2="12" y2="12" />
              <line x1="12" y1="16" x2="12.01" y2="16" />
            </svg>
            <span style={{ fontWeight: 600, fontSize: '0.875rem', color: 'var(--text-primary)' }}>
              Operation Errors &amp; Failed Tasks ({failedTasks.length})
            </span>
          </div>
          <div style={{ display: 'flex', flexDirection: 'column' }}>
            {failedTasks.map((t, idx) => (
              <div
                key={idx}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  padding: '1rem 1.25rem',
                  borderBottom: idx < failedTasks.length - 1 ? '1px solid var(--border)' : 'none',
                  gap: '1rem',
                }}
              >
                <div>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                    <span style={{ fontSize: '0.75rem', padding: '0.125rem 0.375rem', borderRadius: 'var(--radius-sm)', backgroundColor: 'var(--status-danger-bg)', color: 'var(--status-danger)', border: '1px solid var(--status-danger)', fontWeight: 600 }}>
                      {t.state.toUpperCase()}
                    </span>
                    <span style={{ fontWeight: 600, fontSize: '0.875rem', color: 'var(--text-primary)' }}>
                      {t.operation || t.kind || 'Background Task'}
                    </span>
                    {t.target_path && (
                      <span className="code-font" style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
                        {t.target_path}
                      </span>
                    )}
                  </div>
                  <div style={{ fontSize: '0.75rem', color: 'var(--status-danger)', marginTop: '0.25rem' }}>
                    Error: {t.last_error || t.error_code || 'Task failed after retries'}
                  </div>
                </div>

                <div style={{ display: 'flex', gap: '0.375rem' }}>
                  <button
                    type="button"
                    onClick={() => handleRetryTask(t)}
                    disabled={runningAction}
                    className="btn btn-secondary btn-sm"
                    style={{ padding: '0.375rem 0.75rem', fontSize: '0.75rem' }}
                  >
                    Retry
                  </button>
                  <button
                    type="button"
                    onClick={() => handleCancelTask(t)}
                    disabled={runningAction}
                    className="btn btn-secondary btn-sm"
                    style={{ padding: '0.375rem 0.75rem', fontSize: '0.75rem', color: 'var(--status-danger)' }}
                  >
                    Cancel
                  </button>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* 5. DOCTOR WARNINGS / ERRORS */}
      {failingDoctorChecks.length > 0 && (
        <div style={{ backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)', overflow: 'hidden' }}>
          <div style={{ padding: '0.875rem 1.25rem', borderBottom: '1px solid var(--border)', backgroundColor: 'var(--surface-raised)' }}>
            <span style={{ fontWeight: 600, fontSize: '0.875rem', color: 'var(--text-primary)' }}>
              Diagnostic Health Checks ({failingDoctorChecks.length})
            </span>
          </div>
          <div style={{ display: 'flex', flexDirection: 'column' }}>
            {failingDoctorChecks.map((chk, idx) => (
              <div key={idx} style={{ padding: '1rem 1.25rem', borderBottom: idx < failingDoctorChecks.length - 1 ? '1px solid var(--border)' : 'none' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                  <span style={{ fontSize: '0.75rem', fontWeight: 600, padding: '0.125rem 0.375rem', borderRadius: 'var(--radius-sm)', backgroundColor: chk.status === 'fail' ? 'var(--status-danger-bg)' : 'var(--status-warning-bg)', color: chk.status === 'fail' ? 'var(--status-danger)' : 'var(--status-warning)' }}>
                    {chk.status.toUpperCase()}
                  </span>
                  <span style={{ fontWeight: 600, fontSize: '0.875rem', color: 'var(--text-primary)' }}>
                    {chk.name}
                  </span>
                </div>
                <div style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', marginTop: '0.375rem' }}>
                  {chk.message}
                </div>
                {chk.remediation && (
                  <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginTop: '0.25rem', fontStyle: 'italic' }}>
                    Remediation: {chk.remediation}
                  </div>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* 6. STORAGE USAGE & CAPACITY MAINTENANCE */}
      {storageUsage && (
        <div style={{ backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)', overflow: 'hidden' }}>
          <div style={{ padding: '0.875rem 1.25rem', borderBottom: '1px solid var(--border)', backgroundColor: 'var(--surface-raised)', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <span style={{ fontWeight: 600, fontSize: '0.875rem', color: 'var(--text-primary)' }}>
              Storage &amp; Managed Objects
            </span>
            {folders[0] && (
              <button
                type="button"
                id="btn-run-gc"
                onClick={() => handleRunGC(folders[0].folder || folders[0].folder_id || '')}
                disabled={runningAction}
                className="btn btn-secondary btn-sm"
                style={{ padding: '0.25rem 0.625rem', fontSize: '0.75rem' }}
              >
                Run Garbage Collection
              </button>
            )}
          </div>
          <div style={{ padding: '1rem 1.25rem', display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(160px, 1fr))', gap: '0.75rem' }}>
            <div style={{ padding: '0.625rem', backgroundColor: 'var(--surface-raised)', borderRadius: 'var(--radius-sm)' }}>
              <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>Total Managed</div>
              <div style={{ fontSize: '0.9375rem', fontWeight: 600, color: 'var(--text-primary)', marginTop: '0.25rem' }}>
                {formatBytes(storageUsage.total_managed_bytes)}
              </div>
            </div>
            <div style={{ padding: '0.625rem', backgroundColor: 'var(--surface-raised)', borderRadius: 'var(--radius-sm)' }}>
              <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>CAS Objects</div>
              <div style={{ fontSize: '0.9375rem', fontWeight: 600, color: 'var(--text-primary)', marginTop: '0.25rem' }}>
                {formatBytes(storageUsage.object_bytes)}
              </div>
            </div>
            <div style={{ padding: '0.625rem', backgroundColor: 'var(--surface-raised)', borderRadius: 'var(--radius-sm)' }}>
              <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>Metadata &amp; DB</div>
              <div style={{ fontSize: '0.9375rem', fontWeight: 600, color: 'var(--text-primary)', marginTop: '0.25rem' }}>
                {formatBytes(storageUsage.metadata_bytes + storageUsage.wal_bytes)}
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Conflict Resolve Modal */}
      {activeConflict && (
        <ConflictResolveModal
          folder={activeConflict.folder}
          conflict={activeConflict.conflict}
          peerAliases={peersMap[activeConflict.folder]?.aliases}
          localAuthor={folders.find((f) => (f.folder || f.folder_id) === activeConflict.folder)?.local_author}
          onClose={() => setActiveConflict(null)}
          onResolved={() => {
            setActiveConflict(null);
            onRefresh();
          }}
          onRefresh={onRefresh}
        />
      )}
    </div>
  );
};
