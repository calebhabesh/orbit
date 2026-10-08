import React, { useState, useEffect } from 'react';
import {
  FolderRecord,
  ProductSettings,
  ServiceStatusResult,
  StorageUsage,
  DoctorReport,
  RecoveryInspectionResult,
} from '../types';
import { api, formatBytes, APIError } from '../api';
import { DoctorModal } from '../components/DoctorModal';
import { RetentionModal } from '../components/RetentionModal';
import { RelocateFolderForm } from '../components/RelocateFolderForm';
import { UnregisterModal } from '../components/UnregisterModal';

interface SettingsViewProps {
  folders: FolderRecord[];
  settings: ProductSettings | null;
  storageUsage: StorageUsage | null;
  doctorReport: DoctorReport | null;
  onRefresh: () => void;
  onLogout: () => void;
}

export const SettingsView: React.FC<SettingsViewProps> = ({
  folders,
  settings,
  storageUsage,
  doctorReport,
  onRefresh,
  onLogout,
}) => {
  // Device label editing
  const [deviceLabel, setDeviceLabel] = useState(settings?.device_label || '');
  const [savingLabel, setSavingLabel] = useState(false);
  const [labelMessage, setLabelMessage] = useState<string | null>(null);

  // Service status
  const [serviceStatus, setServiceStatus] = useState<ServiceStatusResult | null>(null);
  const [loadingService, setLoadingService] = useState(false);
  const [serviceActionMessage, setServiceActionMessage] = useState<string | null>(null);

  // Modals
  const [showDoctorModal, setShowDoctorModal] = useState(false);
  const [retentionFolder, setRetentionFolder] = useState<string | null>(null);
  const [unregisterTarget, setUnregisterTarget] = useState<{ folderId: string; rootPath: string } | null>(null);

  // Storage actions
  const [runningGC, setRunningGC] = useState(false);
  const [gcMessage, setGcMessage] = useState<string | null>(null);
  const [reclaiming, setReclaiming] = useState(false);
  const [reclaimMessage, setReclaimMessage] = useState<string | null>(null);
  const [checkingIntegrity, setCheckingIntegrity] = useState(false);
  const [integrityMessage, setIntegrityMessage] = useState<string | null>(null);

  // Maintenance & Recovery
  const [recoveryInspection, setRecoveryInspection] = useState<RecoveryInspectionResult | null>(null);
  const [creatingBackup, setCreatingBackup] = useState(false);
  const [backupMessage, setBackupMessage] = useState<string | null>(null);
  const [pruningRecords, setPruningRecords] = useState(false);
  const [pruneMessage, setPruneMessage] = useState<string | null>(null);
  const [showLostDeviceGuide, setShowLostDeviceGuide] = useState(false);

  // Diagnostics
  const [exportingSupport, setExportingSupport] = useState(false);
  const [supportMessage, setSupportMessage] = useState<string | null>(null);

  useEffect(() => {
    if (settings?.device_label) {
      setDeviceLabel(settings.device_label);
    }
  }, [settings?.device_label]);

  useEffect(() => {
    loadServiceStatus();
    loadRecoveryInspection();
  }, []);

  const loadServiceStatus = async () => {
    try {
      const st = await api.getServiceStatus();
      setServiceStatus(st);
    } catch {
      // Ignore if unavailable
    }
  };

  const loadRecoveryInspection = async () => {
    try {
      const insp = await api.getRecoveryInspection();
      setRecoveryInspection(insp);
    } catch {
      // Ignore if unavailable
    }
  };

  const handleSaveDeviceLabel = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!deviceLabel.trim()) return;
    setSavingLabel(true);
    setLabelMessage(null);
    try {
      await api.updateSettings({ device_label: deviceLabel.trim() });
      setLabelMessage('Device name updated.');
      onRefresh();
      setTimeout(() => setLabelMessage(null), 3000);
    } catch (err: any) {
      setLabelMessage(`Failed to update: ${err.message}`);
    } finally {
      setSavingLabel(false);
    }
  };

  const handleServiceAction = async (action: 'enable' | 'start' | 'stop' | 'restart') => {
    setLoadingService(true);
    setServiceActionMessage(null);
    try {
      const res = await api.serviceAction(action);
      setServiceStatus(res.status);
      setServiceActionMessage(res.message || `Action ${action} completed.`);
      setTimeout(() => setServiceActionMessage(null), 4000);
    } catch (err: any) {
      if (err instanceof APIError) {
        setServiceActionMessage(`${err.code}: ${err.message}`);
      } else {
        setServiceActionMessage(err.message || 'Service operation failed');
      }
    } finally {
      setLoadingService(false);
    }
  };

  const handlePauseFolder = async (folderId: string) => {
    try {
      await api.pauseFolder(folderId, 'USER_PAUSED');
      onRefresh();
    } catch (err: any) {
      alert(`Pause failed: ${err.message}`);
    }
  };

  const handleResumeFolder = async (folderId: string) => {
    try {
      await api.resumeFolder(folderId);
      onRefresh();
    } catch (err: any) {
      alert(`Resume failed: ${err.message}`);
    }
  };

  const handleRunGC = async () => {
    const fid = folders[0]?.folder || folders[0]?.folder_id;
    if (!fid) return;
    setRunningGC(true);
    setGcMessage(null);
    try {
      const res = await api.runGC(fid);
      const reclaimed = res?.report?.reclaimed_bytes ?? 0;
      setGcMessage(`Garbage collection completed. Reclaimed ${formatBytes(reclaimed)} unreferenced historical chunks.`);
      onRefresh();
      setTimeout(() => setGcMessage(null), 5000);
    } catch (err: any) {
      setGcMessage(`Cleanup failed: ${err.message}`);
    } finally {
      setRunningGC(false);
    }
  };

  const handleReclaimRecovery = async () => {
    const fid = folders[0]?.folder || folders[0]?.folder_id;
    if (!fid) return;
    setReclaiming(true);
    setReclaimMessage(null);
    try {
      const res = await api.reclaimRecovery(fid);
      setReclaimMessage(`Reclaimed ${res.reclaimed_count} displaced recovery files (${formatBytes(res.reclaimed_bytes)}).`);
      onRefresh();
      setTimeout(() => setReclaimMessage(null), 5000);
    } catch (err: any) {
      setReclaimMessage(`Reclaim failed: ${err.message}`);
    } finally {
      setReclaiming(false);
    }
  };

  const handleCheckIntegrity = async () => {
    setCheckingIntegrity(true);
    setIntegrityMessage(null);
    try {
      const fid = folders[0]?.folder || folders[0]?.folder_id;
      const res = await api.checkStorageIntegrity(fid, false);
      const clean = res?.clean_chunks ?? 0;
      const total = res?.total_chunks_checked ?? 0;
      const corrupt = res?.corrupt_chunks?.length ?? 0;
      if (corrupt > 0) {
        setIntegrityMessage(`Integrity check found ${corrupt} corrupt chunk(s) across ${total} inspected.`);
      } else {
        setIntegrityMessage(`Integrity check clean: all ${clean} inspected chunks verified.`);
      }
      setTimeout(() => setIntegrityMessage(null), 5000);
    } catch (err: any) {
      setIntegrityMessage(`Integrity check error: ${err.message}`);
    } finally {
      setCheckingIntegrity(false);
    }
  };

  const handleCreateBackup = async () => {
    setCreatingBackup(true);
    setBackupMessage(null);
    try {
      const res = await api.createBackup();
      setBackupMessage(`Consistent database snapshot created (${formatBytes(res.size_bytes)}): ${res.backup_path}`);
      setTimeout(() => setBackupMessage(null), 7000);
    } catch (err: any) {
      setBackupMessage(`Backup creation failed: ${err.message}`);
    } finally {
      setCreatingBackup(false);
    }
  };

  const handlePruneRecords = async () => {
    setPruningRecords(true);
    setPruneMessage(null);
    try {
      const res = await api.pruneLifecycleRecords(86400);
      const r = res.report;
      setPruneMessage(`Pruned ${r.total_pruned} finished lifecycle records: ${r.tasks_pruned} tasks, ${r.invitations_pruned} invitations, ${r.enrollment_requests_pruned} requests, ${r.read_leases_pruned} leases, ${r.operations_pruned} operations.`);
      onRefresh();
      setTimeout(() => setPruneMessage(null), 7000);
    } catch (err: any) {
      setPruneMessage(`Prune failed: ${err.message}`);
    } finally {
      setPruningRecords(false);
    }
  };

  const handleExportSupport = async () => {
    setExportingSupport(true);
    setSupportMessage(null);
    try {
      const res = await api.exportSupport(false);
      setSupportMessage(`Support export created: ${res.file_path || 'Ready'}`);
      setTimeout(() => setSupportMessage(null), 5000);
    } catch (err: any) {
      setSupportMessage(`Export failed: ${err.message}`);
    } finally {
      setExportingSupport(false);
    }
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '1.5rem', maxWidth: '840px' }}>
      
      {/* 1. GENERAL PREFERENCES */}
      <section style={{ backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)', padding: '1.5rem' }}>
        <h3 style={{ fontSize: '1.125rem', fontWeight: 600, color: 'var(--text-primary)', margin: '0 0 0.5rem 0' }}>
          Device & Appearance
        </h3>
        <p style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', margin: '0 0 1.25rem 0' }}>
          Owner-local display name and preferences. Changes here do not rotate cryptographic identity or wire keys.
        </p>

        <form onSubmit={handleSaveDeviceLabel} style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}>
          <div>
            <label htmlFor="settings-device-label" style={{ display: 'block', fontSize: '0.875rem', fontWeight: 500, color: 'var(--text-primary)', marginBottom: '0.375rem' }}>
              Device Label
            </label>
            <div style={{ display: 'flex', gap: '0.5rem' }}>
              <input
                id="settings-device-label"
                type="text"
                className="form-input"
                value={deviceLabel}
                onChange={(e) => setDeviceLabel(e.target.value)}
                placeholder="linux-workstation"
                style={{ flex: 1, padding: '0.5rem 0.75rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-primary)', fontSize: '0.875rem' }}
              />
              <button
                type="submit"
                id="btn-save-device-label"
                disabled={savingLabel || deviceLabel === settings?.device_label}
                className="btn btn-secondary"
                style={{ padding: '0.5rem 1rem', fontSize: '0.8125rem', fontWeight: 600, backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', color: 'var(--text-primary)', borderRadius: 'var(--radius-sm)', cursor: 'pointer' }}
              >
                {savingLabel ? 'Saving…' : 'Save Name'}
              </button>
            </div>
            {labelMessage && (
              <span style={{ fontSize: '0.75rem', color: 'var(--status-success)', marginTop: '0.25rem', display: 'block' }}>
                {labelMessage}
              </span>
            )}
          </div>
        </form>
      </section>

      {/* 2. LOCAL SYNC ROOTS & WORKSPACES (U04) */}
      <section style={{ backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)', padding: '1.5rem' }}>
        <h3 style={{ fontSize: '1.125rem', fontWeight: 600, color: 'var(--text-primary)', margin: '0 0 0.5rem 0' }}>
          Workspaces & Sync Folders
        </h3>
        <p style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', margin: '0 0 1.25rem 0' }}>
          Registered root directories continuously synchronized. Unregistering preserves all your local files and emits no deletion.
        </p>

        {folders.length > 0 ? (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}>
            {folders.map((f) => {
              const fid = f.folder || f.folder_id || '';
              const root = f.root_path || fid;
              const isPaused = f.paused;
              const isUnavailable = f.pause_reason === 'ROOT_UNAVAILABLE';

              return (
                <div
                  key={fid}
                  style={{
                    padding: '1rem',
                    backgroundColor: 'var(--surface-raised)',
                    border: '1px solid var(--border)',
                    borderRadius: 'var(--radius-sm)',
                    display: 'flex',
                    flexDirection: 'column',
                    gap: '0.75rem',
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: '0.5rem' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                      <span className="code-font" style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)' }}>
                        {fid.slice(0, 12)}…
                      </span>
                      {f.display_name && (
                        <span style={{ fontSize: '0.875rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                          ({f.display_name})
                        </span>
                      )}
                    </div>
                    <div>
                      {isUnavailable ? (
                        <span style={{ fontSize: '0.75rem', padding: '0.125rem 0.375rem', borderRadius: 'var(--radius-sm)', backgroundColor: 'var(--status-danger-bg)', color: 'var(--status-danger)', border: '1px solid var(--status-danger)' }}>
                          Root Unavailable
                        </span>
                      ) : isPaused ? (
                        <span style={{ fontSize: '0.75rem', padding: '0.125rem 0.375rem', borderRadius: 'var(--radius-sm)', backgroundColor: 'var(--status-warning-bg)', color: 'var(--status-warning)', border: '1px solid var(--status-warning)' }}>
                          Paused ({f.pause_reason || 'Manual'})
                        </span>
                      ) : (
                        <span style={{ fontSize: '0.75rem', padding: '0.125rem 0.375rem', borderRadius: 'var(--radius-sm)', backgroundColor: 'var(--status-success-bg)', color: 'var(--status-success)', border: '1px solid var(--status-success)' }}>
                          Verified & Active
                        </span>
                      )}
                    </div>
                  </div>

                  <div className="code-font" style={{ fontSize: '0.875rem', color: 'var(--text-primary)', wordBreak: 'break-all' }}>
                    {root}
                  </div>

                  <RelocateFolderForm folderId={fid} rootPath={root} onSuccess={onRefresh} />

                  {/* Folder Actions */}
                  <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap', marginTop: '0.25rem' }}>
                    <button
                      type="button"
                      id={`btn-revalidate-root-${fid}`}
                      onClick={() => api.revalidateFolder(fid).then(onRefresh)}
                      className="btn btn-secondary btn-sm"
                      style={{ padding: '0.375rem 0.75rem', fontSize: '0.75rem', backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-primary)', cursor: 'pointer' }}
                    >
                      Revalidate Root
                    </button>
                    <button
                      type="button"
                      onClick={() => api.openLocalFolder(fid)}
                      className="btn btn-secondary btn-sm"
                      style={{ padding: '0.375rem 0.75rem', fontSize: '0.75rem', backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-primary)', cursor: 'pointer' }}
                    >
                      Open in File Manager
                    </button>
                    {isPaused ? (
                      <button
                        type="button"
                        id={`btn-resume-folder-${fid}`}
                        onClick={() => handleResumeFolder(fid)}
                        className="btn btn-secondary btn-sm"
                        style={{ padding: '0.375rem 0.75rem', fontSize: '0.75rem', backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--status-success)', cursor: 'pointer' }}
                      >
                        Resume Sync
                      </button>
                    ) : (
                      <button
                        type="button"
                        id={`btn-pause-folder-${fid}`}
                        onClick={() => handlePauseFolder(fid)}
                        className="btn btn-secondary btn-sm"
                        style={{ padding: '0.375rem 0.75rem', fontSize: '0.75rem', backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-secondary)', cursor: 'pointer' }}
                      >
                        Pause Sync
                      </button>
                    )}
                    <button
                      type="button"
                      id={`btn-unregister-folder-${fid}`}
                      onClick={() => setUnregisterTarget({ folderId: fid, rootPath: root })}
                      className="btn btn-secondary btn-sm"
                      style={{ padding: '0.375rem 0.75rem', fontSize: '0.75rem', backgroundColor: 'transparent', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--status-danger)', cursor: 'pointer' }}
                    >
                      Unregister
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        ) : (
          <div style={{ color: 'var(--text-secondary)', fontSize: '0.875rem' }}>No workspace registered.</div>
        )}
      </section>

      {/* 3. SYSTEMD USER SERVICE LIFECYCLE (U12) */}
      <section style={{ backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)', padding: '1.5rem' }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '0.5rem' }}>
          <h3 style={{ fontSize: '1.125rem', fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>
            System Startup & Background Service
          </h3>
          {serviceStatus && (
            <span style={{ fontSize: '0.75rem', padding: '0.125rem 0.375rem', borderRadius: 'var(--radius-sm)', backgroundColor: serviceStatus.currently_running ? 'var(--status-success-bg)' : 'var(--status-warning-bg)', color: serviceStatus.currently_running ? 'var(--status-success)' : 'var(--status-warning)', border: `1px solid ${serviceStatus.currently_running ? 'var(--status-success)' : 'var(--status-warning)'}` }}>
              {serviceStatus.currently_running ? 'Daemon Running' : 'Daemon Inactive'}
            </span>
          )}
        </div>
        <p style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', margin: '0 0 1.25rem 0' }}>
          Integrates with systemd user session for seamless startup on desktop login.
        </p>

        {serviceActionMessage && (
          <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', fontSize: '0.8125rem', color: 'var(--text-primary)', marginBottom: '1rem' }}>
            {serviceActionMessage}
          </div>
        )}

        {serviceStatus && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
            {/* 7 Status Indicators */}
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: '0.75rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', padding: '1rem' }}>
              <div style={{ fontSize: '0.8125rem' }}>
                <span style={{ color: 'var(--text-secondary)' }}>Systemd Available: </span>
                <span style={{ fontWeight: 600, color: serviceStatus.systemd_available ? 'var(--status-success)' : 'var(--status-warning)' }}>
                  {serviceStatus.systemd_available ? 'Yes' : 'No'}
                </span>
              </div>
              <div style={{ fontSize: '0.8125rem' }}>
                <span style={{ color: 'var(--text-secondary)' }}>Unit Installed: </span>
                <span style={{ fontWeight: 600, color: serviceStatus.unit_installed ? 'var(--status-success)' : 'var(--text-secondary)' }}>
                  {serviceStatus.unit_installed ? 'Yes' : 'No'}
                </span>
              </div>
              <div style={{ fontSize: '0.8125rem' }}>
                <span style={{ color: 'var(--text-secondary)' }}>Enabled on Login: </span>
                <span style={{ fontWeight: 600, color: serviceStatus.enabled_on_login ? 'var(--status-success)' : 'var(--text-secondary)' }}>
                  {serviceStatus.enabled_on_login ? 'Yes' : 'Disabled'}
                </span>
              </div>
              <div style={{ fontSize: '0.8125rem' }}>
                <span style={{ color: 'var(--text-secondary)' }}>Currently Running: </span>
                <span style={{ fontWeight: 600, color: serviceStatus.currently_running ? 'var(--status-success)' : 'var(--status-warning)' }}>
                  {serviceStatus.currently_running ? 'Active' : 'Inactive'}
                </span>
              </div>
              <div style={{ fontSize: '0.8125rem' }}>
                <span style={{ color: 'var(--text-secondary)' }}>Root Verified: </span>
                <span style={{ fontWeight: 600, color: serviceStatus.root_verified ? 'var(--status-success)' : 'var(--status-warning)' }}>
                  {serviceStatus.root_verified ? 'Yes' : 'Pending'}
                </span>
              </div>
              <div style={{ fontSize: '0.8125rem' }}>
                <span style={{ color: 'var(--text-secondary)' }}>Initial Capture: </span>
                <span style={{ fontWeight: 600, color: serviceStatus.capture_successful ? 'var(--status-success)' : 'var(--text-secondary)' }}>
                  {serviceStatus.capture_successful ? 'Complete' : 'In Progress'}
                </span>
              </div>
            </div>

            {/* Actions */}
            {serviceStatus.systemd_available && (
              <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap', marginTop: '0.5rem' }}>
                <button
                  type="button"
                  onClick={() => handleServiceAction('enable')}
                  disabled={loadingService}
                  className="btn btn-secondary btn-sm"
                  style={{ padding: '0.4375rem 0.875rem', fontSize: '0.8125rem', fontWeight: 500, backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-primary)', cursor: 'pointer' }}
                >
                  Enable on Login
                </button>
                <button
                  type="button"
                  onClick={() => handleServiceAction('start')}
                  disabled={loadingService}
                  className="btn btn-secondary btn-sm"
                  style={{ padding: '0.4375rem 0.875rem', fontSize: '0.8125rem', fontWeight: 500, backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-primary)', cursor: 'pointer' }}
                >
                  Start Service
                </button>
                <button
                  type="button"
                  id="btn-restart-service"
                  onClick={() => handleServiceAction('restart')}
                  disabled={loadingService}
                  className="btn btn-secondary btn-sm"
                  style={{ padding: '0.4375rem 0.875rem', fontSize: '0.8125rem', fontWeight: 500, backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-primary)', cursor: 'pointer' }}
                >
                  Restart Service
                </button>
                <button
                  type="button"
                  onClick={() => handleServiceAction('stop')}
                  disabled={loadingService}
                  className="btn btn-secondary btn-sm"
                  style={{ padding: '0.4375rem 0.875rem', fontSize: '0.8125rem', fontWeight: 500, backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-secondary)', cursor: 'pointer' }}
                >
                  Stop Service
                </button>
              </div>
            )}

            {/* Lingering Guidance */}
            {serviceStatus.lingering_instruction && (
              <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
                <strong>Always-on Storage Host (Optional):</strong> {serviceStatus.lingering_instruction}
              </div>
            )}

            {/* Fallback Manual Command */}
            {!serviceStatus.systemd_available && serviceStatus.manual_command && (
              <div>
                <span style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>Manual Startup Command:</span>
                <code className="code-font" style={{ display: 'block', padding: '0.5rem 0.75rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-primary)', fontSize: '0.8125rem', marginTop: '0.25rem' }}>
                  {serviceStatus.manual_command}
                </code>
              </div>
            )}
          </div>
        )}
      </section>

      {/* 4. STORAGE USAGE & ACCOUNTING (S14, S17) */}
      <section style={{ backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)', padding: '1.5rem' }}>
        <h3 style={{ fontSize: '1.125rem', fontWeight: 600, color: 'var(--text-primary)', margin: '0 0 0.5rem 0' }}>
          Storage Accounting & Capacity Limits
        </h3>
        <p style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', margin: '0 0 1.25rem 0' }}>
          Accounting across working roots, immutable objects, staging, recovery copies, metadata, and filesystem free space.
        </p>

        {/* Capacity Warnings */}
        {storageUsage?.warnings && storageUsage.warnings.length > 0 && (
          <div style={{ padding: '0.75rem 1rem', backgroundColor: 'var(--status-warning-bg)', border: '1px solid var(--status-warning)', borderRadius: 'var(--radius-sm)', fontSize: '0.8125rem', color: 'var(--status-warning)', marginBottom: '1rem', display: 'flex', flexDirection: 'column', gap: '0.25rem' }}>
            <strong style={{ display: 'flex', alignItems: 'center', gap: '0.375rem' }}>
              ⚠ Capacity Warning:
            </strong>
            {storageUsage.warnings.map((w, i) => (
              <div key={i}>{w}</div>
            ))}
          </div>
        )}

        {storageUsage && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '1rem', marginBottom: '1.25rem' }}>
            {/* 5 Distinct Categories */}
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))', gap: '0.75rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', padding: '1rem' }}>
              <div>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>Working Root Files</div>
                <div style={{ fontSize: '1.125rem', fontWeight: 600, color: 'var(--text-primary)', marginTop: '0.25rem' }}>
                  {formatBytes(storageUsage.total_working_root_bytes ?? 0)}
                </div>
              </div>
              <div>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>Managed Objects (CAS)</div>
                <div style={{ fontSize: '1.125rem', fontWeight: 600, color: 'var(--text-primary)', marginTop: '0.25rem' }}>
                  {formatBytes(storageUsage.object_bytes)}
                </div>
              </div>
              <div>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>Staging & Recovery</div>
                <div style={{ fontSize: '1.125rem', fontWeight: 600, color: 'var(--text-primary)', marginTop: '0.25rem' }}>
                  {formatBytes(storageUsage.staging_bytes + (storageUsage.recovery_bytes || 0))}
                </div>
              </div>
              <div>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>SQLite Metadata</div>
                <div style={{ fontSize: '1.125rem', fontWeight: 600, color: 'var(--text-primary)', marginTop: '0.25rem' }}>
                  {formatBytes(storageUsage.metadata_bytes)}
                </div>
              </div>
              <div>
                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>State Filesystem Free</div>
                <div style={{ fontSize: '1.125rem', fontWeight: 600, color: (storageUsage.state_filesystem?.available_bytes || 0) < (storageUsage.free_space_reserve_bytes || 512*1024*1024) ? 'var(--status-warning)' : 'var(--status-success)', marginTop: '0.25rem' }}>
                  {formatBytes(storageUsage.state_filesystem?.available_bytes ?? 0)}
                </div>
              </div>
            </div>

            {/* Default Limits & Reserves */}
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: '0.75rem', padding: '0.75rem 1rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', fontSize: '0.8125rem' }}>
              <div>
                <span style={{ color: 'var(--text-secondary)' }}>Metadata Budget: </span>
                <span style={{ fontWeight: 600, color: 'var(--text-primary)' }}>{formatBytes(storageUsage.metadata_budget_bytes || 256*1024*1024)}</span>
              </div>
              <div>
                <span style={{ color: 'var(--text-secondary)' }}>Free Space Reserve: </span>
                <span style={{ fontWeight: 600, color: 'var(--text-primary)' }}>{formatBytes(storageUsage.free_space_reserve_bytes || 512*1024*1024)}</span>
              </div>
            </div>
          </div>
        )}

        {gcMessage && (
          <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', fontSize: '0.8125rem', color: 'var(--text-primary)', marginBottom: '1rem' }}>
            {gcMessage}
          </div>
        )}
        {reclaimMessage && (
          <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', fontSize: '0.8125rem', color: 'var(--text-primary)', marginBottom: '1rem' }}>
            {reclaimMessage}
          </div>
        )}
        {integrityMessage && (
          <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', fontSize: '0.8125rem', color: 'var(--text-primary)', marginBottom: '1rem' }}>
            {integrityMessage}
          </div>
        )}

        {/* Storage Action Buttons */}
        <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
          <button
            type="button"
            id="btn-open-retention-modal"
            onClick={() => setRetentionFolder(folders[0]?.folder || folders[0]?.folder_id || '')}
            className="btn btn-secondary btn-sm"
            style={{ padding: '0.4375rem 0.875rem', fontSize: '0.8125rem', fontWeight: 500, backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-primary)', cursor: 'pointer' }}
          >
            Retention & GC Preview
          </button>
          <button
            type="button"
            id="btn-run-gc-settings"
            onClick={handleRunGC}
            disabled={runningGC}
            className="btn btn-secondary btn-sm"
            style={{ padding: '0.4375rem 0.875rem', fontSize: '0.8125rem', fontWeight: 500, backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-primary)', cursor: 'pointer' }}
          >
            {runningGC ? 'Cleaning up…' : 'Run Storage Cleanup (GC)'}
          </button>
          <button
            type="button"
            id="btn-reclaim-recovery"
            onClick={handleReclaimRecovery}
            disabled={reclaiming}
            className="btn btn-secondary btn-sm"
            style={{ padding: '0.4375rem 0.875rem', fontSize: '0.8125rem', fontWeight: 500, backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-primary)', cursor: 'pointer' }}
          >
            {reclaiming ? 'Reclaiming…' : 'Reclaim Recovery Copies'}
          </button>
          <button
            type="button"
            id="btn-storage-check"
            onClick={handleCheckIntegrity}
            disabled={checkingIntegrity}
            className="btn btn-secondary btn-sm"
            style={{ padding: '0.4375rem 0.875rem', fontSize: '0.8125rem', fontWeight: 500, backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-primary)', cursor: 'pointer' }}
          >
            {checkingIntegrity ? 'Checking…' : 'Check Chunk Integrity'}
          </button>
        </div>
      </section>

      {/* 5. MAINTENANCE & RECOVERY STATUS (U14, Invariant I28) */}
      <section style={{ backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)', padding: '1.5rem' }}>
        <h3 style={{ fontSize: '1.125rem', fontWeight: 600, color: 'var(--text-primary)', margin: '0 0 0.5rem 0' }}>
          Maintenance & Replacement Recovery
        </h3>
        <p style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', margin: '0 0 1.25rem 0' }}>
          Database snapshots, lifecycle record pruning, and lost-device replacement recovery.
        </p>

        {/* Consistency Status */}
        {recoveryInspection && (
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '0.75rem 1rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', marginBottom: '1rem' }}>
            <span style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)' }}>
              Database & Identity Consistency (Invariant I20):
            </span>
            <span style={{ fontSize: '0.75rem', padding: '0.125rem 0.5rem', borderRadius: 'var(--radius-sm)', fontWeight: 600, backgroundColor: recoveryInspection.consistent ? 'var(--status-success-bg)' : 'var(--status-danger-bg)', color: recoveryInspection.consistent ? 'var(--status-success)' : 'var(--status-danger)', border: `1px solid ${recoveryInspection.consistent ? 'var(--status-success)' : 'var(--status-danger)'}` }}>
              {recoveryInspection.consistent ? 'Verified Consistent' : `Inconsistency: ${recoveryInspection.consistency_error || 'Attention Needed'}`}
            </span>
          </div>
        )}

        {backupMessage && (
          <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', fontSize: '0.8125rem', color: 'var(--text-primary)', marginBottom: '1rem' }}>
            {backupMessage}
          </div>
        )}
        {pruneMessage && (
          <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', fontSize: '0.8125rem', color: 'var(--text-primary)', marginBottom: '1rem' }}>
            {pruneMessage}
          </div>
        )}

        <div style={{ display: 'flex', gap: '0.75rem', flexWrap: 'wrap', marginBottom: '1rem' }}>
          <button
            type="button"
            id="btn-create-backup"
            onClick={handleCreateBackup}
            disabled={creatingBackup}
            className="btn btn-secondary btn-sm"
            style={{ padding: '0.4375rem 0.875rem', fontSize: '0.8125rem', fontWeight: 500, backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-primary)', cursor: 'pointer' }}
          >
            {creatingBackup ? 'Creating…' : 'Create Consistent Backup'}
          </button>
          <button
            type="button"
            id="btn-prune-records"
            onClick={handlePruneRecords}
            disabled={pruningRecords}
            className="btn btn-secondary btn-sm"
            style={{ padding: '0.4375rem 0.875rem', fontSize: '0.8125rem', fontWeight: 500, backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-primary)', cursor: 'pointer' }}
          >
            {pruningRecords ? 'Pruning…' : 'Prune Finished Records'}
          </button>
          <button
            type="button"
            id="btn-lost-device-guide"
            onClick={() => setShowLostDeviceGuide(!showLostDeviceGuide)}
            className="btn btn-secondary btn-sm"
            style={{ padding: '0.4375rem 0.875rem', fontSize: '0.8125rem', fontWeight: 500, backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-secondary)', cursor: 'pointer' }}
          >
            {showLostDeviceGuide ? 'Hide Replacement Guide' : 'Lost Device & Replacement Guide'}
          </button>
        </div>

        {/* Lost Device Replacement Guidance */}
        {showLostDeviceGuide && (
          <div id="lost-device-guide-panel" style={{ padding: '1rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', fontSize: '0.8125rem', lineHeight: 1.5, color: 'var(--text-secondary)' }}>
            <h4 style={{ margin: '0 0 0.5rem 0', color: 'var(--text-primary)', fontSize: '0.875rem' }}>
              Lost or Replaced Device Procedure:
            </h4>
            <ol style={{ margin: 0, paddingLeft: '1.25rem' }}>
              <li>
                <strong>Decommission lost device:</strong> On any surviving computer, navigate to <em>Devices</em> and click <em>Retire</em> (or run <code className="code-font" style={{ color: 'var(--text-primary)' }}>orbit devices retire &lt;id&gt;</code>). This permanently locks out the old key and increments the membership revision.
              </li>
              <li>
                <strong>Set up replacement device:</strong> Install Orbit on your replacement machine and choose <em>Join existing workspace</em> (or run <code className="code-font" style={{ color: 'var(--text-primary)' }}>orbit setup --join</code>).
              </li>
              <li>
                <strong>Approve new enrollment:</strong> Generate an invitation on your active machine, submit the join request from the new device, and explicitly approve it.
              </li>
              <li>
                <strong>Restoring from metadata backup:</strong> If restoring an existing SQLite backup, stop the agent first (<code className="code-font" style={{ color: 'var(--text-primary)' }}>systemctl --user stop orbit</code>) and restore via <code className="code-font" style={{ color: 'var(--text-primary)' }}>orbit maintenance restore-backup --file &lt;backup.sqlite&gt;</code>. Identity and counters are safely reset to fresh keys (Invariant I08). Missing CAS chunks are fetched from peers.
              </li>
            </ol>
          </div>
        )}
      </section>

      {/* 6. DIAGNOSTICS & SUPPORT */}
      <section style={{ backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)', padding: '1.5rem' }}>
        <h3 style={{ fontSize: '1.125rem', fontWeight: 600, color: 'var(--text-primary)', margin: '0 0 0.5rem 0' }}>
          Diagnostics & Support
        </h3>
        <p style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', margin: '0 0 1.25rem 0' }}>
          Inspect self-checks and generate private diagnostics bundles.
        </p>

        {supportMessage && (
          <div style={{ padding: '0.75rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', fontSize: '0.8125rem', color: 'var(--text-primary)', marginBottom: '1rem' }}>
            {supportMessage}
          </div>
        )}

        <div style={{ display: 'flex', gap: '0.75rem', flexWrap: 'wrap' }}>
          <button
            type="button"
            onClick={() => setShowDoctorModal(true)}
            className="btn btn-secondary btn-sm"
            style={{ padding: '0.4375rem 0.875rem', fontSize: '0.8125rem', fontWeight: 500, backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-primary)', cursor: 'pointer' }}
          >
            Run Doctor Report
          </button>
          <button
            type="button"
            onClick={handleExportSupport}
            disabled={exportingSupport}
            className="btn btn-secondary btn-sm"
            style={{ padding: '0.4375rem 0.875rem', fontSize: '0.8125rem', fontWeight: 500, backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-primary)', cursor: 'pointer' }}
          >
            {exportingSupport ? 'Generating…' : 'Export Support Bundle'}
          </button>
        </div>
      </section>

      {/* 7. WEB SESSION & SECURITY */}
      <section style={{ backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)', padding: '1.5rem' }}>
        <h3 style={{ fontSize: '1.125rem', fontWeight: 600, color: 'var(--text-primary)', margin: '0 0 0.5rem 0' }}>
          UI Session
        </h3>
        <p style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', margin: '0 0 1.25rem 0' }}>
          Signing out invalidates your browser session cookie. The background sync daemon continues running uninterrupted.
        </p>

        <button
          type="button"
          onClick={onLogout}
          className="btn btn-secondary"
          style={{ padding: '0.5rem 1rem', fontSize: '0.875rem', fontWeight: 500, backgroundColor: 'transparent', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--status-danger)', cursor: 'pointer' }}
        >
          Sign Out of Web UI
        </button>
      </section>

      {/* Modals */}
      {showDoctorModal && (
        <DoctorModal
          report={doctorReport}
          onClose={() => setShowDoctorModal(false)}
        />
      )}

      {retentionFolder && (
        <RetentionModal
          folderId={retentionFolder}
          onClose={() => setRetentionFolder(null)}
          onGCComplete={onRefresh}
        />
      )}

      {unregisterTarget && (
        <UnregisterModal
          folderId={unregisterTarget.folderId}
          rootPath={unregisterTarget.rootPath}
          onClose={() => setUnregisterTarget(null)}
          onSuccess={() => {
            setUnregisterTarget(null);
            onRefresh();
          }}
        />
      )}
    </div>
  );
};
