import React, { useState } from 'react';
import { api, APIError } from '../api';

interface BootstrapViewProps {
  onAuthenticated: () => void;
}

export const BootstrapView: React.FC<BootstrapViewProps> = ({ onAuthenticated }) => {
  const [token, setToken] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token.trim()) return;

    setLoading(true);
    setError(null);

    try {
      await api.bootstrap(token.trim());
      onAuthenticated();
    } catch (err: any) {
      if (err instanceof APIError) {
        setError(`${err.code}: ${err.message}. ${err.action || 'Ensure token is valid and not expired.'}`);
      } else {
        setError(err.message || 'Authorization failed');
      }
    } finally {
      setLoading(false);
    }
  };

  return (
    <div style={{ minHeight: '80vh', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <div className="panel-card" style={{ maxWidth: '480px', width: '100%', padding: '2rem' }}>
        <div style={{ textAlign: 'center', marginBottom: '1.5rem' }}>
          <svg
            className="brand-icon"
            style={{ width: '3rem', height: '3rem', margin: '0 auto 0.75rem', color: 'var(--accent-primary)' }}
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
          <h2 style={{ fontSize: '1.5rem', fontWeight: 700 }}>Operator Authentication</h2>
          <p style={{ color: 'var(--text-muted)', fontSize: '0.875rem', marginTop: '0.5rem' }}>
            Loopback control authorization required.
          </p>
        </div>

        {error && (
          <div className="storage-banner danger" style={{ marginBottom: '1.25rem' }} role="alert">
            <div>{error}</div>
          </div>
        )}

        <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: '1.25rem' }}>
          <div>
            <label htmlFor="bootstrap-token" style={{ display: 'block', fontSize: '0.875rem', fontWeight: 600, marginBottom: '0.5rem' }}>
              One-Time Bootstrap Token
            </label>
            <input
              id="bootstrap-token"
              type="password"
              className="form-input code-font"
              placeholder="Paste 64-character hex bootstrap token"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              required
              autoFocus
            />
          </div>

          <div style={{ backgroundColor: 'var(--bg-main)', padding: '0.875rem', borderRadius: 'var(--radius-sm)', fontSize: '0.8125rem', color: 'var(--text-muted)' }}>
            <div>To generate a new token, run in your terminal:</div>
            <code className="code-font" style={{ display: 'block', color: 'var(--accent-primary)', marginTop: '0.25rem' }}>
              filesync control bootstrap-token
            </code>
          </div>

          <button
            type="submit"
            className="btn btn-primary"
            style={{ width: '100%', padding: '0.75rem' }}
            disabled={loading || !token.trim()}
          >
            {loading ? (
              <>
                <span className="spinner" style={{ width: '1rem', height: '1rem' }} />
                Authenticating…
              </>
            ) : (
              'Authenticate Session'
            )}
          </button>
        </form>
      </div>
    </div>
  );
};
