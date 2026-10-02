import React, { useState, useEffect, useRef } from 'react';
import { api, APIError } from '../api';
import { DirectoryEntry } from '../types';

interface DirectoryPickerModalProps {
  isOpen: boolean;
  initialPath?: string;
  onSelect: (path: string) => void;
  onClose: () => void;
}

export const DirectoryPickerModal: React.FC<DirectoryPickerModalProps> = ({
  isOpen,
  initialPath,
  onSelect,
  onClose,
}) => {
  const [currentPath, setCurrentPath] = useState<string>(initialPath || '');
  const [parentPath, setParentPath] = useState<string | undefined>();
  const [entries, setEntries] = useState<DirectoryEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const modalRef = useRef<HTMLDivElement>(null);
  const closeButtonRef = useRef<HTMLButtonElement>(null);

  // Load directories whenever currentPath changes or modal opens
  useEffect(() => {
    if (!isOpen) return;
    loadDirectories(currentPath);
  }, [isOpen, currentPath]);

  // Trap focus and handle Escape
  useEffect(() => {
    if (!isOpen) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        onClose();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    closeButtonRef.current?.focus();

    return () => {
      window.removeEventListener('keydown', handleKeyDown);
    };
  }, [isOpen, onClose]);

  const loadDirectories = async (path: string) => {
    setLoading(true);
    setError(null);
    try {
      const res = await api.browseDirectories(path, 100, 0);
      setCurrentPath(res.current_path);
      setParentPath(res.parent_path);
      setEntries(res.entries || []);
    } catch (err: any) {
      if (err instanceof APIError) {
        setError(`${err.code}: ${err.message}`);
      } else {
        setError(err.message || 'Failed to list directory contents');
      }
    } finally {
      setLoading(false);
    }
  };

  if (!isOpen) return null;

  return (
    <div
      className="modal-overlay"
      role="dialog"
      aria-modal="true"
      aria-labelledby="dir-picker-title"
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
    >
      <div
        ref={modalRef}
        className="modal-content"
        style={{
          width: '100%',
          maxWidth: '640px',
          maxHeight: '85vh',
          display: 'flex',
          flexDirection: 'column',
          backgroundColor: 'var(--surface)',
          border: '1px solid var(--border)',
          borderRadius: 'var(--radius-lg)',
          boxShadow: '0 12px 32px rgba(0, 0, 0, 0.6)',
          overflow: 'hidden',
        }}
      >
        <div
          className="modal-header"
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: '1.25rem 1.5rem',
            borderBottom: '1px solid var(--border)',
          }}
        >
          <div>
            <h2 id="dir-picker-title" style={{ fontSize: '1.125rem', fontWeight: 600, margin: 0, color: 'var(--text-primary)' }}>
              Choose Local Folder
            </h2>
            <p style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', margin: '0.25rem 0 0 0' }}>
              Select a directory on this machine for your synchronized files.
            </p>
          </div>
          <button
            ref={closeButtonRef}
            onClick={onClose}
            aria-label="Close dialog"
            className="btn-icon"
            style={{
              background: 'transparent',
              border: 'none',
              color: 'var(--text-secondary)',
              cursor: 'pointer',
              padding: '0.375rem',
              borderRadius: 'var(--radius-sm)',
            }}
          >
            <svg style={{ width: '1.25rem', height: '1.25rem' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <line x1="18" y1="6" x2="6" y2="18" />
              <line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </div>

        {/* Current Path Bar */}
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '0.5rem',
            padding: '0.75rem 1.5rem',
            backgroundColor: 'var(--surface-raised)',
            borderBottom: '1px solid var(--border)',
          }}
        >
          <span style={{ fontSize: '0.8125rem', color: 'var(--text-secondary)', fontWeight: 500 }}>Current:</span>
          <span className="code-font" style={{ fontSize: '0.875rem', color: 'var(--text-primary)', wordBreak: 'break-all' }}>
            {currentPath || '/'}
          </span>
        </div>

        {/* Directory Listing Body */}
        <div
          style={{
            flex: 1,
            overflowY: 'auto',
            padding: '0.75rem 1.5rem',
            minHeight: '280px',
          }}
        >
          {error && (
            <div className="alert alert-danger" style={{ marginBottom: '1rem', padding: '0.75rem', borderRadius: 'var(--radius-sm)', backgroundColor: 'var(--status-danger-bg)', border: '1px solid var(--status-danger)', color: 'var(--status-danger)', fontSize: '0.8125rem' }}>
              {error}
            </div>
          )}

          {loading ? (
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '200px', color: 'var(--text-secondary)' }}>
              <span className="spinner" style={{ width: '1.5rem', height: '1.5rem', border: '2px solid var(--border)', borderTopColor: 'var(--text-primary)', borderRadius: '50%', animation: 'spin 0.8s linear infinite', marginRight: '0.75rem' }} />
              Reading filesystem…
            </div>
          ) : (
            <ul style={{ listStyle: 'none', margin: 0, padding: 0, display: 'flex', flexDirection: 'column', gap: '0.25rem' }}>
              {parentPath && (
                <li>
                  <button
                    onClick={() => setCurrentPath(parentPath)}
                    style={{
                      width: '100%',
                      display: 'flex',
                      alignItems: 'center',
                      gap: '0.75rem',
                      padding: '0.625rem 0.75rem',
                      background: 'transparent',
                      border: '1px solid transparent',
                      borderRadius: 'var(--radius-sm)',
                      color: 'var(--text-primary)',
                      cursor: 'pointer',
                      textAlign: 'left',
                    }}
                    className="dir-item"
                  >
                    <svg style={{ width: '1.25rem', height: '1.25rem', color: 'var(--text-secondary)' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                      <path d="M15 18l-6-6 6-6" />
                    </svg>
                    <span style={{ fontWeight: 500 }}>.. (Up to {parentPath})</span>
                  </button>
                </li>
              )}

              {entries.length === 0 && !parentPath && (
                <div style={{ textAlign: 'center', padding: '2rem', color: 'var(--text-secondary)', fontSize: '0.875rem' }}>
                  No directories found.
                </div>
              )}

              {entries.map((entry) => (
                <li key={entry.path}>
                  <button
                    onClick={() => {
                      if (entry.accessible) {
                        setCurrentPath(entry.path);
                      }
                    }}
                    disabled={!entry.accessible}
                    style={{
                      width: '100%',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                      padding: '0.625rem 0.75rem',
                      background: 'transparent',
                      border: '1px solid transparent',
                      borderRadius: 'var(--radius-sm)',
                      color: entry.accessible ? 'var(--text-primary)' : 'var(--text-secondary)',
                      cursor: entry.accessible ? 'pointer' : 'not-allowed',
                      opacity: entry.accessible ? 1 : 0.5,
                      textAlign: 'left',
                    }}
                    className="dir-item"
                  >
                    <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
                      <svg style={{ width: '1.25rem', height: '1.25rem', color: entry.accessible ? 'var(--text-primary)' : 'var(--text-secondary)' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                        <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
                      </svg>
                      <span style={{ fontWeight: 500, fontSize: '0.875rem' }}>{entry.name}</span>
                    </div>

                    {!entry.accessible && entry.denied_reason && (
                      <span style={{ fontSize: '0.75rem', color: 'var(--status-danger)', padding: '0.125rem 0.375rem', backgroundColor: 'var(--status-danger-bg)', borderRadius: 'var(--radius-sm)' }}>
                        {entry.denied_reason}
                      </span>
                    )}
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        {/* Modal Footer */}
        <div
          className="modal-footer"
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'flex-end',
            gap: '0.75rem',
            padding: '1rem 1.5rem',
            borderTop: '1px solid var(--border)',
            backgroundColor: 'var(--surface-raised)',
          }}
        >
          <button
            type="button"
            onClick={onClose}
            className="btn btn-secondary"
            style={{
              padding: '0.5rem 1rem',
              backgroundColor: 'transparent',
              border: '1px solid var(--border)',
              borderRadius: 'var(--radius-sm)',
              color: 'var(--text-primary)',
              fontSize: '0.875rem',
              fontWeight: 500,
              cursor: 'pointer',
            }}
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => {
              if (currentPath) {
                onSelect(currentPath);
                onClose();
              }
            }}
            className="btn btn-primary"
            disabled={!currentPath}
            style={{
              padding: '0.5rem 1rem',
              backgroundColor: 'var(--text-primary)',
              border: 'none',
              borderRadius: 'var(--radius-sm)',
              color: 'var(--canvas)',
              fontSize: '0.875rem',
              fontWeight: 600,
              cursor: currentPath ? 'pointer' : 'not-allowed',
            }}
          >
            Select Current Folder
          </button>
        </div>
      </div>
    </div>
  );
};
