import React, { useState, useEffect } from 'react';
import { api, formatBytes } from '../api';
import { RetentionPreview } from '../types';

interface RetentionModalProps {
  folderId: string;
  onClose: () => void;
  onGCComplete?: () => void;
}

export const RetentionModal: React.FC<RetentionModalProps> = ({
  folderId,
  onClose,
  onGCComplete,
}) => {
  const [retentionDays, setRetentionDays] = useState(30);
  const [minSuperseded, setMinSuperseded] = useState(20);
  const [preview, setPreview] = useState<RetentionPreview | null>(null);
  const [loadingPreview, setLoadingPreview] = useState(false);
  const [savingPolicy, setSavingPolicy] = useState(false);
  const [runningGC, setRunningGC] = useState(false);
  const [statusMessage, setStatusMessage] = useState<string | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  useEffect(() => {
    loadPreview(retentionDays, minSuperseded);
  }, [folderId]);

  const loadPreview = async (days: number, minSup: number) => {
    setLoadingPreview(true);
    setErrorMessage(null);
    try {
      const res = await api.getRetentionPreview(folderId, days, minSup);
      setPreview(res.preview);
      if (res.preview.policy) {
        setRetentionDays(res.preview.policy.retention_days);
        setMinSuperseded(res.preview.policy.min_superseded);
      }
    } catch (err: any) {
      setErrorMessage(err.message || 'Failed to load retention preview');
    } finally {
      setLoadingPreview(false);
    }
  };

  const handleApplyPreview = () => {
    loadPreview(retentionDays, minSuperseded);
  };

  const handleSavePolicy = async () => {
    setSavingPolicy(true);
    setStatusMessage(null);
    setErrorMessage(null);
    try {
      await api.changeRetention(folderId, retentionDays, minSuperseded);
      setStatusMessage('Retention policy updated successfully.');
      await loadPreview(retentionDays, minSuperseded);
      setTimeout(() => setStatusMessage(null), 3000);
    } catch (err: any) {
      setErrorMessage(err.message || 'Failed to save retention policy');
    } finally {
      setSavingPolicy(false);
    }
  };

  const handleRunGC = async () => {
    setRunningGC(true);
    setStatusMessage(null);
    setErrorMessage(null);
    try {
      const res = await api.runGC(folderId);
      const reclaimed = res?.report?.reclaimed_bytes ?? 0;
      const count = res?.report?.unlinked_objects ?? 0;
      setStatusMessage(`Garbage collection finished: reclaimed ${formatBytes(reclaimed)} across ${count} unreferenced chunks.`);
      await loadPreview(retentionDays, minSuperseded);
      if (onGCComplete) onGCComplete();
      setTimeout(() => setStatusMessage(null), 5000);
    } catch (err: any) {
      setErrorMessage(err.message || 'Garbage collection failed');
    } finally {
      setRunningGC(false);
    }
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="retention-modal-title"
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
          maxWidth: '580px',
          maxHeight: '90vh',
          overflowY: 'auto',
          padding: '1.5rem',
          display: 'flex',
          flexDirection: 'column',
          gap: '1.25rem',
        }}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
          <div>
            <h3 id="retention-modal-title" style={{ margin: 0, fontSize: '1.125rem', fontWeight: 600, color: 'var(--text-primary)' }}>
              Retention Policy & Storage Cleanup
            </h3>
            <p style={{ margin: '0.25rem 0 0 0', fontSize: '0.8125rem', color: 'var(--text-secondary)' }}>
              Configure historical version retention rules and preview eligible reclamation.
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

        {/* Informative Explanation (Packet O11) */}
        <div
          style={{
            padding: '0.75rem 1rem',
            backgroundColor: 'var(--surface-raised)',
            border: '1px solid var(--border)',
            borderRadius: 'var(--radius-sm)',
            fontSize: '0.8125rem',
            lineHeight: 1.45,
            color: 'var(--text-secondary)',
          }}
        >
          <strong style={{ color: 'var(--text-primary)' }}>Conditional Restore Safety:</strong> Current heads, active read leases, and in-flight transfers are always protected. Expired historical versions become eligible for collection, which is why Deleted files restore is conditional on chunk availability. Inspection reads never delete files; cleanup requires explicit confirmation.
        </div>

        {/* Inputs */}
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1rem' }}>
          <div>
            <label htmlFor="input-retention-days" style={{ display: 'block', fontSize: '0.8125rem', fontWeight: 500, color: 'var(--text-primary)', marginBottom: '0.375rem' }}>
              Retention Duration (Days)
            </label>
            <input
              id="input-retention-days"
              type="number"
              min="0"
              max="3650"
              value={retentionDays}
              onChange={(e) => setRetentionDays(parseInt(e.target.value, 10) || 0)}
              style={{
                width: '100%',
                padding: '0.5rem 0.75rem',
                backgroundColor: 'var(--surface-raised)',
                border: '1px solid var(--border)',
                borderRadius: 'var(--radius-sm)',
                color: 'var(--text-primary)',
                fontSize: '0.875rem',
              }}
            />
          </div>
          <div>
            <label htmlFor="input-min-superseded" style={{ display: 'block', fontSize: '0.8125rem', fontWeight: 500, color: 'var(--text-primary)', marginBottom: '0.375rem' }}>
              Min Superseded Versions
            </label>
            <input
              id="input-min-superseded"
              type="number"
              min="0"
              max="500"
              value={minSuperseded}
              onChange={(e) => setMinSuperseded(parseInt(e.target.value, 10) || 0)}
              style={{
                width: '100%',
                padding: '0.5rem 0.75rem',
                backgroundColor: 'var(--surface-raised)',
                border: '1px solid var(--border)',
                borderRadius: 'var(--radius-sm)',
                color: 'var(--text-primary)',
                fontSize: '0.875rem',
              }}
            />
          </div>
        </div>

        <button
          type="button"
          onClick={handleApplyPreview}
          disabled={loadingPreview}
          className="btn btn-secondary btn-sm"
          style={{
            alignSelf: 'flex-start',
            padding: '0.375rem 0.75rem',
            fontSize: '0.8125rem',
            backgroundColor: 'var(--surface-raised)',
            border: '1px solid var(--border)',
            borderRadius: 'var(--radius-sm)',
            color: 'var(--text-primary)',
            cursor: 'pointer',
          }}
        >
          {loadingPreview ? 'Recalculating…' : 'Update Preview'}
        </button>

        {/* Live Preview Stats */}
        {preview && (
          <div
            style={{
              backgroundColor: 'var(--surface-raised)',
              border: '1px solid var(--border)',
              borderRadius: 'var(--radius-sm)',
              padding: '1rem',
              display: 'flex',
              flexDirection: 'column',
              gap: '0.75rem',
            }}
          >
            <div style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-primary)' }}>
              Reclamation Preview
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))', gap: '0.75rem' }}>
              <div>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>Total Versions</div>
                <div style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)', marginTop: '0.125rem' }}>
                  {preview.total_versions}
                </div>
              </div>
              <div>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>Current Heads</div>
                <div style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--status-success)', marginTop: '0.125rem' }}>
                  {preview.head_versions} (Protected)
                </div>
              </div>
              <div>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>Retained Historical</div>
                <div style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)', marginTop: '0.125rem' }}>
                  {preview.retained_versions}
                </div>
              </div>
              <div>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>Expired Versions</div>
                <div style={{ fontSize: '1rem', fontWeight: 600, color: preview.expired_versions > 0 ? 'var(--status-warning)' : 'var(--text-secondary)', marginTop: '0.125rem' }}>
                  {preview.expired_versions}
                </div>
              </div>
            </div>

            <div style={{ borderTop: '1px solid var(--border)', paddingTop: '0.75rem', display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))', gap: '0.75rem' }}>
              <div>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>Protected Chunks</div>
                <div style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)', marginTop: '0.125rem' }}>
                  {preview.protected_chunks}
                </div>
              </div>
              <div>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>Eligible Candidates</div>
                <div style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)', marginTop: '0.125rem' }}>
                  {preview.candidate_chunks}
                </div>
              </div>
              <div>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>Reclaimable Space</div>
                <div style={{ fontSize: '1.125rem', fontWeight: 700, color: 'var(--status-success)', marginTop: '0.125rem' }}>
                  {formatBytes(preview.reclaimable_bytes)}
                </div>
              </div>
            </div>

            {preview.suspended && (
              <div style={{ padding: '0.5rem', backgroundColor: 'var(--status-warning-bg)', border: '1px solid var(--status-warning)', borderRadius: 'var(--radius-sm)', fontSize: '0.75rem', color: 'var(--status-warning)' }}>
                <strong>Cleanup Suspended:</strong> {preview.suspend_reason || 'Retention rules suspended by engine policy.'}
              </div>
            )}
          </div>
        )}

        {statusMessage && (
          <div style={{ padding: '0.75rem', backgroundColor: 'var(--status-success-bg)', border: '1px solid var(--status-success)', borderRadius: 'var(--radius-sm)', fontSize: '0.8125rem', color: 'var(--status-success)' }}>
            {statusMessage}
          </div>
        )}

        {errorMessage && (
          <div style={{ padding: '0.75rem', backgroundColor: 'var(--status-danger-bg)', border: '1px solid var(--status-danger)', borderRadius: 'var(--radius-sm)', fontSize: '0.8125rem', color: 'var(--status-danger)' }}>
            {errorMessage}
          </div>
        )}

        {/* Modal Actions */}
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '0.75rem', marginTop: '0.5rem', flexWrap: 'wrap' }}>
          <button
            type="button"
            onClick={handleSavePolicy}
            disabled={savingPolicy}
            id="btn-save-retention-policy"
            className="btn btn-secondary btn-sm"
            style={{
              padding: '0.5rem 1rem',
              fontSize: '0.8125rem',
              fontWeight: 500,
              backgroundColor: 'var(--surface-raised)',
              border: '1px solid var(--border)',
              borderRadius: 'var(--radius-sm)',
              color: 'var(--text-primary)',
              cursor: 'pointer',
            }}
          >
            {savingPolicy ? 'Saving…' : 'Save Policy'}
          </button>
          <button
            type="button"
            onClick={handleRunGC}
            disabled={runningGC || preview?.suspended}
            id="btn-run-gc-modal"
            className="btn btn-primary btn-sm"
            style={{
              padding: '0.5rem 1rem',
              fontSize: '0.8125rem',
              fontWeight: 600,
              backgroundColor: 'var(--text-primary)',
              border: 'none',
              borderRadius: 'var(--radius-sm)',
              color: 'var(--canvas)',
              cursor: 'pointer',
            }}
          >
            {runningGC ? 'Cleaning up…' : 'Execute Cleanup (GC)'}
          </button>
          <button
            type="button"
            id="btn-close-retention-modal"
            onClick={onClose}
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
            Close
          </button>
        </div>
      </div>
    </div>
  );
};
