import React, { useState, useEffect, useRef } from 'react';
import { formatBytes, APIError } from '../api';

// --- 1. CREATE FOLDER MODAL ---

interface CreateFolderModalProps {
  isOpen: boolean;
  currentSubpath: string;
  onClose: () => void;
  onCreate: (folderName: string) => Promise<void>;
}

export const CreateFolderModal: React.FC<CreateFolderModalProps> = ({
  isOpen,
  currentSubpath,
  onClose,
  onCreate,
}) => {
  const [folderName, setFolderName] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (isOpen) {
      setFolderName('');
      setError(null);
      setSubmitting(false);
      setTimeout(() => inputRef.current?.focus(), 50);
    }
  }, [isOpen]);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && isOpen && !submitting) {
        onClose();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, submitting, onClose]);

  if (!isOpen) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const name = folderName.trim();
    if (!name) {
      setError('Please enter a folder name');
      return;
    }
    if (name.includes('/') || name.includes('\\')) {
      setError('Folder name cannot contain slashes');
      return;
    }
    if (name === '.' || name === '..') {
      setError('Invalid folder name');
      return;
    }

    setSubmitting(true);
    setError(null);
    try {
      await onCreate(name);
      onClose();
    } catch (err: any) {
      if (err instanceof APIError) {
        setError(`${err.code}: ${err.message}. ${err.action || ''}`);
      } else {
        setError(err.message || 'Failed to create folder');
      }
    } finally {
      setSubmitting(false);
    }
  };

  const targetDisplayPath = currentSubpath ? `${currentSubpath}/${folderName || '…'}` : (folderName || '…');

  return (
    <div
      className="modal-backdrop"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-labelledby="create-folder-title"
      style={{
        position: 'fixed',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.7)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 100,
        padding: '1rem',
      }}
    >
      <div
        className="modal-content"
        onClick={(e) => e.stopPropagation()}
        style={{
          width: '100%',
          maxWidth: '460px',
          backgroundColor: 'var(--surface)',
          border: '1px solid var(--border)',
          borderRadius: 'var(--radius-md)',
          boxShadow: '0 8px 32px rgba(0,0,0,0.4)',
          overflow: 'hidden',
        }}
      >
        <div
          style={{
            padding: '1rem 1.25rem',
            borderBottom: '1px solid var(--border)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
          }}
        >
          <h2 id="create-folder-title" style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>
            Create New Folder
          </h2>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close"
            style={{ background: 'transparent', border: 'none', color: 'var(--text-secondary)', cursor: 'pointer', fontSize: '1rem' }}
          >
            ✕
          </button>
        </div>

        <form onSubmit={handleSubmit} style={{ padding: '1.25rem', display: 'flex', flexDirection: 'column', gap: '1rem' }}>
          {error && (
            <div
              className="alert alert-danger"
              style={{
                padding: '0.625rem 0.875rem',
                backgroundColor: 'var(--status-danger-bg)',
                border: '1px solid var(--status-danger)',
                borderRadius: 'var(--radius-sm)',
                color: 'var(--status-danger)',
                fontSize: '0.8125rem',
              }}
            >
              {error}
            </div>
          )}

          <div>
            <label htmlFor="input-new-folder-name" style={{ display: 'block', fontSize: '0.8125rem', fontWeight: 500, color: 'var(--text-secondary)', marginBottom: '0.375rem' }}>
              Folder Name
            </label>
            <input
              id="input-new-folder-name"
              ref={inputRef}
              type="text"
              value={folderName}
              onChange={(e) => setFolderName(e.target.value)}
              placeholder="e.g. documents, archive, photos"
              disabled={submitting}
              style={{
                width: '100%',
                padding: '0.5rem 0.75rem',
                backgroundColor: 'var(--surface-raised)',
                border: '1px solid var(--border)',
                borderRadius: 'var(--radius-sm)',
                color: 'var(--text-primary)',
                fontSize: '0.875rem',
                outline: 'none',
              }}
            />
          </div>

          <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
            Location: <code className="code-font" style={{ color: 'var(--text-primary)' }}>/{targetDisplayPath}</code>
          </div>

          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '0.5rem', marginTop: '0.5rem' }}>
            <button
              type="button"
              onClick={onClose}
              disabled={submitting}
              className="btn btn-secondary btn-sm"
              style={{ padding: '0.4rem 0.875rem', fontSize: '0.8125rem' }}
            >
              Cancel
            </button>
            <button
              type="submit"
              id="btn-confirm-create-folder"
              disabled={submitting || !folderName.trim()}
              className="btn btn-primary btn-sm"
              style={{ padding: '0.4rem 1rem', fontSize: '0.8125rem' }}
            >
              {submitting ? 'Creating…' : 'Create Folder'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};


