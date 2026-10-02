import React, { useState, useEffect } from 'react';
import { api, APIError } from '../api';
import {
  InspectSetupResult,
  PreviewCreateRootResult,
  ServiceStatusResult,
  StartSetupResult,
  ResumeSetupResult,
  PeerTestResult,
} from '../types';
import { DirectoryPickerModal } from '../components/DirectoryPickerModal';

interface SetupWizardProps {
  inspectData: InspectSetupResult;
  onComplete: () => void;
}

type WizardStep = 'choice' | 'create' | 'join' | 'waiting_approval' | 'progress';

export const SetupWizard: React.FC<SetupWizardProps> = ({ inspectData, onComplete }) => {
  const hasIncompleteSetup = Boolean(
    inspectData.current_phase &&
    inspectData.current_phase !== 'initial' &&
    !inspectData.setup_completed
  );
  const [step, setStep] = useState<WizardStep>(hasIncompleteSetup ? 'progress' : 'choice');

  // Form State
  const [deviceLabel, setDeviceLabel] = useState(inspectData.settings?.device_label || 'linux-workstation');
  const [workspaceName, setWorkspaceName] = useState('Orbit');
  const [rootPath, setRootPath] = useState(inspectData.suggested_root || '~/Orbit');
  const [enableService, setEnableService] = useState(true);

  // Directory Picker Modal
  const [showDirPicker, setShowDirPicker] = useState(false);

  // Validation Preview State
  const [rootPreview, setRootPreview] = useState<PreviewCreateRootResult | null>(null);
  const [validatingRoot, setValidatingRoot] = useState(false);
  const [rootError, setRootError] = useState<string | null>(null);

  // Service Status
  const [serviceStatus, setServiceStatus] = useState<ServiceStatusResult | null>(null);

  // Progress State
  const [operationResult, setOperationResult] = useState<StartSetupResult | ResumeSetupResult | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [progressError, setProgressError] = useState<string | null>(null);
  const [progressStep, setProgressStep] = useState<number>(0);
  const [captureFinished, setCaptureFinished] = useState(false);

  // Join Flow State
  const [joinInvitationInput, setJoinInvitationInput] = useState('');
  const [joinToken, setJoinToken] = useState('');
  const [joinFolder, setJoinFolder] = useState('');
  const [joinEndpoint, setJoinEndpoint] = useState('');
  const [testingJoinEndpoint, setTestingJoinEndpoint] = useState(false);
  const [joinEndpointTestResult, setJoinEndpointTestResult] = useState<PeerTestResult | null>(null);
  const [submittingJoin, setSubmittingJoin] = useState(false);
  const [joinError, setJoinError] = useState<string | null>(null);

  // Waiting Approval & Completion State
  const [joinRequestId, setJoinRequestId] = useState<string | null>(null);
  const [joinKeyPin, setJoinKeyPin] = useState<string | null>(null);
  const [joinDeviceId, setJoinDeviceId] = useState<string | null>(null);
  const [approvalStatus, setApprovalStatus] = useState<'pending' | 'approved' | 'declined' | null>(null);
  const [completingJoin, setCompletingJoin] = useState(false);

  // Check service status on mount
  useEffect(() => {
    api.getServiceStatus().then(setServiceStatus).catch(() => {});
  }, []);

  // Debounced root path validation
  useEffect(() => {
    if (!rootPath.trim()) {
      setRootPreview(null);
      setRootError('Root folder path cannot be empty');
      return;
    }

    const timer = setTimeout(async () => {
      setValidatingRoot(true);
      setRootError(null);
      try {
        const preview = await api.previewCreateRoot(rootPath.trim());
        setRootPreview(preview);
        if (preview.disallowed) {
          setRootError(preview.reason || 'This directory path is disallowed');
        }
      } catch (err: any) {
        setRootPreview(null);
        if (err instanceof APIError) {
          setRootError(`${err.code}: ${err.message}`);
        } else {
          setRootError(err.message || 'Failed to inspect directory path');
        }
      } finally {
        setValidatingRoot(false);
      }
    }, 300);

    return () => clearTimeout(timer);
  }, [rootPath]);

  // Handle Create Submit
  const handleStartCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!rootPath.trim() || rootError) return;

    setSubmitting(true);
    setProgressError(null);
    setStep('progress');
    setProgressStep(1); // 1: Initializing configuration

    try {
      // Step 1: Start setup via backend control operation
      const res = await api.startSetup({
        root_path: rootPath.trim(),
        device_label: deviceLabel.trim(),
        workspace_name: workspaceName.trim(),
      });
      setOperationResult(res);
      setProgressStep(2); // 2: Root verified

      // Step 2: If requested and systemd is available, enable service
      if (enableService && serviceStatus?.systemd_available) {
        try {
          await api.serviceAction('enable');
        } catch {
          // Non-fatal service enable attempt
        }
      }
      setProgressStep(3); // 3: Service active

      // Step 3: Poll setup status to verify initial capture completion
      let pollCount = 0;
      const pollInterval = setInterval(async () => {
        pollCount++;
        try {
          const st = await api.getSetupStatus();
          if (st.completed || pollCount >= 8) {
            clearInterval(pollInterval);
            setProgressStep(4); // 4: Capture complete
            setCaptureFinished(true);
            setSubmitting(false);
          }
        } catch {
          if (pollCount >= 8) {
            clearInterval(pollInterval);
            setProgressStep(4);
            setCaptureFinished(true);
            setSubmitting(false);
          }
        }
      }, 500);
    } catch (err: any) {
      setSubmitting(false);
      if (err instanceof APIError) {
        setProgressError(`${err.code}: ${err.message}. ${err.action || ''}`);
      } else {
        setProgressError(err.message || 'Setup creation encountered an error');
      }
    }
  };

  // Handle Resume Incomplete Setup
  const handleResume = async () => {
    setSubmitting(true);
    setProgressError(null);
    setStep('progress');
    setProgressStep(1);

    try {
      const res = await api.resumeSetup();
      setOperationResult(res);
      setProgressStep(3);

      let pollCount = 0;
      const pollInterval = setInterval(async () => {
        pollCount++;
        try {
          const st = await api.getSetupStatus();
          if (st.completed || pollCount >= 6) {
            clearInterval(pollInterval);
            setProgressStep(4);
            setCaptureFinished(true);
            setSubmitting(false);
          }
        } catch {
          if (pollCount >= 6) {
            clearInterval(pollInterval);
            setProgressStep(4);
            setCaptureFinished(true);
            setSubmitting(false);
          }
        }
      }, 500);
    } catch (err: any) {
      setSubmitting(false);
      if (err instanceof APIError) {
        setProgressError(`${err.code}: ${err.message}. ${err.action || ''}`);
      } else {
        setProgressError(err.message || 'Resume failed');
      }
    }
  };

  // Handle invitation input and auto-parsing
  const handleInvitationChange = (val: string) => {
    setJoinInvitationInput(val);
    const trimmed = val.trim();
    if (trimmed.startsWith('orbit-invitation:v1?') || trimmed.includes('token=')) {
      try {
        const qs = trimmed.includes('?') ? trimmed.split('?')[1] : trimmed;
        const params = new URLSearchParams(qs);
        const token = params.get('token');
        const folder = params.get('folder');
        const endpoint = params.get('endpoint');
        if (token) setJoinToken(token);
        if (folder) setJoinFolder(folder);
        if (endpoint) setJoinEndpoint(endpoint);
      } catch {
        // Fallback
      }
    } else if (trimmed.length > 20 && !trimmed.includes(' ')) {
      setJoinToken(trimmed);
    }
  };

  // Test reachability to remote endpoint
  const handleTestJoinEndpoint = async () => {
    if (!joinEndpoint.trim()) return;
    setTestingJoinEndpoint(true);
    setJoinEndpointTestResult(null);
    try {
      const res = await api.testPeerEndpoint(joinEndpoint.trim());
      setJoinEndpointTestResult(res);
    } catch (err: any) {
      setJoinEndpointTestResult({
        reachable: false,
        status: 'error',
        error: err.message || 'Connection test failed',
      });
    } finally {
      setTestingJoinEndpoint(false);
    }
  };

  // Handle submit join request
  const handleStartJoin = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!joinToken.trim() || !joinFolder.trim() || !joinEndpoint.trim() || !rootPath.trim() || !!rootError) {
      return;
    }

    setSubmittingJoin(true);
    setJoinError(null);

    try {
      const res = await api.submitJoinFlow({
        invitation_token: joinToken.trim(),
        target_folder: joinFolder.trim(),
        remote_endpoint: joinEndpoint.trim(),
        device_label: deviceLabel.trim(),
        root_path: rootPath.trim(),
      });

      setJoinRequestId(res.request_id);
      setJoinKeyPin(res.key_pin);
      setJoinDeviceId(res.device_id);
      setApprovalStatus('pending');
      setStep('waiting_approval');
    } catch (err: any) {
      setJoinError(err.message || 'Failed to submit join request');
    } finally {
      setSubmittingJoin(false);
    }
  };

  // Polling loop for waiting approval
  useEffect(() => {
    if (step !== 'waiting_approval' || !joinRequestId) return;
    let active = true;

    const poll = async () => {
      try {
        const res = await api.getEnrollmentStatus(joinRequestId, joinEndpoint.trim());
        if (!active) return;
        if (res.status === 'approved') {
          setApprovalStatus('approved');
          setCompletingJoin(true);
          try {
            await api.completeJoinFlow({
              request_id: joinRequestId,
              remote_endpoint: joinEndpoint.trim(),
              target_folder: joinFolder.trim(),
              root_path: rootPath.trim(),
              device_label: deviceLabel.trim(),
            });
            if (active) {
              onComplete();
            }
          } catch (err: any) {
            if (active) {
              setJoinError(err.message || 'Failed to finalize join configuration');
              setCompletingJoin(false);
            }
          }
        } else if (res.status === 'declined') {
          setApprovalStatus('declined');
        }
      } catch {
        // Transient network error while polling is ignored
      }
    };

    const timer = setInterval(poll, 1500);
    poll();

    return () => {
      active = false;
      clearInterval(timer);
    };
  }, [step, joinRequestId, joinEndpoint, joinFolder, rootPath, deviceLabel, onComplete]);

  return (
    <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '2rem 1.5rem', backgroundColor: 'var(--canvas)' }}>
      <div style={{ maxWidth: '640px', width: '100%', backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-lg)', boxShadow: '0 12px 32px rgba(0,0,0,0.6)', padding: '2.5rem' }}>
        
        {/* Brand Header */}
        <div style={{ textAlign: 'center', marginBottom: '2rem' }}>
          <div style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', width: '3.5rem', height: '3.5rem', borderRadius: '50%', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', marginBottom: '1rem' }}>
            <svg style={{ width: '2rem', height: '2rem', color: 'var(--text-primary)' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="12" cy="12" r="10" />
              <path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20" />
              <path d="M2 12h20" />
            </svg>
          </div>
          <h1 style={{ fontSize: '1.75rem', fontWeight: 600, color: 'var(--text-primary)', letterSpacing: '-0.02em', margin: 0 }}>
            Welcome to Orbit
          </h1>
          <p style={{ color: 'var(--text-secondary)', fontSize: '0.9375rem', marginTop: '0.5rem', lineHeight: 1.5 }}>
            Synchronize and preserve your files across your own Linux devices without cloud accounts or external lock-in.
          </p>
        </div>

        {/* STEP 1: CHOICE ENTRY */}
        {step === 'choice' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '1.5rem' }}>
            {hasIncompleteSetup && (
              <div style={{ padding: '1rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '1rem' }}>
                <div>
                  <div style={{ fontWeight: 600, fontSize: '0.875rem', color: 'var(--text-primary)' }}>
                    Incomplete Setup Detected
                  </div>
                  <div style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', marginTop: '0.25rem' }}>
                    A prior setup was interrupted in phase <code className="code-font" style={{ color: 'var(--text-primary)' }}>{inspectData.current_phase}</code>.
                  </div>
                </div>
                <button
                  type="button"
                  onClick={handleResume}
                  className="btn btn-secondary"
                  style={{ padding: '0.5rem 1rem', fontSize: '0.8125rem', fontWeight: 600, backgroundColor: 'var(--surface)', border: '1px solid var(--border)', color: 'var(--text-primary)', borderRadius: 'var(--radius-sm)', cursor: 'pointer', whiteSpace: 'nowrap' }}
                >
                  Resume Setup
                </button>
              </div>
            )}

            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(260px, 1fr))', gap: '1.25rem' }}>
              {/* Card 1: Create your Orbit */}
              <button
                type="button"
                id="btn-choice-create"
                onClick={() => setStep('create')}
                className="choice-card"
                style={{
                  display: 'flex',
                  flexDirection: 'column',
                  padding: '1.5rem',
                  backgroundColor: 'var(--surface-raised)',
                  border: '1px solid var(--border)',
                  borderRadius: 'var(--radius-md)',
                  textAlign: 'left',
                  cursor: 'pointer',
                  transition: 'border-color 0.15s ease, transform 0.15s ease',
                }}
              >
                <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', marginBottom: '0.75rem' }}>
                  <div style={{ width: '2.5rem', height: '2.5rem', borderRadius: 'var(--radius-sm)', backgroundColor: 'var(--surface)', border: '1px solid var(--border)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                    <svg style={{ width: '1.25rem', height: '1.25rem', color: 'var(--text-primary)' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                      <line x1="12" y1="5" x2="12" y2="19" />
                      <line x1="5" y1="12" x2="19" y2="12" />
                    </svg>
                  </div>
                  <h2 style={{ fontSize: '1.125rem', fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>
                    Create your Orbit
                  </h2>
                </div>
                <p style={{ fontSize: '0.875rem', color: 'var(--text-secondary)', lineHeight: 1.4, margin: 0 }}>
                  Start synchronizing files on this Linux device with a new workspace and local folder.
                </p>
                <div style={{ marginTop: '1.25rem', fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-primary)', display: 'flex', alignItems: 'center', gap: '0.375rem' }}>
                  Begin Setup →
                </div>
              </button>

              {/* Card 2: Join an existing Orbit */}
              <button
                type="button"
                id="btn-choice-join"
                onClick={() => setStep('join')}
                className="choice-card"
                style={{
                  display: 'flex',
                  flexDirection: 'column',
                  padding: '1.5rem',
                  backgroundColor: 'var(--surface-raised)',
                  border: '1px solid var(--border)',
                  borderRadius: 'var(--radius-md)',
                  textAlign: 'left',
                  cursor: 'pointer',
                  transition: 'border-color 0.15s ease, transform 0.15s ease',
                }}
              >
                <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', marginBottom: '0.75rem' }}>
                  <div style={{ width: '2.5rem', height: '2.5rem', borderRadius: 'var(--radius-sm)', backgroundColor: 'var(--surface)', border: '1px solid var(--border)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                    <svg style={{ width: '1.25rem', height: '1.25rem', color: 'var(--text-primary)' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                      <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71" />
                      <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" />
                    </svg>
                  </div>
                  <h2 style={{ fontSize: '1.125rem', fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>
                    Join an existing Orbit
                  </h2>
                </div>
                <p style={{ fontSize: '0.875rem', color: 'var(--text-secondary)', lineHeight: 1.4, margin: 0 }}>
                  Connect this device to an existing workspace using an invitation link from an enrolled device.
                </p>
                <div style={{ marginTop: '1.25rem', fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-secondary)', display: 'flex', alignItems: 'center', gap: '0.375rem' }}>
                  Pair Device →
                </div>
              </button>
            </div>
          </div>
        )}

        {/* STEP 2: CREATE WORKSPACE FORM */}
        {step === 'create' && (
          <form onSubmit={handleStartCreate} style={{ display: 'flex', flexDirection: 'column', gap: '1.25rem' }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', borderBottom: '1px solid var(--border)', paddingBottom: '0.75rem' }}>
              <h2 style={{ fontSize: '1.125rem', fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>
                Workspace Setup
              </h2>
              <button
                type="button"
                onClick={() => setStep('choice')}
                style={{ background: 'transparent', border: 'none', color: 'var(--text-secondary)', fontSize: '0.8125rem', cursor: 'pointer' }}
              >
                ← Change choice
              </button>
            </div>

            {/* Device Label */}
            <div>
              <label htmlFor="device-label" style={{ display: 'block', fontSize: '0.875rem', fontWeight: 500, color: 'var(--text-primary)', marginBottom: '0.375rem' }}>
                Device Name
              </label>
              <input
                id="device-label"
                type="text"
                className="form-input"
                value={deviceLabel}
                onChange={(e) => setDeviceLabel(e.target.value)}
                placeholder="e.g. laptop-thinkpad"
                required
                style={{ width: '100%', padding: '0.625rem 0.875rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-primary)', fontSize: '0.875rem' }}
              />
              <span style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginTop: '0.25rem', display: 'block' }}>
                Human-readable name identifying this computer to other devices.
              </span>
            </div>

            {/* Workspace Name */}
            <div>
              <label htmlFor="workspace-name" style={{ display: 'block', fontSize: '0.875rem', fontWeight: 500, color: 'var(--text-primary)', marginBottom: '0.375rem' }}>
                Workspace Name
              </label>
              <input
                id="workspace-name"
                type="text"
                className="form-input"
                value={workspaceName}
                onChange={(e) => setWorkspaceName(e.target.value)}
                placeholder="Orbit"
                required
                style={{ width: '100%', padding: '0.625rem 0.875rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-primary)', fontSize: '0.875rem' }}
              />
            </div>

            {/* Local Folder Path */}
            <div>
              <label htmlFor="root-path" style={{ display: 'block', fontSize: '0.875rem', fontWeight: 500, color: 'var(--text-primary)', marginBottom: '0.375rem' }}>
                Local Folder Path
              </label>
              <div style={{ display: 'flex', gap: '0.5rem' }}>
                <input
                  id="root-path"
                  type="text"
                  className="form-input code-font"
                  value={rootPath}
                  onChange={(e) => setRootPath(e.target.value)}
                  placeholder="~/Orbit"
                  required
                  style={{ flex: 1, padding: '0.625rem 0.875rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-primary)', fontSize: '0.875rem' }}
                />
                <button
                  type="button"
                  onClick={() => setShowDirPicker(true)}
                  className="btn btn-secondary"
                  style={{ padding: '0.625rem 1rem', fontSize: '0.8125rem', fontWeight: 500, backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', color: 'var(--text-primary)', borderRadius: 'var(--radius-sm)', cursor: 'pointer', whiteSpace: 'nowrap' }}
                >
                  Browse…
                </button>
              </div>

              {/* Dynamic Path Inspection Feedback */}
              <div style={{ marginTop: '0.5rem' }}>
                {validatingRoot && (
                  <span style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)' }}>
                    Inspecting folder path…
                  </span>
                )}

                {rootError && (
                  <div className="alert alert-danger" style={{ padding: '0.625rem 0.75rem', backgroundColor: 'var(--status-danger-bg)', border: '1px solid var(--status-danger)', borderRadius: 'var(--radius-sm)', color: 'var(--status-danger)', fontSize: '0.8125rem', marginTop: '0.25rem' }}>
                    <strong>Invalid Location:</strong> {rootError}
                  </div>
                )}

                {!validatingRoot && !rootError && rootPreview && (
                  <div style={{ padding: '0.625rem 0.75rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', fontSize: '0.8125rem' }}>
                    {!rootPreview.exists ? (
                      <span style={{ color: 'var(--text-primary)' }}>
                        Folder does not exist yet. Orbit will create it automatically.
                      </span>
                    ) : rootPreview.is_empty ? (
                      <span style={{ color: 'var(--status-success)' }}>
                        ✓ Empty directory verified and ready for synchronization.
                      </span>
                    ) : (
                      <div>
                        <div style={{ color: 'var(--status-warning)', fontWeight: 500 }}>
                          ⚠ Preexisting content found: {rootPreview.preexisting_rows} items
                        </div>
                        <div style={{ color: 'var(--text-secondary)', marginTop: '0.25rem' }}>
                          Preexisting files will be safely adopted into your Orbit version history without deletion.
                        </div>
                        {rootPreview.existing_samples && rootPreview.existing_samples.length > 0 && (
                          <div style={{ marginTop: '0.375rem', color: 'var(--text-secondary)', fontSize: '0.75rem' }}>
                            Samples: {rootPreview.existing_samples.slice(0, 3).join(', ')}
                            {rootPreview.existing_samples.length > 3 && ' …'}
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                )}
              </div>
            </div>

            {/* System Service Checkbox */}
            <div style={{ padding: '0.75rem 1rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)' }}>
              <label style={{ display: 'flex', alignItems: 'flex-start', gap: '0.625rem', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={enableService}
                  onChange={(e) => setEnableService(e.target.checked)}
                  style={{ marginTop: '0.2rem' }}
                />
                <div>
                  <div style={{ fontSize: '0.875rem', fontWeight: 500, color: 'var(--text-primary)' }}>
                    Start Orbit automatically on login
                  </div>
                  <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginTop: '0.125rem' }}>
                    {serviceStatus?.systemd_available
                      ? 'Integrates with systemd user service for background synchronization.'
                      : 'systemd user bus unavailable; manual startup command will be shown in Settings.'}
                  </div>
                </div>
              </label>
            </div>

            {/* Submit Actions */}
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: '0.75rem', marginTop: '0.5rem' }}>
              <button
                type="button"
                onClick={() => setStep('choice')}
                className="btn btn-secondary"
                style={{ padding: '0.625rem 1.25rem', backgroundColor: 'transparent', border: '1px solid var(--border)', color: 'var(--text-primary)', borderRadius: 'var(--radius-sm)', fontSize: '0.875rem', fontWeight: 500, cursor: 'pointer' }}
              >
                Back
              </button>
              <button
                type="submit"
                className="btn btn-primary"
                disabled={submitting || !!rootError || validatingRoot || !rootPath.trim()}
                style={{ padding: '0.625rem 1.25rem', backgroundColor: 'var(--text-primary)', border: 'none', color: 'var(--canvas)', borderRadius: 'var(--radius-sm)', fontSize: '0.875rem', fontWeight: 600, cursor: !!rootError ? 'not-allowed' : 'pointer' }}
              >
                {submitting ? 'Setting up…' : 'Create Orbit Workspace'}
              </button>
            </div>
          </form>
        )}

        {/* STEP 3: JOIN SCREEN (Functional pairing & network prerequisites) */}
        {step === 'join' && (
          <form onSubmit={handleStartJoin} style={{ display: 'flex', flexDirection: 'column', gap: '1.25rem' }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', borderBottom: '1px solid var(--border)', paddingBottom: '0.75rem' }}>
              <h2 style={{ fontSize: '1.125rem', fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>
                Join an Existing Orbit
              </h2>
              <button
                type="button"
                onClick={() => setStep('choice')}
                style={{ background: 'transparent', border: 'none', color: 'var(--text-secondary)', fontSize: '0.8125rem', cursor: 'pointer' }}
              >
                ← Return to choices
              </button>
            </div>

            {/* Network Prerequisites Explanation */}
            <div style={{ padding: '0.875rem 1rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)', display: 'flex', flexDirection: 'column', gap: '0.375rem' }}>
              <div style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-primary)', display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                <svg style={{ width: '1rem', height: '1rem', color: 'var(--text-primary)' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <circle cx="12" cy="12" r="10" />
                  <line x1="12" y1="16" x2="12" y2="12" />
                  <line x1="12" y1="8" x2="12.01" y2="8" />
                </svg>
                Network Prerequisites Before Joining
              </div>
              <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', lineHeight: 1.4 }}>
                • <strong>Direct Reachability:</strong> Both devices must communicate directly over the same Wi-Fi / local network, or a private VPN (such as Tailscale or WireGuard).
              </div>
              <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', lineHeight: 1.4 }}>
                • <strong>Inviting Node Online:</strong> The inviting device must have its Orbit daemon running to receive your cryptographic join proof.
              </div>
            </div>

            {joinError && (
              <div className="alert alert-danger" style={{ padding: '0.75rem 1rem', backgroundColor: 'var(--status-danger-bg)', border: '1px solid var(--status-danger)', borderRadius: 'var(--radius-sm)', color: 'var(--status-danger)', fontSize: '0.8125rem' }}>
                <strong>Join Request Failed:</strong> {joinError}
              </div>
            )}

            {/* Formatted Invitation Link / One-Paste Input */}
            <div>
              <label htmlFor="input-invitation-string" style={{ display: 'block', fontSize: '0.875rem', fontWeight: 500, color: 'var(--text-primary)', marginBottom: '0.375rem' }}>
                Invitation Link or Token
              </label>
              <input
                id="input-invitation-string"
                type="text"
                className="form-input code-font"
                value={joinInvitationInput}
                onChange={(e) => handleInvitationChange(e.target.value)}
                placeholder="orbit-invitation:v1?token=...&folder=...&endpoint=..."
                style={{ width: '100%', padding: '0.625rem 0.875rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-primary)', fontSize: '0.8125rem' }}
              />
              <span style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginTop: '0.25rem', display: 'block' }}>
                Paste the invitation link generated on your other device, or fill in details below.
              </span>
            </div>

            {/* Target Folder ID */}
            <div>
              <label htmlFor="input-join-folder" style={{ display: 'block', fontSize: '0.8125rem', fontWeight: 500, color: 'var(--text-primary)', marginBottom: '0.375rem' }}>
                Workspace Folder ID
              </label>
              <input
                id="input-join-folder"
                type="text"
                required
                className="form-input code-font"
                value={joinFolder}
                onChange={(e) => setJoinFolder(e.target.value)}
                placeholder="64-character hex folder ID"
                style={{ width: '100%', padding: '0.5rem 0.75rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-primary)', fontSize: '0.8125rem' }}
              />
            </div>

            {/* Invitation Token */}
            <div>
              <label htmlFor="input-join-token" style={{ display: 'block', fontSize: '0.8125rem', fontWeight: 500, color: 'var(--text-primary)', marginBottom: '0.375rem' }}>
                Invitation Secret Token
              </label>
              <input
                id="input-join-token"
                type="text"
                required
                className="form-input code-font"
                value={joinToken}
                onChange={(e) => setJoinToken(e.target.value)}
                placeholder="64-character secret token"
                style={{ width: '100%', padding: '0.5rem 0.75rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-primary)', fontSize: '0.8125rem' }}
              />
            </div>

            {/* Remote Endpoint & Reachability Test */}
            <div>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '0.375rem' }}>
                <label htmlFor="input-join-endpoint" style={{ fontSize: '0.8125rem', fontWeight: 500, color: 'var(--text-primary)' }}>
                  Inviting Device Endpoint URL
                </label>
                {joinEndpointTestResult && (
                  <span
                    style={{
                      fontSize: '0.6875rem',
                      padding: '0.125rem 0.375rem',
                      borderRadius: 'var(--radius-sm)',
                      fontWeight: 600,
                      backgroundColor: joinEndpointTestResult.reachable ? 'rgba(34, 197, 94, 0.15)' : 'rgba(239, 68, 68, 0.15)',
                      color: joinEndpointTestResult.reachable ? 'var(--status-success)' : 'var(--status-error)',
                      border: `1px solid ${joinEndpointTestResult.reachable ? 'var(--status-success)' : 'var(--status-error)'}`,
                    }}
                  >
                    {joinEndpointTestResult.reachable ? `Reachable (${joinEndpointTestResult.latency_ms ?? 0}ms)` : 'Unreachable'}
                  </span>
                )}
              </div>
              <div style={{ display: 'flex', gap: '0.5rem' }}>
                <input
                  id="input-join-endpoint"
                  type="text"
                  required
                  className="form-input code-font"
                  value={joinEndpoint}
                  onChange={(e) => setJoinEndpoint(e.target.value)}
                  placeholder="https://192.168.1.50:8443"
                  style={{ flex: 1, padding: '0.5rem 0.75rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-primary)', fontSize: '0.8125rem' }}
                />
                <button
                  type="button"
                  id="btn-test-join-endpoint"
                  disabled={testingJoinEndpoint || !joinEndpoint.trim()}
                  onClick={handleTestJoinEndpoint}
                  className="btn btn-secondary"
                  style={{ padding: '0.5rem 0.875rem', fontSize: '0.8125rem', whiteSpace: 'nowrap' }}
                >
                  {testingJoinEndpoint ? 'Testing…' : 'Test Connection'}
                </button>
              </div>
              {joinEndpointTestResult && !joinEndpointTestResult.reachable && (
                <div style={{ fontSize: '0.75rem', color: 'var(--status-error)', marginTop: '0.375rem' }}>
                  <strong>Diagnostic:</strong> {joinEndpointTestResult.error || 'Connection failed'}. Ensure the remote device is powered on, on the same network or VPN, and the port is reachable.
                </div>
              )}
            </div>

            {/* This Device Name */}
            <div>
              <label htmlFor="input-join-device-label" style={{ display: 'block', fontSize: '0.8125rem', fontWeight: 500, color: 'var(--text-primary)', marginBottom: '0.375rem' }}>
                This Device Name
              </label>
              <input
                id="input-join-device-label"
                type="text"
                required
                className="form-input"
                value={deviceLabel}
                onChange={(e) => setDeviceLabel(e.target.value)}
                placeholder="e.g. Work Laptop, Studio PC"
                style={{ width: '100%', padding: '0.5rem 0.75rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-primary)', fontSize: '0.8125rem' }}
              />
            </div>

            {/* Local Sync Folder */}
            <div>
              <label htmlFor="input-join-root-path" style={{ display: 'block', fontSize: '0.8125rem', fontWeight: 500, color: 'var(--text-primary)', marginBottom: '0.375rem' }}>
                Local Folder Path
              </label>
              <div style={{ display: 'flex', gap: '0.5rem' }}>
                <input
                  id="input-join-root-path"
                  type="text"
                  required
                  className="form-input code-font"
                  value={rootPath}
                  onChange={(e) => setRootPath(e.target.value)}
                  placeholder="~/Orbit"
                  style={{ flex: 1, padding: '0.5rem 0.75rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-primary)', fontSize: '0.8125rem' }}
                />
                <button
                  type="button"
                  onClick={() => setShowDirPicker(true)}
                  className="btn btn-secondary"
                  style={{ padding: '0.5rem 0.875rem', fontSize: '0.8125rem', whiteSpace: 'nowrap' }}
                >
                  Browse…
                </button>
              </div>

              {/* Path Validation Feedback */}
              <div style={{ marginTop: '0.375rem' }}>
                {rootError && (
                  <div className="alert alert-danger" style={{ padding: '0.5rem 0.75rem', backgroundColor: 'var(--status-danger-bg)', border: '1px solid var(--status-danger)', borderRadius: 'var(--radius-sm)', color: 'var(--status-danger)', fontSize: '0.75rem' }}>
                    {rootError}
                  </div>
                )}
                {!validatingRoot && !rootError && rootPreview && (
                  <div style={{ padding: '0.5rem 0.75rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', fontSize: '0.75rem' }}>
                    {!rootPreview.exists ? (
                      <span style={{ color: 'var(--text-primary)' }}>
                        Folder does not exist yet. Orbit will create it automatically.
                      </span>
                    ) : rootPreview.is_empty ? (
                      <span style={{ color: 'var(--status-success)' }}>
                        ✓ Empty directory verified.
                      </span>
                    ) : (
                      <span style={{ color: 'var(--status-warning)' }}>
                        ⚠ Preexisting content found ({rootPreview.preexisting_rows} items). Preexisting files will be safely adopted without deletion (Invariant I22).
                      </span>
                    )}
                  </div>
                )}
              </div>
            </div>

            {/* Actions */}
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '0.75rem', marginTop: '0.5rem' }}>
              <button
                type="button"
                onClick={() => setStep('choice')}
                className="btn btn-secondary"
                style={{ padding: '0.5rem 1rem', fontSize: '0.8125rem' }}
              >
                Back
              </button>
              <button
                type="submit"
                id="btn-submit-join"
                disabled={submittingJoin || !joinToken.trim() || !joinFolder.trim() || !joinEndpoint.trim() || !rootPath.trim() || !!rootError}
                className="btn btn-primary"
                style={{
                  padding: '0.5rem 1.25rem',
                  backgroundColor: 'var(--text-primary)',
                  color: 'var(--canvas)',
                  border: 'none',
                  borderRadius: 'var(--radius-sm)',
                  fontSize: '0.8125rem',
                  fontWeight: 600,
                  cursor: submittingJoin || !joinToken.trim() ? 'not-allowed' : 'pointer',
                }}
              >
                {submittingJoin ? 'Submitting…' : 'Submit Join Request'}
              </button>
            </div>
          </form>
        )}

        {/* STEP 3B: WAITING FOR APPROVAL SCREEN */}
        {step === 'waiting_approval' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '1.5rem', textAlign: 'center' }}>
            <div>
              <h2 style={{ fontSize: '1.25rem', fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>
                {approvalStatus === 'declined'
                  ? 'Join Request Declined'
                  : approvalStatus === 'approved'
                  ? 'Request Approved!'
                  : 'Waiting for Owner Approval'}
              </h2>
              <p style={{ fontSize: '0.875rem', color: 'var(--text-secondary)', marginTop: '0.375rem' }}>
                {approvalStatus === 'declined'
                  ? 'The workspace owner declined this join request.'
                  : approvalStatus === 'approved'
                  ? 'Finalizing membership revision and configuring local replica…'
                  : 'Your cryptographic identity proof was submitted. Please authorize this device on the owner node.'}
              </p>
            </div>

            {approvalStatus === 'declined' ? (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '1rem', alignItems: 'center' }}>
                <div style={{ padding: '1rem', backgroundColor: 'rgba(239, 68, 68, 0.1)', border: '1px solid var(--status-error)', borderRadius: 'var(--radius-md)', color: 'var(--text-primary)', fontSize: '0.875rem', maxWidth: '480px' }}>
                  The join request was declined on the owner node. You can return to the form to re-check your invitation credentials or retry.
                </div>
                <button
                  type="button"
                  onClick={() => setStep('join')}
                  className="btn btn-primary"
                  style={{ padding: '0.5rem 1.25rem', fontSize: '0.8125rem' }}
                >
                  Return to Join Form
                </button>
              </div>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '1.25rem', alignItems: 'center' }}>
                {/* Spinner */}
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '1rem' }}>
                  <span className="spinner" style={{ width: '2.5rem', height: '2.5rem', border: '3px solid var(--border)', borderTopColor: 'var(--text-primary)', borderRadius: '50%', animation: 'spin 0.8s linear infinite' }} />
                </div>

                {/* Proof Information Box */}
                <div style={{ width: '100%', padding: '1.25rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)', textAlign: 'left', display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
                  <div>
                    <div style={{ fontSize: '0.6875rem', color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                      Request ID
                    </div>
                    <code className="code-font" style={{ fontSize: '0.8125rem', color: 'var(--text-primary)' }}>
                      {joinRequestId}
                    </code>
                  </div>

                  <div>
                    <div style={{ fontSize: '0.6875rem', color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                      This Device's Ed25519 Key Pin
                    </div>
                    <code className="code-font" style={{ fontSize: '0.8125rem', color: 'var(--text-primary)' }}>
                      {joinKeyPin}
                    </code>
                  </div>

                  {joinDeviceId && (
                    <div>
                      <div style={{ fontSize: '0.6875rem', color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                        Device ID
                      </div>
                      <code className="code-font" style={{ fontSize: '0.75rem', color: 'var(--text-primary)' }}>
                        {joinDeviceId}
                      </code>
                    </div>
                  )}

                  {completingJoin && (
                    <div style={{ padding: '0.5rem 0.75rem', backgroundColor: 'rgba(34, 197, 94, 0.1)', border: '1px solid var(--status-success)', borderRadius: 'var(--radius-sm)', fontSize: '0.8125rem', color: 'var(--status-success)', fontWeight: 500 }}>
                      ✓ Approved! Adopting membership revision and starting sync…
                    </div>
                  )}

                  <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', lineHeight: 1.4, borderTop: '1px solid var(--border)', paddingTop: '0.75rem' }}>
                    <strong>Action Required on Inviting Device:</strong> Open <strong>Devices & Replicas</strong> on the inviting computer. Look for the pending request matching this key pin, and click <strong>Approve</strong>.
                  </div>
                </div>

                <button
                  type="button"
                  id="btn-cancel-waiting"
                  onClick={() => setStep('join')}
                  className="btn btn-secondary"
                  style={{ padding: '0.5rem 1.25rem', fontSize: '0.8125rem' }}
                >
                  Cancel & Edit Details
                </button>
              </div>
            )}
          </div>
        )}

        {/* STEP 4: SETUP PROGRESS & ADOPTION */}
        {step === 'progress' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '1.5rem' }}>
            <div style={{ textAlign: 'center' }}>
              <h2 style={{ fontSize: '1.25rem', fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>
                {captureFinished ? 'Workspace Ready' : 'Setting up Workspace…'}
              </h2>
              <p style={{ fontSize: '0.875rem', color: 'var(--text-secondary)', marginTop: '0.375rem' }}>
                {captureFinished
                  ? 'Your local folder is verified and initial file history is captured.'
                  : 'Performing atomic initialization, verification, and file capture.'}
              </p>
            </div>

            {progressError && (
              <div className="alert alert-danger" style={{ padding: '0.875rem', backgroundColor: 'var(--status-danger-bg)', border: '1px solid var(--status-danger)', borderRadius: 'var(--radius-sm)', color: 'var(--status-danger)', fontSize: '0.875rem' }}>
                <div style={{ fontWeight: 600, marginBottom: '0.25rem' }}>Setup Interrupted</div>
                <div>{progressError}</div>
                <button
                  type="button"
                  onClick={handleResume}
                  className="btn btn-secondary"
                  style={{ marginTop: '0.75rem', padding: '0.5rem 1rem', fontSize: '0.8125rem', fontWeight: 600, backgroundColor: 'var(--surface)', border: '1px solid var(--border)', color: 'var(--text-primary)', borderRadius: 'var(--radius-sm)', cursor: 'pointer' }}
                >
                  Retry / Resume Setup
                </button>
              </div>
            )}

            {/* Step Indicators */}
            <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-md)', padding: '1.25rem' }}>
              
              {/* Step 1: Configuration */}
              <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
                {progressStep >= 1 ? (
                  <svg style={{ width: '1.25rem', height: '1.25rem', color: 'var(--status-success)', flexShrink: 0 }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                    <polyline points="20 6 9 17 4 12" />
                  </svg>
                ) : (
                  <span className="spinner" style={{ width: '1.25rem', height: '1.25rem', border: '2px solid var(--border)', borderTopColor: 'var(--text-primary)', borderRadius: '50%', animation: 'spin 0.8s linear infinite', flexShrink: 0 }} />
                )}
                <div style={{ fontSize: '0.875rem', color: progressStep >= 1 ? 'var(--text-primary)' : 'var(--text-secondary)' }}>
                  Workspace identity and product configuration initialized
                </div>
              </div>

              {/* Step 2: Root Verification */}
              <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
                {progressStep >= 2 ? (
                  <svg style={{ width: '1.25rem', height: '1.25rem', color: 'var(--status-success)', flexShrink: 0 }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                    <polyline points="20 6 9 17 4 12" />
                  </svg>
                ) : progressStep === 1 ? (
                  <span className="spinner" style={{ width: '1.25rem', height: '1.25rem', border: '2px solid var(--border)', borderTopColor: 'var(--text-primary)', borderRadius: '50%', animation: 'spin 0.8s linear infinite', flexShrink: 0 }} />
                ) : (
                  <div style={{ width: '1.25rem', height: '1.25rem', borderRadius: '50%', border: '1px solid var(--border)', flexShrink: 0 }} />
                )}
                <div style={{ fontSize: '0.875rem', color: progressStep >= 2 ? 'var(--text-primary)' : 'var(--text-secondary)' }}>
                  Local root folder verified and permissions secured (0700)
                </div>
              </div>

              {/* Step 3: Background Service */}
              <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
                {progressStep >= 3 ? (
                  <svg style={{ width: '1.25rem', height: '1.25rem', color: 'var(--status-success)', flexShrink: 0 }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                    <polyline points="20 6 9 17 4 12" />
                  </svg>
                ) : progressStep === 2 ? (
                  <span className="spinner" style={{ width: '1.25rem', height: '1.25rem', border: '2px solid var(--border)', borderTopColor: 'var(--text-primary)', borderRadius: '50%', animation: 'spin 0.8s linear infinite', flexShrink: 0 }} />
                ) : (
                  <div style={{ width: '1.25rem', height: '1.25rem', borderRadius: '50%', border: '1px solid var(--border)', flexShrink: 0 }} />
                )}
                <div style={{ fontSize: '0.875rem', color: progressStep >= 3 ? 'var(--text-primary)' : 'var(--text-secondary)' }}>
                  Background synchronization engine running
                </div>
              </div>

              {/* Step 4: Initial Capture */}
              <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
                {progressStep >= 4 ? (
                  <svg style={{ width: '1.25rem', height: '1.25rem', color: 'var(--status-success)', flexShrink: 0 }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                    <polyline points="20 6 9 17 4 12" />
                  </svg>
                ) : progressStep === 3 ? (
                  <span className="spinner" style={{ width: '1.25rem', height: '1.25rem', border: '2px solid var(--border)', borderTopColor: 'var(--text-primary)', borderRadius: '50%', animation: 'spin 0.8s linear infinite', flexShrink: 0 }} />
                ) : (
                  <div style={{ width: '1.25rem', height: '1.25rem', borderRadius: '50%', border: '1px solid var(--border)', flexShrink: 0 }} />
                )}
                <div style={{ fontSize: '0.875rem', color: progressStep >= 4 ? 'var(--text-primary)' : 'var(--text-secondary)' }}>
                  Initial directory scan and version capture complete
                </div>
              </div>

              {operationResult && (
                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginTop: '0.5rem', textAlign: 'center' }}>
                  Operation: <code className="code-font">{operationResult.operation_id}</code>
                </div>
              )}
            </div>


            {/* Completion Action */}
            {captureFinished && (
              <div style={{ display: 'flex', justifyContent: 'center', marginTop: '0.5rem' }}>
                <button
                  type="button"
                  id="btn-open-files"
                  onClick={onComplete}
                  className="btn btn-primary"
                  style={{ padding: '0.75rem 2rem', backgroundColor: 'var(--text-primary)', border: 'none', color: 'var(--canvas)', borderRadius: 'var(--radius-sm)', fontSize: '0.9375rem', fontWeight: 600, cursor: 'pointer' }}
                >
                  Open Files
                </button>
              </div>
            )}
          </div>
        )}

        {/* Directory Picker Modal */}
        <DirectoryPickerModal
          isOpen={showDirPicker}
          initialPath={rootPath}
          onSelect={(selected) => setRootPath(selected)}
          onClose={() => setShowDirPicker(false)}
        />
      </div>
    </div>
  );
};
