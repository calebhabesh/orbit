import React, { useState } from 'react';
import { api, APIError } from '../api';

interface OrbitTopbarProps {
  breadcrumbs: string[];
  onNavigateBreadcrumb?: (index: number) => void;
  searchQuery: string;
  onSearchChange: (q: string) => void;
  onRefresh: () => void;
  currentFolderId?: string;
  currentSubpath?: string;
  onToggleMobileNav?: () => void;
}

export const OrbitTopbar: React.FC<OrbitTopbarProps> = ({
  breadcrumbs,
  onNavigateBreadcrumb,
  searchQuery,
  onSearchChange,
  onRefresh,
  currentFolderId,
  currentSubpath,
  onToggleMobileNav,
}) => {
  const [openingFolder, setOpeningFolder] = useState(false);
  const [desktopMessage, setDesktopMessage] = useState<string | null>(null);

  const handleOpenLocalFolder = async () => {
    if (!currentFolderId) return;
    setOpeningFolder(true);
    setDesktopMessage(null);
    try {
      const res = await api.openLocalFolder(currentFolderId, currentSubpath || '');
      setDesktopMessage(`Opened: ${res.path}`);
      setTimeout(() => setDesktopMessage(null), 3500);
    } catch (err: any) {
      if (err instanceof APIError) {
        setDesktopMessage(`${err.code}: ${err.message}`);
      } else {
        setDesktopMessage(err.message || 'Desktop helper unavailable');
      }
      setTimeout(() => setDesktopMessage(null), 5000);
    } finally {
      setOpeningFolder(false);
    }
  };

  return (
    <header
      className="orbit-topbar"
      style={{
        display: 'flex',
        flexWrap: 'wrap',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: '1rem',
        padding: '0.875rem 1.5rem',
        backgroundColor: 'var(--surface)',
        borderBottom: '1px solid var(--border)',
        position: 'sticky',
        top: 0,
        zIndex: 50,
      }}
    >
      {/* Left: Mobile Toggle & Breadcrumbs */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', flex: 1, minWidth: '240px' }}>
        {onToggleMobileNav && (
          <button
            type="button"
            className="mobile-nav-toggle"
            onClick={onToggleMobileNav}
            aria-label="Toggle navigation menu"
            style={{
              display: 'none', // Controlled by CSS media queries
              background: 'transparent',
              border: 'none',
              color: 'var(--text-primary)',
              cursor: 'pointer',
              padding: '0.375rem',
              borderRadius: 'var(--radius-sm)',
            }}
          >
            <svg style={{ width: '1.25rem', height: '1.25rem' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <line x1="3" y1="12" x2="21" y2="12" />
              <line x1="3" y1="6" x2="21" y2="6" />
              <line x1="3" y1="18" x2="21" y2="18" />
            </svg>
          </button>
        )}

        <nav aria-label="Breadcrumb" style={{ display: 'flex', alignItems: 'center', gap: '0.375rem', overflow: 'hidden' }}>
          {breadcrumbs.map((crumb, idx) => (
            <React.Fragment key={idx}>
              {idx > 0 && (
                <span style={{ color: 'var(--text-secondary)', fontSize: '0.875rem', userSelect: 'none' }}>
                  /
                </span>
              )}
              {idx === breadcrumbs.length - 1 ? (
                <span
                  style={{
                    fontSize: '0.875rem',
                    fontWeight: 600,
                    color: 'var(--text-primary)',
                    textOverflow: 'ellipsis',
                    overflow: 'hidden',
                    whiteSpace: 'nowrap',
                  }}
                  aria-current="page"
                >
                  {crumb}
                </span>
              ) : (
                <button
                  type="button"
                  onClick={() => onNavigateBreadcrumb && onNavigateBreadcrumb(idx)}
                  style={{
                    background: 'transparent',
                    border: 'none',
                    color: 'var(--text-secondary)',
                    fontSize: '0.875rem',
                    cursor: 'pointer',
                    padding: 0,
                  }}
                >
                  {crumb}
                </button>
              )}
            </React.Fragment>
          ))}
        </nav>
      </div>

      {/* Center/Right: Search & Actions */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', flexWrap: 'wrap' }}>
        {/* Search */}
        <div style={{ position: 'relative', width: '220px' }}>
          <input
            type="search"
            placeholder="Search files…"
            value={searchQuery}
            onChange={(e) => onSearchChange(e.target.value)}
            aria-label="Filter current items"
            style={{
              width: '100%',
              padding: '0.4375rem 0.75rem 0.4375rem 2rem',
              backgroundColor: 'var(--surface-raised)',
              border: '1px solid var(--border)',
              borderRadius: 'var(--radius-sm)',
              color: 'var(--text-primary)',
              fontSize: '0.8125rem',
            }}
          />
          <svg
            style={{
              position: 'absolute',
              left: '0.625rem',
              top: '50%',
              transform: 'translateY(-50%)',
              width: '0.875rem',
              height: '0.875rem',
              color: 'var(--text-secondary)',
              pointerEvents: 'none',
            }}
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <circle cx="11" cy="11" r="8" />
            <line x1="21" y1="21" x2="16.65" y2="16.65" />
          </svg>
        </div>

        {/* Open local folder button */}
        {currentFolderId && (
          <button
            type="button"
            onClick={handleOpenLocalFolder}
            disabled={openingFolder}
            className="btn btn-secondary"
            title="Open folder in desktop file manager"
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '0.375rem',
              padding: '0.4375rem 0.75rem',
              backgroundColor: 'var(--surface-raised)',
              border: '1px solid var(--border)',
              borderRadius: 'var(--radius-sm)',
              color: 'var(--text-primary)',
              fontSize: '0.8125rem',
              fontWeight: 500,
              cursor: 'pointer',
            }}
          >
            <svg style={{ width: '0.875rem', height: '0.875rem' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
              <polyline points="15 3 21 3 21 9" />
              <line x1="10" y1="14" x2="21" y2="3" />
            </svg>
            <span>{openingFolder ? 'Opening…' : 'Open Local Folder'}</span>
          </button>
        )}

        {/* Refresh button */}
        <button
          type="button"
          onClick={onRefresh}
          className="btn btn-secondary"
          title="Refresh view"
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            padding: '0.4375rem 0.625rem',
            backgroundColor: 'var(--surface-raised)',
            border: '1px solid var(--border)',
            borderRadius: 'var(--radius-sm)',
            color: 'var(--text-primary)',
            fontSize: '0.8125rem',
            cursor: 'pointer',
          }}
        >
          <svg style={{ width: '0.875rem', height: '0.875rem' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <polyline points="23 4 23 10 17 10" />
            <polyline points="1 20 1 14 7 14" />
            <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
          </svg>
        </button>
      </div>

      {/* Desktop Helper Notice Toast */}
      {desktopMessage && (
        <div
          role="status"
          style={{
            position: 'absolute',
            bottom: '-2.5rem',
            right: '1.5rem',
            padding: '0.375rem 0.75rem',
            backgroundColor: 'var(--surface-raised)',
            border: '1px solid var(--border)',
            borderRadius: 'var(--radius-sm)',
            fontSize: '0.75rem',
            color: 'var(--text-primary)',
            boxShadow: '0 4px 12px rgba(0,0,0,0.4)',
            zIndex: 100,
          }}
        >
          {desktopMessage}
        </div>
      )}
    </header>
  );
};