// --- 2. MOVE / RENAME MODAL ---

interface MoveFileModalProps {
  isOpen: boolean;
  sourcePath: string;
  isDir: boolean;
  onClose: () => void;
  onMove: (sourcePath: string, destPath: string, overwrite: boolean) => Promise<void>;
}

export const MoveFileModal: React.FC<MoveFileModalProps> = ({
  isOpen,
  sourcePath,
  isDir,
  onClose,
  onMove,
}) => {
  const [destPath, setDestPath] = useState(sourcePath);
  const [overwrite, setOverwrite] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (isOpen) {
      setDestPath(sourcePath);
      setOverwrite(false);
      setError(null);
      setSubmitting(false);
      setTimeout(() => {
        inputRef.current?.focus();
        inputRef.current?.select();
      }, 50);
    }
  }, [isOpen, sourcePath]);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && isOpen && !submitting) {
        onClose();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, submitting, onClose]);

  if (!isOpen) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const dest = destPath.trim();
    if (!dest) {
      setError('Destination path cannot be empty');
      return;
    }
    if (dest === sourcePath) {
      setError('Destination path is identical to source path');
      return;
    }

    setSubmitting(true);
    setError(null);
    try {
      await onMove(sourcePath, dest, overwrite);
      onClose();
    } catch (err: any) {
      if (err instanceof APIError) {
        if (err.code === 'DESTINATION_EXISTS') {
          setError('Destination path already exists. Check "Overwrite existing destination" below if you wish to overwrite.');
        } else if (err.code === 'STALE_VIEW' || err.code === 'SUBTREE_INVALIDATED') {
          setError(`${err.code}: The source item or directory contents were modified concurrently. Review the latest workspace state before retrying.`);
        } else {
          setError(`${err.code}: ${err.message}. ${err.action || ''}`);
        }
      } else {
        setError(err.message || 'Move/rename operation failed');
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div
      className="modal-backdrop"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-labelledby="move-file-title"
      style={{
        position: 'fixed',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.7)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 100,
        padding: '1rem',
      }}
    >
      <div
        className="modal-content"
        onClick={(e) => e.stopPropagation()}
        style={{
          width: '100%',
          maxWidth: '480px',
          backgroundColor: 'var(--surface)',
          border: '1px solid var(--border)',
          borderRadius: 'var(--radius-md)',
          boxShadow: '0 8px 32px rgba(0,0,0,0.4)',
          overflow: 'hidden',
        }}
      >
        <div
          style={{
            padding: '1rem 1.25rem',
            borderBottom: '1px solid var(--border)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
          }}
        >
          <h2 id="move-file-title" style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>
            {isDir ? 'Move or Rename Directory' : 'Move or Rename File'}
          </h2>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close"
            style={{ background: 'transparent', border: 'none', color: 'var(--text-secondary)', cursor: 'pointer', fontSize: '1rem' }}
          >
            ✕
          </button>
        </div>

        <form onSubmit={handleSubmit} style={{ padding: '1.25rem', display: 'flex', flexDirection: 'column', gap: '1rem' }}>
          {error && (
            <div
              className="alert alert-danger"
              style={{
                padding: '0.625rem 0.875rem',
                backgroundColor: 'var(--status-danger-bg)',
                border: '1px solid var(--status-danger)',
                borderRadius: 'var(--radius-sm)',
                color: 'var(--status-danger)',
                fontSize: '0.8125rem',
              }}
            >
              {error}
            </div>
          )}

          <div>
            <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginBottom: '0.25rem' }}>
              Current Source Path:
            </div>
            <div
              className="code-font"
              style={{
                padding: '0.5rem 0.75rem',
                backgroundColor: 'var(--surface-raised)',
                border: '1px solid var(--border)',
                borderRadius: 'var(--radius-sm)',
                color: 'var(--text-primary)',
                fontSize: '0.8125rem',
                wordBreak: 'break-all',
              }}
            >
              {sourcePath}
            </div>
          </div>

          <div>
            <label htmlFor="input-move-dest-path" style={{ display: 'block', fontSize: '0.8125rem', fontWeight: 500, color: 'var(--text-secondary)', marginBottom: '0.375rem' }}>
              Destination Relative Path
            </label>
            <input
              id="input-move-dest-path"
              ref={inputRef}
              type="text"
              value={destPath}
              onChange={(e) => setDestPath(e.target.value)}
              placeholder="e.g. documents/archive/file.txt"
              disabled={submitting}
              style={{
                width: '100%',
                padding: '0.5rem 0.75rem',
                backgroundColor: 'var(--surface-raised)',
                border: '1px solid var(--border)',
                borderRadius: 'var(--radius-sm)',
                color: 'var(--text-primary)',
                fontSize: '0.875rem',
                outline: 'none',
              }}
            />
          </div>

          {!isDir && (
            <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
              <input
                id="check-move-overwrite"
                type="checkbox"
                checked={overwrite}
                onChange={(e) => setOverwrite(e.target.checked)}
                disabled={submitting}
                style={{ cursor: 'pointer' }}
              />
              <label htmlFor="check-move-overwrite" style={{ fontSize: '0.8125rem', color: 'var(--text-primary)', cursor: 'pointer' }}>
                Overwrite destination if an existing file is present
              </label>
            </div>
          )}

          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '0.5rem', marginTop: '0.5rem' }}>
            <button
              type="button"
              onClick={onClose}
              disabled={submitting}
              className="btn btn-secondary btn-sm"
              style={{ padding: '0.4rem 0.875rem', fontSize: '0.8125rem' }}
            >
              Cancel
            </button>
            <button
              type="submit"
              id="btn-confirm-move"
              disabled={submitting || !destPath.trim() || destPath === sourcePath}
              className="btn btn-primary btn-sm"
              style={{ padding: '0.4rem 1rem', fontSize: '0.8125rem' }}
            >
              {submitting ? 'Moving…' : 'Move / Rename'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};


