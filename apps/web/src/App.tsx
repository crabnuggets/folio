import { useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { Session, GoTrueClient } from '@supabase/auth-js';
import { ApiClient, errorMessage } from './lib/api';
import { tokenExpiry } from './lib/session';
import { authStorageKey, loadBrowserAuth, signInWithGoogle, type BrowserAuth } from './lib/googleAuth';
import { Form, text } from './components/Form';
import { MonthlySheet } from './MonthlySheet';

const api = new ApiClient();
export function App() {
  return window.location.pathname === '/auth/callback' ? <GoogleCallback /> : <SignedInApp />;
}

function GoogleCallback() {
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const code = params.get('code');
    window.history.replaceState(null, '', '/auth/callback');
    if (window.opener) {
      window.opener.postMessage({ type: 'folio-google-callback', code }, window.location.origin);
      window.close();
    }
  }, []);
  return <main className="login-page"><p>Return to your Folio tab to finish signing in.</p></main>;
}

function SignedInApp() {
  const queryClient = useQueryClient();
  const [signedIn, setSignedIn] = useState(false);
  const [started, setStarted] = useState(false);
  const [expiresAt, setExpiresAt] = useState<number | null>(null);
  const [message, setMessage] = useState('');
  const [config, setConfig] = useState<BrowserAuth | null>(null);
  const [client, setClient] = useState<GoTrueClient | null>(null);
  const [busy, setBusy] = useState(false);
  const hasEdits = useRef(false);
  const authorizedSubject = useRef<string | null>(null);
  const validation = useRef(0);
  const acceptSessions = useRef(true);
  const expire = (showMessage = true) => {
    validation.current++; authorizedSubject.current = null;
    api.setToken(null); void queryClient.cancelQueries(); queryClient.clear(); setSignedIn(false);
    if (showMessage) setMessage('Your session expired. Sign in again to continue. Your unsaved edits and retry commands remain in this tab.');
  };
  api.onUnauthorized = expire;
  useEffect(() => {
    let disposed = false;
    let unsubscribe: (() => void) | undefined;
    const accept = async (session: Session | null) => {
      if (disposed || !acceptSessions.current) return;
      if (!session) {
        expire(authorizedSubject.current !== null);
        return;
      }
      if (authorizedSubject.current === session.user.id) {
        api.refreshToken(session.access_token); setExpiresAt(session.expires_at ? session.expires_at * 1000 : null);
        return;
      }
      authorizedSubject.current = null; setSignedIn(false);
      const attempt = ++validation.current;
      api.setToken(session.access_token);
      try {
        await api.get<'Settings'>('/settings');
        if (disposed || attempt !== validation.current) return;
        authorizedSubject.current = session.user.id;
        setExpiresAt(session.expires_at ? session.expires_at * 1000 : null);
        setMessage(''); setStarted(true); setSignedIn(true);
      } catch (error) {
        if (disposed || attempt !== validation.current) return;
        api.setToken(null); setSignedIn(false); setMessage(errorMessage(error));
      }
    };
    void loadBrowserAuth().then(({ config, client }) => {
      if (disposed) return;
      setConfig(config); setClient(client);
      if (!client) return;
      const { data } = client.onAuthStateChange((_event, session) => {
        // Defer API work until the auth client's event callback releases its lock.
        setTimeout(() => void accept(session), 0);
      });
      unsubscribe = () => data.subscription.unsubscribe();
      void client.getSession().then(({ error }) => {
        if (!disposed && error) setMessage('Your session could not be restored. Sign in again.');
      });
    }).catch(error => { if (!disposed) setMessage(errorMessage(error)); });
    return () => { disposed = true; unsubscribe?.(); validation.current++; };
  }, [queryClient]);
  useEffect(() => {
    if (!signedIn || expiresAt == null) return;
    const timer = setTimeout(expire, Math.max(0, Math.min(expiresAt - Date.now(), 2147483647)));
    return () => clearTimeout(timer);
  }, [signedIn, expiresAt]);
  useEffect(() => {
    const warn = (event: BeforeUnloadEvent) => { if (hasEdits.current) { event.preventDefault(); event.returnValue = ''; } };
    window.addEventListener('beforeunload', warn);
    return () => window.removeEventListener('beforeunload', warn);
  }, []);
  return <>
    {!signedIn && <main className="login-page"><div className="login-card">
      <div className="brand"><span className="brand-mark" aria-hidden="true">f</span>Folio</div>
      <p className="eyebrow">One month at a time</p><h1>Your monthly sheet.</h1>
      <p>Enter what comes in and goes out. See what remains.</p>
      {message && <p className="notice" role="status">{message}</p>}
      {client && <button className="google-sign-in" disabled={busy} onClick={() => {
        acceptSessions.current = true; client.startAutoRefresh();
        setBusy(true); setMessage('');
        void signInWithGoogle(client).catch(error => setMessage(errorMessage(error))).finally(() => setBusy(false));
      }}><GoogleMark />{busy ? 'Signing in...' : 'Sign in with Google'}</button>}
      {config?.development_key === true && <>
        <Form submit="Sign in" fields={[{ name: 'token', label: 'Access token', type: 'password', required: true, hint: 'Your development sign-in key stays in memory for this tab only.' }]} onSubmit={async values => {
          const token = text(values, 'token');
          if (!token || /\s/.test(token)) throw new Error('Enter a sign-in key without spaces.');
          api.setToken(token);
          try {
            await api.get<'Settings'>('/settings');
            setExpiresAt(tokenExpiry(token)); setMessage(''); setStarted(true); setSignedIn(true);
          } catch (error) { api.setToken(null); throw error; }
        }} />
        <p className="fine-print">Development sign-in. No key is saved to browser storage.</p>
      </>}
      {!config && !message && <p role="status">Loading sign-in...</p>}
      {config && !client && !config.development_key && <p role="status">Sign-in is not configured. Contact the application owner.</p>}
    </div></main>}
    {started && <div hidden={!signedIn}><MonthlySheet api={api} signedIn={signedIn} onDirty={value => { hasEdits.current = value; }} onLogout={() => {
      acceptSessions.current = false;
      validation.current++; authorizedSubject.current = null;
      api.setToken(null); void queryClient.cancelQueries(); queryClient.clear(); setStarted(false); setSignedIn(false); hasEdits.current = false;
      setMessage('Signed out. Financial data was cleared from this tab.');
      if (client) {
        setBusy(true);
        void client.signOut({ scope: 'local' }).then(({ error }) => {
          if (error) {
            client.stopAutoRefresh(); sessionStorage.removeItem(authStorageKey);
            setMessage('Signed out of this tab. The server could not revoke the session.');
          }
        }).finally(() => setBusy(false));
      }
    }} /></div>}
  </>;
}

function GoogleMark() {
  return <svg aria-hidden="true" width="20" height="20" viewBox="0 0 48 48">
    <path fill="#4285F4" d="M43.6 24.5c0-1.5-.1-2.9-.4-4.3H24v8.1h11a9.4 9.4 0 0 1-4.1 6.2v5.2h6.7c3.9-3.6 6-8.8 6-15.2Z" />
    <path fill="#34A853" d="M24 44c5.4 0 9.9-1.8 13.2-4.8l-6.7-5.2c-1.8 1.2-4.1 1.9-6.5 1.9-5.2 0-9.6-3.5-11.2-8.2H5.9v5.4A20 20 0 0 0 24 44Z" />
    <path fill="#FBBC05" d="M12.8 27.7a12 12 0 0 1 0-7.4v-5.4H5.9a20 20 0 0 0 0 18.2l6.9-5.4Z" />
    <path fill="#EA4335" d="M24 12.1c2.9 0 5.4 1 7.4 2.9l5.6-5.6A19.8 19.8 0 0 0 24 4 20 20 0 0 0 5.9 14.9l6.9 5.4c1.6-4.7 6-8.2 11.2-8.2Z" />
  </svg>;
}
