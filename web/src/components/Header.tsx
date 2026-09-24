import React from 'react';
import { DoctorReport } from '../types';

interface HeaderProps {
  currentTab: 'folders' | 'files' | 'conflicts';
  onTabChange: (tab: 'folders' | 'files' | 'conflicts') => void;
  conflictCount: number;
  doctorReport: DoctorReport | null;
  onOpenDoctor: () => void;
  onLogout: () => void;
}

export const Header: React.FC<HeaderProps> = ({
  currentTab,
  onTabChange,
  conflictCount,
  doctorReport,
  onOpenDoctor,
  onLogout,
}) => {
  const rawStatus = doctorReport?.overall_status || doctorReport?.status || 'OK';
  const doctorStatus = rawStatus.toUpperCase();
  const doctorBadgeClass =
    doctorStatus === 'OK' || doctorStatus === 'HEALTHY'
      ? 'badge-success'
      : doctorStatus === 'WARN' || doctorStatus === 'WARNING'
      ? 'badge-warning'
      : 'badge-danger';

  return (
    <header className="app-header">
      <div className="brand-section">
        <svg
          className="brand-icon"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z" />
          <polyline points="3.27 6.96 12 12.01 20.73 6.96" />
          <line x1="12" y1="22.08" x2="12" y2="12" />
        </svg>
        <div>
          <h1 className="brand-title">
            File Sync
            <span className="brand-badge">Operator Console</span>
          </h1>
        </div>
      </div>

      <nav className="app-nav" aria-label="Main Navigation">
        <button
          type="button"
          className={`nav-tab ${currentTab === 'folders' ? 'active' : ''}`}
          onClick={() => onTabChange('folders')}
          aria-current={currentTab === 'folders' ? 'page' : undefined}
        >
          Folders &amp; Work
        </button>
        <button
          type="button"
          className={`nav-tab ${currentTab === 'files' ? 'active' : ''}`}
          onClick={() => onTabChange('files')}
          aria-current={currentTab === 'files' ? 'page' : undefined}
        >
          Files &amp; History
        </button>
        <button
          type="button"
          className={`nav-tab ${currentTab === 'conflicts' ? 'active' : ''}`}
          onClick={() => onTabChange('conflicts')}
          aria-current={currentTab === 'conflicts' ? 'page' : undefined}
        >
          Conflicts
          {conflictCount > 0 && <span className="tab-badge">{conflictCount}</span>}
        </button>
      </nav>

      <div className="header-actions">
        <button
          type="button"
          className={`btn btn-secondary btn-sm badge ${doctorBadgeClass}`}
          onClick={onOpenDoctor}
          title="Open System Doctor Health Report"
          aria-label={`System Health: ${doctorStatus}. Click for details.`}
        >
          <span>Doctor: {doctorStatus}</span>
        </button>
        <button
          type="button"
          className="btn btn-secondary btn-sm"
          onClick={onLogout}
          title="End local browser session"
        >
          Logout
        </button>
      </div>
    </header>
  );
};
