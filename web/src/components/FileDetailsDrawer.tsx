import React, { useState, useEffect, useCallback } from 'react';
import {
  FileDetails,
  FileHistoryResult,
  FileHistoryItem,
  FileVersionDetail,
} from '../types';
import {
  api,
  formatBytes,
  formatShortID,
  formatTimestamp,
  APIError,
} from '../api';

interface FileDetailsDrawerProps {
  folder: string;
  path: string | null;
  onClose: () => void;
  onOpenFolder?: (path: string) => void;
  onRestorePreview?: (item: FileHistoryItem) => void;
  onMove?: (path: string, isDir: boolean) => void;
  onDelete?: (path: string, isDir: boolean, size?: number) => void;
}

export const FileDetailsDrawer: React.FC<FileDetailsDrawerProps> = ({
  folder,
  path,
  onClose,
  onOpenFolder,
  onRestorePreview,
  onMove,
  onDelete,
}) => {
  const [details, setDetails] = useState<FileDetails | null>(null);
  const [loadingDetails, setLoadingDetails] = useState(false);
  const [errorDetails, setErrorDetails] = useState<string | null>(null);

  // Preview state
  const [textPreview, setTextPreview] = useState<string | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [previewError, setPreviewError] = useState<string | null>(null);

  // History timeline state
  const [historyResult, setHistoryResult] = useState<FileHistoryResult | null>(null);
  const [loadingHistory, setLoadingHistory] = useState(false);
  const [loadingMoreHistory, setLoadingMoreHistory] = useState(false);

  // Active tab in drawer: 'details' | 'preview' | 'history'
  const [activeTab, setActiveTab] = useState<'details' | 'preview' | 'history'>('details');

  // Copy feedback
  const [copiedPath, setCopiedPath] = useState(false);

  // Close on Escape
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [onClose]);

  const loadData = useCallback(async (currentFolder: string, currentPath: string) => {
    setLoadingDetails(true);
    setErrorDetails(null);
    setTextPreview(null);
    setPreviewError(null);
    setHistoryResult(null);

    try {
      const d = await api.getFileDetails(currentFolder, currentPath);
      setDetails(d);

      // Auto-select tab: if it's a file, default to 'preview' or 'details'
      if (!d.is_dir) {
        // Load initial preview if it's text or raster
        loadPreviewForFile(currentFolder, d);
      } else {
        setActiveTab('details');
      }

      // Also load initial history
      loadHistory(currentFolder, currentPath);
    } catch (err: any) {
      if (err instanceof APIError) {
        setErrorDetails(`${err.code}: ${err.message}. ${err.action || ''}`);
      } else {
        setErrorDetails(err.message || 'Failed to load item details');
      }
    } finally {
      setLoadingDetails(false);
    }
  }, []);

  useEffect(() => {
    if (!folder || !path) {
      setDetails(null);
      return;
    }
    loadData(folder, path);
  }, [folder, path, loadData]);

  const isTextFile = (name: string): boolean => {
    const lower = name.toLowerCase();
    const exts = [
      '.txt', '.md', '.markdown', '.go', '.ts', '.tsx', '.js', '.jsx', '.json',
      '.css', '.html', '.py', '.sh', '.bash', '.yaml', '.yml', '.toml', '.rs',
      '.c', '.h', '.cpp', '.sql', '.log', '.csv', '.env', '.xml', '.svg',
    ];
    return exts.some((ext) => lower.endsWith(ext)) || !lower.includes('.');
  };

  const isRasterImage = (name: string): boolean => {
    const lower = name.toLowerCase();
    return lower.endsWith('.png') || lower.endsWith('.jpg') || lower.endsWith('.jpeg') || lower.endsWith('.gif');
  };

  const loadPreviewForFile = async (currentFolder: string, d: FileDetails) => {
    if (d.is_dir || d.heads.length === 0) return;
    const primaryHead = d.heads[0];

    if (!primaryHead.cas_available) {
      setPreviewError('Content bytes are not locally available in CAS to preview');
      return;
    }

    if (isTextFile(d.name)) {
      if (d.size > 1024 * 1024) {
        setPreviewError('File exceeds plain text preview limit (1 MiB). Download file to view.');
        return;
      }
      setPreviewLoading(true);
      setPreviewError(null);
      try {
        const text = await api.fetchTextPreview(currentFolder, primaryHead.author_id, primaryHead.counter);
        setTextPreview(text);
      } catch (err: any) {
        setPreviewError(err.message || 'Unable to preview text');
      } finally {
        setPreviewLoading(false);
      }
    } else if (isRasterImage(d.name)) {
      if (d.size > 10 * 1024 * 1024) {
        setPreviewError('Image exceeds raster preview limit (10 MiB). Download file to view.');
        return;
      }
      // Image renders directly via <img> pointing to /api/v1/content?preview=raster
    } else {
      setPreviewError('Inline preview is not supported for this file type. Download the exact version to view.');
    }
  };

  const loadHistory = async (currentFolder: string, currentPath: string, cursor?: string) => {
    if (cursor) {
      setLoadingMoreHistory(true);
    } else {
      setLoadingHistory(true);
    }
    try {
      const res = await api.getBrowseHistory(currentFolder, currentPath, { limit: 20, cursor });
      if (cursor && historyResult) {
        setHistoryResult({
          ...res,
          items: [...historyResult.items, ...res.items],
        });
      } else {
        setHistoryResult(res);
      }
    } catch {
      // Historical versions might be empty
    } finally {
      setLoadingHistory(false);
      setLoadingMoreHistory(false);
    }
  };

  const handleCopyPath = () => {
    if (!path) return;
    navigator.clipboard.writeText(path);
    setCopiedPath(true);
    setTimeout(() => setCopiedPath(false), 2000);
  };

  const handleDownloadHead = (head: FileVersionDetail) => {
    if (!folder || !details) return;
    api.downloadContent(folder, head.author_id, head.counter, details.name);
  };

  const handleDownloadHistoryItem = (item: FileHistoryItem) => {
    if (!folder || !details) return;
    api.downloadContent(folder, item.author_id, item.counter, details.name);
  };

  if (!path) return null;

  const primaryHead = details?.heads?.[0];
  const hasConflict = !!(details?.has_conflict || (details?.heads && details.heads.length > 1) || details?.structural_conflict);

  return (
    <aside
      className="file-details-drawer"
      aria-label="Item details and preview panel"
      style={{
        width: '380px',
        backgroundColor: 'var(--surface)',
        borderLeft: '1px solid var(--border)',
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        minHeight: '100%',
        overflowY: 'auto',
        position: 'relative',
        zIndex: 40,
        boxShadow: '-4px 0 16px rgba(0,0,0,0.25)',
      }}
    >
      {/* Header */}
      <div
        style={{
          padding: '1rem 1.25rem',
          borderBottom: '1px solid var(--border)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          backgroundColor: 'var(--surface)',
          position: 'sticky',
          top: 0,
          zIndex: 10,
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', overflow: 'hidden' }}>
          <span style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)' }}>
            {details?.is_dir ? 'Directory Details' : 'File Details'}
          </span>
        </div>
        <button
          type="button"
          onClick={onClose}
          aria-label="Close details panel"
          style={{
            background: 'transparent',
            border: 'none',
            color: 'var(--text-secondary)',
            fontSize: '1rem',
            cursor: 'pointer',
            padding: '0.25rem 0.5rem',
            borderRadius: 'var(--radius-sm)',
          }}
        >
          ✕
        </button>
      </div>

      {/* Tabs: Details, Preview, History */}
      <div
        style={{
          display: 'flex',
          borderBottom: '1px solid var(--border)',
          backgroundColor: 'var(--surface-raised)',
          padding: '0 0.5rem',
        }}
      >
        <button
          type="button"
          onClick={() => setActiveTab('details')}
          style={{
            flex: 1,
            padding: '0.625rem 0.5rem',
            background: 'transparent',
            border: 'none',
            borderBottom: activeTab === 'details' ? '2px solid var(--text-primary)' : '2px solid transparent',
            color: activeTab === 'details' ? 'var(--text-primary)' : 'var(--text-secondary)',
            fontSize: '0.8125rem',
            fontWeight: 500,
            cursor: 'pointer',
          }}
        >
          Details
        </button>
        {!details?.is_dir && (
          <button
            type="button"
            onClick={() => setActiveTab('preview')}
            style={{
              flex: 1,
              padding: '0.625rem 0.5rem',
              background: 'transparent',
              border: 'none',
              borderBottom: activeTab === 'preview' ? '2px solid var(--text-primary)' : '2px solid transparent',
              color: activeTab === 'preview' ? 'var(--text-primary)' : 'var(--text-secondary)',
              fontSize: '0.8125rem',
              fontWeight: 500,
              cursor: 'pointer',
            }}
          >
            Preview
          </button>
        )}
        <button
          type="button"
          onClick={() => setActiveTab('history')}
          style={{
            flex: 1,
            padding: '0.625rem 0.5rem',
            background: 'transparent',
            border: 'none',
            borderBottom: activeTab === 'history' ? '2px solid var(--text-primary)' : '2px solid transparent',
            color: activeTab === 'history' ? 'var(--text-primary)' : 'var(--text-secondary)',
            fontSize: '0.8125rem',
            fontWeight: 500,
            cursor: 'pointer',
          }}
        >
          History
        </button>
      </div>

      {/* Body Content */}
      <div style={{ padding: '1.25rem', flex: 1, display: 'flex', flexDirection: 'column', gap: '1.25rem' }}>
        
        {loadingDetails && (
          <div style={{ textAlign: 'center', padding: '3rem 1rem', color: 'var(--text-secondary)' }}>
            <span
              className="spinner"
              style={{
                width: '1.5rem',
                height: '1.5rem',
                border: '2px solid var(--border)',
                borderTopColor: 'var(--text-primary)',
                borderRadius: '50%',
                animation: 'spin 0.8s linear infinite',
                marginBottom: '0.5rem',
              }}
            />
            <div>Loading details…</div>
          </div>
        )}

        {errorDetails && (
          <div
            className="alert alert-danger"
            style={{
              padding: '0.75rem 1rem',
              backgroundColor: 'var(--status-danger-bg)',
              border: '1px solid var(--status-danger)',
              borderRadius: 'var(--radius-sm)',
              color: 'var(--status-danger)',
              fontSize: '0.8125rem',
            }}
          >
            {errorDetails}
          </div>
        )}

        {!loadingDetails && details && (
          <>
            {/* Identity Card */}
            <div
              style={{
                padding: '1rem',
                backgroundColor: 'var(--surface-raised)',
                border: '1px solid var(--border)',
                borderRadius: 'var(--radius-sm)',
                display: 'flex',
                flexDirection: 'column',
                gap: '0.5rem',
              }}
            >
              <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '0.5rem' }}>
                <h3
                  title={details.name}
                  style={{
                    fontSize: '1rem',
                    fontWeight: 600,
                    color: 'var(--text-primary)',
                    wordBreak: 'break-word',
                    lineHeight: 1.3,
                  }}
                >
                  {details.name}
                </h3>
                {details.executable && (
                  <span
                    style={{
                      fontSize: '0.6875rem',
                      padding: '0.125rem 0.375rem',
                      borderRadius: 'var(--radius-sm)',
                      backgroundColor: 'var(--surface)',
                      border: '1px solid var(--border)',
                      color: 'var(--text-secondary)',
                      whiteSpace: 'nowrap',
                    }}
                  >
                    executable
                  </span>
                )}
              </div>

              {/* Full relative path */}
              <div
                style={{
                  fontSize: '0.75rem',
                  color: 'var(--text-secondary)',
                  wordBreak: 'break-all',
                  display: 'flex',
                  alignItems: 'center',
                  gap: '0.375rem',
                }}
              >
                <span className="code-font" style={{ flex: 1 }}>{details.path}</span>
                <button
                  type="button"
                  onClick={handleCopyPath}
                  title="Copy relative path"
                  style={{
                    background: 'transparent',
                    border: 'none',
                    color: copiedPath ? 'var(--status-success)' : 'var(--text-secondary)',
                    cursor: 'pointer',
                    fontSize: '0.75rem',
                    padding: '0.125rem 0.25rem',
                  }}
                >
                  {copiedPath ? '✓ Copied' : 'Copy'}
                </button>
              </div>

              {/* Working State Badge */}
              <div style={{ marginTop: '0.25rem' }}>
                <WorkingStateBadge
                  state={details.working_state}
                  contentState={details.content_state}
                  hasConflict={hasConflict}
                  blockReason={details.block_reason}
                />
              </div>
            </div>

            {/* TAB 1: DETAILS */}
            {activeTab === 'details' && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '1.25rem' }}>
                {/* Conflict Banner */}
                {hasConflict && (
                  <div
                    style={{
                      padding: '0.875rem 1rem',
                      backgroundColor: 'var(--status-danger-bg)',
                      border: '1px solid var(--status-danger)',
                      borderRadius: 'var(--radius-sm)',
                      color: 'var(--text-primary)',
                      fontSize: '0.8125rem',
                    }}
                  >
                    <div style={{ fontWeight: 600, color: 'var(--status-danger)', marginBottom: '0.25rem' }}>
                      ⚠ Needs Review: Conflicting Versions Recorded
                    </div>
                    <div style={{ color: 'var(--text-secondary)', lineHeight: 1.4 }}>
                      Multiple competing versions exist across replicas. Use the Needs Attention view or Restore history to review choices without silent data loss.
                    </div>
                    {details.structural_conflict && (
                      <div style={{ marginTop: '0.375rem', fontStyle: 'italic', color: 'var(--text-primary)' }}>
                        Structural: {details.structural_conflict}
                      </div>
                    )}
                  </div>
                )}

                {/* Primary Download / Action */}
                {!details.is_dir && primaryHead && (
                  <div style={{ display: 'flex', gap: '0.5rem' }}>
                    <button
                      type="button"
                      onClick={() => handleDownloadHead(primaryHead)}
                      className="btn btn-primary"
                      style={{
                        flex: 1,
                        padding: '0.5rem 0.875rem',
                        fontSize: '0.8125rem',
                      }}
                    >
                      <svg
                        style={{ width: '1rem', height: '1rem' }}
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        strokeWidth="2"
                        strokeLinecap="round"
                        strokeLinejoin="round"
                      >
                        <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
                        <polyline points="7 10 12 15 17 10" />
                        <line x1="12" y1="15" x2="12" y2="3" />
                      </svg>
                      Download File
                    </button>
                  </div>
                )}

                {details.is_dir && onOpenFolder && (
                  <button
                    type="button"
                    onClick={() => onOpenFolder(details.path)}
                    className="btn btn-secondary"
                    style={{
                      padding: '0.5rem 0.875rem',
                      fontSize: '0.8125rem',
                      width: '100%',
                    }}
                  >
                    Open Directory in Browser
                  </button>
                )}

                {/* File / Directory Actions: Move/Rename, Delete */}
                <div style={{ display: 'flex', gap: '0.5rem' }}>
                  {onMove && (
                    <button
                      type="button"
                      id="drawer-btn-move"
                      onClick={() => onMove(details.path, details.is_dir)}
                      className="btn btn-secondary btn-sm"
                      style={{ flex: 1, padding: '0.45rem', fontSize: '0.75rem' }}
                    >
                      Rename / Move
                    </button>
                  )}
                  {onDelete && (
                    <button
                      type="button"
                      id="drawer-btn-delete"
                      onClick={() => onDelete(details.path, details.is_dir, details.size)}
                      className="btn btn-secondary btn-sm"
                      style={{ flex: 1, padding: '0.45rem', fontSize: '0.75rem', color: 'var(--status-danger)' }}
                    >
                      Delete
                    </button>
                  )}
                </div>

                {/* Technical Metadata Table */}
                <div
                  style={{
                    backgroundColor: 'var(--surface-raised)',
                    border: '1px solid var(--border)',
                    borderRadius: 'var(--radius-sm)',
                    overflow: 'hidden',
                  }}
                >
                  <div
                    style={{
                      padding: '0.625rem 0.875rem',
                      borderBottom: '1px solid var(--border)',
                      fontSize: '0.75rem',
                      fontWeight: 600,
                      color: 'var(--text-secondary)',
                      textTransform: 'uppercase',
                      letterSpacing: '0.05em',
                    }}
                  >
                    Technical Metadata
                  </div>
                  <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '0.8125rem' }}>
                    <tbody>
                      <tr style={{ borderBottom: '1px solid var(--border)' }}>
                        <td style={{ padding: '0.5rem 0.875rem', color: 'var(--text-secondary)', width: '40%' }}>Type</td>
                        <td style={{ padding: '0.5rem 0.875rem', color: 'var(--text-primary)', fontWeight: 500 }}>
                          {details.is_dir ? 'Directory' : 'Regular File'}
                        </td>
                      </tr>
                      {!details.is_dir && (
                        <tr style={{ borderBottom: '1px solid var(--border)' }}>
                          <td style={{ padding: '0.5rem 0.875rem', color: 'var(--text-secondary)' }}>Size</td>
                          <td style={{ padding: '0.5rem 0.875rem', color: 'var(--text-primary)' }}>
                            {formatBytes(details.size)}{' '}
                            <span style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
                              ({details.size.toLocaleString()} B)
                            </span>
                          </td>
                        </tr>
                      )}
                      <tr style={{ borderBottom: '1px solid var(--border)' }}>
                        <td style={{ padding: '0.5rem 0.875rem', color: 'var(--text-secondary)' }}>Modified</td>
                        <td style={{ padding: '0.5rem 0.875rem', color: 'var(--text-primary)' }}>
                          {details.mtime_ns ? formatTimestamp(details.mtime_ns) : '—'}
                        </td>
                      </tr>
                      {details.inode > 0 && (
                        <tr style={{ borderBottom: '1px solid var(--border)' }}>
                          <td style={{ padding: '0.5rem 0.875rem', color: 'var(--text-secondary)' }}>Inode</td>
                          <td style={{ padding: '0.5rem 0.875rem', color: 'var(--text-primary)' }}>
                            <span className="code-font">{details.inode}</span>
                          </td>
                        </tr>
                      )}
                      {primaryHead && (
                        <>
                          <tr style={{ borderBottom: '1px solid var(--border)' }}>
                            <td style={{ padding: '0.5rem 0.875rem', color: 'var(--text-secondary)' }}>Exact Version</td>
                            <td style={{ padding: '0.5rem 0.875rem', color: 'var(--text-primary)' }}>
                              <span className="code-font">
                                {formatShortID(primaryHead.author_id)}:{primaryHead.counter}
                              </span>
                            </td>
                          </tr>
                          {primaryHead.file_digest && (
                            <tr>
                              <td style={{ padding: '0.5rem 0.875rem', color: 'var(--text-secondary)' }}>Digest (SHA-256)</td>
                              <td style={{ padding: '0.5rem 0.875rem', color: 'var(--text-primary)' }}>
                                <span className="code-font" title={primaryHead.file_digest}>
                                  {formatShortID(primaryHead.file_digest)}
                                </span>
                              </td>
                            </tr>
                          )}
                        </>
                      )}
                    </tbody>
                  </table>
                </div>

                {/* Device Copies / Peer Replication Progress */}
                {details.peers && details.peers.length > 0 && (
                  <div
                    style={{
                      backgroundColor: 'var(--surface-raised)',
                      border: '1px solid var(--border)',
                      borderRadius: 'var(--radius-sm)',
                      overflow: 'hidden',
                    }}
                  >
                    <div
                      style={{
                        padding: '0.625rem 0.875rem',
                        borderBottom: '1px solid var(--border)',
                        fontSize: '0.75rem',
                        fontWeight: 600,
                        color: 'var(--text-secondary)',
                        textTransform: 'uppercase',
                        letterSpacing: '0.05em',
                      }}
                    >
                      Device Replicas & Copies
                    </div>
                    <div style={{ display: 'flex', flexDirection: 'column' }}>
                      {details.peers.map((peer, idx) => (
                        <div
                          key={idx}
                          style={{
                            padding: '0.625rem 0.875rem',
                            borderBottom: idx < details.peers!.length - 1 ? '1px solid var(--border)' : 'none',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'space-between',
                            gap: '0.5rem',
                            fontSize: '0.8125rem',
                          }}
                        >
                          <div>
                            <div style={{ fontWeight: 500, color: 'var(--text-primary)' }}>
                              {peer.peer_name || formatShortID(peer.peer_id)}
                            </div>
                            <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginTop: '0.125rem' }}>
                              {peer.last_contact_ns > 0
                                ? `Last contact: ${formatTimestamp(peer.last_contact_ns)}`
                                : 'No contact recorded'}
                            </div>
                          </div>
                          <div>
                            {peer.remote_status === 'applied' ? (
                              <span style={{ color: 'var(--status-success)', fontSize: '0.75rem', fontWeight: 500 }}>
                                Updated on device
                              </span>
                            ) : peer.receipt ? (
                              <span style={{ color: 'var(--status-info)', fontSize: '0.75rem', fontWeight: 500 }}>
                                Stored on device
                              </span>
                            ) : (
                              <span style={{ color: 'var(--text-muted)', fontSize: '0.75rem' }}>
                                Awaiting transfer
                              </span>
                            )}
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            )}

            {/* TAB 2: PREVIEW */}
            {activeTab === 'preview' && !details.is_dir && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}>
                {previewLoading && (
                  <div style={{ textAlign: 'center', padding: '2rem', color: 'var(--text-secondary)' }}>
                    Loading preview…
                  </div>
                )}

                {previewError && (
                  <div
                    style={{
                      padding: '1rem',
                      backgroundColor: 'var(--surface-raised)',
                      border: '1px solid var(--border)',
                      borderRadius: 'var(--radius-sm)',
                      textAlign: 'center',
                    }}
                  >
                    <div style={{ fontSize: '0.875rem', color: 'var(--text-secondary)', marginBottom: '0.75rem' }}>
                      {previewError}
                    </div>
                    {primaryHead && (
                      <button
                        type="button"
                        onClick={() => handleDownloadHead(primaryHead)}
                        className="btn btn-secondary btn-sm"
                        style={{ padding: '0.375rem 0.75rem', fontSize: '0.8125rem' }}
                      >
                        Download Exact Version
                      </button>
                    )}
                  </div>
                )}

                {!previewLoading && !previewError && textPreview !== null && (
                  <div>
                    <div
                      style={{
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                      marginBottom: '0.5rem',
                      fontSize: '0.75rem',
                      color: 'var(--text-secondary)',
                    }}
                    >
                      <span>UTF-8 Plain Text Preview (Bounded)</span>
                      <span>{formatBytes(details.size)}</span>
                    </div>
                    <pre
                      tabIndex={0}
                      className="code-font"
                      style={{
                        padding: '0.875rem',
                        backgroundColor: 'var(--surface-raised)',
                        border: '1px solid var(--border)',
                        borderRadius: 'var(--radius-sm)',
                        fontSize: '0.8125rem',
                        lineHeight: 1.4,
                        color: 'var(--text-primary)',
                        maxHeight: '380px',
                        overflow: 'auto',
                        whiteSpace: 'pre-wrap',
                        wordBreak: 'break-word',
                      }}
                    >
                      {textPreview}
                    </pre>
                  </div>
                )}

                {!previewLoading && !previewError && isRasterImage(details.name) && primaryHead && (
                  <div>
                    <div
                      style={{
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'space-between',
                        marginBottom: '0.5rem',
                        fontSize: '0.75rem',
                        color: 'var(--text-secondary)',
                      }}
                    >
                      <span>Raster Image Preview</span>
                      <span>{formatBytes(details.size)}</span>
                    </div>
                    <div
                      style={{
                        padding: '0.5rem',
                        backgroundColor: 'var(--surface-raised)',
                        border: '1px solid var(--border)',
                        borderRadius: 'var(--radius-sm)',
                        textAlign: 'center',
                        maxHeight: '360px',
                        overflow: 'hidden',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                      }}
                    >
                      <img
                        src={api.getContentURL(folder, primaryHead.author_id, primaryHead.counter, 'raster')}
                        alt={details.name}
                        onError={() => setPreviewError('Failed to decode raster preview image')}
                        style={{
                          maxWidth: '100%',
                          maxHeight: '340px',
                          objectFit: 'contain',
                        }}
                      />
                    </div>
                  </div>
                )}
              </div>
            )}

            {/* TAB 3: HISTORY */}
            {activeTab === 'history' && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginBottom: '0.25rem' }}>
                  Recorded causal versions for this path:
                </div>

                {loadingHistory && (
                  <div style={{ textAlign: 'center', padding: '2rem', color: 'var(--text-secondary)' }}>
                    Loading history timeline…
                  </div>
                )}

                {!loadingHistory && (!historyResult || historyResult.items.length === 0) && (
                  <div style={{ textAlign: 'center', padding: '2rem', color: 'var(--text-secondary)' }}>
                    No historical versions recorded for this path.
                  </div>
                )}

                {!loadingHistory && historyResult && historyResult.items.map((item, idx) => {
                  const isTombstone = item.kind === 3;
                  return (
                    <div
                      key={idx}
                      style={{
                        padding: '0.75rem 0.875rem',
                        backgroundColor: 'var(--surface-raised)',
                        border: '1px solid var(--border)',
                        borderRadius: 'var(--radius-sm)',
                        display: 'flex',
                        flexDirection: 'column',
                        gap: '0.375rem',
                      }}
                    >
                      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '0.5rem' }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: '0.375rem' }}>
                          <span className="code-font" style={{ fontWeight: 600, fontSize: '0.8125rem', color: 'var(--text-primary)' }}>
                            {formatShortID(item.author_id)}:{item.counter}
                          </span>
                          {item.is_head && (
                            <span
                              style={{
                                fontSize: '0.625rem',
                                padding: '0.125rem 0.3125rem',
                                borderRadius: 'var(--radius-sm)',
                                backgroundColor: 'var(--status-info-bg)',
                                color: 'var(--status-info)',
                                border: '1px solid var(--status-info)',
                              }}
                            >
                              Head
                            </span>
                          )}
                          {isTombstone && (
                            <span
                              style={{
                                fontSize: '0.625rem',
                                padding: '0.125rem 0.3125rem',
                                borderRadius: 'var(--radius-sm)',
                                backgroundColor: 'var(--status-danger-bg)',
                                color: 'var(--status-danger)',
                                border: '1px solid var(--status-danger)',
                              }}
                            >
                              Deleted
                            </span>
                          )}
                        </div>

                        {/* Actions: Download & Restore */}
                        <div style={{ display: 'flex', alignItems: 'center', gap: '0.375rem' }}>
                          {!isTombstone && item.cas_available && (
                            <button
                              type="button"
                              onClick={() => handleDownloadHistoryItem(item)}
                              title="Download this exact version"
                              className="btn btn-secondary btn-sm"
                              style={{
                                padding: '0.25rem 0.5rem',
                                fontSize: '0.6875rem',
                              }}
                            >
                              Download
                            </button>
                          )}

                          {!item.is_head && !isTombstone && onRestorePreview && item.cas_available && (
                            <button
                              type="button"
                              onClick={() => onRestorePreview(item)}
                              className="btn btn-secondary btn-sm"
                              style={{
                                padding: '0.25rem 0.5rem',
                                fontSize: '0.6875rem',
                              }}
                            >
                              Restore
                            </button>
                          )}
                        </div>
                      </div>

                      <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
                        {item.display_time || 'Recorded'}{' '}
                        {!isTombstone && `• ${formatBytes(item.file_size)}`}
                      </div>

                      <div style={{ fontSize: '0.6875rem', color: item.cas_available ? 'var(--status-success)' : 'var(--text-muted)' }}>
                        {item.cas_available ? '✓ Bytes locally available' : '○ Bytes not in local CAS'}
                      </div>
                    </div>
                  );
                })}

                {historyResult?.next_cursor && (
                  <button
                    type="button"
                    onClick={() => loadHistory(folder, path, historyResult.next_cursor)}
                    disabled={loadingMoreHistory}
                    className="btn btn-secondary btn-sm"
                    style={{ padding: '0.5rem', fontSize: '0.8125rem', marginTop: '0.5rem' }}
                  >
                    {loadingMoreHistory ? 'Loading…' : 'Load earlier versions'}
                  </button>
                )}
              </div>
            )}
          </>
        )}
      </div>
    </aside>
  );
};

