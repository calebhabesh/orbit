import React from 'react';
import { ProductSettings } from '../types';

export type OrbitNavTab = 'files' | 'attention' | 'devices' | 'deleted' | 'settings';

interface OrbitSidebarProps {
  currentTab: OrbitNavTab;
  onTabChange: (tab: OrbitNavTab) => void;
  attentionCount: number;
  settings: ProductSettings | null;
  workspaceName?: string;
  isSyncing?: boolean;
  onLogout: () => void;
  isOpenMobile?: boolean;
  onCloseMobile?: () => void;
}

export const OrbitSidebar: React.FC<OrbitSidebarProps> = ({
  currentTab,
  onTabChange,
  attentionCount,
  settings,
  workspaceName = 'Orbit',
  isSyncing = false,
  onLogout,
  isOpenMobile = false,
  onCloseMobile,
}) => {
  const handleNavClick = (tab: OrbitNavTab) => {
    onTabChange(tab);
    if (onCloseMobile) onCloseMobile();
  };

  return (
    <aside
      className={`orbit-sidebar ${isOpenMobile ? 'mobile-open' : ''}`}
      style={{
        width: '240px',
        backgroundColor: 'var(--surface)',
        borderRight: '1px solid var(--border)',
        display: 'flex',
        flexDirection: 'column',
        justifyContent: 'space-between',
        flexShrink: 0,
        height: '100vh',
        position: 'sticky',
        top: 0,
      }}
    >
      {/* Top Branding & Workspace */}
      <div>
        <div
          style={{
            padding: '1.25rem 1rem',
            borderBottom: '1px solid var(--border)',
            display: 'flex',
            alignItems: 'center',
            gap: '0.75rem',
          }}
        >
          <div
            style={{
              width: '2rem',
              height: '2rem',
              borderRadius: '50%',
              backgroundColor: 'var(--surface-raised)',
              border: '1px solid var(--border)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              flexShrink: 0,
            }}
          >
            <svg style={{ width: '1.125rem', height: '1.125rem', color: 'var(--text-primary)' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="12" cy="12" r="10" />
              <path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20" />
              <path d="M2 12h20" />
            </svg>
          </div>
          <div style={{ overflow: 'hidden' }}>
            <div style={{ fontSize: '1rem', fontWeight: 700, color: 'var(--text-primary)', letterSpacing: '-0.02em', lineHeight: 1.2 }}>
              Orbit
            </div>
            <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', textOverflow: 'ellipsis', overflow: 'hidden', whiteSpace: 'nowrap' }}>
              {workspaceName}
            </div>
          </div>
        </div>

        {/* Navigation Items */}
        <nav aria-label="Main Navigation" style={{ padding: '0.75rem 0.5rem', display: 'flex', flexDirection: 'column', gap: '0.25rem' }}>
          
          {/* Files */}
          <button
            type="button"
            className={`nav-item ${currentTab === 'files' ? 'active' : ''}`}
            onClick={() => handleNavClick('files')}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '0.75rem',
              width: '100%',
              padding: '0.625rem 0.75rem',
              borderRadius: 'var(--radius-sm)',
              border: 'none',
              background: currentTab === 'files' ? 'var(--surface-selected)' : 'transparent',
              color: currentTab === 'files' ? 'var(--text-primary)' : 'var(--text-secondary)',
              fontSize: '0.875rem',
              fontWeight: currentTab === 'files' ? 600 : 500,
              cursor: 'pointer',
              textAlign: 'left',
              transition: 'background 0.15s ease',
            }}
          >
            <svg style={{ width: '1.125rem', height: '1.125rem' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
            </svg>
            <span style={{ flex: 1 }}>Files</span>
          </button>

          {/* Needs attention */}
          <button
            type="button"
            className={`nav-item ${currentTab === 'attention' ? 'active' : ''}`}
            onClick={() => handleNavClick('attention')}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '0.75rem',
              width: '100%',
              padding: '0.625rem 0.75rem',
              borderRadius: 'var(--radius-sm)',
              border: 'none',
              background: currentTab === 'attention' ? 'var(--surface-selected)' : 'transparent',
              color: currentTab === 'attention' ? 'var(--text-primary)' : 'var(--text-secondary)',
              fontSize: '0.875rem',
              fontWeight: currentTab === 'attention' ? 600 : 500,
              cursor: 'pointer',
              textAlign: 'left',
              transition: 'background 0.15s ease',
            }}
          >
            <svg style={{ width: '1.125rem', height: '1.125rem', color: attentionCount > 0 ? 'var(--status-warning)' : 'inherit' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
              <line x1="12" y1="9" x2="12" y2="13" />
              <line x1="12" y1="17" x2="12.01" y2="17" />
            </svg>
            <span style={{ flex: 1 }}>Needs attention</span>
            {attentionCount > 0 && (
              <span
                style={{
                  fontSize: '0.75rem',
                  fontWeight: 600,
                  padding: '0.125rem 0.375rem',
                  borderRadius: '9999px',
                  backgroundColor: 'var(--status-warning-bg)',
                  color: 'var(--status-warning)',
                  border: '1px solid var(--status-warning)',
                }}
              >
                {attentionCount}
              </span>
            )}
          </button>

          {/* Devices */}
          <button
            type="button"
            className={`nav-item ${currentTab === 'devices' ? 'active' : ''}`}
            onClick={() => handleNavClick('devices')}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '0.75rem',
              width: '100%',
              padding: '0.625rem 0.75rem',
              borderRadius: 'var(--radius-sm)',
              border: 'none',
              background: currentTab === 'devices' ? 'var(--surface-selected)' : 'transparent',
              color: currentTab === 'devices' ? 'var(--text-primary)' : 'var(--text-secondary)',
              fontSize: '0.875rem',
              fontWeight: currentTab === 'devices' ? 600 : 500,
              cursor: 'pointer',
              textAlign: 'left',
              transition: 'background 0.15s ease',
            }}
          >
            <svg style={{ width: '1.125rem', height: '1.125rem' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <rect x="2" y="3" width="20" height="14" rx="2" ry="2" />
              <line x1="8" y1="21" x2="16" y2="21" />
              <line x1="12" y1="17" x2="12" y2="21" />
            </svg>
            <span style={{ flex: 1 }}>Devices</span>
          </button>

          {/* Deleted files */}
          <button
            type="button"
            className={`nav-item ${currentTab === 'deleted' ? 'active' : ''}`}
            onClick={() => handleNavClick('deleted')}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '0.75rem',
              width: '100%',
              padding: '0.625rem 0.75rem',
              borderRadius: 'var(--radius-sm)',
              border: 'none',
              background: currentTab === 'deleted' ? 'var(--surface-selected)' : 'transparent',
              color: currentTab === 'deleted' ? 'var(--text-primary)' : 'var(--text-secondary)',
              fontSize: '0.875rem',
              fontWeight: currentTab === 'deleted' ? 600 : 500,
              cursor: 'pointer',
              textAlign: 'left',
              transition: 'background 0.15s ease',
            }}
          >
            <svg style={{ width: '1.125rem', height: '1.125rem' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <polyline points="3 6 5 6 21 6" />
              <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
            </svg>
            <span style={{ flex: 1 }}>Deleted files</span>
          </button>

          {/* Settings */}
          <button
            type="button"
            className={`nav-item ${currentTab === 'settings' ? 'active' : ''}`}
            onClick={() => handleNavClick('settings')}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '0.75rem',
              width: '100%',
              padding: '0.625rem 0.75rem',
              borderRadius: 'var(--radius-sm)',
              border: 'none',
              background: currentTab === 'settings' ? 'var(--surface-selected)' : 'transparent',
              color: currentTab === 'settings' ? 'var(--text-primary)' : 'var(--text-secondary)',
              fontSize: '0.875rem',
              fontWeight: currentTab === 'settings' ? 600 : 500,
              cursor: 'pointer',
              textAlign: 'left',
              transition: 'background 0.15s ease',
            }}
          >
            <svg style={{ width: '1.125rem', height: '1.125rem' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="12" cy="12" r="3" />
              <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z" />
            </svg>
            <span style={{ flex: 1 }}>Settings</span>
          </button>
        </nav>
      </div>

      {/* Bottom Status & Logout */}
      <div style={{ padding: '0.75rem', borderTop: '1px solid var(--border)' }}>
        <div style={{ padding: '0.5rem', backgroundColor: 'var(--surface-raised)', borderRadius: 'var(--radius-sm)', marginBottom: '0.5rem' }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '0.25rem' }}>
            <span style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>This Device</span>
            <span style={{ display: 'flex', alignItems: 'center', gap: '0.25rem', fontSize: '0.75rem', color: isSyncing ? 'var(--status-info)' : 'var(--status-success)' }}>
              {isSyncing ? (
                <>
                  <span className="spinner" style={{ width: '0.5rem', height: '0.5rem', border: '1.5px solid var(--border)', borderTopColor: 'var(--status-info)', borderRadius: '50%', animation: 'spin 0.8s linear infinite' }} />
                  Syncing
                </>
              ) : (
                <>
                  <span style={{ width: '6px', height: '6px', borderRadius: '50%', backgroundColor: 'var(--status-success)' }} />
                  Saved
                </>
              )}
            </span>
          </div>
          <div style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-primary)', textOverflow: 'ellipsis', overflow: 'hidden', whiteSpace: 'nowrap' }}>
            {settings?.device_label || 'Local Node'}
          </div>
        </div>

        <button
          type="button"
          onClick={onLogout}
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            gap: '0.5rem',
            width: '100%',
            padding: '0.5rem',
            background: 'transparent',
            border: '1px solid var(--border)',
            borderRadius: 'var(--radius-sm)',
            color: 'var(--text-secondary)',
            fontSize: '0.8125rem',
            cursor: 'pointer',
            transition: 'color 0.15s ease',
          }}
          title="Disconnect UI session (sync daemon continues running)"
        >
          <svg style={{ width: '1rem', height: '1rem' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
            <polyline points="16 17 21 12 16 7" />
            <line x1="21" y1="12" x2="9" y2="12" />
          </svg>
          Sign Out of UI
        </button>
      </div>
    </aside>
  );
};
