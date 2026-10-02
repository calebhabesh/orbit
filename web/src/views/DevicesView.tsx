import React, { useState, useEffect } from 'react';
import {
  FolderRecord,
  PeerListResult,
  ProductSettings,
  EnrollmentRequestRecord,
  PeerEndpointConfig,
} from '../types';
import { api, formatShortID, formatTimestamp } from '../api';
import { AddDeviceModal } from '../components/AddDeviceModal';
import { DeviceDetailModal } from '../components/DeviceDetailModal';
import { RetireDeviceModal } from '../components/RetireDeviceModal';

interface DevicesViewProps {
  folders: FolderRecord[];
  peersMap: Record<string, PeerListResult>;
  settings: ProductSettings | null;
  onRefresh: () => void;
}

export const DevicesView: React.FC<DevicesViewProps> = ({
  folders,
  peersMap,
  settings,
  onRefresh,
}) => {
  const [showAddModal, setShowAddModal] = useState(false);
  const [selectedDeviceForDetails, setSelectedDeviceForDetails] = useState<{
    deviceId: string;
    keyPin?: string;
    alias?: string;
    isLocal?: boolean;
    revision?: number;
    isRetired?: boolean;
  } | null>(null);

  const [selectedDeviceForRetire, setSelectedDeviceForRetire] = useState<{
    deviceId: string;
  } | null>(null);

  const [pendingRequests, setPendingRequests] = useState<EnrollmentRequestRecord[]>([]);
  const [requestForms, setRequestForms] = useState<Record<string, { label: string; endpoint: string; submitting: boolean; error?: string }>>({});
  const [endpoints, setEndpoints] = useState<PeerEndpointConfig[]>([]);
  const [actionMessage, setActionMessage] = useState<string | null>(null);

  const currentFolder = folders[0]?.folder || folders[0]?.folder_id || '';
  const currentPeerList = peersMap[currentFolder] || { active: [], retired: [], revision: 0 };

  const fetchEnrollmentRequests = async () => {
    if (!currentFolder) return;
    try {
      const res = await api.listEnrollmentRequests(currentFolder, 'pending');
      setPendingRequests(res.requests || []);
      // Initialize form state for each pending request
      setRequestForms((prev) => {
        const next = { ...prev };
        for (const req of res.requests || []) {
          if (!next[req.request_id]) {
            next[req.request_id] = {
              label: req.suggested_label || `Device ${formatShortID(req.device_id)}`,
              endpoint: '',
              submitting: false,
            };
          }
        }
        return next;
      });
    } catch {
      // Non-fatal
    }
  };

  const fetchEndpoints = async () => {
    try {
      const eps = await api.getPeerEndpoints();
      setEndpoints(eps || []);
    } catch {
      // Non-fatal
    }
  };

  useEffect(() => {
    fetchEnrollmentRequests();
    fetchEndpoints();
    const interval = setInterval(() => {
      fetchEnrollmentRequests();
      fetchEndpoints();
    }, 4000);
    return () => clearInterval(interval);
  }, [currentFolder]);

  const handleApprove = async (req: EnrollmentRequestRecord) => {
    const reqId = req.request_id || (req as any).RequestID || '';
    const suggested = req.suggested_label || (req as any).SuggestedLabel || '';
    const devId = req.device_id || (req as any).DeviceID || '';
    const form = requestForms[reqId] || { label: suggested, endpoint: '', submitting: false };
    setRequestForms((prev) => ({
      ...prev,
      [reqId]: { ...form, submitting: true, error: undefined },
    }));

    try {
      await api.approveEnrollment(
        reqId,
        currentFolder,
        form.label.trim() || undefined,
        form.endpoint.trim() || undefined
      );
      setActionMessage(`Approved device ${form.label || formatShortID(devId)}. Membership updated to Revision ${(currentPeerList.revision || 1) + 1}.`);
      fetchEnrollmentRequests();
      fetchEndpoints();
      onRefresh();
    } catch (err: any) {
      setRequestForms((prev) => ({
        ...prev,
        [reqId]: { ...form, submitting: false, error: err.message || 'Approval failed' },
      }));
    }
  };

  const handleDecline = async (req: EnrollmentRequestRecord) => {
    const reqId = req.request_id || (req as any).RequestID || '';
    const devId = req.device_id || (req as any).DeviceID || '';
    const form = requestForms[reqId] || { label: req.suggested_label || (req as any).SuggestedLabel || '', endpoint: '', submitting: false };
    setRequestForms((prev) => ({
      ...prev,
      [reqId]: { ...form, submitting: true, error: undefined },
    }));

    try {
      await api.declineEnrollment(reqId);
      setActionMessage(`Declined join request from device ${formatShortID(devId)}.`);
      fetchEnrollmentRequests();
      onRefresh();
    } catch (err: any) {
      setRequestForms((prev) => ({
        ...prev,
        [reqId]: { ...form, submitting: false, error: err.message || 'Decline failed' },
      }));
    }
  };

  const getEndpointForDevice = (deviceId: string): string | undefined => {
    if (!Array.isArray(endpoints)) return undefined;
    const found = endpoints.find(
      (e) => e.device.toLowerCase() === deviceId.toLowerCase() && (!e.folder || e.folder === currentFolder)
    );
    return found?.url;
  };

  const getAliasForDevice = (deviceId: string): string => {
    if (currentPeerList.aliases && currentPeerList.aliases[deviceId]) {
      return currentPeerList.aliases[deviceId];
    }
    return `Device ${formatShortID(deviceId)}`;
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '1.25rem' }}>
      
      {/* Top Banner */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '1rem 1.25rem', backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)' }}>
        <div>
          <h2 style={{ fontSize: '1.125rem', fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>
            Devices & Replicas
          </h2>
          <p style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', margin: '0.25rem 0 0 0' }}>
            Authorized devices maintaining complete local copies in this Orbit workspace.
          </p>
        </div>

        <button
          type="button"
          id="btn-add-device-main"
          onClick={() => setShowAddModal(true)}
          className="btn btn-primary"
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '0.5rem',
            padding: '0.5rem 1rem',
            fontSize: '0.8125rem',
            fontWeight: 600,
            backgroundColor: 'var(--text-primary)',
            color: 'var(--canvas)',
            border: 'none',
            borderRadius: 'var(--radius-sm)',
            cursor: 'pointer',
          }}
        >
          <svg style={{ width: '0.875rem', height: '0.875rem' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <line x1="12" y1="5" x2="12" y2="19" />
            <line x1="5" y1="12" x2="19" y2="12" />
          </svg>
          Add Device
        </button>
      </div>

      {actionMessage && (
        <div style={{ padding: '0.75rem 1rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', fontSize: '0.8125rem', color: 'var(--text-primary)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span>{actionMessage}</span>
          <button
            type="button"
            onClick={() => setActionMessage(null)}
            style={{ background: 'transparent', border: 'none', color: 'var(--text-secondary)', cursor: 'pointer' }}
          >
            ✕
          </button>
        </div>
      )}

      {/* Pending Join Requests Inbox */}
      {pendingRequests.length > 0 && (
        <div
          id="pending-join-requests-inbox"
          style={{
            backgroundColor: 'var(--surface)',
            border: '1px solid var(--border)',
            borderRadius: 'var(--radius-md)',
            overflow: 'hidden',
          }}
        >
          <div
            style={{
              padding: '0.875rem 1.25rem',
              borderBottom: '1px solid var(--border)',
              backgroundColor: 'var(--surface-raised)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
            }}
          >
            <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
              <span style={{ width: '8px', height: '8px', borderRadius: '50%', backgroundColor: 'var(--status-warning, #f59e0b)' }} />
              <span style={{ fontWeight: 600, fontSize: '0.875rem', color: 'var(--text-primary)' }}>
                Pending Join Requests ({pendingRequests.length})
              </span>
            </div>
            <span style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
              Requires explicit owner approval
            </span>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column' }}>
            {pendingRequests.map((req, idx) => {
              const reqId = req.request_id || (req as any).RequestID || String(idx);
              const devId = req.device_id || (req as any).DeviceID || '';
              const suggested = req.suggested_label || (req as any).SuggestedLabel || '';
              const keyPin = req.key_pin || (req as any).KeyPin || '';
              const form = requestForms[reqId] || { label: suggested, endpoint: '', submitting: false };
              return (
                <div
                  key={reqId}
                  className="pending-request-card"
                  style={{
                    padding: '1.25rem',
                    borderBottom: '1px solid var(--border)',
                    display: 'flex',
                    flexDirection: 'column',
                    gap: '1rem',
                    backgroundColor: 'var(--surface)',
                  }}
                >
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', flexWrap: 'wrap', gap: '0.75rem' }}>
                    <div>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                        <span style={{ fontWeight: 600, fontSize: '0.9375rem', color: 'var(--text-primary)' }}>
                          {suggested || 'New Device'}
                        </span>
                        <span className="code-font" style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', padding: '0.125rem 0.375rem', backgroundColor: 'var(--surface-raised)', borderRadius: 'var(--radius-sm)' }}>
                          {formatShortID(devId)}
                        </span>
                      </div>
                      <div className="code-font" style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginTop: '0.25rem' }}>
                        Key pin: {keyPin} • Requested {formatTimestamp(req.created_ns || (req as any).CreatedNS)}
                      </div>
                    </div>

                    <div style={{ display: 'flex', gap: '0.5rem' }}>
                      <button
                        type="button"
                        className="btn-decline-join"
                        disabled={form.submitting}
                        onClick={() => handleDecline(req)}
                        style={{
                          padding: '0.4rem 0.875rem',
                          fontSize: '0.75rem',
                          fontWeight: 500,
                          backgroundColor: 'transparent',
                          border: '1px solid var(--border)',
                          color: 'var(--text-secondary)',
                          borderRadius: 'var(--radius-sm)',
                          cursor: form.submitting ? 'not-allowed' : 'pointer',
                        }}
                      >
                        Decline
                      </button>
                      <button
                        type="button"
                        className="btn-approve-join"
                        disabled={form.submitting}
                        onClick={() => handleApprove(req)}
                        style={{
                          padding: '0.4rem 1rem',
                          fontSize: '0.75rem',
                          fontWeight: 600,
                          backgroundColor: 'var(--text-primary)',
                          color: 'var(--canvas)',
                          border: 'none',
                          borderRadius: 'var(--radius-sm)',
                          cursor: form.submitting ? 'not-allowed' : 'pointer',
                        }}
                      >
                        {form.submitting ? 'Approving…' : 'Approve & Mint Revision'}
                      </button>
                    </div>
                  </div>

                  {/* Editable Fields */}
                  <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: '0.75rem' }}>
                    <div>
                      <label style={{ display: 'block', fontSize: '0.6875rem', fontWeight: 600, color: 'var(--text-secondary)', marginBottom: '0.25rem' }}>
                        Display Name / Alias
                      </label>
                      <input
                        type="text"
                        className="input-request-label"
                        value={form.label}
                        onChange={(e) =>
                          setRequestForms((prev) => ({
                            ...prev,
                            [reqId]: { ...form, label: e.target.value },
                          }))
                        }
                        placeholder="e.g. Work Laptop"
                        style={{
                          width: '100%',
                          padding: '0.4rem 0.625rem',
                          backgroundColor: 'var(--surface-raised)',
                          border: '1px solid var(--border)',
                          borderRadius: 'var(--radius-sm)',
                          color: 'var(--text-primary)',
                          fontSize: '0.75rem',
                        }}
                      />
                    </div>

                    <div>
                      <label style={{ display: 'block', fontSize: '0.6875rem', fontWeight: 600, color: 'var(--text-secondary)', marginBottom: '0.25rem' }}>
                        Peer Endpoint URL (Optional)
                      </label>
                      <input
                        type="text"
                        className="input-request-endpoint"
                        value={form.endpoint}
                        onChange={(e) =>
                          setRequestForms((prev) => ({
                            ...prev,
                            [reqId]: { ...form, endpoint: e.target.value },
                          }))
                        }
                        placeholder="https://192.168.1.x:8443"
                        style={{
                          width: '100%',
                          padding: '0.4rem 0.625rem',
                          backgroundColor: 'var(--surface-raised)',
                          border: '1px solid var(--border)',
                          borderRadius: 'var(--radius-sm)',
                          color: 'var(--text-primary)',
                          fontSize: '0.75rem',
                        }}
                      />
                    </div>
                  </div>

                  {form.error && (
                    <div style={{ fontSize: '0.75rem', color: 'var(--status-error)' }}>
                      {form.error}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </div>
      )}

      {/* Active Devices List */}
      <div style={{ backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)', overflow: 'hidden' }}>
        <div style={{ padding: '0.875rem 1.25rem', borderBottom: '1px solid var(--border)', backgroundColor: 'var(--surface-raised)', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <span style={{ fontWeight: 600, fontSize: '0.875rem', color: 'var(--text-primary)' }}>
            Active Replicas ({currentPeerList.active.length > 0 ? currentPeerList.active.length : 1})
          </span>
          <span style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
            Membership Rev: {currentPeerList.revision || 1}
          </span>
        </div>

        <div style={{ display: 'flex', flexDirection: 'column' }}>
          {/* Always show current local device */}
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              padding: '1rem 1.25rem',
              borderBottom: currentPeerList.active.length > 0 ? '1px solid var(--border)' : 'none',
              gap: '1rem',
            }}
          >
            <div style={{ display: 'flex', alignItems: 'center', gap: '1rem' }}>
              <div style={{ width: '2.5rem', height: '2.5rem', borderRadius: 'var(--radius-sm)', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                <svg style={{ width: '1.25rem', height: '1.25rem', color: 'var(--text-primary)' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <rect x="2" y="3" width="20" height="14" rx="2" ry="2" />
                  <line x1="8" y1="21" x2="16" y2="21" />
                  <line x1="12" y1="17" x2="12" y2="21" />
                </svg>
              </div>
              <div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                  <span style={{ fontWeight: 600, fontSize: '0.875rem', color: 'var(--text-primary)' }}>
                    {settings?.device_label || 'Local Computer'}
                  </span>
                  <span style={{ fontSize: '0.6875rem', padding: '0.125rem 0.375rem', borderRadius: 'var(--radius-sm)', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', color: 'var(--text-secondary)' }}>
                    This Device
                  </span>
                </div>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginTop: '0.25rem' }}>
                  Complete local copy • Daemon running
                </div>
              </div>
            </div>

            <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '0.375rem', fontSize: '0.8125rem', color: 'var(--status-success)' }}>
                <span style={{ width: '6px', height: '6px', borderRadius: '50%', backgroundColor: 'var(--status-success)' }} />
                Active
              </div>
              <button
                type="button"
                onClick={() =>
                  setSelectedDeviceForDetails({
                    deviceId: settings?.default_workspace || 'local',
                    alias: settings?.device_label || 'Local Computer',
                    isLocal: true,
                    revision: currentPeerList.revision || 1,
                  })
                }
                className="btn btn-secondary btn-sm"
                style={{ padding: '0.375rem 0.625rem', fontSize: '0.75rem' }}
              >
                Details
              </button>
            </div>
          </div>

          {/* Other Active Replicas */}
          {currentPeerList.active.map((peer, idx) => {
            const devId = peer.device || (peer as any).Device || '';
            const keyPin = peer.key_pin || (peer as any).KeyPin || '';
            const ep = getEndpointForDevice(devId);
            const alias = getAliasForDevice(devId);
            return (
              <div
                key={devId || idx}
                className="active-replica-row"
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  padding: '1rem 1.25rem',
                  borderBottom: idx < currentPeerList.active.length - 1 ? '1px solid var(--border)' : 'none',
                  gap: '1rem',
                }}
              >
                <div style={{ display: 'flex', alignItems: 'center', gap: '1rem' }}>
                  <div style={{ width: '2.5rem', height: '2.5rem', borderRadius: 'var(--radius-sm)', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                    <svg style={{ width: '1.25rem', height: '1.25rem', color: 'var(--text-secondary)' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                      <rect x="2" y="2" width="20" height="8" rx="2" ry="2" />
                      <rect x="2" y="14" width="20" height="8" rx="2" ry="2" />
                      <line x1="6" y1="6" x2="6.01" y2="6" />
                      <line x1="6" y1="18" x2="6.01" y2="18" />
                    </svg>
                  </div>
                  <div>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                      <span style={{ fontWeight: 600, fontSize: '0.875rem', color: 'var(--text-primary)' }}>
                        {alias}
                      </span>
                      <span className="code-font" style={{ fontSize: '0.6875rem', color: 'var(--text-secondary)', padding: '0.0625rem 0.25rem', backgroundColor: 'var(--surface-raised)', borderRadius: 'var(--radius-sm)' }}>
                        {formatShortID(devId)}
                      </span>
                    </div>
                    <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginTop: '0.25rem' }}>
                      {ep ? (
                        <span>Replication endpoint: <code className="code-font">{ep}</code></span>
                      ) : (
                        <span>Direct sync • No peer endpoint configured</span>
                      )}
                    </div>
                  </div>
                </div>

                <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                  <button
                    type="button"
                    onClick={() =>
                      setSelectedDeviceForDetails({
                        deviceId: devId,
                        keyPin,
                        alias,
                        isLocal: false,
                        revision: currentPeerList.revision || 1,
                      })
                    }
                    className="btn btn-secondary btn-sm"
                    style={{ padding: '0.375rem 0.625rem', fontSize: '0.75rem' }}
                  >
                    Details
                  </button>
                  <button
                    type="button"
                    onClick={() => setSelectedDeviceForRetire({ deviceId: devId })}
                    className="btn btn-secondary btn-sm"
                    style={{ padding: '0.375rem 0.625rem', fontSize: '0.75rem', color: 'var(--status-error)' }}
                  >
                    Retire
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* Retired Devices Section */}
      {currentPeerList.retired && currentPeerList.retired.length > 0 && (
        <div style={{ backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)', overflow: 'hidden' }}>
          <div style={{ padding: '0.875rem 1.25rem', borderBottom: '1px solid var(--border)', backgroundColor: 'var(--surface-raised)' }}>
            <span style={{ fontWeight: 600, fontSize: '0.875rem', color: 'var(--text-secondary)' }}>
              Retired Devices ({currentPeerList.retired.length})
            </span>
          </div>
          <div style={{ display: 'flex', flexDirection: 'column' }}>
            {currentPeerList.retired.map((ret, idx) => (
              <div key={idx} style={{ padding: '0.875rem 1.25rem', borderBottom: idx < currentPeerList.retired.length - 1 ? '1px solid var(--border)' : 'none', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
                  <span style={{ fontWeight: 500, fontSize: '0.8125rem', color: 'var(--text-secondary)' }}>
                    {getAliasForDevice(ret.device)}
                  </span>
                  <span className="code-font" style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
                    ({formatShortID(ret.device)})
                  </span>
                </div>
                <span style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
                  Permanently retired • Revision {ret.retired_at || currentPeerList.revision}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Add Device Modal */}
      {showAddModal && (
        <AddDeviceModal
          folders={folders}
          currentFolderId={currentFolder}
          onClose={() => setShowAddModal(false)}
        />
      )}

      {/* Device Detail Modal */}
      {selectedDeviceForDetails && (
        <DeviceDetailModal
          folder={currentFolder}
          deviceId={selectedDeviceForDetails.deviceId}
          keyPin={selectedDeviceForDetails.keyPin}
          initialAlias={selectedDeviceForDetails.alias}
          isLocal={selectedDeviceForDetails.isLocal}
          revision={selectedDeviceForDetails.revision}
          isRetired={selectedDeviceForDetails.isRetired}
          onClose={() => setSelectedDeviceForDetails(null)}
          onRefresh={() => {
            fetchEndpoints();
            onRefresh();
          }}
          onOpenRetire={(devId: string) => {
            setSelectedDeviceForDetails(null);
            setSelectedDeviceForRetire({ deviceId: devId });
          }}
        />
      )}

      {/* Retire Device Modal */}
      {selectedDeviceForRetire && (
        <RetireDeviceModal
          folder={currentFolder}
          deviceId={selectedDeviceForRetire.deviceId}
          onClose={() => setSelectedDeviceForRetire(null)}
          onSuccess={() => {
            setActionMessage('Device was retired. Surviving replicas updated.');
            onRefresh();
          }}
        />
      )}
    </div>
  );
};
