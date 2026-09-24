import React from 'react';
import { StorageUsage } from '../types';
import { formatBytes } from '../api';

interface StorageBannerProps {
  usage: StorageUsage | null;
  onRunGC?: () => void;
}

export const StorageBanner: React.FC<StorageBannerProps> = ({ usage, onRunGC }) => {
  if (!usage) return null;

  const walCapBytes = 256 * 1024 * 1024; // 256 MiB
  const isWALWarning = usage.wal_bytes >= walCapBytes;
  const isQuarantineWarning = usage.quarantine_bytes > 0;

  if (!isWALWarning && !isQuarantineWarning) {
    return null;
  }

  return (
    <div
      className={`storage-banner ${isWALWarning ? 'danger' : 'warning'}`}
      role="alert"
      aria-live="polite"
    >
      <div>
        <strong>Storage Warning: </strong>
        {isWALWarning && (
          <span>
            SQLite WAL size ({formatBytes(usage.wal_bytes)}) has reached the 256 MiB soft cap. Checkpoints or GC recommended.
          </span>
        )}
        {isQuarantineWarning && (
          <span>
            Quarantined chunks detected ({formatBytes(usage.quarantine_bytes)}). Bitrot diagnosis or peer repair needed.
          </span>
        )}
      </div>
      {onRunGC && (
        <button type="button" className="btn btn-secondary btn-sm" onClick={onRunGC}>
          Run GC
        </button>
      )}
    </div>
  );
};
