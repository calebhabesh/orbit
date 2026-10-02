import React, { useState, useEffect } from 'react';
import { ConflictSet, VersionID } from '../types';
import { api, formatBytes, formatShortID, formatVersionID, APIError } from '../api';

interface ConflictResolveModalProps {
  folder: string;
  conflict: ConflictSet;
  peerAliases?: Record<string, string>;
  localAuthor?: string;
  onClose: () => void;
  onResolved: () => void;
  onRefresh?: () => void;
}

export const ConflictResolveModal: React.FC<ConflictResolveModalProps> = ({
  folder,
  conflict: initialConflict,
  peerAliases = {},
  localAuthor = '',
  onClose,
  onResolved,
  onRefresh,
}) => {
  const [currentConflict, setCurrentConflict] = useState<ConflictSet>(initialConflict);
  const [activeTab, setActiveTab] = useState<'select' | 'keep' | 'merge'>('select');
  const [selectedHeadIdx, setSelectedHeadIdx] = useState<number>(0);
  const [mergeContent, setMergeContent] = useState<string>('');
  const [mergeExecutable, setMergeExecutable] = useState<boolean>(false);
  const [submitting, setSubmitting] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setCurrentConflict(initialConflict);
  }, [initialConflict]);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !submitting) {
        onClose();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [submitting, onClose]);

  const reviewedIDs: VersionID[] = currentConflict.heads.map((h) => h.id);

  const handleSelectWinner = async () => {
    setSubmitting(true);
    setError(null);
    try {
      const selected = currentConflict.heads[selectedHeadIdx].id;
      await api.resolveSelect(folder, currentConflict.path, reviewedIDs, currentConflict.head_token, selected);
      onResolved();
    } catch (err: any) {
      await handleAPIError(err);
    } finally {
      setSubmitting(false);
    }
  };

  const handleKeepCopies = async () => {
    setSubmitting(true);
    setError(null);
    try {
      await api.resolveKeepCopies(folder, currentConflict.path, reviewedIDs, currentConflict.head_token);
      onResolved();
    } catch (err: any) {
      await handleAPIError(err);
    } finally {
      setSubmitting(false);
    }
  };

  const handleManualMerge = async () => {
    setSubmitting(true);
    setError(null);
    try {
      await api.resolveManualMerge(
        folder,
        currentConflict.path,
        reviewedIDs,
        currentConflict.head_token,
        mergeExecutable,
        mergeContent
      );
      onResolved();
    } catch (err: any) {
      await handleAPIError(err);
    } finally {
      setSubmitting(false);
    }
  };

  const handleAPIError = async (err: any) => {
    if (err instanceof APIError) {
      if (err.code === 'STALE_VIEW') {
        // Attempt to fetch fresh conflict state from server
        try {
          const fresh = await api.getConflicts(folder);
          const updated = fresh.content_conflicts.find((c) => c.path === currentConflict.path);
          if (updated) {
            setCurrentConflict(updated);
            setSelectedHeadIdx(0);
            setError(
              'STALE_VIEW: Conflict heads were modified concurrently! The reviewed heads have been refreshed below. Please re-review before resolving.'
            );
          } else {
            setError(
              'STALE_VIEW: The conflict on this path was already resolved by another device or process.'
            );
            if (onRefresh) onRefresh();
          }
        } catch {
          setError(
            'STALE_VIEW: Conflict heads changed concurrently. Please refresh the view to re-review.'
          );
        }
      } else {
        setError(`${err.code}: ${err.message}. ${err.action || ''}`);
      }
    } else {
      setError(err.message || 'Operation failed');
    }
  };

  const getAuthorDisplay = (authorId: string): string => {
    if (!authorId) return 'Unknown Device';
    if (peerAliases[authorId]) return peerAliases[authorId];
    if (localAuthor && authorId === localAuthor) return 'This Device';
    return formatShortID(authorId);
  };

  return (
    <div
      className="modal-backdrop"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-labelledby="conflict-modal-title"
    >
      <div
        className="modal-content"
        onClick={(e) => e.stopPropagation()}
        style={{ maxWidth: '780px' }}
      >
        <div className="modal-header">
          <h2 id="conflict-modal-title" className="panel-title">
            Review &amp; Resolve Conflict: <span className="code-font">{currentConflict.path}</span>
          </h2>
          <button
            type="button"
            className="btn btn-secondary btn-sm"
            onClick={onClose}
            aria-label="Close dialog"
          >
            ✕
          </button>
        </div>

        <div className="modal-body" style={{ display: 'flex', flexDirection: 'column', gap: '0.875rem' }}>
          {error && (
            <div className="storage-banner danger" role="alert">
              <div>{error}</div>
            </div>
          )}

          {/* Context Notice (Invariant: device names and times as context, never winner rules) */}
          <div
            style={{
              padding: '0.625rem 0.875rem',
              backgroundColor: 'var(--surface-raised)',
              borderRadius: 'var(--radius-sm)',
              border: '1px solid var(--border)',
              fontSize: '0.75rem',
              color: 'var(--text-secondary)',
              lineHeight: 1.4,
            }}
          >
            ℹ <strong>Human Review Context:</strong> Device origins and authored timestamps are shown to assist your decision. Orbit preserves all causal versions and never uses timestamps or device names as automatic winner rules.
          </div>

          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)' }}>
              Conflict Kind: <strong style={{ color: 'var(--text-primary)' }}>{currentConflict.conflict_kind}</strong>
            </span>
            <span className="code-font" style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
              Head Token: {formatShortID(currentConflict.head_token)}
            </span>
          </div>

          <nav className="app-nav" aria-label="Resolution Action Tabs" style={{ margin: '0.25rem 0' }}>
            <button
              type="button"
              id="tab-select-winner"
              className={`nav-tab ${activeTab === 'select' ? 'active' : ''}`}
              onClick={() => setActiveTab('select')}
            >
              1. Select Authoritative Version
            </button>
            <button
              type="button"
              id="tab-keep-copies"
              className={`nav-tab ${activeTab === 'keep' ? 'active' : ''}`}
              onClick={() => setActiveTab('keep')}
            >
              2. Keep Separate Copies
            </button>
            <button
              type="button"
              id="tab-manual-merge"
              className={`nav-tab ${activeTab === 'merge' ? 'active' : ''}`}
              onClick={() => setActiveTab('merge')}
            >
              3. Manual Merge
            </button>
          </nav>

          {/* 1. SELECT WINNER WORKFLOW */}
          {activeTab === 'select' && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
              <p style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', margin: 0 }}>
                Select one of the reviewed concurrent versions as the authoritative state. The chosen version's content will be preserved, and the other versions will be marked superseded. Both choices remain recorded in historical versions.
              </p>

              <div className="table-wrapper">
                <table className="data-table">
                  <thead>
                    <tr>
                      <th style={{ width: '40px' }}>Select</th>
                      <th>Version &amp; Origin</th>
                      <th>Content Type &amp; Size</th>
                      <th>Working Copy</th>
                      <th>Content Bytes</th>
                    </tr>
                  </thead>
                  <tbody>
                    {currentConflict.heads.map((h, idx) => {
                      const authorId = h.id.author || h.id.Author || '';
                      const originDevice = getAuthorDisplay(authorId);
                      const timeStr = h.display_time || 'Recorded';
                      const isTombstone = h.kind === 3;

                      return (
                        <tr
                          key={idx}
                          onClick={() => setSelectedHeadIdx(idx)}
                          style={{
                            cursor: 'pointer',
                            backgroundColor: selectedHeadIdx === idx ? 'var(--surface-selected)' : undefined,
                          }}
                        >
                          <td>
                            <input
                              type="radio"
                              name="winner-head"
                              checked={selectedHeadIdx === idx}
                              onChange={() => setSelectedHeadIdx(idx)}
                              aria-label={`Select version ${formatVersionID(h.id)}`}
                            />
                          </td>
                          <td>
                            <div className="code-font" style={{ fontWeight: 600, color: 'var(--text-primary)' }}>
                              {formatVersionID(h.id)}
                            </div>
                            <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginTop: '0.125rem' }}>
                              From <strong style={{ color: 'var(--text-primary)' }}>{originDevice}</strong> • {timeStr}
                            </div>
                          </td>
                          <td>
                            {isTombstone ? (
                              <div>
                                <span className="badge badge-danger">
                                  Deleted (Tombstone)
                                </span>
                                <div style={{ fontSize: '0.6875rem', color: 'var(--text-secondary)', marginTop: '0.125rem' }}>
                                  Accepting this deletes the file
                                </div>
                              </div>
                            ) : (
                              <div>
                                <span style={{ fontWeight: 500, color: 'var(--text-primary)' }}>
                                  {formatBytes(h.manifest?.size ?? h.manifest?.Size ?? 0)}{' '}
                                  {h.manifest?.executable || h.manifest?.Executable ? '(+x)' : ''}
                                </span>
                                <div style={{ fontSize: '0.6875rem', color: 'var(--text-secondary)', marginTop: '0.125rem' }}>
                                  Regular file content
                                </div>
                              </div>
                            )}
                          </td>
                          <td>
                            {h.applied ? (
                              <span className="badge badge-success">Applied on disk</span>
                            ) : (
                              <span className="badge badge-neutral">Not applied</span>
                            )}
                          </td>
                          <td>
                            <span
                              className={`badge ${
                                h.content_state === 'ready'
                                  ? 'badge-success'
                                  : h.content_state === 'pending'
                                  ? 'badge-warning'
                                  : 'badge-danger'
                              }`}
                            >
                              {h.content_state}
                            </span>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>

              <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: '0.5rem' }}>
                <button
                  type="button"
                  id="btn-confirm-resolve-winner"
                  className="btn btn-primary"
                  onClick={handleSelectWinner}
                  disabled={submitting}
                >
                  {submitting ? 'Applying resolution…' : 'Select Authoritative Version'}
                </button>
              </div>
            </div>
          )}

          {/* 2. KEEP SEPARATE COPIES WORKFLOW */}
          {activeTab === 'keep' && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
              <p style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', margin: 0 }}>
                Keep all conflicting heads by creating distinct side-by-side files. The engine will author separate copies named with author fingerprints and counters, completely preserving all data without overwriting.
              </p>

              <div style={{ backgroundColor: 'var(--surface-raised)', padding: '0.875rem 1rem', borderRadius: 'var(--radius-sm)', border: '1px solid var(--border)' }}>
                <div style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-primary)', marginBottom: '0.5rem' }}>
                  Target Copies to be Generated:
                </div>
                <ul style={{ listStyleType: 'disc', paddingLeft: '1.25rem', fontSize: '0.8125rem', margin: 0, color: 'var(--text-secondary)' }}>
                  {currentConflict.heads.map((h, i) => (
                    <li key={i} className="code-font" style={{ marginTop: '0.25rem' }}>
                      {currentConflict.path} (conflict {formatShortID(h.id.author || h.id.Author || '')}-{h.id.counter ?? h.id.Counter})
                    </li>
                  ))}
                </ul>
              </div>

              <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: '0.5rem' }}>
                <button
                  type="button"
                  id="btn-confirm-keep-copies"
                  className="btn btn-primary"
                  onClick={handleKeepCopies}
                  disabled={submitting}
                >
                  {submitting ? 'Creating copies…' : 'Keep Separate Copies'}
                </button>
              </div>
            </div>
          )}

          {/* 3. MANUAL MERGE WORKFLOW */}
          {activeTab === 'merge' && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
              <p style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', margin: 0 }}>
                Provide the combined, resolved content text. A new unified version will be authored with all reviewed heads as its parents.
              </p>

              <div>
                <label htmlFor="merge-content" style={{ display: 'block', fontSize: '0.8125rem', fontWeight: 500, color: 'var(--text-secondary)', marginBottom: '0.375rem' }}>
                  Merged Content:
                </label>
                <textarea
                  id="merge-content"
                  className="form-textarea code-font"
                  rows={8}
                  placeholder="Paste or type resolved file content here…"
                  value={mergeContent}
                  onChange={(e) => setMergeContent(e.target.value)}
                  style={{
                    width: '100%',
                    padding: '0.625rem',
                    backgroundColor: 'var(--surface-raised)',
                    border: '1px solid var(--border)',
                    borderRadius: 'var(--radius-sm)',
                    color: 'var(--text-primary)',
                    fontSize: '0.8125rem',
                    lineHeight: 1.4,
                  }}
                />
              </div>

              <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                <input
                  type="checkbox"
                  id="merge-exec"
                  checked={mergeExecutable}
                  onChange={(e) => setMergeExecutable(e.target.checked)}
                />
                <label htmlFor="merge-exec" style={{ fontSize: '0.8125rem', color: 'var(--text-primary)', cursor: 'pointer' }}>
                  Executable bit (+x)
                </label>
              </div>

              <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: '0.5rem' }}>
                <button
                  type="button"
                  id="btn-confirm-manual-merge"
                  className="btn btn-primary"
                  onClick={handleManualMerge}
                  disabled={submitting || mergeContent === ''}
                >
                  {submitting ? 'Submitting merge…' : 'Submit Merged Version'}
                </button>
              </div>
            </div>
          )}
        </div>

        <div className="modal-footer" style={{ padding: '0.875rem 1.25rem', borderTop: '1px solid var(--border)', display: 'flex', justifyContent: 'flex-end' }}>
          <button type="button" className="btn btn-secondary" onClick={onClose} disabled={submitting}>
            Cancel
          </button>
        </div>
      </div>
    </div>
  );
};
