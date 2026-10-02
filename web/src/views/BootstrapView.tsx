import React, { useState, useEffect } from 'react';
import { api, APIError } from '../api';

interface BootstrapViewProps {
  onAuthenticated: () => void;
}

export const BootstrapView: React.FC<BootstrapViewProps> = ({ onAuthenticated }) => {
  const [token, setToken] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Auto-detect and exchange bootstrap token from URL fragment (#bootstrap=<token>)
  useEffect(() => {
    const hash = window.location.hash;
    if (hash && hash.includes('bootstrap=')) {
      const match = hash.match(/bootstrap=([a-fA-F0-9]+)/);
      if (match && match[1]) {
        const foundToken = match[1];
        setLoading(true);
        setError(null);
        api.bootstrap(foundToken)
          .then(() => {
            // Strip token from browser URL and history immediately (Invariant I21)
            window.history.replaceState(null, '', window.location.pathname + window.location.search);
            onAuthenticated();
          })
          .catch((err: any) => {
            setLoading(false);
            if (err instanceof APIError) {
              setError(`${err.code}: ${err.message}. ${err.action || 'Ensure token is valid and not expired.'}`);
            } else {
              setError(err.message || 'Bootstrap token exchange failed');
            }
          });
      }
    }
  }, [onAuthenticated]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token.trim()) return;

    setLoading(true);
    setError(null);

    try {
      await api.bootstrap(token.trim());
      // Clean any leftover URL fragment
      if (window.location.hash.includes('bootstrap=')) {
        window.history.replaceState(null, '', window.location.pathname + window.location.search);
      }
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
    <div className="bootstrap-container" style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '1.5rem', backgroundColor: 'var(--canvas)' }}>
      <div className="panel-card" style={{ maxWidth: '440px', width: '100%', padding: '2.5rem', backgroundColor: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-lg)', boxShadow: '0 8px 24px rgba(0, 0, 0, 0.5)' }}>
        <div style={{ textAlign: 'center', marginBottom: '2rem' }}>
          <div style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', width: '3.5rem', height: '3.5rem', borderRadius: '50%', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', marginBottom: '1rem' }}>
            <svg
              className="brand-icon"
              style={{ width: '2rem', height: '2rem', color: 'var(--text-primary)' }}
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
              aria-hidden="true"
            >
              <circle cx="12" cy="12" r="10" />
              <path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20" />
              <path d="M2 12h20" />
            </svg>
          </div>
          <h1 style={{ fontSize: '1.5rem', fontWeight: 600, color: 'var(--text-primary)', letterSpacing: '-0.02em', margin: 0 }}>
            Orbit
          </h1>
          <p style={{ color: 'var(--text-secondary)', fontSize: '0.875rem', marginTop: '0.5rem', lineHeight: 1.4 }}>
            Local daemon authentication required. Enter your single-use bootstrap token to establish an authorized UI session.
          </p>
        </div>

        {error && (
          <div className="alert alert-danger" style={{ marginBottom: '1.5rem', padding: '0.875rem', borderRadius: 'var(--radius-sm)', backgroundColor: 'var(--status-danger-bg)', border: '1px solid var(--status-danger)', color: 'var(--status-danger)', fontSize: '0.875rem' }} role="alert">
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: '0.5rem' }}>
              <svg style={{ width: '1.25rem', height: '1.25rem', flexShrink: 0, marginTop: '0.125rem' }} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <circle cx="12" cy="12" r="10" />
                <line x1="12" y1="8" x2="12" y2="12" />
                <line x1="12" y1="16" x2="12.01" y2="16" />
              </svg>
              <span>{error}</span>
            </div>
          </div>
        )}

        <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: '1.25rem' }}>
          <div>
            <label htmlFor="bootstrap-token" style={{ display: 'block', fontSize: '0.875rem', fontWeight: 500, color: 'var(--text-primary)', marginBottom: '0.5rem' }}>
              One-Time Bootstrap Token
            </label>
            <input
              id="bootstrap-token"
              type="password"
              className="form-input code-font"
              placeholder="Paste 64-character token"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              required
              autoFocus
              style={{ width: '100%', padding: '0.625rem 0.875rem', backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 'var(--radius-sm)', color: 'var(--text-primary)', fontSize: '0.875rem' }}
            />
          </div>

          <div style={{ backgroundColor: 'var(--surface-raised)', border: '1px solid var(--border)', padding: '0.75rem 1rem', borderRadius: 'var(--radius-sm)', fontSize: '0.8125rem', color: 'var(--text-secondary)' }}>
            <div>To generate a new bootstrap token in your terminal:</div>
            <code className="code-font" style={{ display: 'block', color: 'var(--text-primary)', marginTop: '0.375rem', userSelect: 'all' }}>
              orbit status --token
            </code>
          </div>

          <button
            type="submit"
            className="btn btn-primary"
            style={{ width: '100%', padding: '0.75rem', backgroundColor: 'var(--text-primary)', color: 'var(--canvas)', border: 'none', borderRadius: 'var(--radius-sm)', fontWeight: 600, fontSize: '0.875rem', cursor: 'pointer', display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '0.5rem', transition: 'opacity 0.15s ease' }}
            disabled={loading || !token.trim()}
          >
            {loading ? (
              <>
                <span className="spinner" style={{ width: '1rem', height: '1rem', border: '2px solid rgba(0,0,0,0.3)', borderTopColor: '#000', borderRadius: '50%', animation: 'spin 0.8s linear infinite' }} />
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
