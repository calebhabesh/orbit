import React, { useState, useEffect } from 'react';
import { FolderRecord, CreateInvitationResult, InvitationRecord } from '../types';
import { api, formatShortID, formatTimestamp } from '../api';
import { QRCodeDisplay } from './QRCodeDisplay';

interface AddDeviceModalProps {
  folders?: FolderRecord[];
  currentFolderId?: string;
  folder?: string;
  onClose: () => void;
  onInvitationsChanged?: () => void;
}

export const AddDeviceModal: React.FC<AddDeviceModalProps> = ({
  folders = [],
  currentFolderId = '',
  folder = '',
  onClose,
  onInvitationsChanged,
}) => {
  const initialFolder = folder || currentFolderId || (folders[0]?.folder || folders[0]?.folder_id || '');
  const [selectedFolder, setSelectedFolder] = useState(initialFolder);
  const [ttlSecs, setTtlSecs] = useState<number>(86400); // 24h default
  const [maxUses, setMaxUses] = useState<number>(1); // single-use default
  const [endpoint, setEndpoint] = useState<string>('');

  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [createdInvitation, setCreatedInvitation] = useState<CreateInvitationResult | null>(null);
  const [copied, setCopied] = useState(false);

  // Active invitations list
  const [activeInvitations, setActiveInvitations] = useState<InvitationRecord[]>([]);
  const [loadingInvs, setLoadingInvs] = useState(false);
  const [activeTab, setActiveTab] = useState<'create' | 'manage'>('create');

  // Load existing invitations for folder
  const loadInvitations = async (folder: string) => {
    if (!folder) return;
    setLoadingInvs(true);
    try {
      const res = await api.listInvitations(folder);
      setActiveInvitations(res.invitations || []);
    } catch {
      setActiveInvitations([]);
    } finally {
      setLoadingInvs(false);
    }
  };

  useEffect(() => {
    loadInvitations(selectedFolder);
  }, [selectedFolder]);

  // Handle Escape key to close modal
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [onClose]);

  const handleGenerate = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError(null);
    setCopied(false);

    try {
      const res = await api.createInvitation(selectedFolder, ttlSecs, maxUses, endpoint.trim() || undefined);
      setCreatedInvitation(res);
      loadInvitations(selectedFolder);
      if (onInvitationsChanged) onInvitationsChanged();
    } catch (err: any) {
      setError(err.message || 'Failed to generate invitation');
    } finally {
      setLoading(false);
    }
  };

  const handleCopy = () => {
    if (!createdInvitation) return;
    const textToCopy = createdInvitation.invitation_code || createdInvitation.token;
    navigator.clipboard.writeText(textToCopy).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2500);
    });
  };

  const handleRevoke = async (digest: string) => {
    if (!window.confirm('Are you sure you want to revoke this invitation? Any pending attempts with this token will fail.')) {
      return;
    }
    try {
      await api.revokeInvitation(digest);
      loadInvitations(selectedFolder);
      if (onInvitationsChanged) onInvitationsChanged();
    } catch (err: any) {
      alert(`Revoke failed: ${err.message}`);
    }
  };

  return (
    <div
      className="modal-overlay"
      role="dialog"
      aria-modal="true"
      aria-labelledby="add-device-title"
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
          maxWidth: '560px',
          maxHeight: '90vh',
          backgroundColor: 'var(--surface)',
          border: '1px solid var(--border)',
          borderRadius: 'var(--radius-lg)',
          overflowY: 'auto',
          boxShadow: '0 16px 40px rgba(0, 0, 0, 0.7)',
          display: 'flex',
          flexDirection: 'column',
        }}
      >
        {/* Modal Header */}
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
          <div>
            <h3 id="add-device-title" style={{ fontSize: '1.125rem', fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>
              Add a Device to Orbit
            </h3>
            <p style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', margin: '0.25rem 0 0 0' }}>
              Pair a second Linux device to synchronize files directly via mutual authentication.
            </p>
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
            aria-label="Close"
          >
            ✕
          </button>
        </div>

        {/* Tab Toggle */}
        <div style={{ display: 'flex', borderBottom: '1px solid var(--border)', backgroundColor: 'var(--canvas)' }}>
          <button
            type="button"
            onClick={() => setActiveTab('create')}
            style={{
              flex: 1,
              padding: '0.75rem 1rem',
              fontSize: '0.8125rem',
              fontWeight: 600,
              backgroundColor: activeTab === 'create' ? 'var(--surface)' : 'transparent',
              color: activeTab === 'create' ? 'var(--text-primary)' : 'var(--text-secondary)',
              border: 'none',
              borderBottom: activeTab === 'create' ? '2px solid var(--text-primary)' : 'none',
              cursor: 'pointer',
            }}
          >
            New Invitation
          </button>
          <button
            type="button"
            onClick={() => setActiveTab('manage')}
            style={{
              flex: 1,
              padding: '0.75rem 1rem',
              fontSize: '0.8125rem',
              fontWeight: 600,
              backgroundColor: activeTab === 'manage' ? 'var(--surface)' : 'transparent',
              color: activeTab === 'manage' ? 'var(--text-primary)' : 'var(--text-secondary)',
              border: 'none',
              borderBottom: activeTab === 'manage' ? '2px solid var(--text-primary)' : 'none',
              cursor: 'pointer',
            }}
          >
            Active Invitations ({activeInvitations.filter(i => !i.revoked).length})
          </button>
        </div>

        {/* Modal Body */}
        <div style={{ padding: '1.5rem', display: 'flex', flexDirection: 'column', gap: '1.25rem' }}>
          {error && (
            <div
              className="alert alert-danger"
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

          {activeTab === 'create' && (
            <>
              {!createdInvitation ? (
                <form onSubmit={handleGenerate} style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}>
                  {folders.length > 1 && (
                    <div>
                      <label htmlFor="invitation-folder" style={{ display: 'block', fontSize: '0.8125rem', fontWeight: 500, color: 'var(--text-primary)', marginBottom: '0.375rem' }}>
                        Workspace
                      </label>
                      <select
                        id="invitation-folder"
                        value={selectedFolder}
                        onChange={(e) => setSelectedFolder(e.target.value)}
                        className="form-input"
                        style={{ width: '100%' }}
                      >
                        {folders.map((f, idx) => (
                          <option key={idx} value={f.folder || f.folder_id}>
                            {f.root_path || formatShortID(f.folder || f.folder_id || '')}
                          </option>
                        ))}
                      </select>
                    </div>
                  )}

                  {/* Expiry Selector */}
                  <div>
                    <label htmlFor="invitation-ttl" style={{ display: 'block', fontSize: '0.8125rem', fontWeight: 500, color: 'var(--text-primary)', marginBottom: '0.375rem' }}>
                      Invitation Expiry (TTL)
                    </label>
                    <select
                      id="invitation-ttl"
                      value={ttlSecs}
                      onChange={(e) => setTtlSecs(Number(e.target.value))}
                      className="form-input"
                      style={{ width: '100%' }}
                    >
                      <option value={3600}>1 hour</option>
                      <option value={86400}>24 hours (recommended)</option>
                      <option value={604800}>7 days</option>
                    </select>
                    <span style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginTop: '0.25rem', display: 'block' }}>
                      Invitations automatically expire after this duration. Raw tokens are never logged or stored in plain text.
                    </span>
                  </div>

                  {/* Uses Limit */}
                  <div>
                    <label htmlFor="invitation-uses" style={{ display: 'block', fontSize: '0.8125rem', fontWeight: 500, color: 'var(--text-primary)', marginBottom: '0.375rem' }}>
                      Usage Limit
                    </label>
                    <select
                      id="invitation-uses"
                      value={maxUses}
                      onChange={(e) => setMaxUses(Number(e.target.value))}
                      className="form-input"
                      style={{ width: '100%' }}
                    >
                      <option value={1}>Single-use (1 device - recommended)</option>
                      <option value={3}>Multi-use (up to 3 devices)</option>
                      <option value={5}>Multi-use (up to 5 devices)</option>
                    </select>
                  </div>

                  {/* Advertised Endpoint URL */}
                  <div>
                    <label htmlFor="invitation-endpoint" style={{ display: 'block', fontSize: '0.8125rem', fontWeight: 500, color: 'var(--text-primary)', marginBottom: '0.375rem' }}>
                      Advertised Sync Endpoint (Optional)
                    </label>
                    <input
                      id="invitation-endpoint"
                      type="text"
                      placeholder="https://192.168.1.x:8443 (leave blank to auto-detect)"
                      value={endpoint}
                      onChange={(e) => setEndpoint(e.target.value)}
                      className="form-input code-font"
                      style={{ width: '100%', fontSize: '0.8125rem' }}
                    />
                    <span style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginTop: '0.25rem', display: 'block' }}>
                      If set, this address will be embedded into the invitation link for direct connection.
                    </span>
                  </div>

                  <div style={{ padding: '0.875rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', fontSize: '0.8125rem', color: 'var(--text-secondary)', lineHeight: 1.4 }}>
                    <strong style={{ color: 'var(--text-primary)' }}>Decentralized Security Notice:</strong> An invitation token grants only the capability to submit a bounded cryptographic join request. You must explicitly approve the joining device identity before it is admitted to the workspace (Invariant I23).
                  </div>

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
                      type="submit"
                      disabled={loading}
                      className="btn btn-primary"
                      id="btn-generate-invitation"
                      style={{ padding: '0.5rem 1.25rem', fontSize: '0.8125rem', fontWeight: 600 }}
                    >
                      {loading ? 'Generating…' : 'Generate Invitation'}
                    </button>
                  </div>
                </form>
              ) : (
                /* Generated Invitation Screen */
                <div style={{ display: 'flex', flexDirection: 'column', gap: '1.25rem', alignItems: 'center' }}>
                  <div style={{ textAlign: 'center' }}>
                    <div style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                      Invitation Ready for Pairing
                    </div>
                    <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginTop: '0.25rem' }}>
                      Expires: {new Date(createdInvitation.expires_at).toLocaleString()} • {createdInvitation.max_uses} use(s) allowed
                    </div>
                  </div>

                  {/* QR Code Display */}
                  <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', padding: '0.5rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)' }}>
                    <QRCodeDisplay value={createdInvitation.invitation_code || createdInvitation.token} size={180} />
                    <span style={{ fontSize: '0.6875rem', color: 'var(--text-secondary)', marginTop: '0.5rem' }}>
                      Scan from the joining device or copy the invitation code below
                    </span>
                  </div>

                  {/* Code Snippet Box */}
                  <div style={{ width: '100%' }}>
                    <label style={{ display: 'block', fontSize: '0.75rem', fontWeight: 500, color: 'var(--text-secondary)', marginBottom: '0.375rem' }}>
                      Workspace Invitation Code
                    </label>
                    <div style={{ position: 'relative' }}>
                      <textarea
                        readOnly
                        id="invitation-code-display"
                        className="code-font"
                        rows={3}
                        value={createdInvitation.invitation_code || createdInvitation.token}
                        style={{
                          width: '100%',
                          padding: '0.625rem 0.875rem',
                          backgroundColor: 'var(--canvas)',
                          border: '1px solid var(--border)',
                          borderRadius: 'var(--radius-sm)',
                          color: 'var(--text-primary)',
                          fontSize: '0.75rem',
                          resize: 'none',
                          wordBreak: 'break-all',
                        }}
                      />
                    </div>
                  </div>

                  {/* Action Buttons */}
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', width: '100%', gap: '0.75rem' }}>
                    <button
                      type="button"
                      onClick={() => setCreatedInvitation(null)}
                      className="btn btn-secondary"
                      style={{ padding: '0.5rem 1rem', fontSize: '0.8125rem' }}
                    >
                      ← Create Another
                    </button>

                    <div style={{ display: 'flex', gap: '0.75rem' }}>
                      <button
                        type="button"
                        onClick={handleCopy}
                        id="btn-copy-invitation"
                        className="btn btn-primary"
                        style={{
                          padding: '0.5rem 1.25rem',
                          fontSize: '0.8125rem',
                          fontWeight: 600,
                          backgroundColor: copied ? 'var(--status-success)' : 'var(--text-primary)',
                          color: 'var(--canvas)',
                        }}
                      >
                        {copied ? '✓ Copied to Clipboard!' : 'Copy Code'}
                      </button>
                      <button
                        type="button"
                        onClick={onClose}
                        className="btn btn-secondary"
                        style={{ padding: '0.5rem 1rem', fontSize: '0.8125rem' }}
                      >
                        Done
                      </button>
                    </div>
                  </div>
                </div>
              )}
            </>
          )}

          {activeTab === 'manage' && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
              {loadingInvs ? (
                <div style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-secondary)', fontSize: '0.8125rem' }}>
                  Loading invitations…
                </div>
              ) : activeInvitations.length === 0 ? (
                <div style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-secondary)', fontSize: '0.8125rem' }}>
                  No invitations recorded for this workspace.
                </div>
              ) : (
                activeInvitations.map((inv, idx) => {
                  const isExpired = Date.now() >= inv.expires_ns / 1000000;
                  const isRevoked = inv.revoked;
                  return (
                    <div
                      key={idx}
                      style={{
                        padding: '0.875rem 1rem',
                        backgroundColor: 'var(--surface-raised)',
                        border: '1px solid var(--border)',
                        borderRadius: 'var(--radius-sm)',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'space-between',
                        gap: '0.75rem',
                      }}
                    >
                      <div>
                        <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                          <span className="code-font" style={{ fontSize: '0.8125rem', color: 'var(--text-primary)', fontWeight: 600 }}>
                            Digest: {formatShortID(inv.digest)}
                          </span>
                          {isRevoked ? (
                            <span style={{ fontSize: '0.6875rem', padding: '0.125rem 0.375rem', borderRadius: 'var(--radius-sm)', backgroundColor: 'rgba(239, 68, 68, 0.1)', color: 'var(--status-error)', border: '1px solid var(--status-error)' }}>
                              Revoked
                            </span>
                          ) : isExpired ? (
                            <span style={{ fontSize: '0.6875rem', padding: '0.125rem 0.375rem', borderRadius: 'var(--radius-sm)', backgroundColor: 'var(--surface)', color: 'var(--text-secondary)', border: '1px solid var(--border)' }}>
                              Expired
                            </span>
                          ) : (
                            <span style={{ fontSize: '0.6875rem', padding: '0.125rem 0.375rem', borderRadius: 'var(--radius-sm)', backgroundColor: 'rgba(34, 197, 94, 0.1)', color: 'var(--status-success)', border: '1px solid var(--status-success)' }}>
                              Active ({inv.uses_count}/{inv.max_uses} uses)
                            </span>
                          )}
                        </div>
                        <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginTop: '0.25rem' }}>
                          Expires: {formatTimestamp(inv.expires_ns)}
                        </div>
                      </div>

                      {!isRevoked && !isExpired && (
                        <button
                          type="button"
                          onClick={() => handleRevoke(inv.digest)}
                          className="btn btn-secondary btn-sm"
                          style={{
                            padding: '0.25rem 0.625rem',
                            fontSize: '0.75rem',
                            color: 'var(--status-error)',
                            borderColor: 'var(--border)',
                          }}
                        >
                          Revoke
                        </button>
                      )}
                    </div>
                  );
                })
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