// Subcomponent for status badge
interface WorkingStateBadgeProps {
  state: string;
  contentState?: string;
  hasConflict?: boolean;
  blockReason?: string;
}

export const WorkingStateBadge: React.FC<WorkingStateBadgeProps> = ({
  state,
  contentState,
  hasConflict,
  blockReason,
}) => {
  if (hasConflict || state === 'conflict') {
    return (
      <span
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: '0.375rem',
          padding: '0.125rem 0.5rem',
          borderRadius: 'var(--radius-sm)',
          backgroundColor: 'var(--status-danger-bg)',
          color: 'var(--status-danger)',
          border: '1px solid var(--status-danger)',
          fontSize: '0.75rem',
          fontWeight: 600,
        }}
      >
        <span style={{ width: '6px', height: '6px', borderRadius: '50%', backgroundColor: 'var(--status-danger)' }} />
        Needs review (Conflict)
      </span>
    );
  }

  if (state === 'blocked' || blockReason) {
    return (
      <span
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: '0.375rem',
          padding: '0.125rem 0.5rem',
          borderRadius: 'var(--radius-sm)',
          backgroundColor: 'var(--status-danger-bg)',
          color: 'var(--status-danger)',
          border: '1px solid var(--status-danger)',
          fontSize: '0.75rem',
          fontWeight: 500,
        }}
        title={blockReason}
      >
        <span style={{ width: '6px', height: '6px', borderRadius: '50%', backgroundColor: 'var(--status-danger)' }} />
        Blocked: {blockReason || 'Path error'}
      </span>
    );
  }

  if (contentState === 'unavailable') {
    return (
      <span
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: '0.375rem',
          padding: '0.125rem 0.5rem',
          borderRadius: 'var(--radius-sm)',
          backgroundColor: 'rgba(255,255,255,0.08)',
          color: 'var(--text-secondary)',
          border: '1px solid var(--border)',
          fontSize: '0.75rem',
          fontWeight: 500,
        }}
      >
        <span style={{ width: '6px', height: '6px', borderRadius: '50%', backgroundColor: 'var(--text-muted)' }} />
        Content unavailable
      </span>
    );
  }

  if (state === 'pending' || contentState === 'pending') {
    return (
      <span
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: '0.375rem',
          padding: '0.125rem 0.5rem',
          borderRadius: 'var(--radius-sm)',
          backgroundColor: 'var(--status-warning-bg)',
          color: 'var(--status-warning)',
          border: '1px solid var(--status-warning)',
          fontSize: '0.75rem',
          fontWeight: 500,
        }}
      >
        <span style={{ width: '6px', height: '6px', borderRadius: '50%', backgroundColor: 'var(--status-warning)' }} />
        Syncing / Pending content
      </span>
    );
  }

  if (state === 'unobserved' || state === 'saving') {
    return (
      <span
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: '0.375rem',
          padding: '0.125rem 0.5rem',
          borderRadius: 'var(--radius-sm)',
          backgroundColor: 'var(--status-info-bg)',
          color: 'var(--status-info)',
          border: '1px solid var(--status-info)',
          fontSize: '0.75rem',
          fontWeight: 500,
        }}
      >
        <span style={{ width: '6px', height: '6px', borderRadius: '50%', backgroundColor: 'var(--status-info)' }} />
        Saving locally
      </span>
    );
  }

  // Observed / clean durable head
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: '0.375rem',
        padding: '0.125rem 0.5rem',
        borderRadius: 'var(--radius-sm)',
        backgroundColor: 'var(--status-success-bg)',
        color: 'var(--status-success)',
        border: '1px solid var(--status-success)',
        fontSize: '0.75rem',
        fontWeight: 500,
      }}
    >
      <span style={{ width: '6px', height: '6px', borderRadius: '50%', backgroundColor: 'var(--status-success)' }} />
      Saved on this device
    </span>
  );
};
