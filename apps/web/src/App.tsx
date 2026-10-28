import { useEffect, useRef, useState, type ReactNode } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiClient, ApiError, errorMessage, isUncertain } from './lib/api';
import { tokenExpiry } from './lib/session';
import { ReviewContext, amountParser, type ReviewData, type Task } from './context';
import { Form, text } from './components/Form';
import { Dialog } from './components/Dialog';
import { Overview, Records, Statements, PlanReview, Accounts, Claims, Drafts } from './screens';
import { EntryForm } from './forms';

const api = new ApiClient();
const destinations = ['Overview', 'Records', 'Statements', 'Plan & review', 'Claims', 'Drafts', 'Accounts & settings'] as const;
type Destination = typeof destinations[number];
type Pending = { label: string; task: Task; error?: unknown; running: boolean; uncertain?: boolean };

function currentMonth() {
  const parts = new Intl.DateTimeFormat('en', { timeZone: 'Asia/Singapore', year: 'numeric', month: '2-digit' }).formatToParts(new Date());
  return `${parts.find(p => p.type === 'year')!.value}-${parts.find(p => p.type === 'month')!.value}`;
}
function hashDestination(): Destination {
  try { return destinations.find(value => value === decodeURIComponent(location.hash.slice(1))) ?? 'Overview'; }
  catch { return 'Overview'; }
}

export function App() {
  const queryClient = useQueryClient();
  const [signedIn, setSignedIn] = useState(false);
  const [expiresAt, setExpiresAt] = useState<number | null>(null);
  const [sessionMessage, setSessionMessage] = useState('');
  const [pending, setPending] = useState<Pending | null>(null);
  const pendingRef = useRef<Pending | null>(null);
  const [month, setMonth] = useState(currentMonth);
  const [destination, setDestination] = useState<Destination>(hashDestination);
  useEffect(() => {
    const change = () => setDestination(hashDestination());
    window.addEventListener('hashchange', change);
    return () => window.removeEventListener('hashchange', change);
  }, []);
  const expire = () => {
    api.setToken(null);
    void queryClient.cancelQueries();
    queryClient.clear();
    setSignedIn(false);
    setSessionMessage('Your session expired. Sign in again to continue. Any pending action retains its retry identity while this page stays open.');
  };
  api.onUnauthorized = expire;
  useEffect(() => {
    if (!signedIn || expiresAt == null) return;
    const timer = setTimeout(expire, Math.max(0, Math.min(expiresAt - Date.now(), 2147483647)));
    return () => clearTimeout(timer);
  }, [signedIn, expiresAt]);

  useEffect(() => {
    const warn = (event: BeforeUnloadEvent) => { if (pendingRef.current) { event.preventDefault(); event.returnValue = ''; } };
    window.addEventListener('beforeunload', warn);
    return () => window.removeEventListener('beforeunload', warn);
  }, []);
  const updatePending = (value: Pending | null) => { pendingRef.current = value; setPending(value); };

  if (!signedIn) return <main className="login-page"><div className="login-card">
    <a href="#Overview" className="brand"><span className="brand-mark" aria-hidden="true">f</span>Folio</a>
    <p className="eyebrow">A little clarity, every month</p><h1>Your monthly review, in one place.</h1>
    <p>Review what came in, what went out, and what still needs your attention.</p>
    {sessionMessage && <p className="notice" role="status">{sessionMessage}</p>}
    {pending && <p className="notice warning">A pending action is retained. Sign in with the same owner’s access token to recover its result. Keep this page open.</p>}
    <Form submit="Sign in" fields={[{ name: 'token', label: 'Access token', type: 'password', required: true, hint: 'Use your configured owner bearer token. It stays in memory for this tab only.' }]} onSubmit={async values => {
      const token = text(values, 'token');
      if (!token || /\s/.test(token)) throw new Error('Enter a bearer access token without spaces.');
      api.setToken(token);
      try {
        const settings = await api.get<'Settings'>('/settings');
        queryClient.setQueryData(['settings'], settings);
        setExpiresAt(tokenExpiry(token)); setSessionMessage(''); setSignedIn(true);
      } catch (error) { api.setToken(null); throw error; }
    }} />
    <p className="fine-print">Local/manual bearer sign-in · No token is saved to browser storage. Hosted sign-in is a later milestone.</p>
  </div></main>;

  return <ReviewSession api={api} month={month} onMonth={setMonth} destination={destination} pending={pending} hasPending={() => pendingRef.current !== null} updatePending={updatePending} onLogout={() => {
    api.setToken(null); void queryClient.cancelQueries(); queryClient.clear(); setSignedIn(false); setSessionMessage('Signed out. Your financial data was cleared from this tab.');
  }} />;
}

