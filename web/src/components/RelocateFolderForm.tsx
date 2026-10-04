import React, { useRef, useState } from 'react';
import { api } from '../api';

export function RelocateFolderForm({ folderId, rootPath, onSuccess }: {
  folderId: string; rootPath: string; onSuccess: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [path, setPath] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const input = useRef<HTMLInputElement>(null);
  const button = useRef<HTMLButtonElement>(null);
  const id = `relocation-${folderId}`;
  const close = () => { setOpen(false); button.current?.focus(); };
  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setError(''); setMessage('');
    if (!path.trim().startsWith('/')) {
      setError('Enter a full folder path beginning with /.'); input.current?.focus(); return;
    }
    setBusy(true);
    try {
      const result = await api.relocateFolder(folderId, rootPath, path.trim());
      setMessage(result.source_retained
        ? `Location changed to ${result.path}. Original safety copy kept at ${result.source_path}; review it before removing it.`
        : `Location changed to ${result.path}.`);
      close(); onSuccess();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not change location.');
      input.current?.focus();
    } finally { setBusy(false); }
  };
  return <div>
    <button ref={button} type="button" id={`btn-change-location-${folderId}`} className="btn btn-secondary btn-sm"
      aria-expanded={open} aria-controls={id} disabled={busy}
      onClick={() => { setOpen(!open); setError(''); setMessage(''); }}>
      Change location
    </button>
    {open && <form id={id} onSubmit={submit} style={{ marginTop: '0.75rem', display: 'grid', gap: '0.75rem' }}>
      <p id={`${id}-help`} style={{ margin: 0, color: 'var(--text-secondary)' }}>
        Orbit moves this device’s folder and keeps its history and shared devices.
        Choose an unused folder name inside an existing directory. Across drives, the original stays as a safety copy.
        Close editors and other programs using this folder before continuing.
      </p>
      <label htmlFor={`${id}-path`}>New local folder location</label>
      <input ref={input} autoFocus id={`${id}-path`} value={path} disabled={busy}
        onChange={e => setPath(e.target.value)} placeholder="/home/you/Documents/Orbit"
        aria-describedby={`${id}-help${error ? ` ${id}-error` : ''}`} aria-invalid={!!error} required />
      {error && <p role="alert" id={`${id}-error`} style={{ margin: 0 }}>{error}</p>}
      <div style={{ display: 'flex', gap: '0.5rem' }}>
        <button className="btn btn-primary btn-sm" type="submit" disabled={busy || !path.trim()}>
          {busy ? 'Changing location…' : 'Move folder'}
        </button>
        <button className="btn btn-secondary btn-sm" type="button" disabled={busy} onClick={close}>Cancel</button>
      </div>
    </form>}
    {message && <p role="status">{message}</p>}
  </div>;
}
