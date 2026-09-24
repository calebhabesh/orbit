import React, { useState } from 'react';
import { ConflictSet, VersionID } from '../types';
import { api, formatBytes, formatShortID, formatVersionID, APIError } from '../api';

interface ConflictResolveModalProps {
  folder: string;
  conflict: ConflictSet;
  onClose: () => void;
  onResolved: () => void;
}

export const ConflictResolveModal: React.FC<ConflictResolveModalProps> = ({
  folder,
  conflict,
  onClose,
  onResolved,
}) => {
  const [activeTab, setActiveTab] = useState<'select' | 'keep' | 'merge'>('select');
  const [selectedHeadIdx, setSelectedHeadIdx] = useState<number>(0);
  const [mergeContent, setMergeContent] = useState<string>('');
  const [mergeExecutable, setMergeExecutable] = useState<boolean>(false);
  const [submitting, setSubmitting] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);

  const reviewedIDs: VersionID[] = conflict.heads.map((h) => h.id);

  const handleSelectWinner = async () => {
    setSubmitting(true);
    setError(null);
    try {
      const selected = conflict.heads[selectedHeadIdx].id;
      await api.resolveSelect(folder, conflict.path, reviewedIDs, conflict.head_token, selected);
      onResolved();
    } catch (err: any) {
      handleAPIError(err);
    } finally {
      setSubmitting(false);
    }
  };

  const handleKeepCopies = async () => {
    setSubmitting(true);
    setError(null);
    try {
      await api.resolveKeepCopies(folder, conflict.path, reviewedIDs, conflict.head_token);
      onResolved();
    } catch (err: any) {
      handleAPIError(err);
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
        conflict.path,
        reviewedIDs,
        conflict.head_token,
        mergeExecutable,
        mergeContent
      );
      onResolved();
    } catch (err: any) {
      handleAPIError(err);
    } finally {
      setSubmitting(false);
    }
  };

  const handleAPIError = (err: any) => {
    if (err instanceof APIError) {
      if (err.code === 'STALE_VIEW') {
        setError(
          'STALE_VIEW: Conflict heads changed while you were reviewing! Another peer or local process created a new version. The heads have been refreshed. Please re-review before resolving.'
        );
      } else {
        setError(`${err.code}: ${err.message}. ${err.action || ''}`);
      }
    } else {
      setError(err.message || 'Operation failed');
    }
  };

  return (
    <div className="modal-backdrop" onClick={onClose} role="dialog" aria-modal="true" aria-labelledby="conflict-modal-title">
      <div className="modal-content" onClick={(e) => e.stopPropagation()} style={{ maxWidth: '720px' }}>
        <div className="modal-header">
          <h2 id="conflict-modal-title" className="panel-title">
            Resolve Conflict: <span className="code-font">{conflict.path}</span>
          </h2>
          <button type="button" className="btn btn-secondary btn-sm" onClick={onClose} aria-label="Close dialog">
            ✕
          </button>
        </div>

        <div className="modal-body">
          {error && (
            <div className="storage-banner danger" role="alert">
              <div>{error}</div>
            </div>
          )}

          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span style={{ fontSize: '0.875rem', color: 'var(--text-muted)' }}>
              Conflict Kind: <strong>{conflict.conflict_kind}</strong>
            </span>
            <span className="code-font" style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
              Head Token: {formatShortID(conflict.head_token)}
            </span>
          </div>

          <nav className="app-nav" aria-label="Resolution Action Tabs" style={{ margin: '0.25rem 0' }}>
            <button
              type="button"
              className={`nav-tab ${activeTab === 'select' ? 'active' : ''}`}
              onClick={() => setActiveTab('select')}
            >
              1. Select Winner
            </button>
            <button
              type="button"
              className={`nav-tab ${activeTab === 'keep' ? 'active' : ''}`}
              onClick={() => setActiveTab('keep')}
            >
              2. Keep Separate Copies
            </button>
            <button
              type="button"
              className={`nav-tab ${activeTab === 'merge' ? 'active' : ''}`}
              onClick={() => setActiveTab('merge')}
            >
              3. Manual Merge
            </button>
          </nav>

          {/* 1. SELECT WINNER WORKFLOW */}
          {activeTab === 'select' && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
              <p style={{ fontSize: '0.875rem', color: 'var(--text-muted)' }}>
                Select one of the reviewed concurrent heads as the authoritative version. The selected head's content will be preserved, and the other heads will be marked superseded.
              </p>

              <div className="table-wrapper">
                <table className="data-table">
                  <thead>
                    <tr>
                      <th style={{ width: '40px' }}>Select</th>
                      <th>Head ID</th>
                      <th>Kind &amp; Size</th>
                      <th>Applied?</th>
                      <th>Content State</th>
                    </tr>
                  </thead>
                  <tbody>
                    {conflict.heads.map((h, idx) => (
                      <tr
                        key={idx}
                        onClick={() => setSelectedHeadIdx(idx)}
                        style={{ cursor: 'pointer', backgroundColor: selectedHeadIdx === idx ? 'rgba(56, 189, 248, 0.08)' : undefined }}
                      >
                        <td>
                          <input
                            type="radio"
                            name="winner-head"
                            checked={selectedHeadIdx === idx}
                            onChange={() => setSelectedHeadIdx(idx)}
                            aria-label={`Select head ${formatVersionID(h.id)}`}
                          />
                        </td>
                        <td className="code-font" style={{ fontWeight: 600 }}>
                          {formatVersionID(h.id)}
                        </td>
                        <td>
                          {h.kind === 3 ? (
                            <span className="badge badge-danger">TOMBSTONE (Deleted)</span>
                          ) : (
                            <span>
                              {formatBytes(h.manifest?.size ?? h.manifest?.Size ?? 0)}{' '}
                              {h.manifest?.executable || h.manifest?.Executable ? '(+x)' : ''}
                            </span>
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
                    ))}
                  </tbody>
                </table>
              </div>

              <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: '0.5rem' }}>
                <button
                  type="button"
                  className="btn btn-primary"
                  onClick={handleSelectWinner}
                  disabled={submitting}
                >
                  {submitting ? 'Applying resolution…' : 'Select Winner'}
                </button>
              </div>
            </div>
          )}

          {/* 2. KEEP SEPARATE COPIES WORKFLOW */}
          {activeTab === 'keep' && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
              <p style={{ fontSize: '0.875rem', color: 'var(--text-muted)' }}>
                Keep all conflicting heads by creating distinct side-by-side files. The engine will author separate copies named with author fingerprints and counters, completely preserving all data without overwriting.
              </p>

              <div style={{ backgroundColor: 'var(--bg-main)', padding: '1rem', borderRadius: 'var(--radius-sm)', border: '1px solid var(--border-default)' }}>
                <div style={{ fontSize: '0.8125rem', fontWeight: 600, marginBottom: '0.5rem' }}>
                  Target Copies to be Generated:
                </div>
                <ul style={{ listStyleType: 'disc', paddingLeft: '1.25rem', fontSize: '0.8125rem' }}>
                  {conflict.heads.map((h, i) => (
                    <li key={i} className="code-font" style={{ marginTop: '0.25rem' }}>
                      {conflict.path} (conflict {formatShortID(h.id.author || h.id.Author || '')}-{h.id.counter ?? h.id.Counter})
                    </li>
                  ))}
                </ul>
              </div>

              <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: '0.5rem' }}>
                <button
                  type="button"
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
              <p style={{ fontSize: '0.875rem', color: 'var(--text-muted)' }}>
                Provide the combined, resolved content text. A new unified version will be authored with all reviewed heads as its parents.
              </p>

              <div>
                <label htmlFor="merge-content" style={{ display: 'block', fontSize: '0.875rem', marginBottom: '0.25rem' }}>
                  Merged Content:
                </label>
                <textarea
                  id="merge-content"
                  className="form-textarea code-font"
                  rows={8}
                  placeholder="Paste or type resolved file content here…"
                  value={mergeContent}
                  onChange={(e) => setMergeContent(e.target.value)}
                />
              </div>

              <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                <input
                  type="checkbox"
                  id="merge-exec"
                  checked={mergeExecutable}
                  onChange={(e) => setMergeExecutable(e.target.checked)}
                />
                <label htmlFor="merge-exec" style={{ fontSize: '0.875rem' }}>
                  Executable bit (+x)
                </label>
              </div>

              <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: '0.5rem' }}>
                <button
                  type="button"
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

        <div className="modal-footer">
          <button type="button" className="btn btn-secondary" onClick={onClose} disabled={submitting}>
            Cancel
          </button>
        </div>
      </div>
    </div>
  );
};
