import React, { useState, useEffect, useRef, useCallback } from 'react';
import {
  FolderRecord,
  BrowseItem,
  BrowseResult,
  SearchResult,
  RestorePreview,
  VersionID,
  FileHistoryItem,
} from '../types';
import {
  api,
  formatBytes,
  formatVersionID,
  formatTimestamp,
  APIError,
} from '../api';
import { FileDetailsDrawer, WorkingStateBadge } from '../components/FileDetailsDrawer';
import { RestoreModal } from '../components/RestoreModal';
import {
  CreateFolderModal,
  MoveFileModal,
  DeleteFileModal,
  OverwriteUploadModal,
} from '../components/FileActionModals';

interface FilesViewProps {
  folders: FolderRecord[];
  searchQuery?: string;
  onSearchChange?: (q: string) => void;
  onOpenFolder?: () => void;
}

export const FilesView: React.FC<FilesViewProps> = ({
  folders,
  searchQuery = '',
  onSearchChange,
  onOpenFolder,
}) => {
  const [selectedFolder, setSelectedFolder] = useState<string>(
    folders[0]?.folder || folders[0]?.folder_id || ''
  );

  // Navigation path & history stack
  const [currentSubpath, setCurrentSubpath] = useState<string>('');
  const [historyStack, setHistoryStack] = useState<string[]>(['']);
  const [historyIndex, setHistoryIndex] = useState<number>(0);
  const [parentPath, setParentPath] = useState<string>('');

  // Browse / Search state
  const [items, setItems] = useState<BrowseItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [totalItems, setTotalItems] = useState<number>(0);
  const [error, setError] = useState<string | null>(null);
  const [notification, setNotification] = useState<string | null>(null);

  // Mutation Modals State
  const [createFolderOpen, setCreateFolderOpen] = useState(false);
  const [moveModalOpen, setMoveModalOpen] = useState(false);
  const [moveTarget, setMoveTarget] = useState<{ path: string; isDir: boolean } | null>(null);
  const [deleteModalOpen, setDeleteModalOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<{ path: string; isDir: boolean; size?: number } | null>(null);
  const [overwriteModalOpen, setOverwriteModalOpen] = useState(false);
  const [overwriteFile, setOverwriteFile] = useState<File | null>(null);
  const [overwriteTarget, setOverwriteTarget] = useState<{ path: string; existingSize?: number } | null>(null);

  // Operation progress & drag-over state
  const [actionProgress, setActionProgress] = useState<string | null>(null);
  const [isDraggingOver, setIsDraggingOver] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Sort & View mode
  const [sortBy, setSortBy] = useState<'name' | 'size' | 'mtime' | 'kind'>('name');
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc');
  const [viewMode, setViewMode] = useState<'list' | 'grid'>('list');

  // Selection & Details panel
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [focusedIndex, setFocusedIndex] = useState<number>(-1);

  // Restore Modal State
  const [restorePreview, setRestorePreview] = useState<RestorePreview | null>(null);
  const [restoringPath, setRestoringPath] = useState<string | null>(null);

  // Desktop Helper notice
  const [desktopMessage, setDesktopMessage] = useState<string | null>(null);

  // Request sequencing to discard obsolete responses
  const reqSeq = useRef<number>(0);

  // Update selected folder if folders list changes
  useEffect(() => {
    if (!selectedFolder && folders.length > 0) {
      setSelectedFolder(folders[0].folder || folders[0].folder_id || '');
    }
  }, [folders, selectedFolder]);

  // Navigate to a directory (with history stack management)
  const navigateToDir = useCallback((targetPath: string) => {
    setCurrentSubpath(targetPath);
    setHistoryStack((prev) => {
      const truncated = prev.slice(0, historyIndex + 1);
      return [...truncated, targetPath];
    });
    setHistoryIndex((prev) => prev + 1);
    setFocusedIndex(-1);
    if (onSearchChange && searchQuery) {
      onSearchChange('');
    }
  }, [historyIndex, onSearchChange, searchQuery]);

  const navigateBack = () => {
    if (historyIndex > 0) {
      const newIdx = historyIndex - 1;
      setHistoryIndex(newIdx);
      setCurrentSubpath(historyStack[newIdx]);
      setFocusedIndex(-1);
    }
  };

  const navigateForward = () => {
    if (historyIndex < historyStack.length - 1) {
      const newIdx = historyIndex + 1;
      setHistoryIndex(newIdx);
      setCurrentSubpath(historyStack[newIdx]);
      setFocusedIndex(-1);
    }
  };

  const navigateUp = () => {
    if (currentSubpath) {
      navigateToDir(parentPath);
    }
  };

  // Main data loader (Browse or Search)
  const loadData = useCallback(async (isLoadMore = false) => {
    if (!selectedFolder) return;

    const mySeq = ++reqSeq.current;
    if (isLoadMore) {
      setLoadingMore(true);
    } else {
      setLoading(true);
      setError(null);
    }

    try {
      if (searchQuery.trim()) {
        // Backend Search
        const res: SearchResult = await api.search(selectedFolder, {
          query: searchQuery.trim(),
          limit: 50,
          cursor: isLoadMore && nextCursor ? nextCursor : undefined,
        });

        if (mySeq !== reqSeq.current) return; // Obsolete response

        if (isLoadMore) {
          setItems((prev) => [...prev, ...res.items]);
        } else {
          setItems(res.items);
        }
        setNextCursor(res.next_cursor || null);
        setTotalItems(res.total_found);
        setParentPath('');
      } else {
        // Hierarchical Directory Browse
        const res: BrowseResult = await api.browse(selectedFolder, {
          path: currentSubpath,
          sort: sortBy,
          direction: sortDir,
          limit: 50,
          cursor: isLoadMore && nextCursor ? nextCursor : undefined,
        });

        if (mySeq !== reqSeq.current) return; // Obsolete response

        if (isLoadMore) {
          setItems((prev) => [...prev, ...res.items]);
        } else {
          setItems(res.items);
        }
        setNextCursor(res.next_cursor || null);
        setTotalItems(res.total_items);
        setParentPath(res.parent_path);
      }
    } catch (err: any) {
      if (mySeq !== reqSeq.current) return;
      if (err instanceof APIError && err.code === 'STALE_VIEW') {
        // Cursor or generation was stale — refresh view from scratch
        setNotification('View updated by recent changes; refreshed listing.');
        setNextCursor(null);
        setTimeout(() => setNotification(null), 4000);
        // Retry initial page
        reqSeq.current++;
        try {
          const fresh = await api.browse(selectedFolder, {
            path: currentSubpath,
            sort: sortBy,
            direction: sortDir,
            limit: 50,
          });
          setItems(fresh.items);
          setNextCursor(fresh.next_cursor || null);
          setTotalItems(fresh.total_items);
          setParentPath(fresh.parent_path);
        } catch {}
      } else {
        setError(err.message || 'Failed to load files');
      }
    } finally {
      if (mySeq === reqSeq.current) {
        setLoading(false);
        setLoadingMore(false);
      }
    }
  }, [selectedFolder, searchQuery, currentSubpath, sortBy, sortDir, nextCursor]);

  // Trigger reload when folder, subpath, sort, or search changes
  useEffect(() => {
    setNextCursor(null);
    loadData(false);
  }, [selectedFolder, currentSubpath, sortBy, sortDir, searchQuery]);

  // Handle Sort Toggle
  const handleSort = (column: 'name' | 'size' | 'mtime' | 'kind') => {
    if (sortBy === column) {
      setSortDir(sortDir === 'asc' ? 'desc' : 'asc');
    } else {
      setSortBy(column);
      setSortDir('asc');
    }
  };

  // Open Local Folder Helper
  const handleOpenLocalFolder = async () => {
    if (!selectedFolder) return;
    setDesktopMessage(null);
    try {
      const res = await api.openLocalFolder(selectedFolder, currentSubpath);
      setDesktopMessage(`Opened: ${res.path}`);
      setTimeout(() => setDesktopMessage(null), 3500);
    } catch (err: any) {
      if (err instanceof APIError) {
        setDesktopMessage(`${err.code}: ${err.message}. ${err.action || ''}`);
      } else {
        setDesktopMessage(err.message || 'Desktop helper unavailable on headless system');
      }
      setTimeout(() => setDesktopMessage(null), 5000);
    }
  };

  // Restore Flow
  const handleOpenRestore = async (item: FileHistoryItem) => {
    if (!selectedFolder || !selectedPath) return;
    setError(null);
    setRestoringPath(selectedPath);
    try {
      const prev = await api.previewRestore(selectedFolder, selectedPath, {
        Folder: selectedFolder,
        Author: item.author_id,
        Counter: item.counter,
      });
      setRestorePreview(prev);
    } catch (err: any) {
      setError(err.message || 'Failed to prepare restore preview');
    }
  };

  const handleRestoreSuccess = (newVersion: VersionID) => {
    setNotification(
      `Restored ${restoringPath}! Created new causal version ${formatVersionID(newVersion)}.`
    );
    setRestorePreview(null);
    setRestoringPath(null);
    loadData(false);
    setTimeout(() => setNotification(null), 5000);
  };

  // File Mutation Handlers
  const handleCreateFolder = async (folderName: string) => {
    if (!selectedFolder) return;
    const targetPath = currentSubpath ? `${currentSubpath}/${folderName}` : folderName;
    setActionProgress(`Creating directory "${targetPath}"…`);
    try {
      const res = await api.createDir(selectedFolder, targetPath);
      setNotification(`Created directory ${res.path}`);
      setTimeout(() => setNotification(null), 4000);
      loadData(false);
    } finally {
      setActionProgress(null);
    }
  };

  const handleInitiateUpload = (file: File) => {
    if (!selectedFolder) return;
    const targetPath = currentSubpath ? `${currentSubpath}/${file.name}` : file.name;
    const existing = items.find((it) => it.name === file.name || it.path === targetPath);
    if (existing) {
      setOverwriteFile(file);
      setOverwriteTarget({ path: targetPath, existingSize: existing.size });
      setOverwriteModalOpen(true);
    } else {
      doUpload(file, targetPath, false);
    }
  };

  const doUpload = async (file: File, targetPath: string, overwrite: boolean) => {
    if (!selectedFolder) return;
    setActionProgress(`Uploading "${file.name}" to /${targetPath}…`);
    setError(null);
    try {
      const res = await api.importFile(selectedFolder, targetPath, file, { overwrite });
      setNotification(`Uploaded ${res.path} (${formatBytes(res.size)})`);
      setTimeout(() => setNotification(null), 4000);
      loadData(false);
    } catch (err: any) {
      if (err instanceof APIError && err.code === 'DESTINATION_EXISTS' && !overwrite) {
        setOverwriteFile(file);
        setOverwriteTarget({ path: targetPath });
        setOverwriteModalOpen(true);
      } else if (err instanceof APIError) {
        setError(`${err.code}: ${err.message}. ${err.action || ''}`);
      } else {
        setError(err.message || 'Upload failed');
      }
    } finally {
      setActionProgress(null);
    }
  };

  const handleOpenMove = (path: string, isDir: boolean) => {
    setMoveTarget({ path, isDir });
    setMoveModalOpen(true);
  };

  const handleMove = async (sourcePath: string, destPath: string, overwrite: boolean) => {
    if (!selectedFolder) return;
    setActionProgress(`Moving "${sourcePath}" to "${destPath}"…`);
    try {
      const res = await api.moveFile(selectedFolder, sourcePath, destPath, { overwrite });
      setNotification(`Moved ${res.source_path} → ${res.dest_path}`);
      setTimeout(() => setNotification(null), 4000);
      if (selectedPath === sourcePath) {
        setSelectedPath(res.dest_path);
      }
      loadData(false);
    } finally {
      setActionProgress(null);
    }
  };

  const handleOpenDelete = (path: string, isDir: boolean, size?: number) => {
    setDeleteTarget({ path, isDir, size });
    setDeleteModalOpen(true);
  };

  const handleDelete = async (path: string, recursive: boolean) => {
    if (!selectedFolder) return;
    setActionProgress(`Deleting "${path}"…`);
    try {
      const res = await api.deleteFile(selectedFolder, path, { recursive });
      setNotification(`Deleted ${res.path} (${res.deleted_count} item${res.deleted_count === 1 ? '' : 's'} removed)`);
      setTimeout(() => setNotification(null), 4000);
      if (selectedPath === path) {
        setSelectedPath(null);
      }
      loadData(false);
    } finally {
      setActionProgress(null);
    }
  };

  const selectedItem = items.find((it) => it.path === selectedPath);

  const handleDownloadSelected = async () => {
    if (!selectedFolder || !selectedItem || selectedItem.is_dir) return;
    try {
      const details = await api.getFileDetails(selectedFolder, selectedItem.path);
      if (details.heads && details.heads.length > 0) {
        api.downloadContent(selectedFolder, details.heads[0].author_id, details.heads[0].counter, selectedItem.name);
      }
    } catch (err: any) {
      setError(err.message || 'Failed to download file');
    }
  };

  // Keyboard navigation
  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) {
      return;
    }

    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setFocusedIndex((prev) => Math.min(prev + 1, items.length - 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setFocusedIndex((prev) => Math.max(prev - 1, 0));
    } else if (e.key === 'Enter') {
      if (focusedIndex >= 0 && focusedIndex < items.length) {
        const item = items[focusedIndex];
        if (item.is_dir) {
          navigateToDir(item.path);
        } else {
          setSelectedPath(item.path);
        }
      }
    } else if (e.key === ' ') {
      e.preventDefault();
      if (focusedIndex >= 0 && focusedIndex < items.length) {
        setSelectedPath(items[focusedIndex].path);
      }
    } else if (e.key === 'Backspace' || (e.altKey && e.key === 'ArrowLeft')) {
      if (currentSubpath) {
        e.preventDefault();
        navigateUp();
      }
    } else if (e.key === 'Escape') {
      if (selectedPath) {
        setSelectedPath(null);
      } else if (searchQuery && onSearchChange) {
        onSearchChange('');
      }
    }
  };

  // Breadcrumbs elements
  const pathSegments = currentSubpath ? currentSubpath.split('/').filter(Boolean) : [];
  const breadcrumbItems = [
    { label: 'Root', path: '' },
    ...pathSegments.map((seg, idx) => ({
      label: seg,
      path: pathSegments.slice(0, idx + 1).join('/'),
    })),
  ];

  return (
    <div
      className="files-view-container"
      onKeyDown={handleKeyDown}
      tabIndex={0}
      style={{
        display: 'flex',
        flexDirection: 'row',
        gap: 0,
        height: 'calc(100vh - 120px)',
        minHeight: '600px',
        backgroundColor: 'var(--surface)',
        border: '1px solid var(--border)',
        borderRadius: 'var(--radius-md)',
        overflow: 'hidden',
        position: 'relative',
        outline: 'none',
      }}
    >
      {/* Left/Main Column: Toolbar, Breadcrumbs, File List / Grid */}
      <div
        style={{
          flex: 1,
          display: 'flex',
          flexDirection: 'column',
          minWidth: 0,
          height: '100%',
          overflow: 'hidden',
        }}
      >
        {/* Notifications / Errors */}
        {error && (
          <div
            className="alert alert-danger"
            style={{
              padding: '0.625rem 1rem',
              backgroundColor: 'var(--status-danger-bg)',
              borderBottom: '1px solid var(--status-danger)',
              color: 'var(--status-danger)',
              fontSize: '0.8125rem',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
            }}
          >
            <div>{error}</div>
            <button
              type="button"
              onClick={() => setError(null)}
              style={{ background: 'transparent', border: 'none', color: 'inherit', cursor: 'pointer' }}
            >
              ✕
            </button>
          </div>
        )}

        {notification && (
          <div
            style={{
              padding: '0.625rem 1rem',
              backgroundColor: 'var(--status-success-bg)',
              borderBottom: '1px solid var(--status-success)',
              color: 'var(--status-success)',
              fontSize: '0.8125rem',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
            }}
          >
            <div>{notification}</div>
            <button
              type="button"
              onClick={() => setNotification(null)}
              style={{ background: 'transparent', border: 'none', color: 'inherit', cursor: 'pointer' }}
            >
              ✕
            </button>
          </div>
        )}

        {/* Toolbar: Navigation Buttons, Breadcrumbs, Workspace Dropdown & View Mode */}
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            flexWrap: 'wrap',
            gap: '0.75rem',
            padding: '0.75rem 1rem',
            borderBottom: '1px solid var(--border)',
            backgroundColor: 'var(--surface)',
          }}
        >
          {/* History Nav (Back, Forward, Up) & Breadcrumbs */}
          <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', flex: 1, minWidth: '260px' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '0.25rem' }}>
              <button
                type="button"
                id="btn-nav-back"
                onClick={navigateBack}
                disabled={historyIndex <= 0}
                title="Back in history"
                className="btn btn-secondary btn-sm"
                style={{ padding: '0.3125rem 0.5rem', fontSize: '0.75rem' }}
              >
                ←
              </button>
              <button
                type="button"
                id="btn-nav-forward"
                onClick={navigateForward}
                disabled={historyIndex >= historyStack.length - 1}
                title="Forward in history"
                className="btn btn-secondary btn-sm"
                style={{ padding: '0.3125rem 0.5rem', fontSize: '0.75rem' }}
              >
                →
              </button>
              {currentSubpath && (
                <button
                  type="button"
                  id="btn-nav-up"
                  onClick={navigateUp}
                  title="Up to parent directory"
                  className="btn btn-secondary btn-sm"
                  style={{ padding: '0.3125rem 0.5rem', fontSize: '0.75rem' }}
                >
                  ↑ Up
                </button>
              )}
            </div>

            {/* Breadcrumb Trail */}
            <nav
              aria-label="Directory navigation"
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '0.375rem',
                overflowX: 'auto',
                whiteSpace: 'nowrap',
                padding: '0.25rem 0',
              }}
            >
              {breadcrumbItems.map((crumb, idx) => {
                const isLast = idx === breadcrumbItems.length - 1 && !searchQuery;
                return (
                  <React.Fragment key={crumb.path}>
                    {idx > 0 && <span style={{ color: 'var(--text-secondary)', fontSize: '0.8125rem' }}>/</span>}
                    {isLast ? (
                      <span
                        style={{
                          fontSize: '0.8125rem',
                          fontWeight: 600,
                          color: 'var(--text-primary)',
                        }}
                        aria-current="page"
                      >
                        {crumb.label}
                      </span>
                    ) : (
                      <button
                        type="button"
                        onClick={() => navigateToDir(crumb.path)}
                        style={{
                          background: 'transparent',
                          border: 'none',
                          color: 'var(--text-secondary)',
                          fontSize: '0.8125rem',
                          cursor: 'pointer',
                          padding: '0.125rem 0.25rem',
                          borderRadius: 'var(--radius-sm)',
                        }}
                      >
                        {crumb.label}
                      </button>
                    )}
                  </React.Fragment>
                );
              })}

              {searchQuery && (
                <>
                  <span style={{ color: 'var(--text-secondary)', fontSize: '0.8125rem' }}>/</span>
                  <span style={{ fontSize: '0.8125rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                    Search: "{searchQuery}"
                  </span>
                </>
              )}
            </nav>
          </div>

          {/* Right Toolbar Controls: Upload, New Folder, Open Local Folder, View Mode Toggle, Workspace Select */}
          <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
            {/* Action Button: Upload */}
            <input
              type="file"
              id="file-upload-input"
              ref={fileInputRef}
              style={{ display: 'none' }}
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) handleInitiateUpload(f);
                e.target.value = '';
              }}
            />
            <button
              type="button"
              id="btn-upload-file"
              onClick={() => fileInputRef.current?.click()}
              className="btn btn-secondary btn-sm"
              title="Upload file to current directory"
              style={{ padding: '0.3125rem 0.625rem', fontSize: '0.75rem', display: 'flex', alignItems: 'center', gap: '0.375rem' }}
            >
              <svg style={{ width: '0.875rem', height: '0.875rem' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
                <polyline points="17 8 12 3 7 8" />
                <line x1="12" y1="3" x2="12" y2="15" />
              </svg>
              <span>Upload</span>
            </button>

            {/* Action Button: New Folder */}
            <button
              type="button"
              id="btn-new-folder"
              onClick={() => setCreateFolderOpen(true)}
              className="btn btn-secondary btn-sm"
              title="Create new directory in current path"
              style={{ padding: '0.3125rem 0.625rem', fontSize: '0.75rem', display: 'flex', alignItems: 'center', gap: '0.375rem' }}
            >
              <svg style={{ width: '0.875rem', height: '0.875rem' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
                <line x1="12" y1="11" x2="12" y2="17" />
                <line x1="9" y1="14" x2="15" y2="14" />
              </svg>
              <span>New Folder</span>
            </button>

            {/* Open Folder in OS Desktop Manager */}
            <button
              type="button"
              id="btn-open-desktop-folder"
              onClick={handleOpenLocalFolder}
              className="btn btn-secondary btn-sm"
              title="Open current directory in desktop file manager"
              style={{ padding: '0.3125rem 0.625rem', fontSize: '0.75rem', display: 'flex', alignItems: 'center', gap: '0.375rem' }}
            >
              <svg style={{ width: '0.875rem', height: '0.875rem' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
                <polyline points="15 3 21 3 21 9" />
                <line x1="10" y1="14" x2="21" y2="3" />
              </svg>
              <span>Open Local Folder</span>
            </button>

            {/* View Mode Toggle: List vs Grid */}
            <div
              style={{
                display: 'inline-flex',
                backgroundColor: 'var(--surface-raised)',
                border: '1px solid var(--border)',
                borderRadius: 'var(--radius-sm)',
                overflow: 'hidden',
              }}
            >
              <button
                type="button"
                id="btn-view-list"
                onClick={() => setViewMode('list')}
                title="List view"
                style={{
                  padding: '0.3125rem 0.5rem',
                  background: viewMode === 'list' ? 'var(--surface-selected)' : 'transparent',
                  border: 'none',
                  color: viewMode === 'list' ? 'var(--text-primary)' : 'var(--text-secondary)',
                  cursor: 'pointer',
                  display: 'flex',
                  alignItems: 'center',
                }}
              >
                <svg style={{ width: '0.875rem', height: '0.875rem' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <line x1="8" y1="6" x2="21" y2="6" />
                  <line x1="8" y1="12" x2="21" y2="12" />
                  <line x1="8" y1="18" x2="21" y2="18" />
                  <line x1="3" y1="6" x2="3.01" y2="6" />
                  <line x1="3" y1="12" x2="3.01" y2="12" />
                  <line x1="3" y1="18" x2="3.01" y2="18" />
                </svg>
              </button>
              <button
                type="button"
                id="btn-view-grid"
                onClick={() => setViewMode('grid')}
                title="Grid view"
                style={{
                  padding: '0.3125rem 0.5rem',
                  background: viewMode === 'grid' ? 'var(--surface-selected)' : 'transparent',
                  border: 'none',
                  color: viewMode === 'grid' ? 'var(--text-primary)' : 'var(--text-secondary)',
                  cursor: 'pointer',
                  display: 'flex',
                  alignItems: 'center',
                }}
              >
                <svg style={{ width: '0.875rem', height: '0.875rem' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <rect x="3" y="3" width="7" height="7" />
                  <rect x="14" y="3" width="7" height="7" />
                  <rect x="14" y="14" width="7" height="7" />
                  <rect x="3" y="14" width="7" height="7" />
                </svg>
              </button>
            </div>

            {/* Workspace Select (if multiple workspaces exist) */}
            {folders.length > 1 && (
              <select
                value={selectedFolder}
                onChange={(e) => {
                  setSelectedFolder(e.target.value);
                  setCurrentSubpath('');
                  setHistoryStack(['']);
                  setHistoryIndex(0);
                  setSelectedPath(null);
                }}
                style={{
                  padding: '0.3125rem 0.625rem',
                  backgroundColor: 'var(--surface-raised)',
                  border: '1px solid var(--border)',
                  borderRadius: 'var(--radius-sm)',
                  color: 'var(--text-primary)',
                  fontSize: '0.75rem',
                }}
              >
                {folders.map((f) => (
                  <option key={f.folder || f.folder_id} value={f.folder || f.folder_id}>
                    {f.root_path || f.folder || f.folder_id}
                  </option>
                ))}
              </select>
            )}
          </div>
        </div>

        {/* Action Progress Banner */}
        {actionProgress && (
          <div
            id="action-progress-banner"
            style={{
              padding: '0.5rem 1rem',
              backgroundColor: 'var(--surface-raised)',
              borderBottom: '1px solid var(--border)',
              display: 'flex',
              alignItems: 'center',
              gap: '0.625rem',
              fontSize: '0.8125rem',
              color: 'var(--text-primary)',
            }}
          >
            <span
              className="spinner"
              style={{
                width: '0.875rem',
                height: '0.875rem',
                border: '2px solid var(--border)',
                borderTopColor: 'var(--text-primary)',
                borderRadius: '50%',
                animation: 'spin 0.8s linear infinite',
              }}
            />
            <span>{actionProgress}</span>
          </div>
        )}

        {/* Contextual Selection Action Bar */}
        {selectedItem && (
          <div
            id="selection-toolbar"
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              padding: '0.5rem 1rem',
              backgroundColor: 'var(--surface-raised)',
              borderBottom: '1px solid var(--border)',
              fontSize: '0.8125rem',
            }}
          >
            <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', minWidth: 0 }}>
              <span style={{ fontWeight: 600, color: 'var(--text-primary)' }}>Selected:</span>
              <span
                className="code-font"
                style={{
                  color: 'var(--text-primary)',
                  maxWidth: '300px',
                  textOverflow: 'ellipsis',
                  overflow: 'hidden',
                  whiteSpace: 'nowrap',
                }}
              >
                {selectedItem.name}
              </span>
              <span style={{ color: 'var(--text-secondary)', fontSize: '0.75rem' }}>
                ({selectedItem.is_dir ? 'Directory' : formatBytes(selectedItem.size)})
              </span>
            </div>

            <div style={{ display: 'flex', alignItems: 'center', gap: '0.375rem' }}>
              {!selectedItem.is_dir && (
                <button
                  type="button"
                  id="btn-action-download"
                  onClick={handleDownloadSelected}
                  className="btn btn-secondary btn-sm"
                  style={{ padding: '0.25rem 0.625rem', fontSize: '0.75rem' }}
                >
                  Download
                </button>
              )}
              <button
                type="button"
                id="btn-action-move"
                onClick={() => handleOpenMove(selectedItem.path, selectedItem.is_dir)}
                className="btn btn-secondary btn-sm"
                style={{ padding: '0.25rem 0.625rem', fontSize: '0.75rem' }}
              >
                Rename / Move
              </button>
              <button
                type="button"
                id="btn-action-delete"
                onClick={() => handleOpenDelete(selectedItem.path, selectedItem.is_dir, selectedItem.size)}
                className="btn btn-secondary btn-sm"
                style={{ padding: '0.25rem 0.625rem', fontSize: '0.75rem', color: 'var(--status-danger)' }}
              >
                Delete
              </button>
              <button
                type="button"
                onClick={() => setSelectedPath(null)}
                title="Deselect item"
                style={{
                  background: 'transparent',
                  border: 'none',
                  color: 'var(--text-secondary)',
                  cursor: 'pointer',
                  padding: '0.125rem 0.375rem',
                  fontSize: '0.875rem',
                }}
              >
                ✕
              </button>
            </div>
          </div>
        )}

        {/* Desktop Helper Toast */}
        {desktopMessage && (
          <div
            role="status"
            style={{
              padding: '0.5rem 1rem',
              backgroundColor: 'var(--surface-raised)',
              borderBottom: '1px solid var(--border)',
              fontSize: '0.75rem',
              color: 'var(--text-primary)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
            }}
          >
            <span>{desktopMessage}</span>
            <button
              type="button"
              onClick={() => setDesktopMessage(null)}
              style={{ background: 'transparent', border: 'none', color: 'inherit', cursor: 'pointer' }}
            >
              ✕
            </button>
          </div>
        )}

        {/* Search Notice (if active) */}
        {searchQuery && (
          <div
            style={{
              padding: '0.5rem 1rem',
              backgroundColor: 'var(--surface-raised)',
              borderBottom: '1px solid var(--border)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              fontSize: '0.8125rem',
            }}
          >
            <div>
              Showing search results matching <span style={{ fontWeight: 600, color: 'var(--text-primary)' }}>"{searchQuery}"</span> across workspace ({totalItems} found)
            </div>
            {onSearchChange && (
              <button
                type="button"
                onClick={() => onSearchChange('')}
                className="btn btn-secondary btn-sm"
                style={{ padding: '0.25rem 0.5rem', fontSize: '0.75rem' }}
              >
                Clear Search
              </button>
            )}
          </div>
        )}

        {/* Content Area: Loading, Empty, List or Grid (with Drag & Drop zone) */}
        <div
          onDragOver={(e) => {
            e.preventDefault();
            setIsDraggingOver(true);
          }}
          onDragLeave={() => setIsDraggingOver(false)}
          onDrop={(e) => {
            e.preventDefault();
            setIsDraggingOver(false);
            const f = e.dataTransfer.files?.[0];
            if (f) handleInitiateUpload(f);
          }}
          style={{
            flex: 1,
            overflowY: 'auto',
            display: 'flex',
            flexDirection: 'column',
            position: 'relative',
          }}
        >
          {isDraggingOver && (
            <div
              id="drag-drop-overlay"
              style={{
                position: 'absolute',
                top: 0,
                left: 0,
                right: 0,
                bottom: 0,
                backgroundColor: 'rgba(0, 0, 0, 0.75)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                zIndex: 30,
                pointerEvents: 'none',
                border: '2px dashed var(--text-primary)',
              }}
            >
              <div style={{ textAlign: 'center', color: 'var(--text-primary)', fontWeight: 600, fontSize: '1.125rem' }}>
                Drop file to upload to /{currentSubpath || 'root'}
              </div>
            </div>
          )}
          {loading && (
            <div style={{ padding: '3.5rem 1rem', textAlign: 'center', color: 'var(--text-secondary)' }}>
              <span
                className="spinner"
                style={{
                  width: '1.5rem',
                  height: '1.5rem',
                  border: '2px solid var(--border)',
                  borderTopColor: 'var(--text-primary)',
                  borderRadius: '50%',
                  animation: 'spin 0.8s linear infinite',
                  display: 'inline-block',
                  marginBottom: '0.5rem',
                }}
              />
              <div>Loading contents…</div>
            </div>
          )}

          {!loading && items.length === 0 && (
            <div style={{ padding: '4rem 2rem', textAlign: 'center', margin: 'auto' }}>
              <div
                style={{
                  width: '3.5rem',
                  height: '3.5rem',
                  borderRadius: '50%',
                  backgroundColor: 'var(--surface-raised)',
                  border: '1px solid var(--border)',
                  display: 'inline-flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  marginBottom: '1rem',
                }}
              >
                <svg
                  style={{ width: '1.75rem', height: '1.75rem', color: 'var(--text-secondary)' }}
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                >
                  <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
                </svg>
              </div>
              <h3 style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>
                {searchQuery ? 'No matching files found' : 'This folder is empty'}
              </h3>
              <p
                style={{
                  fontSize: '0.8125rem',
                  color: 'var(--text-secondary)',
                  marginTop: '0.5rem',
                  maxWidth: '380px',
                  margin: '0.5rem auto 1.5rem auto',
                }}
              >
                {searchQuery
                  ? `No items match "${searchQuery}". Clear your search query to view all items.`
                  : 'Drop or save files into this directory in your local OS file manager to synchronize.'}
              </p>
              {onOpenFolder && (
                <button
                  type="button"
                  onClick={onOpenFolder}
                  className="btn btn-secondary"
                  style={{ padding: '0.5rem 1rem', fontSize: '0.8125rem' }}
                >
                  Open in Desktop File Manager
                </button>
              )}
            </div>
          )}

          {/* LIST VIEW */}
          {!loading && items.length > 0 && viewMode === 'list' && (
            <div style={{ flex: 1 }}>
              <table
                role="grid"
                aria-label="Files and directories"
                style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left', fontSize: '0.8125rem' }}
              >
                <thead>
                  <tr
                    style={{
                      borderBottom: '1px solid var(--border)',
                      backgroundColor: 'var(--surface-raised)',
                      color: 'var(--text-secondary)',
                      fontSize: '0.75rem',
                      textTransform: 'uppercase',
                      letterSpacing: '0.05em',
                      position: 'sticky',
                      top: 0,
                      zIndex: 1,
                    }}
                  >
                    <th
                      onClick={() => handleSort('name')}
                      style={{ padding: '0.625rem 1rem', fontWeight: 600, cursor: 'pointer', userSelect: 'none' }}
                    >
                      Name {sortBy === 'name' ? (sortDir === 'asc' ? '▲' : '▼') : ''}
                    </th>
                    <th
                      onClick={() => handleSort('size')}
                      style={{ padding: '0.625rem 0.875rem', fontWeight: 600, cursor: 'pointer', userSelect: 'none', width: '110px' }}
                    >
                      Size {sortBy === 'size' ? (sortDir === 'asc' ? '▲' : '▼') : ''}
                    </th>
                    <th style={{ padding: '0.625rem 0.875rem', fontWeight: 600, width: '170px' }}>
                      Status
                    </th>
                    <th
                      onClick={() => handleSort('mtime')}
                      style={{ padding: '0.625rem 0.875rem', fontWeight: 600, cursor: 'pointer', userSelect: 'none', width: '150px' }}
                    >
                      Modified {sortBy === 'mtime' ? (sortDir === 'asc' ? '▲' : '▼') : ''}
                    </th>
                    <th style={{ padding: '0.625rem 1rem', textAlign: 'right', fontWeight: 600, width: '100px' }}>
                      Actions
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((item, idx) => {
                    const isSelected = selectedPath === item.path;
                    const isFocused = focusedIndex === idx;
                    return (
                      <tr
                        key={item.path}
                        tabIndex={0}
                        onClick={() => {
                          setFocusedIndex(idx);
                          if (item.is_dir) {
                            navigateToDir(item.path);
                          } else {
                            setSelectedPath(item.path);
                          }
                        }}
                        className="file-row"
                        style={{
                          borderBottom: '1px solid var(--border-subtle)',
                          backgroundColor: isSelected
                            ? 'var(--surface-selected)'
                            : isFocused
                            ? 'var(--surface-hover)'
                            : 'transparent',
                          cursor: 'pointer',
                          transition: 'background-color 0.15s ease',
                          outline: isFocused ? '2px solid var(--text-primary)' : 'none',
                          outlineOffset: '-2px',
                        }}
                      >
                        {/* Name column */}
                        <td style={{ padding: '0.625rem 1rem' }}>
                          <div style={{ display: 'flex', alignItems: 'center', gap: '0.625rem' }}>
                            {item.is_dir ? (
                              <svg
                                style={{ width: '1.125rem', height: '1.125rem', color: 'var(--text-secondary)', flexShrink: 0 }}
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                strokeWidth="2"
                                strokeLinecap="round"
                                strokeLinejoin="round"
                              >
                                <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
                              </svg>
                            ) : (
                              <svg
                                style={{ width: '1.125rem', height: '1.125rem', color: 'var(--text-secondary)', flexShrink: 0 }}
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                strokeWidth="2"
                                strokeLinecap="round"
                                strokeLinejoin="round"
                              >
                                <path d="M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z" />
                                <polyline points="13 2 13 9 20 9" />
                              </svg>
                            )}
                            <div style={{ minWidth: 0 }}>
                              <div
                                title={item.path}
                                style={{
                                  fontWeight: 500,
                                  color: 'var(--text-primary)',
                                  textOverflow: 'ellipsis',
                                  overflow: 'hidden',
                                  whiteSpace: 'nowrap',
                                }}
                              >
                                {searchQuery ? item.path : item.name}
                                {item.executable && (
                                  <span
                                    style={{
                                      marginLeft: '0.375rem',
                                      fontSize: '0.625rem',
                                      padding: '0.0625rem 0.3125rem',
                                      borderRadius: 'var(--radius-sm)',
                                      backgroundColor: 'var(--surface-raised)',
                                      border: '1px solid var(--border)',
                                      color: 'var(--text-secondary)',
                                    }}
                                  >
                                    exec
                                  </span>
                                )}
                              </div>
                            </div>
                          </div>
                        </td>

                        {/* Size */}
                        <td style={{ padding: '0.625rem 0.875rem', color: 'var(--text-secondary)', whiteSpace: 'nowrap' }}>
                          {item.is_dir ? '—' : formatBytes(item.size)}
                        </td>

                        {/* Status (Invariants I19, I27: explicit distinct status badges) */}
                        <td style={{ padding: '0.625rem 0.875rem' }}>
                          <WorkingStateBadge
                            state={item.working_state}
                            contentState={item.content_state}
                            hasConflict={item.has_conflict}
                            blockReason={item.block_reason}
                          />
                        </td>

                        {/* Modified */}
                        <td style={{ padding: '0.625rem 0.875rem', color: 'var(--text-secondary)', fontSize: '0.75rem', whiteSpace: 'nowrap' }}>
                          {item.mtime_ns ? formatTimestamp(item.mtime_ns) : '—'}
                        </td>

                        {/* Actions */}
                        <td style={{ padding: '0.625rem 1rem', textAlign: 'right' }}>
                          <button
                            type="button"
                            onClick={(e) => {
                              e.stopPropagation();
                              setSelectedPath(item.path);
                            }}
                            className="btn btn-secondary btn-sm"
                            style={{
                              padding: '0.25rem 0.5rem',
                              fontSize: '0.75rem',
                            }}
                          >
                            Details
                          </button>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}

          {/* GRID VIEW */}
          {!loading && items.length > 0 && viewMode === 'grid' && (
            <div
              style={{
                display: 'grid',
                gridTemplateColumns: 'repeat(auto-fill, minmax(160px, 1fr))',
                gap: '0.875rem',
                padding: '1rem',
              }}
            >
              {items.map((item, idx) => {
                const isSelected = selectedPath === item.path;
                const isFocused = focusedIndex === idx;
                return (
                  <div
                    key={item.path}
                    tabIndex={0}
                    onClick={() => {
                      setFocusedIndex(idx);
                      if (item.is_dir) {
                        navigateToDir(item.path);
                      } else {
                        setSelectedPath(item.path);
                      }
                    }}
                    style={{
                      padding: '1rem 0.75rem',
                      backgroundColor: isSelected
                        ? 'var(--surface-selected)'
                        : isFocused
                        ? 'var(--surface-hover)'
                        : 'var(--surface-raised)',
                      border: '1px solid var(--border)',
                      borderRadius: 'var(--radius-sm)',
                      display: 'flex',
                      flexDirection: 'column',
                      alignItems: 'center',
                      textAlign: 'center',
                      gap: '0.5rem',
                      cursor: 'pointer',
                      outline: isFocused ? '2px solid var(--text-primary)' : 'none',
                      outlineOffset: '-2px',
                      transition: 'background-color 0.15s ease',
                    }}
                  >
                    {item.is_dir ? (
                      <svg
                        style={{ width: '2.5rem', height: '2.5rem', color: 'var(--text-secondary)' }}
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        strokeWidth="2"
                        strokeLinecap="round"
                        strokeLinejoin="round"
                      >
                        <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
                      </svg>
                    ) : (
                      <svg
                        style={{ width: '2.5rem', height: '2.5rem', color: 'var(--text-secondary)' }}
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        strokeWidth="2"
                        strokeLinecap="round"
                        strokeLinejoin="round"
                      >
                        <path d="M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z" />
                        <polyline points="13 2 13 9 20 9" />
                      </svg>
                    )}

                    <div
                      title={item.name}
                      style={{
                        width: '100%',
                        fontSize: '0.8125rem',
                        fontWeight: 500,
                        color: 'var(--text-primary)',
                        textOverflow: 'ellipsis',
                        overflow: 'hidden',
                        whiteSpace: 'nowrap',
                      }}
                    >
                      {item.name}
                    </div>

                    <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
                      {item.is_dir ? 'Folder' : formatBytes(item.size)}
                    </div>

                    <div style={{ marginTop: 'auto' }}>
                      <WorkingStateBadge
                        state={item.working_state}
                        contentState={item.content_state}
                        hasConflict={item.has_conflict}
                        blockReason={item.block_reason}
                      />
                    </div>
                  </div>
                );
              })}
            </div>
          )}

          {/* Bounded Pagination: Load More Items */}
          {!loading && nextCursor && (
            <div style={{ padding: '1rem', textAlign: 'center', borderTop: '1px solid var(--border)' }}>
              <button
                type="button"
                id="btn-load-more"
                onClick={() => loadData(true)}
                disabled={loadingMore}
                className="btn btn-secondary"
                style={{ padding: '0.5rem 1.25rem', fontSize: '0.8125rem' }}
              >
                {loadingMore
                  ? 'Loading more items…'
                  : `Load more items (showing ${items.length} of ${totalItems})`}
              </button>
            </div>
          )}
        </div>
      </div>

      {/* Right Column: Context-Preserving Details Drawer */}
      {selectedPath && (
        <FileDetailsDrawer
          folder={selectedFolder}
          path={selectedPath}
          onClose={() => setSelectedPath(null)}
          onOpenFolder={(dir) => {
            setSelectedPath(null);
            navigateToDir(dir);
          }}
          onRestorePreview={(historyItem) => handleOpenRestore(historyItem)}
          onMove={(p, isDir) => handleOpenMove(p, isDir)}
          onDelete={(p, isDir, sz) => handleOpenDelete(p, isDir, sz)}
        />
      )}

      {/* Restore Version Modal */}
      {restorePreview && (
        <RestoreModal
          folder={selectedFolder}
          preview={restorePreview}
          onClose={() => setRestorePreview(null)}
          onSuccess={handleRestoreSuccess}
        />
      )}

      {/* Create Folder Modal */}
      <CreateFolderModal
        isOpen={createFolderOpen}
        currentSubpath={currentSubpath}
        onClose={() => setCreateFolderOpen(false)}
        onCreate={handleCreateFolder}
      />

      {/* Move / Rename Modal */}
      {moveTarget && (
        <MoveFileModal
          isOpen={moveModalOpen}
          sourcePath={moveTarget.path}
          isDir={moveTarget.isDir}
          onClose={() => {
            setMoveModalOpen(false);
            setMoveTarget(null);
          }}
          onMove={handleMove}
        />
      )}

      {/* Delete Confirmation Modal */}
      {deleteTarget && (
        <DeleteFileModal
          isOpen={deleteModalOpen}
          itemPath={deleteTarget.path}
          isDir={deleteTarget.isDir}
          size={deleteTarget.size}
          onClose={() => {
            setDeleteModalOpen(false);
            setDeleteTarget(null);
          }}
          onDelete={handleDelete}
        />
      )}

      {/* Overwrite Upload Confirmation Modal */}
      <OverwriteUploadModal
        isOpen={overwriteModalOpen}
        file={overwriteFile}
        targetPath={overwriteTarget?.path || ''}
        existingSize={overwriteTarget?.existingSize}
        onClose={() => {
          setOverwriteModalOpen(false);
          setOverwriteFile(null);
          setOverwriteTarget(null);
        }}
        onConfirmOverwrite={async () => {
          if (overwriteFile && overwriteTarget) {
            await doUpload(overwriteFile, overwriteTarget.path, true);
          }
        }}
      />
    </div>
  );
};
