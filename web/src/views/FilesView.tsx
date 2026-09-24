import React, { useState, useEffect } from 'react';
import { FolderRecord, FileItem, HistoryItem, RestorePreview, VersionID } from '../types';
import { api, formatBytes, formatShortID, formatVersionID, formatTimestamp } from '../api';
import { RestoreModal } from '../components/RestoreModal';

interface FilesViewProps {
  folders: FolderRecord[];
}

export const FilesView: React.FC<FilesViewProps> = ({ folders }) => {
  const [selectedFolder, setSelectedFolder] = useState<string>(folders[0]?.folder || folders[0]?.folder_id || '');
  const [files, setFiles] = useState<FileItem[]>([]);
  const [searchTerm, setSearchTerm] = useState('');
  const [loadingFiles, setLoadingFiles] = useState(false);
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [historyItems, setHistoryItems] = useState<HistoryItem[]>([]);
  const [loadingHistory, setLoadingHistory] = useState(false);
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
    loadFiles(selectedFolder);
  }, [selectedFolder]);

  const loadFiles = async (folder: string) => {
    setLoadingFiles(true);
    setError(null);
    try {
      const res = await api.getFiles(folder);
      setFiles(res);
    } catch (err: any) {
      setError(err.message || 'Failed to load files');
    } finally {
      setLoadingFiles(false);
    }
  };

  const handleSelectPath = async (path: string) => {
    setSelectedPath(path);
    setLoadingHistory(true);
    setError(null);
    try {
      const hist = await api.getFileHistory(selectedFolder, path);
      setHistoryItems(hist);
    } catch (err: any) {
      setError(err.message || 'Failed to load history');
    } finally {
      setLoadingHistory(false);
    }
  };

  const handleOpenRestore = async (item: HistoryItem) => {
    setError(null);
    try {
      const prev = await api.previewRestore(selectedFolder, item.path, item.id);
      setRestorePreview(prev);
    } catch (err: any) {
      setError(err.message || 'Failed to preview restore');
    }
  };

  const handleRestoreSuccess = (newVersion: VersionID) => {
    setNotification(
      `Restored ${selectedPath}! Created new causal version ${formatVersionID(newVersion)} extending current DAG.`
    );
    setRestorePreview(null);
    if (selectedFolder && selectedPath) {
      loadFiles(selectedFolder);
      handleSelectPath(selectedPath);
    }
  };

  const filteredFiles = files.filter((f) =>
    f.path.toLowerCase().includes(searchTerm.toLowerCase())
  );

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '1.5rem' }}>
      {error && (
        <div className="storage-banner danger" role="alert">
          <div>{error}</div>
          <button type="button" className="btn btn-secondary btn-sm" onClick={() => setError(null)}>✕</button>
        </div>
      )}
      {notification && (
        <div className="storage-banner" style={{ backgroundColor: 'var(--status-success-bg)', border: '1px solid rgba(52, 211, 153, 0.4)', color: '#d1fae5' }} role="status">
          <div>{notification}</div>
          <button type="button" className="btn btn-secondary btn-sm" onClick={() => setNotification(null)}>✕</button>
        </div>
      )}

      {/* TOP CONTROLS: FOLDER SELECTOR & SEARCH */}
      <div style={{ display: 'flex', gap: '1rem', flexWrap: 'wrap', alignItems: 'center' }}>
        <div style={{ minWidth: '220px' }}>
          <label htmlFor="files-folder-select" style={{ display: 'block', fontSize: '0.75rem', color: 'var(--text-muted)', marginBottom: '0.25rem' }}>
            Active Folder
          </label>
          <select
            id="files-folder-select"
            className="form-select code-font"
            value={selectedFolder}
            onChange={(e) => {
              setSelectedFolder(e.target.value);
              setSelectedPath(null);
              setHistoryItems([]);
            }}
          >
            {folders.map((f) => {
              const fid = f.folder || f.folder_id || '';
              return (
                <option key={fid} value={fid}>
                  {formatShortID(fid)} ({f.root_path || 'no root'})
                </option>
              );
            })}
          </select>
        </div>

        <div style={{ flex: 1, minWidth: '200px' }}>
          <label htmlFor="file-search-input" style={{ display: 'block', fontSize: '0.75rem', color: 'var(--text-muted)', marginBottom: '0.25rem' }}>
            Filter by Path
          </label>
          <input
            id="file-search-input"
            type="search"
            className="form-input"
            placeholder="Type path or filename…"
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
          />
        </div>

        <div style={{ alignSelf: 'flex-end' }}>
          <button
            type="button"
            className="btn btn-secondary"
            onClick={() => selectedFolder && loadFiles(selectedFolder)}
            disabled={loadingFiles}
          >
            {loadingFiles ? 'Refreshing…' : 'Refresh'}
          </button>
        </div>
      </div>

      {/* TWO COLUMN / STACKED LAYOUT: FILE TABLE AND HISTORY DRAWER */}
      <div style={{ display: 'grid', gridTemplateColumns: selectedPath ? '1fr 1fr' : '1fr', gap: '1.5rem', alignItems: 'start' }}>
        {/* FILE TABLE */}
        <div className="panel-card">
          <div className="panel-header">
            <h2 className="panel-title">
              Workspace Files ({filteredFiles.length})
            </h2>
          </div>

          {loadingFiles ? (
            <div className="empty-state">
              <div className="spinner" />
              <p>Loading files…</p>
            </div>
          ) : filteredFiles.length === 0 ? (
            <div className="empty-state">
              <p>No active files found in this workspace folder.</p>
            </div>
          ) : (
            <div className="table-wrapper" style={{ maxHeight: '600px', overflowY: 'auto' }}>
              <table className="data-table">
                <thead>
                  <tr>
                    <th>Workspace Path</th>
                    <th>Size</th>
                    <th>Kind</th>
                    <th>Status</th>
                    <th>Action</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredFiles.map((file) => (
                    <tr
                      key={file.path}
                      style={{
                        backgroundColor: selectedPath === file.path ? 'rgba(56, 189, 248, 0.08)' : undefined,
                      }}
                    >
                      <td className="code-font" style={{ fontWeight: 500 }}>
                        <span title={file.path}>{file.path}</span>
                        {file.executable && (
                          <span className="badge badge-info" style={{ marginLeft: '0.5rem' }}>
                            +x
                          </span>
                        )}
                      </td>
                      <td>{formatBytes(file.size)}</td>
                      <td>
                        <span className="badge badge-neutral">
                          {file.kind === 2 ? 'DIR' : 'FILE'}
                        </span>
                      </td>
                      <td>
                        {file.block_reason ? (
                          <span className="badge badge-danger" title={file.block_reason}>
                            BLOCKED ({file.block_reason})
                          </span>
                        ) : (
                          <span className="badge badge-success">OK</span>
                        )}
                      </td>
                      <td>
                        <button
                          type="button"
                          className="btn btn-secondary btn-sm"
                          onClick={() => handleSelectPath(file.path)}
                        >
                          History
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>

        {/* VERSION HISTORY PANEL */}
        {selectedPath && (
          <div className="panel-card">
            <div className="panel-header">
              <div>
                <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>File History:</span>
                <h3 className="panel-title code-font" style={{ fontSize: '0.9375rem' }}>
                  {selectedPath}
                </h3>
              </div>
              <button
                type="button"
                className="btn btn-secondary btn-sm"
                onClick={() => setSelectedPath(null)}
                aria-label="Close history"
              >
                ✕ Close
              </button>
            </div>

            {loadingHistory ? (
              <div className="empty-state">
                <div className="spinner" />
                <p>Loading history…</p>
              </div>
            ) : historyItems.length === 0 ? (
              <div className="empty-state">
                <p>No historical versions recorded for this path.</p>
              </div>
            ) : (
              <div className="table-wrapper" style={{ maxHeight: '600px', overflowY: 'auto' }}>
                <table className="data-table">
                  <thead>
                    <tr>
                      <th>Version (Author:Counter)</th>
                      <th>Time</th>
                      <th>Kind &amp; Size</th>
                      <th>Content Availability</th>
                      <th>Action</th>
                    </tr>
                  </thead>
                  <tbody>
                    {historyItems.map((item, idx) => (
                      <tr key={idx}>
                        <td className="code-font" style={{ fontWeight: 600 }}>
                          <div>{formatVersionID(item.id)}</div>
                          <div style={{ display: 'flex', gap: '0.25rem', marginTop: '0.25rem' }}>
                            {item.is_head && <span className="badge badge-info">HEAD</span>}
                            {item.applied && <span className="badge badge-success">APPLIED</span>}
                          </div>
                        </td>
                        <td style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                          {formatTimestamp(item.display_time)}
                        </td>
                        <td>
                          {item.kind === 3 ? (
                            <span className="badge badge-danger">DELETED</span>
                          ) : (
                            <span>
                              {formatBytes(item.manifest?.size ?? item.manifest?.Size ?? 0)}{' '}
                              {item.manifest?.executable || item.manifest?.Executable ? '(+x)' : ''}
                            </span>
                          )}
                        </td>
                        <td>
                          <span
                            className={`badge ${
                              item.content_state === 'ready'
                                ? 'badge-success'
                                : item.content_state === 'expired'
                                ? 'badge-neutral'
                                : 'badge-danger'
                            }`}
                            title={
                              item.content_state === 'expired'
                                ? 'Payload expired under retention policy'
                                : item.content_state === 'unavailable'
                                ? 'Quarantined or missing payload'
                                : 'Verified payload present locally'
                            }
                          >
                            {item.content_state.toUpperCase()}
                          </span>
                        </td>
                        <td>
                          <button
                            type="button"
                            className="btn btn-primary btn-sm"
                            onClick={() => handleOpenRestore(item)}
                            title={
                              item.content_state !== 'ready'
                                ? 'Content payload is not available locally'
                                : 'Restore this historical version'
                            }
                            disabled={item.content_state !== 'ready'}
                          >
                            Restore
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )}
      </div>

      {/* RESTORE PREVIEW MODAL */}
      {restorePreview && (
        <RestoreModal
          folder={selectedFolder}
          preview={restorePreview}
          onClose={() => setRestorePreview(null)}
          onSuccess={handleRestoreSuccess}
        />
      )}
    </div>
  );
};
