import React, { useState } from 'react';
import { VersionID, RestorePreview } from '../types';
import { api, formatBytes, formatShortID, formatVersionID, APIError } from '../api';

interface RestoreModalProps {
  folder: string;
  preview: RestorePreview;
  onClose: () => void;
  onSuccess: (newVersion: VersionID) => void;
}

export const RestoreModal: React.FC<RestoreModalProps> = ({ folder, preview, onClose, onSuccess }) => {
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const isRestorable = preview.content_state === 'ready';

  const handleConfirmRestore = async () => {
    if (!isRestorable) return;
    setSubmitting(true);
    setError(null);

    try {
      const res = await api.restore(
        folder,
        preview.path,
        preview.source_version,
        preview.current_heads,
        preview.expected_head_token
      );
      onSuccess(res.resolved_id);
    } catch (err: any) {
      if (err instanceof APIError) {
        if (err.code === 'STALE_VIEW') {
          setError('Causal heads changed since preview was loaded! Another peer or local editor modified this file. Please refresh and review.');
        } else {
          setError(`${err.code}: ${err.message}. ${err.action || ''}`);
        }
      } else {
        setError(err.message || 'Failed to restore version');
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="modal-backdrop" onClick={onClose} role="dialog" aria-modal="true" aria-labelledby="restore-title">
      <div className="modal-content" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h2 id="restore-title" className="panel-title">
            Historical Restore Preview
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

          <div>
            <span style={{ color: 'var(--text-muted)', fontSize: '0.875rem' }}>Restoring Path:</span>
            <div className="code-font" style={{ fontWeight: 600, fontSize: '1rem', marginTop: '0.25rem' }}>
              {preview.path}
            </div>
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: '1rem' }}>
            <div style={{ backgroundColor: 'var(--bg-main)', padding: '0.75rem', borderRadius: 'var(--radius-sm)' }}>
              <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>Source Version</div>
              <div className="code-font" style={{ fontWeight: 600, marginTop: '0.25rem' }}>
                {formatVersionID(preview.source_version)}
              </div>
            </div>

            <div style={{ backgroundColor: 'var(--bg-main)', padding: '0.75rem', borderRadius: 'var(--radius-sm)' }}>
              <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>Source Size &amp; Exec</div>
              <div style={{ fontWeight: 600, marginTop: '0.25rem' }}>
                {formatBytes(preview.source_size)} {preview.source_executable ? '(+x executable)' : ''}
              </div>
            </div>

            <div style={{ backgroundColor: 'var(--bg-main)', padding: '0.75rem', borderRadius: 'var(--radius-sm)' }}>
              <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>Content Availability</div>
              <div style={{ marginTop: '0.25rem' }}>
                <span
                  className={`badge ${
                    preview.content_state === 'ready'
                      ? 'badge-success'
                      : preview.content_state === 'expired'
                      ? 'badge-neutral'
                      : 'badge-danger'
                  }`}
                >
                  {preview.content_state.toUpperCase()}
                </span>
              </div>
            </div>
          </div>

          {!isRestorable && (
            <div className="storage-banner warning" role="alert">
              <div>
                <strong>Restore Blocked: </strong>
                {preview.content_state === 'expired' && (
                  <span>
                    Payload chunks have been expired and cleaned up under the local retention policy. While causal metadata is retained to ensure DAG convergence, historical bytes are no longer present on disk and cannot be restored.
                  </span>
                )}
                {preview.content_state === 'unavailable' && (
                  <span>
                    Payload chunks are currently quarantined or missing due to bitrot or read failure. Run storage repair to re-fetch valid bytes before restoring.
                  </span>
                )}
                {preview.content_state === 'pending' && (
                  <span>
                    Payload chunks are currently being transferred from peers. Please wait until download completes.
                  </span>
                )}
              </div>
            </div>
          )}

          <div>
            <div style={{ fontSize: '0.875rem', fontWeight: 600, marginBottom: '0.5rem' }}>
              Current Reviewed Heads to Supersede:
            </div>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: '0.5rem' }}>
              {preview.current_heads.map((h, i) => (
                <span key={i} className="badge badge-neutral code-font">
                  {formatVersionID(h)}
                </span>
              ))}
            </div>
            <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: '0.5rem' }}>
              Head Token: <span className="code-font">{formatShortID(preview.expected_head_token)}</span>
            </div>
          </div>

          <div style={{ fontSize: '0.8125rem', color: 'var(--text-muted)', borderTop: '1px solid var(--border-subtle)', paddingTop: '0.75rem' }}>
            <em>
              Note: In this distributed filesystem, historical restore creates a new causal version in the present referencing the reviewed heads as parents. It adopts the historical content without rolling back causal clocks.
            </em>
          </div>
        </div>

        <div className="modal-footer">
          <button type="button" className="btn btn-secondary" onClick={onClose} disabled={submitting}>
            Cancel
          </button>
          <button
            type="button"
            id="btn-confirm-restore"
            className="btn btn-primary"
            onClick={handleConfirmRestore}
            disabled={!isRestorable || submitting}
          >
            {submitting ? (
              <>
                <span className="spinner" style={{ width: '1rem', height: '1rem' }} />
                Restoring…
              </>
            ) : (
              'Confirm Restore'
            )}
          </button>
        </div>
      </div>
    </div>
  );
};
