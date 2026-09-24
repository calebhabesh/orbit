import React from 'react';
import { DoctorReport } from '../types';

interface DoctorModalProps {
  report: DoctorReport | null;
  onClose: () => void;
}

export const DoctorModal: React.FC<DoctorModalProps> = ({ report, onClose }) => {
  return (
    <div className="modal-backdrop" onClick={onClose} role="dialog" aria-modal="true" aria-labelledby="doctor-title">
      <div className="modal-content" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h2 id="doctor-title" className="panel-title">
            Doctor Health Inspection
          </h2>
          <button type="button" className="btn btn-secondary btn-sm" onClick={onClose} aria-label="Close dialog">
            ✕
          </button>
        </div>
        <div className="modal-body">
          {!report ? (
            <div className="empty-state">
              <div className="spinner" />
              <p>Inspecting system health…</p>
            </div>
          ) : (
            <>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <span>Overall Cluster &amp; Storage Health:</span>
                {(() => {
                  const overall = (report.overall_status || report.status || 'OK').toUpperCase();
                  const badgeClass =
                    overall === 'OK' || overall === 'HEALTHY'
                      ? 'badge-success'
                      : overall === 'WARN' || overall === 'WARNING'
                      ? 'badge-warning'
                      : 'badge-danger';
                  return <span className={`badge ${badgeClass}`}>{overall}</span>;
                })()}
              </div>

          <div className="table-wrapper">
            <table className="data-table">
              <thead>
                <tr>
                  <th>Category</th>
                  <th>Check</th>
                  <th>Status</th>
                  <th>Evaluation &amp; Action</th>
                </tr>
              </thead>
              <tbody>
                {report.checks.map((check, idx) => {
                  const cStatus = (check.status || 'UNKNOWN').toUpperCase();
                  const cBadgeClass =
                    cStatus === 'OK' || cStatus === 'HEALTHY'
                      ? 'badge-success'
                      : cStatus === 'WARN' || cStatus === 'WARNING'
                      ? 'badge-warning'
                      : 'badge-danger';
                  const remediation = check.remediation || check.action;
                  return (
                    <tr key={idx}>
                      <td>
                        <span className="badge badge-neutral">{check.category}</span>
                      </td>
                      <td style={{ fontWeight: 500 }}>{check.name}</td>
                      <td>
                        <span className={`badge ${cBadgeClass}`}>{check.status}</span>
                      </td>
                      <td>
                        <div>{check.message}</div>
                        {remediation && (
                          <div style={{ fontSize: '0.75rem', color: 'var(--accent-primary)', marginTop: '0.25rem' }}>
                            Action: {remediation}
                          </div>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
            </>
          )}
        </div>
        <div className="modal-footer">
          <button type="button" className="btn btn-secondary" onClick={onClose}>
            Close
          </button>
        </div>
      </div>
    </div>
  );
};
