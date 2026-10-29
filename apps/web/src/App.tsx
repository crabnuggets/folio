import { useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { ApiClient } from './lib/api';
import { tokenExpiry } from './lib/session';
import { Form, text } from './components/Form';
import { MonthlySheet } from './MonthlySheet';

const api = new ApiClient();
export function App() {
  const queryClient = useQueryClient();
  const [signedIn, setSignedIn] = useState(false);
  const [started, setStarted] = useState(false);
  const [expiresAt, setExpiresAt] = useState<number | null>(null);
  const [message, setMessage] = useState('');
  const hasEdits = useRef(false);
  const expire = () => {
    api.setToken(null); void queryClient.cancelQueries(); queryClient.clear(); setSignedIn(false);
    setMessage('Your session expired. Sign in with the same owner token to continue. Your unsaved edits and retry commands remain in this tab.');
  };
  api.onUnauthorized = expire;
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
      <Form submit="Sign in" fields={[{ name: 'token', label: 'Access token', type: 'password', required: true, hint: 'Your configured owner bearer token stays in memory for this tab only.' }]} onSubmit={async values => {
        const token = text(values, 'token');
        if (!token || /\s/.test(token)) throw new Error('Enter a bearer access token without spaces.');
        api.setToken(token);
        try {
          await api.get<'Settings'>('/settings');
          setExpiresAt(tokenExpiry(token)); setMessage(''); setStarted(true); setSignedIn(true);
        } catch (error) { api.setToken(null); throw error; }
      }} />
      <p className="fine-print">Manual bearer sign-in. No token is saved to browser storage.</p>
    </div></main>}
    {started && <div hidden={!signedIn}><MonthlySheet api={api} signedIn={signedIn} onDirty={value => { hasEdits.current = value; }} onLogout={() => {
      api.setToken(null); void queryClient.cancelQueries(); queryClient.clear(); setStarted(false); setSignedIn(false); hasEdits.current = false;
      setMessage('Signed out. Financial data was cleared from this tab.');
    }} /></div>}
  </>;
}
