import React, { useState, useEffect } from 'react';
import { FolderRecord, ConflictsResult, ConflictSet } from '../types';
import { formatBytes, formatShortID, formatVersionID } from '../api';
import { ConflictResolveModal } from '../components/ConflictResolveModal';

interface ConflictsViewProps {
  folders: FolderRecord[];
  conflictsMap: Record<string, ConflictsResult>;
  onRefresh: () => void;
}

export const ConflictsView: React.FC<ConflictsViewProps> = ({
  folders,
  conflictsMap,
  onRefresh,
}) => {
  const [selectedFolder, setSelectedFolder] = useState<string>(folders[0]?.folder || folders[0]?.folder_id || '');
  const [activeConflict, setActiveConflict] = useState<ConflictSet | null>(null);
  const [successMsg, setSuccessMsg] = useState<string | null>(null);

  useEffect(() => {
    if (!selectedFolder && folders.length > 0) {
      setSelectedFolder(folders[0].folder || folders[0].folder_id || '');
    }
  }, [folders, selectedFolder]);

  const currentConflicts: ConflictsResult = conflictsMap[selectedFolder] || {
    content_conflicts: [],
    structural_conflicts: [],
  };
  const contentConflicts = currentConflicts.content_conflicts || [];
  const structuralConflicts = currentConflicts.structural_conflicts || [];

  const handleResolved = () => {
    setActiveConflict(null);
    setSuccessMsg('Conflict successfully resolved! Causal DAG extended.');
    onRefresh();
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '1.5rem' }}>
      {successMsg && (
        <div className="storage-banner" style={{ backgroundColor: 'var(--status-success-bg)', border: '1px solid rgba(52, 211, 153, 0.4)', color: '#d1fae5' }} role="status">
          <div>{successMsg}</div>
          <button type="button" className="btn btn-secondary btn-sm" onClick={() => setSuccessMsg(null)}>✕</button>
        </div>
      )}

      {/* FOLDER SELECTOR */}
      <div style={{ display: 'flex', gap: '1rem', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap' }}>
        <div style={{ minWidth: '240px' }}>
          <label htmlFor="conflict-folder-select" style={{ display: 'block', fontSize: '0.75rem', color: 'var(--text-muted)', marginBottom: '0.25rem' }}>
            Active Folder
          </label>
          <select
            id="conflict-folder-select"
            className="form-select code-font"
            value={selectedFolder}
            onChange={(e) => {
              setSelectedFolder(e.target.value);
              setActiveConflict(null);
            }}
          >
            {folders.map((f) => {
              const fid = f.folder || f.folder_id || '';
              return (
                <option key={fid} value={fid}>
                  {formatShortID(fid)} ({f.root_path || 'no root'})
                </option>
              );
            })}
          </select>
        </div>

        <button type="button" className="btn btn-secondary" onClick={onRefresh}>
          Refresh Conflicts
        </button>
      </div>

      {/* 1. CONTENT CONFLICTS SECTION */}
      <section className="panel-card" aria-labelledby="content-conflicts-heading">
        <div className="panel-header">
          <h2 id="content-conflicts-heading" className="panel-title">
            Causal Multi-Head Conflicts ({contentConflicts.length})
          </h2>
          <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
            Mutual concurrent edits or edit/delete collisions requiring operator review.
          </span>
        </div>

        {contentConflicts.length === 0 ? (
          <div className="empty-state">
            <p>No active content conflicts in this folder. All paths converged!</p>
          </div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '1.25rem' }}>
            {contentConflicts.map((conflict) => (
              <div
                key={conflict.path}
                style={{
                  backgroundColor: 'var(--bg-main)',
                  border: '1px solid var(--border-default)',
                  borderRadius: 'var(--radius-md)',
                  padding: '1.25rem',
                  display: 'flex',
                  flexDirection: 'column',
                  gap: '0.75rem',
                }}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', flexWrap: 'wrap', gap: '0.5rem' }}>
                  <div>
                    <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>Conflicted Path:</span>
                    <div className="code-font" style={{ fontSize: '1rem', fontWeight: 600 }}>
                      {conflict.path}
                    </div>
                  </div>

                  <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
                    <span className="badge badge-warning">{conflict.conflict_kind}</span>
                    <button
                      type="button"
                      className="btn btn-primary btn-sm"
                      onClick={() => setActiveConflict(conflict)}
                    >
                      Resolve Conflict
                    </button>
                  </div>
                </div>

                <div className="table-wrapper">
                  <table className="data-table">
                    <thead>
                      <tr>
                        <th>Head ID</th>
                        <th>Kind &amp; Size</th>
                        <th>Applied on Disk?</th>
                        <th>Content State</th>
                        <th>Head Digest</th>
                      </tr>
                    </thead>
                    <tbody>
                      {conflict.heads.map((head, idx) => (
                        <tr key={idx}>
                          <td className="code-font" style={{ fontWeight: 600 }}>
                            {formatVersionID(head.id)}
                          </td>
                          <td>
                            {head.kind === 3 ? (
                              <span className="badge badge-danger">TOMBSTONE (Deleted)</span>
                            ) : (
                              <span>
                                {formatBytes(head.manifest?.size ?? head.manifest?.Size ?? 0)}{' '}
                                {head.manifest?.executable || head.manifest?.Executable ? '(+x)' : ''}
                              </span>
                            )}
                          </td>
                          <td>
                            {head.applied ? (
                              <span className="badge badge-success">Applied in workspace</span>
                            ) : (
                              <span className="badge badge-neutral">Preserved in store</span>
                            )}
                          </td>
                          <td>
                            <span
                              className={`badge ${
                                head.content_state === 'ready'
                                  ? 'badge-success'
                                  : head.content_state === 'pending'
                                  ? 'badge-warning'
                                  : 'badge-danger'
                              }`}
                            >
                              {head.content_state}
                            </span>
                          </td>
                          <td className="code-font" style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                            {head.manifest?.digest || head.manifest?.Digest
                              ? formatShortID(head.manifest?.digest || head.manifest?.Digest || '')
                              : 'none'}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>

                <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                  <span>
                    Applied Version:{' '}
                    {conflict.applied ? formatVersionID(conflict.applied) : <em>None (Publication suppressed)</em>}
                  </span>
                  <span className="code-font">Head Token: {formatShortID(conflict.head_token)}</span>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      {/* 2. STRUCTURAL CONFLICTS SECTION */}
      <section className="panel-card" aria-labelledby="structural-conflicts-heading">
        <div className="panel-header">
          <h2 id="structural-conflicts-heading" className="panel-title">
            Structural Conflicts ({structuralConflicts.length})
          </h2>
          <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
            Parent deletion vs. child creation collisions. Child files are preserved.
          </span>
        </div>

        {structuralConflicts.length === 0 ? (
          <div className="empty-state">
            <p>No structural collisions detected.</p>
          </div>
        ) : (
          <div className="table-wrapper">
            <table className="data-table">
              <thead>
                <tr>
                  <th>Ancestor Path (Deleted)</th>
                  <th>Ancestor Version</th>
                  <th>Descendant Path (Preserved)</th>
                  <th>Descendant Version</th>
                </tr>
              </thead>
              <tbody>
                {structuralConflicts.map((sc, i) => (
                  <tr key={i}>
                    <td className="code-font">{sc.ancestor_path}</td>
                    <td className="code-font">{formatVersionID(sc.ancestor)}</td>
                    <td className="code-font" style={{ fontWeight: 600 }}>{sc.descendant_path}</td>
                    <td className="code-font">{formatVersionID(sc.descendant)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {/* CONFLICT RESOLUTION MODAL */}
      {activeConflict && (
        <ConflictResolveModal
          folder={selectedFolder}
          conflict={activeConflict}
          onClose={() => setActiveConflict(null)}
          onResolved={handleResolved}
        />
      )}
    </div>
  );
};