function ReviewSession({ api, month, onMonth, destination, pending, hasPending, updatePending, onLogout }: {
  api: ApiClient; month: string; onMonth(value: string): void; destination: Destination;
  pending: Pending | null; hasPending(): boolean; updatePending(value: Pending | null): void; onLogout(): void;
}) {
  const queryClient = useQueryClient();
  const [modal, setModal] = useState<{ title: string; content: ReactNode } | null>(null);
  const [notice, setNotice] = useState('');
  const [online, setOnline] = useState(navigator.onLine);
  const busy = useRef(false);
  const settings = useQuery({ queryKey: ['settings'], queryFn: () => api.get<'Settings'>('/settings') });
  const accounts = useQuery({ queryKey: ['accounts'], queryFn: () => api.list<'Account'>('/accounts') });
  const entries = useQuery({ queryKey: ['entries'], queryFn: () => api.list<'Entry'>('/entries') });
  const statements = useQuery({ queryKey: ['statements'], queryFn: () => api.list<'Statement'>('/statements') });
  const claims = useQuery({ queryKey: ['claims'], queryFn: () => api.list<'Claim'>('/claims') });
  const drafts = useQuery({ queryKey: ['drafts'], queryFn: () => api.list<'Draft'>('/drafts') });
  const snapshots = useQuery({ queryKey: ['snapshots'], queryFn: () => api.list<'Snapshot'>('/balance-snapshots') });
  const audit = useQuery({ queryKey: ['audit'], queryFn: () => api.list<'Audit'>('/audit-events') });
  const plan = useQuery({ queryKey: ['plan', month], queryFn: () => api.get<'Plan'>(`/months/${month}/plan`) });
  const monthState = useQuery({ queryKey: ['month', month], queryFn: () => api.get<'Month'>(`/months/${month}`) });
  const summary = useQuery({ queryKey: ['summary', month], queryFn: () => api.get<'Summary'>(`/months/${month}/summary`) });
  const queries = [settings, accounts, entries, statements, claims, drafts, snapshots, audit, plan, monthState, summary];
  const error = queries.find(query => query.isError)?.error;
  const ready = queries.every(query => query.data !== undefined);
  const loading = queries.some(query => query.isFetching);
  useEffect(() => {
    const update = () => setOnline(navigator.onLine);
    window.addEventListener('online', update); window.addEventListener('offline', update);
    return () => { window.removeEventListener('online', update); window.removeEventListener('offline', update); };
  }, []);
  useEffect(() => { setNotice(''); }, [month, destination]);

  const refresh = async () => { await queryClient.invalidateQueries(); };
  const execute = async (item: Pending) => {
    if (busy.current) return;
    busy.current = true;
    updatePending({ ...item, running: true, error: undefined });
    try {
      await item.task.run();
      await refresh();
      updatePending(null); setModal(null); setNotice(`${item.label} saved.`);
    } catch (failure) { updatePending({ ...item, running: false, error: failure, uncertain: item.uncertain || isUncertain(failure) }); throw failure; }
    finally { busy.current = false; }
  };
  const perform = async (label: string, task: Task) => {
    if (hasPending() || busy.current) throw new Error('Resolve the pending action first.');
    await execute({ label, task, running: true });
  };
  const data = ready ? {
    settings: settings.data!, accounts: accounts.data!, entries: entries.data!, statements: statements.data!, claims: claims.data!,
    drafts: drafts.data!, snapshots: snapshots.data!, audit: audit.data!, plan: plan.data!, month: monthState.data!, summary: summary.data!,
  } satisfies ReviewData : undefined;
  const closed = data?.month.state === 'closed';
  const noWrite = Boolean(pending) || !online || Boolean(error) || loading;
  const recovery = pending && <div className="notice warning pending" role="alert"><strong>{pending.label}: {pending.running ? 'saving…' : 'needs attention'}</strong>
    <p>{pending.error ? errorMessage(pending.error) : 'Keep this page open while the server confirms this action.'}</p>
    {pending.task.paymentId && <p>Payment recorded: <code>{pending.task.paymentId}</code>. Retry links this payment; it does not create another one.</p>}
    {!pending.running && <div className="actions"><button className="primary" onClick={() => { void execute(pending).catch(() => {}); }}>Retry same action</button>
      {!pending.uncertain && pending.error instanceof ApiError && [400, 422].includes(pending.error.status) && !pending.task.paymentId && modal && <button onClick={() => updatePending(null)}>Edit form</button>}
      {!pending.uncertain && !isUncertain(pending.error) && <button onClick={() => { updatePending(null); setModal(null); void refresh(); setNotice('Action stopped. Review the latest record before starting a revised action. Any recorded payment remains in Records and can be linked as an existing payment.'); }}>Return to review</button>}</div>}
    {pending.task.body && pending.error != null && <details><summary>Retained submitted values</summary><pre>{pending.task.body}</pre></details>}
    <small>Retry identity is retained in memory. Reloading or closing this page loses it; reconcile against existing records before starting again.</small>
  </div>;
  const content = data ? <ReviewContext.Provider value={{ api, data, month, blocked: noWrite,
    open: (title, content) => setModal({ title, content }), close: () => setModal(null), perform,
    money: amountParser(data.settings.currency_exponent),
  }}>
    <div className="page-heading"><div><p className="eyebrow">Your monthly check-in</p><h1>{destination === 'Overview' ? 'A clearer view of your month.' : destination}</h1></div>
      <div className="heading-actions"><span className={`status ${closed ? '' : 'review'}`}>{closed ? 'Month closed' : data.summary.is_provisional ? 'Needs review' : 'Ready to review'}</span>
        <button className="primary" disabled={noWrite || closed} onClick={() => setModal({ title: 'Add entry', content: <EntryForm /> })}>Add entry</button></div></div>
    {closed && <div className="notice">Month closed · Revision {data.month.revision}. Reopen it in Plan & review before making financial changes.</div>}
    {destination === 'Overview' ? <Overview /> : destination === 'Records' ? <Records /> : destination === 'Statements' ? <Statements /> : destination === 'Plan & review' ? <PlanReview /> : destination === 'Accounts & settings' ? <Accounts /> : destination === 'Claims' ? <Claims /> : <Drafts />}
    {modal && <Dialog title={modal.title} locked={Boolean(pending)} onClose={() => setModal(null)}>{recovery}<fieldset disabled={Boolean(pending)}>{modal.content}</fieldset></Dialog>}
  </ReviewContext.Provider> : null;

  return <div className="app-shell"><a className="skip-link" href="#main">Skip to content</a>
    <header className="topbar"><a href="#Overview" className="brand"><span className="brand-mark" aria-hidden="true">f</span>Folio</a>
      <div className="month-selector"><label htmlFor="reporting-month">Review month</label><input id="reporting-month" type="month" min="1900-01" max="9999-12" value={month} disabled={Boolean(pending) || Boolean(modal)} onChange={e => { if (/^(19\d{2}|[2-9]\d{3})-(0[1-9]|1[0-2])$/.test(e.target.value)) onMonth(e.target.value); }} /></div>
      <div className="top-actions"><button onClick={() => void refresh()} disabled={loading || Boolean(pending)}>Refresh</button><button disabled={Boolean(pending)} title={pending ? 'Resolve the pending action before signing out.' : undefined} onClick={onLogout}>Sign out</button></div></header>
    <div className="workspace"><nav aria-label="Main navigation">{destinations.map((item, index) => <a key={item} href={`#${encodeURIComponent(item)}`} aria-current={destination === item ? 'page' : undefined}><span className="nav-glyph" aria-hidden="true">{['◫', '≡', '▤', '✓', '⇄', '▧', '⚙'][index]}</span>{item}{item === 'Drafts' && (data?.drafts.filter(d => d.state === 'pending').length ?? 0) > 0 && <span className="count">{data?.drafts.filter(d => d.state === 'pending').length}</span>}</a>)}<p className="nav-foot">One month at a time.<br />Every figure has a place.</p></nav>
      <main id="main" tabIndex={-1}><div className="freshness" role="status">{!online ? 'Offline · Reconnect before saving.' : loading ? 'Connecting / refreshing… The API may take a moment to wake up.' : `Last fetched ${summary.dataUpdatedAt ? new Date(summary.dataUpdatedAt).toLocaleTimeString() : '—'} · Recorded data, not live bank balances`}</div>
        {notice && <div className="notice success" role="status">{notice}</div>}
        {!modal && recovery}
        {error && <div className="notice error" role="alert">{ready ? 'Last fetched data may be stale. ' : ''}{errorMessage(error)} <button onClick={() => void refresh()}>Retry loading</button></div>}
        {!ready && !error && <div className="loading" role="status"><span className="loading-mark" />Loading your monthly review…</div>}
        {content}
        <footer>Folio · Monthly funding is a flow, not your bank balance.<button disabled={!ready || Boolean(pending)} onClick={async () => {
          try {
            const result = await api.get<'Export'>('/exports');
            const url = URL.createObjectURL(new Blob([JSON.stringify(result, null, 2)], { type: 'application/json' }));
            const link = document.createElement('a'); link.href = url; link.download = `folio-export-${new Date().toISOString().slice(0, 10)}.json`; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000);
            setNotice('JSON export downloaded. It contains sensitive financial records; store it securely.');
          } catch (failure) { setNotice(`Export failed: ${errorMessage(failure)}`); }
        }}>Export JSON</button></footer>
      </main></div></div>;
}
