import React, { useState } from 'react';
import { api } from '../api';

interface UnregisterModalProps {
  folderId: string;
  rootPath: string;
  onClose: () => void;
  onSuccess: () => void;
}

export const UnregisterModal: React.FC<UnregisterModalProps> = ({
  folderId,
  rootPath,
  onClose,
  onSuccess,
}) => {
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleUnregister = async () => {
    setSubmitting(true);
    setError(null);
    try {
      await api.removeFolder(folderId);
      onSuccess();
    } catch (err: any) {
      setError(err.message || 'Failed to unregister workspace');
      setSubmitting(false);
    }
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="unregister-modal-title"
      style={{
        position: 'fixed',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.75)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 1000,
        padding: '1rem',
      }}
      onKeyDown={(e) => {
        if (e.key === 'Escape') onClose();
      }}
    >
      <div
        style={{
          backgroundColor: 'var(--surface)',
          border: '1px solid var(--border)',
          borderRadius: 'var(--radius-md)',
          width: '100%',
          maxWidth: '520px',
          padding: '1.5rem',
          display: 'flex',
          flexDirection: 'column',
          gap: '1.25rem',
        }}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
          <div>
            <h3 id="unregister-modal-title" style={{ margin: 0, fontSize: '1.125rem', fontWeight: 600, color: 'var(--text-primary)' }}>
              Unregister Workspace
            </h3>
            <p style={{ margin: '0.25rem 0 0 0', fontSize: '0.8125rem', color: 'var(--text-secondary)' }}>
              Disconnect this folder from active synchronization.
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            style={{
              background: 'none',
              border: 'none',
              color: 'var(--text-secondary)',
              fontSize: '1.25rem',
              cursor: 'pointer',
              padding: '0.25rem',
            }}
          >
            ×
          </button>
        </div>

        {/* Invariant & Behavior Explanation (Packet O11) */}
        <div
          style={{
            padding: '1rem',
            backgroundColor: 'var(--surface-raised)',
            border: '1px solid var(--border)',
            borderRadius: 'var(--radius-sm)',
            fontSize: '0.8125rem',
            lineHeight: 1.5,
            color: 'var(--text-secondary)',
          }}
        >
          <p style={{ margin: '0 0 0.5rem 0', color: 'var(--text-primary)', fontWeight: 600 }}>
            Filesystem & History Preservation Guarantee:
          </p>
          <ul style={{ margin: 0, paddingLeft: '1.25rem' }}>
            <li>
              <strong>Your local files are completely preserved:</strong> no files or directories under <code className="code-font" style={{ color: 'var(--text-primary)' }}>{rootPath}</code> will be deleted.
            </li>
            <li>
              <strong>Zero deletion tombstones:</strong> this operation does NOT emit deletions to your other devices.
            </li>
            <li>
              <strong>Safe re-adoption:</strong> you can re-register or re-link this folder at any time.
            </li>
          </ul>
        </div>

        <div style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)' }}>
          Workspace ID: <code className="code-font" style={{ color: 'var(--text-primary)' }}>{folderId.slice(0, 16)}…</code>
        </div>

        {error && (
          <div style={{ padding: '0.75rem', backgroundColor: 'var(--status-danger-bg)', border: '1px solid var(--status-danger)', borderRadius: 'var(--radius-sm)', fontSize: '0.8125rem', color: 'var(--status-danger)' }}>
            {error}
          </div>
        )}

        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '0.75rem', marginTop: '0.5rem' }}>
          <button
            type="button"
            id="btn-cancel-unregister"
            onClick={onClose}
            disabled={submitting}
            className="btn btn-secondary btn-sm"
            style={{
              padding: '0.5rem 1rem',
              fontSize: '0.8125rem',
              backgroundColor: 'transparent',
              border: '1px solid var(--border)',
              borderRadius: 'var(--radius-sm)',
              color: 'var(--text-secondary)',
              cursor: 'pointer',
            }}
          >
            Cancel
          </button>
          <button
            type="button"
            id="btn-confirm-unregister"
            onClick={handleUnregister}
            disabled={submitting}
            className="btn btn-sm"
            style={{
              padding: '0.5rem 1rem',
              fontSize: '0.8125rem',
              fontWeight: 600,
              backgroundColor: 'var(--status-danger)',
              border: 'none',
              borderRadius: 'var(--radius-sm)',
              color: '#ffffff',
              cursor: 'pointer',
            }}
          >
            {submitting ? 'Unregistering…' : 'Unregister Workspace'}
          </button>
        </div>
      </div>
    </div>
  );
};
