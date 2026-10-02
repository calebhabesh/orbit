import React, { useState, useEffect } from 'react';
import { RetireDevicePreviewResult } from '../types';
import { api, formatShortID } from '../api';

interface RetireDeviceModalProps {
  folder: string;
  deviceId: string;
  onClose: () => void;
  onSuccess: () => void;
}

export const RetireDeviceModal: React.FC<RetireDeviceModalProps> = ({
  folder,
  deviceId,
  onClose,
  onSuccess,
}) => {
  const [loading, setLoading] = useState(true);
  const [preview, setPreview] = useState<RetireDevicePreviewResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [confirmed, setConfirmed] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    const fetchPreview = async () => {
      setLoading(true);
      setError(null);
      try {
        const res = await api.previewRetireDevice(folder, deviceId);
        setPreview(res);
      } catch (err: any) {
        setError(err.message || 'Failed to load retirement preview');
      } finally {
        setLoading(false);
      }
    };
    fetchPreview();
  }, [folder, deviceId]);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [onClose]);

  const handleRetire = async () => {
    if (!confirmed) return;
    setSubmitting(true);
    setError(null);
    try {
      await api.retireMember(folder, deviceId);
      onSuccess();
      onClose();
    } catch (err: any) {
      setError(err.message || 'Retirement failed');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div
      className="modal-overlay"
      role="dialog"
      aria-modal="true"
      aria-labelledby="retire-modal-title"
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
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        style={{
          width: '100%',
          maxWidth: '520px',
          backgroundColor: 'var(--surface)',
          border: '1px solid var(--border)',
          borderRadius: 'var(--radius-lg)',
          overflow: 'hidden',
          boxShadow: '0 16px 40px rgba(0, 0, 0, 0.7)',
        }}
      >
        {/* Header */}
        <div
          style={{
            padding: '1.25rem 1.5rem',
            borderBottom: '1px solid var(--border)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            backgroundColor: 'var(--surface-raised)',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
            <div
              style={{
                width: '2rem',
                height: '2rem',
                borderRadius: 'var(--radius-sm)',
                backgroundColor: 'rgba(239, 68, 68, 0.1)',
                border: '1px solid var(--status-error)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                color: 'var(--status-error)',
              }}
            >
              <svg style={{ width: '1.125rem', height: '1.125rem' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
                <line x1="12" y1="9" x2="12" y2="13" />
                <line x1="12" y1="17" x2="12.01" y2="17" />
              </svg>
            </div>
            <h3 id="retire-modal-title" style={{ fontSize: '1.125rem', fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>
              Retire Device
            </h3>
          </div>
          <button
            type="button"
            onClick={onClose}
            style={{
              background: 'transparent',
              border: 'none',
              color: 'var(--text-secondary)',
              fontSize: '1.25rem',
              cursor: 'pointer',
              padding: '0.25rem',
            }}
          >
            ✕
          </button>
        </div>

        {/* Body */}
        <div style={{ padding: '1.5rem', display: 'flex', flexDirection: 'column', gap: '1.25rem' }}>
          {error && (
            <div
              style={{
                padding: '0.75rem 1rem',
                backgroundColor: 'rgba(239, 68, 68, 0.1)',
                border: '1px solid var(--status-error)',
                borderRadius: 'var(--radius-sm)',
                color: 'var(--text-primary)',
                fontSize: '0.8125rem',
              }}
            >
              {error}
            </div>
          )}

          {loading ? (
            <div style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-secondary)', fontSize: '0.875rem' }}>
              Preparing retirement preview…
            </div>
          ) : preview ? (
            <>
              <div>
                <p style={{ fontSize: '0.875rem', color: 'var(--text-primary)', margin: '0 0 0.5rem 0', lineHeight: 1.5 }}>
                  You are preparing to retire <strong>{preview.device_name}</strong> (<code className="code-font" style={{ fontSize: '0.75rem' }}>{formatShortID(preview.device_id)}</code>) from this workspace.
                </p>
                <div style={{ padding: '0.75rem 1rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', fontSize: '0.8125rem', color: 'var(--text-secondary)' }}>
                  Membership will progress from <strong>Revision {preview.current_revision}</strong> to <strong>Revision {preview.next_revision}</strong>. Surviving peers: {preview.remaining_count} ({preview.surviving_peers.join(', ') || 'none'}).
                </div>
              </div>

              {/* Invariant & Limit Disclaimers */}
              <div style={{ padding: '0.875rem 1rem', backgroundColor: 'rgba(239, 68, 68, 0.05)', border: '1px solid rgba(239, 68, 68, 0.3)', borderRadius: 'var(--radius-sm)', display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
                <div style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                  Important Limitations & Guarantees:
                </div>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', lineHeight: 1.4 }}>
                  • <strong>No remote disk erasure:</strong> Orbit cannot remotely delete files or data stored on the retired device's physical drive.
                </div>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', lineHeight: 1.4 }}>
                  • <strong>Permanent identity revocation:</strong> This device ID is permanently barred from fetching future revisions or rejoining under this cryptographic identity (Invariant I24).
                </div>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', lineHeight: 1.4 }}>
                  • <strong>Conservative survivor procedure:</strong> Known history already synced remains preserved on surviving replicas.
                </div>
              </div>

              {/* Confirmation Checkbox */}
              <label style={{ display: 'flex', alignItems: 'flex-start', gap: '0.625rem', cursor: 'pointer', fontSize: '0.8125rem', color: 'var(--text-primary)' }}>
                <input
                  type="checkbox"
                  id="confirm-retire-checkbox"
                  checked={confirmed}
                  onChange={(e) => setConfirmed(e.target.checked)}
                  style={{ marginTop: '0.125rem' }}
                />
                <span>
                  I understand this device will be permanently retired from this workspace and cannot rejoin with this key.
                </span>
              </label>

              {/* Actions */}
              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '0.75rem', marginTop: '0.5rem' }}>
                <button
                  type="button"
                  onClick={onClose}
                  className="btn btn-secondary"
                  style={{ padding: '0.5rem 1rem', fontSize: '0.8125rem' }}
                >
                  Cancel
                </button>
                <button
                  type="button"
                  id="btn-confirm-retire"
                  disabled={!confirmed || submitting}
                  onClick={handleRetire}
                  style={{
                    padding: '0.5rem 1.25rem',
                    fontSize: '0.8125rem',
                    fontWeight: 600,
                    backgroundColor: 'var(--status-error)',
                    color: '#ffffff',
                    border: 'none',
                    borderRadius: 'var(--radius-sm)',
                    cursor: confirmed && !submitting ? 'pointer' : 'not-allowed',
                    opacity: confirmed && !submitting ? 1 : 0.5,
                  }}
                >
                  {submitting ? 'Retiring…' : 'Confirm Permanent Retirement'}
                </button>
              </div>
            </>
          ) : null}
        </div>
      </div>
    </div>
  );
};
