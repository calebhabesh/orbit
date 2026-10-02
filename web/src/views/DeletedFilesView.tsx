import React, { useState, useEffect } from 'react';
import { FolderRecord, DeletedFileItem, DeletedFilesResult, RestorePreview, VersionID } from '../types';
import { api, formatBytes, formatShortID, formatVersionID } from '../api';
import { RestoreModal } from '../components/RestoreModal';

interface DeletedFilesViewProps {
  folders: FolderRecord[];
  onRefresh: () => void;
}

export const DeletedFilesView: React.FC<DeletedFilesViewProps> = ({ folders, onRefresh }) => {
  const [selectedFolder, setSelectedFolder] = useState<string>(folders[0]?.folder || folders[0]?.folder_id || '');
  const [deletedFiles, setDeletedFiles] = useState<DeletedFileItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [totalItems, setTotalItems] = useState(0);

  const [restoringPath, setRestoringPath] = useState<string | null>(null);
  const [restorePreview, setRestorePreview] = useState<RestorePreview | null>(null);
  const [notification, setNotification] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!selectedFolder && folders.length > 0) {
      setSelectedFolder(folders[0].folder || folders[0].folder_id || '');
    }
  }, [folders, selectedFolder]);

  useEffect(() => {
    if (!selectedFolder) return;
    loadDeletedFiles(selectedFolder, false);
  }, [selectedFolder]);

  const loadDeletedFiles = async (folder: string, isLoadMore = false) => {
    if (isLoadMore) {
      setLoadingMore(true);
    } else {
      setLoading(true);
      setError(null);
    }
    try {
      const res: DeletedFilesResult = await api.getBrowseDeleted(folder, {
        limit: 50,
        cursor: isLoadMore && nextCursor ? nextCursor : undefined,
      });
      if (isLoadMore) {
        setDeletedFiles((prev) => [...prev, ...res.items]);
      } else {
        setDeletedFiles(res.items || []);
      }
      setNextCursor(res.next_cursor || null);
      setTotalItems(res.total_items);
    } catch (err: any) {
      setError(err.message || 'Failed to load deleted files index');
    } finally {
      setLoading(false);
      setLoadingMore(false);
    }
  };

  const handleInitiateRestore = async (item: DeletedFileItem) => {
    setRestoringPath(item.path);
    setError(null);
    try {
      const history = await api.getBrowseHistory(selectedFolder, item.path, { limit: 20 });
      // Find the most recent non-tombstone version in history
      const candidate = history.items.find((h) => h.kind !== 3);
      if (!candidate) {
        throw new Error('No restorable content version found in history for this path');
      }

      const prev = await api.previewRestore(selectedFolder, item.path, {
        Folder: selectedFolder,
        Author: candidate.author_id,
        Counter: candidate.counter,
      });
      setRestorePreview(prev);
    } catch (err: any) {
      setError(err.message || 'Failed to prepare restore preview');
      setRestoringPath(null);
    }
  };

  const handleRestoreSuccess = (newVersion: VersionID) => {
    setNotification(
      `Restored ${restoringPath}! Created new causal version ${formatVersionID(newVersion)}.`
    );
    setRestorePreview(null);
    setRestoringPath(null);
    if (selectedFolder) {
      loadDeletedFiles(selectedFolder, false);
      onRefresh();
    }
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '1.25rem' }}>
      
      {/* Notifications / Errors */}
      {error && (
        <div className="alert alert-danger" style={{ padding: '0.75rem 1rem', backgroundColor: 'var(--status-danger-bg)', border: '1px solid var(--status-danger)', borderRadius: 'var(--radius-sm)', color: 'var(--status-danger)', fontSize: '0.875rem' }}>
          <div>{error}</div>
        </div>
      )}

      {notification && (
        <div style={{ padding: '0.75rem 1rem', backgroundColor: 'var(--status-success-bg)', border: '1px solid var(--status-success)', borderRadius: 'var(--radius-sm)', color: 'var(--status-success)', fontSize: '0.875rem' }}>
          <div>{notification}</div>
        </div>
      )}

      {/* Header Info */}
      <div style={{ padding: '1rem 1.25rem', backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)', display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: '1rem' }}>
        <div>
          <h2 style={{ fontSize: '1.125rem', fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>
            Deleted Files
          </h2>
          <p style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', margin: '0.25rem 0 0 0' }}>
            Historical deletion index. Restoring creates a fresh causal version from durable history without resurrecting old tombstones.
          </p>
        </div>

        {folders.length > 1 && (
          <select
            value={selectedFolder}
            onChange={(e) => setSelectedFolder(e.target.value)}
            style={{
              padding: '0.375rem 0.75rem',
              backgroundColor: 'var(--surface-raised)',
              border: '1px solid var(--border)',
              borderRadius: 'var(--radius-sm)',
              color: 'var(--text-primary)',
              fontSize: '0.8125rem',
            }}
          >
            {folders.map((f) => (
              <option key={f.folder || f.folder_id} value={f.folder || f.folder_id}>
                {f.root_path || f.folder || f.folder_id}
              </option>
            ))}
          </select>
        )}
      </div>

      {/* Deleted Files Table */}
      <div style={{ backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)', overflow: 'hidden' }}>
        
        {loading && (
          <div style={{ padding: '3rem', textAlign: 'center', color: 'var(--text-secondary)' }}>
            <span className="spinner" style={{ width: '1.5rem', height: '1.5rem', border: '2px solid var(--border)', borderTopColor: 'var(--text-primary)', borderRadius: '50%', animation: 'spin 0.8s linear infinite', display: 'inline-block', marginBottom: '0.5rem' }} />
            <div>Querying deletion history…</div>
          </div>
        )}

        {!loading && deletedFiles.length === 0 && (
          <div style={{ padding: '4rem 2rem', textAlign: 'center' }}>
            <div style={{ width: '3.5rem', height: '3.5rem', borderRadius: '50%', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', display: 'inline-flex', alignItems: 'center', justifyContent: 'center', marginBottom: '1rem' }}>
              <svg style={{ width: '1.75rem', height: '1.75rem', color: 'var(--text-secondary)' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <polyline points="3 6 5 6 21 6" />
                <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
              </svg>
            </div>
            <h3 style={{ fontSize: '1.125rem', fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>
              No Deleted Files
            </h3>
            <p style={{ fontSize: '0.875rem', color: 'var(--text-secondary)', marginTop: '0.5rem', maxWidth: '400px', margin: '0.5rem auto 0 auto' }}>
              Files that are deleted from your synchronized folders will appear here as retained historical records available for conditional restore.
            </p>
          </div>
        )}

        {!loading && deletedFiles.length > 0 && (
          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left', fontSize: '0.875rem' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid var(--border)', backgroundColor: 'var(--surface-raised)', color: 'var(--text-secondary)', fontSize: '0.75rem', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                  <th style={{ padding: '0.75rem 1.25rem', fontWeight: 600 }}>Path</th>
                  <th style={{ padding: '0.75rem 1rem', fontWeight: 600 }}>Last Size</th>
                  <th style={{ padding: '0.75rem 1rem', fontWeight: 600 }}>Deleted At</th>
                  <th style={{ padding: '0.75rem 1rem', fontWeight: 600 }}>Content Bytes</th>
                  <th style={{ padding: '0.75rem 1.25rem', textAlign: 'right', fontWeight: 600 }}>Action</th>
                </tr>
              </thead>
              <tbody>
                {deletedFiles.map((f) => (
                  <tr key={f.path} style={{ borderBottom: '1px solid var(--border)' }} className="file-row">
                    <td style={{ padding: '0.75rem 1.25rem' }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
                        <svg style={{ width: '1.25rem', height: '1.25rem', color: 'var(--status-danger)', flexShrink: 0 }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                          <line x1="18" y1="6" x2="6" y2="18" />
                          <line x1="6" y1="6" x2="18" y2="18" />
                        </svg>
                        <span style={{ fontWeight: 500, color: 'var(--text-primary)', textDecoration: 'line-through' }}>
                          {f.path}
                        </span>
                      </div>
                    </td>
                    <td style={{ padding: '0.75rem 1rem', color: 'var(--text-secondary)' }}>
                      {f.last_active_size ? formatBytes(f.last_active_size) : '—'}
                    </td>
                    <td style={{ padding: '0.75rem 1rem', color: 'var(--text-secondary)', fontSize: '0.8125rem' }}>
                      {f.deleted_at || 'Recorded'}
                      {f.deleted_by && (
                        <span style={{ display: 'block', fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                          by {formatShortID(f.deleted_by)}
                        </span>
                      )}
                    </td>
                    <td style={{ padding: '0.75rem 1rem' }}>
                      {f.cas_available ? (
                        <span style={{ fontSize: '0.75rem', padding: '0.125rem 0.375rem', borderRadius: 'var(--radius-sm)', backgroundColor: 'var(--status-success-bg)', color: 'var(--status-success)', border: '1px solid var(--status-success)' }}>
                          Available in local CAS
                        </span>
                      ) : (
                        <span style={{ fontSize: '0.75rem', padding: '0.125rem 0.375rem', borderRadius: 'var(--radius-sm)', backgroundColor: 'var(--surface-raised)', color: 'var(--text-muted)', border: '1px solid var(--border)' }}>
                          Bytes unavailable
                        </span>
                      )}
                    </td>
                    <td style={{ padding: '0.75rem 1.25rem', textAlign: 'right' }}>
                      <button
                        type="button"
                        id={`btn-restore-file-${f.path.replace(/[^a-zA-Z0-9]/g, '-')}`}
                        onClick={() => handleInitiateRestore(f)}
                        disabled={!f.cas_available}
                        title={f.cas_available ? 'Restore this file' : 'Historical bytes are not present in local CAS'}
                        className="btn btn-secondary btn-sm btn-restore-file"
                        style={{
                          padding: '0.375rem 0.75rem',
                          fontSize: '0.75rem',
                          fontWeight: 500,
                          backgroundColor: 'var(--surface-raised)',
                          border: '1px solid var(--border)',
                          borderRadius: 'var(--radius-sm)',
                          color: 'var(--text-primary)',
                          cursor: f.cas_available ? 'pointer' : 'not-allowed',
                          opacity: f.cas_available ? 1 : 0.5,
                        }}
                      >
                        Restore File
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>

            {nextCursor && (
              <div style={{ padding: '1rem', textAlign: 'center', borderTop: '1px solid var(--border)' }}>
                <button
                  type="button"
                  onClick={() => selectedFolder && loadDeletedFiles(selectedFolder, true)}
                  disabled={loadingMore}
                  className="btn btn-secondary btn-sm"
                  style={{ padding: '0.5rem 1rem' }}
                >
                  {loadingMore ? 'Loading more…' : `Load more (${deletedFiles.length} of ${totalItems})`}
                </button>
              </div>
            )}
          </div>
        )}
      </div>

      {/* Restore Modal */}
      {restorePreview && (
        <RestoreModal
          folder={selectedFolder}
          preview={restorePreview}
          onClose={() => {
            setRestorePreview(null);
            setRestoringPath(null);
          }}
          onSuccess={handleRestoreSuccess}
        />
      )}
    </div>
  );
};
