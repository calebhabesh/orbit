import React, { useState, useEffect } from 'react';
import { api, formatShortID } from '../api';
import { PeerEndpointConfig, PeerTestResult } from '../types';

interface DeviceDetailModalProps {
  folder: string;
  deviceId: string;
  keyPin?: string;
  initialAlias?: string;
  isLocal?: boolean;
  revision?: number;
  isRetired?: boolean;
  onClose: () => void;
  onRefresh: () => void;
  onOpenRetire: (deviceId: string) => void;
}

export const DeviceDetailModal: React.FC<DeviceDetailModalProps> = ({
  folder,
  deviceId,
  keyPin,
  initialAlias = '',
  isLocal = false,
  revision = 1,
  isRetired = false,
  onClose,
  onRefresh,
  onOpenRetire,
}) => {
  const [alias, setAlias] = useState(initialAlias);
  const [savingAlias, setSavingAlias] = useState(false);
  const [aliasSuccess, setAliasSuccess] = useState(false);

  const [endpointUrl, setEndpointUrl] = useState('');
  const [savingEndpoint, setSavingEndpoint] = useState(false);
  const [endpointSuccess, setEndpointSuccess] = useState(false);

  const [testingEndpoint, setTestingEndpoint] = useState(false);
  const [testResult, setTestResult] = useState<PeerTestResult | null>(null);

  const [copiedField, setCopiedField] = useState<string | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  useEffect(() => {
    setAlias(initialAlias);
  }, [initialAlias]);

  // Load existing endpoint config
  useEffect(() => {
    const fetchEndpoint = async () => {
      try {
        const endpoints = await api.getPeerEndpoints();
        const ep = Array.isArray(endpoints) ? endpoints.find(
          (e) => e.device.toLowerCase() === deviceId.toLowerCase() && (!e.folder || e.folder === folder)
        ) : undefined;
        if (ep) {
          setEndpointUrl(ep.url);
        }
      } catch {
        // Non-fatal if loading endpoints fails
      }
    };
    if (!isLocal) {
      fetchEndpoint();
    }
  }, [folder, deviceId, isLocal]);

  // Handle Escape key
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [onClose]);

  const copyToClipboard = (text: string, field: string) => {
    navigator.clipboard.writeText(text);
    setCopiedField(field);
    setTimeout(() => setCopiedField(null), 2000);
  };

  const handleSaveAlias = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!alias.trim()) return;
    setSavingAlias(true);
    setErrorMessage(null);
    setAliasSuccess(false);
    try {
      await api.renameDevice(deviceId, alias.trim());
      setAliasSuccess(true);
      setTimeout(() => setAliasSuccess(false), 2500);
      onRefresh();
    } catch (err: any) {
      setErrorMessage(err.message || 'Failed to save alias');
    } finally {
      setSavingAlias(false);
    }
  };

  const handleSaveEndpoint = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!endpointUrl.trim()) return;
    setSavingEndpoint(true);
    setErrorMessage(null);
    setEndpointSuccess(false);
    try {
      const cfg: PeerEndpointConfig = {
        folder,
        device: deviceId,
        url: endpointUrl.trim(),
      };
      await api.setPeerEndpoint(cfg);
      setEndpointSuccess(true);
      setTimeout(() => setEndpointSuccess(false), 2500);
      onRefresh();
    } catch (err: any) {
      setErrorMessage(err.message || 'Failed to save peer endpoint');
    } finally {
      setSavingEndpoint(false);
    }
  };

  const handleTestEndpoint = async () => {
    if (!endpointUrl.trim()) return;
    setTestingEndpoint(true);
    setTestResult(null);
    setErrorMessage(null);
    try {
      const res = await api.testPeerEndpoint(endpointUrl.trim());
      setTestResult(res);
    } catch (err: any) {
      setTestResult({
        reachable: false,
        status: 'error',
        error: err.message || 'Connection test failed',
      });
    } finally {
      setTestingEndpoint(false);
    }
  };

  return (
    <div
      className="modal-overlay"
      role="dialog"
      aria-modal="true"
      aria-labelledby="device-detail-title"
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
          backgroundColor: 'var(--surface)',
          border: '1px solid var(--border)',
          borderRadius: 'var(--radius-lg)',
          overflow: 'hidden',
          boxShadow: '0 16px 40px rgba(0, 0, 0, 0.7)',
          maxHeight: '90vh',
          display: 'flex',
          flexDirection: 'column',
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
                backgroundColor: 'var(--surface)',
                border: '1px solid var(--border)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                color: 'var(--text-primary)',
              }}
            >
              <svg style={{ width: '1.125rem', height: '1.125rem' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <rect x="2" y="3" width="20" height="14" rx="2" ry="2" />
                <line x1="8" y1="21" x2="16" y2="21" />
                <line x1="12" y1="17" x2="12" y2="21" />
              </svg>
            </div>
            <div>
              <h3 id="device-detail-title" style={{ fontSize: '1.125rem', fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>
                {alias || `Device ${formatShortID(deviceId)}`}
              </h3>
              <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
                {isLocal ? 'This Device (Host)' : isRetired ? 'Retired Device' : 'Active Workspace Replica'}
              </div>
            </div>
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

        {/* Content */}
        <div style={{ padding: '1.5rem', display: 'flex', flexDirection: 'column', gap: '1.25rem', overflowY: 'auto' }}>
          {errorMessage && (
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
              {errorMessage}
            </div>
          )}

          {/* Device Identifiers Section */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
            <div>
              <label style={{ display: 'block', fontSize: '0.75rem', fontWeight: 600, color: 'var(--text-secondary)', marginBottom: '0.375rem', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                Full Device ID (Cryptographic Identifier)
              </label>
              <div style={{ display: 'flex', gap: '0.5rem', alignItems: 'center' }}>
                <input
                  type="text"
                  readOnly
                  value={deviceId}
                  className="code-font"
                  style={{
                    flex: 1,
                    padding: '0.5rem 0.75rem',
                    backgroundColor: 'var(--surface-raised)',
                    border: '1px solid var(--border)',
                    borderRadius: 'var(--radius-sm)',
                    color: 'var(--text-primary)',
                    fontSize: '0.75rem',
                  }}
                />
                <button
                  type="button"
                  onClick={() => copyToClipboard(deviceId, 'device_id')}
                  className="btn btn-secondary"
                  style={{ padding: '0.5rem 0.75rem', fontSize: '0.75rem', whiteSpace: 'nowrap' }}
                >
                  {copiedField === 'device_id' ? 'Copied' : 'Copy'}
                </button>
              </div>
            </div>

            {keyPin && (
              <div>
                <label style={{ display: 'block', fontSize: '0.75rem', fontWeight: 600, color: 'var(--text-secondary)', marginBottom: '0.375rem', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                  Ed25519 Public Key Pin
                </label>
                <div style={{ display: 'flex', gap: '0.5rem', alignItems: 'center' }}>
                  <input
                    type="text"
                    readOnly
                    value={keyPin}
                    className="code-font"
                    style={{
                      flex: 1,
                      padding: '0.5rem 0.75rem',
                      backgroundColor: 'var(--surface-raised)',
                      border: '1px solid var(--border)',
                      borderRadius: 'var(--radius-sm)',
                      color: 'var(--text-primary)',
                      fontSize: '0.75rem',
                    }}
                  />
                  <button
                    type="button"
                    onClick={() => copyToClipboard(keyPin, 'key_pin')}
                    className="btn btn-secondary"
                    style={{ padding: '0.5rem 0.75rem', fontSize: '0.75rem', whiteSpace: 'nowrap' }}
                  >
                    {copiedField === 'key_pin' ? 'Copied' : 'Copy'}
                  </button>
                </div>
              </div>
            )}
          </div>

          {/* Rename / Display Alias Form */}
          <form onSubmit={handleSaveAlias} style={{ padding: '1rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
            <label htmlFor="device-alias-input" style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-primary)' }}>
              Display Name / Alias
            </label>
            <div style={{ display: 'flex', gap: '0.5rem' }}>
              <input
                id="device-alias-input"
                type="text"
                placeholder="e.g. Living Room Mini, Work Laptop"
                value={alias}
                onChange={(e) => setAlias(e.target.value)}
                style={{
                  flex: 1,
                  padding: '0.5rem 0.75rem',
                  backgroundColor: 'var(--surface)',
                  border: '1px solid var(--border)',
                  borderRadius: 'var(--radius-sm)',
                  color: 'var(--text-primary)',
                  fontSize: '0.8125rem',
                }}
              />
              <button
                type="submit"
                disabled={savingAlias || !alias.trim()}
                className="btn btn-secondary"
                style={{ padding: '0.5rem 1rem', fontSize: '0.8125rem', fontWeight: 500 }}
              >
                {savingAlias ? 'Saving…' : aliasSuccess ? 'Saved ✓' : 'Save'}
              </button>
            </div>
            <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
              A friendly label used in notifications, history events, and replica cards.
            </div>
          </form>

          {/* Endpoint Configuration & Reachability Testing (for non-local peers) */}
          {!isLocal && !isRetired && (
            <div style={{ padding: '1rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <label htmlFor="peer-endpoint-input" style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                  Network Sync Endpoint
                </label>
                {testResult && (
                  <span
                    style={{
                      fontSize: '0.6875rem',
                      padding: '0.125rem 0.5rem',
                      borderRadius: 'var(--radius-sm)',
                      fontWeight: 600,
                      backgroundColor: testResult.reachable ? 'rgba(34, 197, 94, 0.15)' : 'rgba(239, 68, 68, 0.15)',
                      color: testResult.reachable ? 'var(--status-success)' : 'var(--status-error)',
                      border: `1px solid ${testResult.reachable ? 'var(--status-success)' : 'var(--status-error)'}`,
                    }}
                  >
                    {testResult.reachable ? `Reachable (${testResult.latency_ms ?? 0}ms)` : 'Unreachable'}
                  </span>
                )}
              </div>

              <div style={{ display: 'flex', gap: '0.5rem' }}>
                <input
                  id="peer-endpoint-input"
                  type="text"
                  placeholder="https://192.168.1.50:8443 or https://vps.example.com:8443"
                  value={endpointUrl}
                  onChange={(e) => setEndpointUrl(e.target.value)}
                  style={{
                    flex: 1,
                    padding: '0.5rem 0.75rem',
                    backgroundColor: 'var(--surface)',
                    border: '1px solid var(--border)',
                    borderRadius: 'var(--radius-sm)',
                    color: 'var(--text-primary)',
                    fontSize: '0.8125rem',
                  }}
                />
                <button
                  type="button"
                  id="btn-test-endpoint"
                  disabled={testingEndpoint || !endpointUrl.trim()}
                  onClick={handleTestEndpoint}
                  className="btn btn-secondary"
                  style={{ padding: '0.5rem 0.875rem', fontSize: '0.8125rem' }}
                >
                  {testingEndpoint ? 'Testing…' : 'Test'}
                </button>
                <button
                  type="button"
                  id="btn-save-endpoint"
                  disabled={savingEndpoint || !endpointUrl.trim()}
                  onClick={handleSaveEndpoint}
                  className="btn btn-secondary"
                  style={{ padding: '0.5rem 0.875rem', fontSize: '0.8125rem' }}
                >
                  {savingEndpoint ? 'Saving…' : endpointSuccess ? 'Saved ✓' : 'Save'}
                </button>
              </div>

              {testResult && !testResult.reachable && (
                <div style={{ fontSize: '0.75rem', color: 'var(--status-error)', lineHeight: 1.4 }}>
                  <strong>Diagnostic:</strong> {testResult.error || 'Failed to reach control health endpoint'}. Ensure both devices are on the same local network or VPN, and the port is open.
                </div>
              )}

              <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
                Configures the target HTTPS URL where this node connects to exchange revisions and pull content chunks.
              </div>
            </div>
          )}

          {/* Membership & Copy Status */}
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '0.75rem' }}>
            <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)' }}>
              <div style={{ fontSize: '0.6875rem', color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                Membership Revision
              </div>
              <div style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)', marginTop: '0.25rem' }}>
                Revision {revision}
              </div>
            </div>

            <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)' }}>
              <div style={{ fontSize: '0.6875rem', color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                Copy State
              </div>
              <div style={{ fontSize: '0.875rem', fontWeight: 500, color: 'var(--text-primary)', marginTop: '0.375rem' }}>
                {isLocal ? 'Complete local copy' : isRetired ? 'Retired' : 'Direct replication peer'}
              </div>
            </div>
          </div>
        </div>

        {/* Footer */}
        <div
          style={{
            padding: '1rem 1.5rem',
            borderTop: '1px solid var(--border)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            backgroundColor: 'var(--surface-raised)',
          }}
        >
          <div>
            {!isLocal && !isRetired && (
              <button
                type="button"
                id="btn-open-retire-preview"
                onClick={() => {
                  onClose();
                  onOpenRetire(deviceId);
                }}
                style={{
                  background: 'transparent',
                  border: '1px solid rgba(239, 68, 68, 0.4)',
                  color: 'var(--status-error)',
                  padding: '0.4rem 0.875rem',
                  borderRadius: 'var(--radius-sm)',
                  fontSize: '0.75rem',
                  fontWeight: 500,
                  cursor: 'pointer',
                }}
              >
                Retire Device…
              </button>
            )}
          </div>

          <button
            type="button"
            onClick={onClose}
            className="btn btn-secondary"
            style={{ padding: '0.5rem 1.25rem', fontSize: '0.8125rem' }}
          >
            Close
          </button>
        </div>
      </div>
    </div>
  );
};