// --- 3. DELETE CONFIRMATION MODAL ---

interface DeleteFileModalProps {
  isOpen: boolean;
  itemPath: string;
  isDir: boolean;
  size?: number;
  onClose: () => void;
  onDelete: (path: string, recursive: boolean) => Promise<void>;
}

export const DeleteFileModal: React.FC<DeleteFileModalProps> = ({
  isOpen,
  itemPath,
  isDir,
  size,
  onClose,
  onDelete,
}) => {
  const [recursive, setRecursive] = useState(isDir);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (isOpen) {
      setRecursive(isDir);
      setError(null);
      setSubmitting(false);
    }
  }, [isOpen, isDir]);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && isOpen && !submitting) {
        onClose();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, submitting, onClose]);

  if (!isOpen) return null;

  const handleDelete = async () => {
    setSubmitting(true);
    setError(null);
    try {
      await onDelete(itemPath, isDir ? recursive : false);
      onClose();
    } catch (err: any) {
      if (err instanceof APIError) {
        if (err.code === 'DIRECTORY_NOT_EMPTY') {
          setError('DIRECTORY_NOT_EMPTY: Directory contains files or subdirectories. Check "Delete directory and all contents recursively" to proceed.');
        } else if (err.code === 'STALE_VIEW') {
          setError('STALE_VIEW: The file was modified concurrently. Review the latest state before deleting.');
        } else {
          setError(`${err.code}: ${err.message}. ${err.action || ''}`);
        }
      } else {
        setError(err.message || 'Delete operation failed');
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div
      className="modal-backdrop"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-labelledby="delete-file-title"
      style={{
        position: 'fixed',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.7)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 100,
        padding: '1rem',
      }}
    >
      <div
        className="modal-content"
        onClick={(e) => e.stopPropagation()}
        style={{
          width: '100%',
          maxWidth: '480px',
          backgroundColor: 'var(--surface)',
          border: '1px solid var(--border)',
          borderRadius: 'var(--radius-md)',
          boxShadow: '0 8px 32px rgba(0,0,0,0.4)',
          overflow: 'hidden',
        }}
      >
        <div
          style={{
            padding: '1rem 1.25rem',
            borderBottom: '1px solid var(--border)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
          }}
        >
          <h2 id="delete-file-title" style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--status-danger)', margin: 0 }}>
            {isDir ? 'Delete Directory' : 'Delete File'}
          </h2>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close"
            style={{ background: 'transparent', border: 'none', color: 'var(--text-secondary)', cursor: 'pointer', fontSize: '1rem' }}
          >
            ✕
          </button>
        </div>

        <div style={{ padding: '1.25rem', display: 'flex', flexDirection: 'column', gap: '1rem' }}>
          {error && (
            <div
              className="alert alert-danger"
              style={{
                padding: '0.625rem 0.875rem',
                backgroundColor: 'var(--status-danger-bg)',
                border: '1px solid var(--status-danger)',
                borderRadius: 'var(--radius-sm)',
                color: 'var(--status-danger)',
                fontSize: '0.8125rem',
              }}
            >
              {error}
            </div>
          )}

          <div>
            <p style={{ margin: 0, fontSize: '0.875rem', color: 'var(--text-primary)' }}>
              Are you sure you want to delete this {isDir ? 'directory' : 'file'}?
            </p>
            <div
              className="code-font"
              style={{
                marginTop: '0.5rem',
                padding: '0.625rem 0.75rem',
                backgroundColor: 'var(--surface-raised)',
                border: '1px solid var(--border)',
                borderRadius: 'var(--radius-sm)',
                fontSize: '0.8125rem',
                color: 'var(--text-primary)',
                wordBreak: 'break-all',
              }}
            >
              {itemPath}
            </div>
            {!isDir && size !== undefined && (
              <div style={{ marginTop: '0.375rem', fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
                Size: {formatBytes(size)}
              </div>
            )}
          </div>

          {isDir && (
            <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
              <input
                id="check-delete-recursive"
                type="checkbox"
                checked={recursive}
                onChange={(e) => setRecursive(e.target.checked)}
                disabled={submitting}
                style={{ cursor: 'pointer' }}
              />
              <label htmlFor="check-delete-recursive" style={{ fontSize: '0.8125rem', color: 'var(--text-primary)', cursor: 'pointer' }}>
                Delete directory and all contents recursively
              </label>
            </div>
          )}

          <div
            style={{
              padding: '0.625rem 0.75rem',
              backgroundColor: 'var(--surface-raised)',
              border: '1px solid var(--border)',
              borderRadius: 'var(--radius-sm)',
              fontSize: '0.75rem',
              color: 'var(--text-secondary)',
              lineHeight: 1.4,
            }}
          >
            ℹ Historical versions remain recoverable in the <strong>Deleted files</strong> index. Deleting creates durable tombstones without immediately purging retained CAS objects.
          </div>

          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '0.5rem', marginTop: '0.5rem' }}>
            <button
              type="button"
              onClick={onClose}
              disabled={submitting}
              className="btn btn-secondary btn-sm"
              style={{ padding: '0.4rem 0.875rem', fontSize: '0.8125rem' }}
            >
              Cancel
            </button>
            <button
              type="button"
              id="btn-confirm-delete"
              onClick={handleDelete}
              disabled={submitting}
              className="btn btn-danger btn-sm"
              style={{
                padding: '0.4rem 1rem',
                fontSize: '0.8125rem',
                backgroundColor: 'var(--status-danger)',
                color: '#fff',
                border: 'none',
                borderRadius: 'var(--radius-sm)',
                cursor: 'pointer',
              }}
            >
              {submitting ? 'Deleting…' : 'Delete'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};


// --- 4. OVERWRITE UPLOAD CONFIRMATION MODAL ---

interface OverwriteUploadModalProps {
  isOpen: boolean;
  file: File | null;
  targetPath: string;
  existingSize?: number;
  onClose: () => void;
  onConfirmOverwrite: () => Promise<void>;
}

export const OverwriteUploadModal: React.FC<OverwriteUploadModalProps> = ({
  isOpen,
  file,
  targetPath,
  existingSize,
  onClose,
  onConfirmOverwrite,
}) => {
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (isOpen) {
      setError(null);
      setSubmitting(false);
    }
  }, [isOpen]);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && isOpen && !submitting) {
        onClose();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, submitting, onClose]);

  if (!isOpen || !file) return null;

  const handleConfirm = async () => {
    setSubmitting(true);
    setError(null);
    try {
      await onConfirmOverwrite();
      onClose();
    } catch (err: any) {
      if (err instanceof APIError) {
        setError(`${err.code}: ${err.message}. ${err.action || ''}`);
      } else {
        setError(err.message || 'Overwrite failed');
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div
      className="modal-backdrop"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-labelledby="overwrite-upload-title"
      style={{
        position: 'fixed',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.7)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 100,
        padding: '1rem',
      }}
    >
      <div
        className="modal-content"
        onClick={(e) => e.stopPropagation()}
        style={{
          width: '100%',
          maxWidth: '480px',
          backgroundColor: 'var(--surface)',
          border: '1px solid var(--border)',
          borderRadius: 'var(--radius-md)',
          boxShadow: '0 8px 32px rgba(0,0,0,0.4)',
          overflow: 'hidden',
        }}
      >
        <div
          style={{
            padding: '1rem 1.25rem',
            borderBottom: '1px solid var(--border)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
          }}
        >
          <h2 id="overwrite-upload-title" style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--status-warning)', margin: 0 }}>
            Destination File Exists
          </h2>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close"
            style={{ background: 'transparent', border: 'none', color: 'var(--text-secondary)', cursor: 'pointer', fontSize: '1rem' }}
          >
            ✕
          </button>
        </div>

        <div style={{ padding: '1.25rem', display: 'flex', flexDirection: 'column', gap: '1rem' }}>
          {error && (
            <div
              className="alert alert-danger"
              style={{
                padding: '0.625rem 0.875rem',
                backgroundColor: 'var(--status-danger-bg)',
                border: '1px solid var(--status-danger)',
                borderRadius: 'var(--radius-sm)',
                color: 'var(--status-danger)',
                fontSize: '0.8125rem',
              }}
            >
              {error}
            </div>
          )}

          <p style={{ margin: 0, fontSize: '0.875rem', color: 'var(--text-primary)' }}>
            A file named <strong className="code-font">{file.name}</strong> already exists in this folder.
          </p>

          <div
            style={{
              padding: '0.75rem',
              backgroundColor: 'var(--surface-raised)',
              border: '1px solid var(--border)',
              borderRadius: 'var(--radius-sm)',
              display: 'flex',
              flexDirection: 'column',
              gap: '0.5rem',
              fontSize: '0.8125rem',
            }}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between' }}>
              <span style={{ color: 'var(--text-secondary)' }}>Target Path:</span>
              <span className="code-font" style={{ color: 'var(--text-primary)' }}>{targetPath}</span>
            </div>
            {existingSize !== undefined && (
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span style={{ color: 'var(--text-secondary)' }}>Existing Size:</span>
                <span style={{ color: 'var(--text-primary)' }}>{formatBytes(existingSize)}</span>
              </div>
            )}
            <div style={{ display: 'flex', justifyContent: 'space-between' }}>
              <span style={{ color: 'var(--text-secondary)' }}>New Upload Size:</span>
              <span style={{ color: 'var(--text-primary)' }}>{formatBytes(file.size)}</span>
            </div>
          </div>

          <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', lineHeight: 1.4 }}>
            Overwriting will displace the existing working copy safely and capture a new durable causal version with previous versions preserved in history.
          </div>

          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '0.5rem', marginTop: '0.5rem' }}>
            <button
              type="button"
              onClick={onClose}
              disabled={submitting}
              className="btn btn-secondary btn-sm"
              style={{ padding: '0.4rem 0.875rem', fontSize: '0.8125rem' }}
            >
              Cancel
            </button>
            <button
              type="button"
              id="btn-confirm-overwrite-upload"
              onClick={handleConfirm}
              disabled={submitting}
              className="btn btn-primary btn-sm"
              style={{ padding: '0.4rem 1rem', fontSize: '0.8125rem' }}
            >
              {submitting ? 'Overwriting…' : 'Overwrite Existing File'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};
