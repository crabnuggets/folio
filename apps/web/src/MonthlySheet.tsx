import { useEffect, useId, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { ApiClient, ApiError, errorMessage, isUncertain, type Command, type S } from './lib/api';
import { formatMoney, moneyInput, parseMoney } from './lib/money';
import { Form, text } from './components/Form';
import { TransactionChat } from './TransactionChat';

type Sheet = S['MonthlySheet'];
type Item = S['MonthlyItem'];
type Section = S['MonthlySection'];
type Job = { key: string; build(sheet: Sheet): Command<Sheet>; command?: Command<Sheet>; status: 'queued' | 'saving' | 'failed' | 'saved'; error?: unknown; uncertain?: boolean; done?(sheet: Sheet): void; resolve?(ok: boolean): void };
type Perform = (key: string, build: Job['build'], done?: Job['done']) => Promise<boolean>;
const normalize = (name: string) => name.trim().replace(/\s+/g, ' ').toLowerCase();
const findItem = (sheet: Sheet, id: string) => sheet.sections.flatMap(section => section.items).find(item => item.id === id)!;
const updateInput = (item: Item): S['MonthlyItemUpdate'] => ({ section_id: item.section_id, name: item.name, amount_minor: item.amount_minor, note: item.note, repeat_monthly: item.repeat_monthly, usual_amount_minor: item.usual_amount_minor });
function amount(value: string, exponent: number) {
  const result = parseMoney(value, exponent);
  if (BigInt(result) < 0n) throw new Error('Enter zero or a positive amount.');
  return result;
}
function currentMonth() {
  const parts = new Intl.DateTimeFormat('en', { timeZone: 'Asia/Singapore', year: 'numeric', month: '2-digit' }).formatToParts(new Date());
  return `${parts.find(p => p.type === 'year')!.value}-${parts.find(p => p.type === 'month')!.value}`;
}
export function MonthlySheet({ api, signedIn, onDirty, onLogout }: { api: ApiClient; signedIn: boolean; onDirty(value: boolean): void; onLogout(): void }) {
  const [redoToken, setRedoToken] = useState<string>();
  const [month, setMonth] = useState(currentMonth);
  const [sheet, setSheet] = useState<Sheet>();
  const sheetRef = useRef<Sheet>(undefined);
  const [loading, setLoading] = useState(false);
  const [loadError, setLoadError] = useState<unknown>();
  const openCommand = useRef<Command<Sheet>>(undefined);
  const loadRequest = useRef(0);
  const loadedMonth = useRef('');
  const [online, setOnline] = useState(navigator.onLine);
  const [jobs, setJobs] = useState<Record<string, Job>>({});
  const queue = useRef<Job[]>([]);
  const running = useRef(false);
  const [dirty, setDirty] = useState<Set<string>>(new Set());
  const dirtyRef = useRef(new Set<string>());
  // Keep dirty rows mounted in their original section until saved or discarded.
  const retainedRows = useRef(new Map<string, Item>());
  const [addSection, setAddSection] = useState<string | null>(null);
  const [notice, setNotice] = useState('');
  const [chatDirty, setChatDirty] = useState(false);
  const blocked = dirty.size > 0 || queue.current.length > 0 || chatDirty;
  const accept = (value: Sheet) => { sheetRef.current = value; setSheet(value); };
  const markDirty = (key: string, value: boolean) => {
    if (value) dirtyRef.current.add(key); else dirtyRef.current.delete(key);
    const id = key.replace(/-details$/, '');
    if (value && !retainedRows.current.has(id) && sheetRef.current) {
      const item = findItem(sheetRef.current, id);
      if (item) retainedRows.current.set(id, item);
    }
    if (!dirtyRef.current.has(id) && !dirtyRef.current.has(`${id}-details`)) retainedRows.current.delete(id);
    setDirty(new Set(dirtyRef.current));
    if (value) setJobs(previous => {
      const jobKey = key.replace(/-details$/, '');
      if (previous[jobKey]?.status !== 'saved') return previous;
      const next = { ...previous }; delete next[jobKey]; return next;
    });
  };
  const publish = (job: Job) => setJobs(previous => ({ ...previous, [job.key]: { ...job } }));
  const drain = async () => {
    if (running.current || !queue.current.length || !sheetRef.current) return;
    const job = queue.current[0];
    if (job.status === 'failed') return;
    running.current = true; job.status = 'saving'; publish(job);
    try {
      job.command ??= job.build(sheetRef.current);
      const result = await job.command.run();
      setRedoToken(job.key === 'undo' ? result.undo_token ?? undefined : undefined);
      accept(result); job.status = 'saved'; job.error = undefined; publish(job);
      queue.current.shift(); job.done?.(result); job.resolve?.(true);
    } catch (error) {
      job.status = 'failed'; job.error = error;
      // Idempotency replay precedes version checking: an exact retry's 412
      // proves this command did not commit. Authentication loss proves nothing.
      job.uncertain = error instanceof ApiError && error.status === 412 ? false : job.uncertain || isUncertain(error);
      publish(job); job.resolve?.(false);
    } finally { running.current = false; if (queue.current[0]?.status === 'queued') void drain(); }
  };
  const perform: Perform = (key, build, done) => {
    if ((key === 'undo' || key === 'redo') && (dirtyRef.current.size > 0 || queue.current.length > 0)) return Promise.resolve(false);
    if (queue.current.some(job => job.key === key)) return Promise.resolve(false);
    return new Promise(resolve => {
      const job: Job = { key, build, done, resolve, status: 'queued' };
      queue.current.push(job); publish(job); void drain();
    });
  };
  const load = async (open = false) => {
    const request = ++loadRequest.current;
    setLoading(true); setLoadError(undefined);
    try {
      let result: Sheet;
      const settings = await api.get<'Settings'>('/settings');
      if (request !== loadRequest.current) return false;
      if (open && settings.currency === 'SGD') {
        openCommand.current ??= api.command<'MonthlySheet'>(`/monthly-sheets/${month}/open`, {});
        result = await openCommand.current.run();
      } else result = await api.get<'MonthlySheet'>(`/monthly-sheets/${month}`);
      if (request !== loadRequest.current) return false;
      setRedoToken(undefined); accept(result); loadedMonth.current = month; return true;
    } catch (error) { if (request === loadRequest.current) setLoadError(error); return false; }
    finally { if (request === loadRequest.current) setLoading(false); }
  };
  useEffect(() => {
    openCommand.current = undefined; setSheet(undefined); sheetRef.current = undefined; setJobs({});
    if (signedIn) void load(true);
    return () => { loadRequest.current++; };
  }, [month]);
  useEffect(() => { if (signedIn && loadedMonth.current !== month && !loading) void load(true); }, [signedIn]);
  useEffect(() => { onDirty(blocked); }, [blocked]);
  useEffect(() => {
    const update = () => setOnline(navigator.onLine);
    window.addEventListener('online', update); window.addEventListener('offline', update);
    return () => { window.removeEventListener('online', update); window.removeEventListener('offline', update); };
  }, []);
  const retry = (job: Job) => { const retained = queue.current.find(value => value.key === job.key); if (retained) { retained.status = 'queued'; void drain(); } };
  const compare = async (job: Job) => {
    // A rejected precondition has no write to recover. Fetch before releasing it.
    if (!await load()) return;
    const released = queue.current.filter(value => {
      const retained = retainedRows.current.get(value.key);
      const current = findItem(sheetRef.current!, value.key);
      return value.key === job.key || retained && (!current || current.section_id !== retained.section_id);
    });
    queue.current = queue.current.filter(value => !released.includes(value));
    released.forEach(value => value.resolve?.(false));
    setJobs(previous => { const next = { ...previous }; released.forEach(value => { delete next[value.key]; }); return next; });
    setNotice(dirtyRef.current.size ? 'Latest saved values loaded. Your unsaved edits remain in their rows or form. Compare them, then save or discard.' : 'Latest saved values loaded. Review the current sheet before trying again.'); void drain();
  };
  const feedback = (key: string) => {
    const job = jobs[key];
    if (!job) return null;
    return <div className={`row-status ${job.error ? 'error' : ''}`} role={job.error ? 'alert' : 'status'}>
      {job.status === 'saving' ? 'Saving…' : job.status === 'queued' ? 'Waiting to save…' : job.status === 'saved' ? 'Saved' : `Not saved. ${errorMessage(job.error)}`}
      {job.status === 'failed' && <div><button onClick={() => retry(job)} disabled={!online}>Retry same action</button>
        {!job.uncertain && job.error instanceof ApiError && [400, 409, 412, 422].includes(job.error.status) && <button onClick={() => void compare(job)}>Compare latest</button>}</div>}
    </div>;
  };
  const locked = (key: string) => Boolean(jobs[key] && jobs[key].status !== 'saved');
  const money = (value: string) => formatMoney(value, sheet!.currency, sheet!.currency_exponent);
  const focusItem = (id: string) => { requestAnimationFrame(() => document.getElementById(`amount-${id}`)?.focus()); };
  const available = online && !loading && !loadError && sheet?.currency === 'SGD';
  const itemsIn = (section: Section) => {
    const items = section.items.filter(item => !retainedRows.current.has(item.id) || retainedRows.current.get(item.id)!.section_id === section.id);
    for (const retained of retainedRows.current.values()) {
      if (retained.section_id === section.id && !items.some(item => item.id === retained.id)) items.push(findItem(sheet!, retained.id) ?? retained);
    }
    return items;
  };
  return <div className="sheet-shell"><a className="skip-link" href="#main">Skip to content</a>
    <header className="sheet-topbar"><div className="brand"><span className="brand-mark" aria-hidden="true">f</span>Folio</div>
      <div className="month-selector" role="group" aria-label="Reporting month">
        <MonthPicker value={month.slice(5)} disabled={blocked || loading} onChange={value => { setMonth(month.slice(0, 5) + value); setAddSection(null); setNotice(''); }} />
        <label className="sr-only" htmlFor="year">Year</label>
        <input id="year" key={month.slice(0, 4)} type="number" min="1900" max="9999" defaultValue={month.slice(0, 4)} disabled={blocked || loading} onKeyDown={event => { if (event.key === 'Enter') event.currentTarget.blur(); }} onBlur={event => { const year = Number(event.target.value); if (Number.isInteger(year) && year >= 1900 && year <= 9999) { setMonth(String(year) + month.slice(4)); setAddSection(null); setNotice(''); } else event.target.value = month.slice(0, 4); }} />
      </div>
        <div className="actions"><button onClick={async () => {
          try {
            const result = await api.get<'Export'>('/exports');
            const url = URL.createObjectURL(new Blob([JSON.stringify(result, null, 2)], { type: 'application/json' }));
            const link = document.createElement('a'); link.href = url; link.download = `folio-export-${new Date().toISOString().slice(0, 10)}.json`; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000);
            setNotice('JSON export downloaded. It includes retained financial history.');
          } catch (error) { setNotice(`Export failed. ${errorMessage(error)}`); }
        }}>Export JSON</button><button disabled={blocked || loading} onClick={onLogout}>Sign out</button></div>
    </header>
    <main id="main" className="sheet-main" tabIndex={-1}>
      <div className="freshness" role="status">{!online ? 'Offline. Edits stay in this tab. Reconnect to save.' : loading ? 'Loading your sheet. The server may take a moment to wake up.' : 'Totals use saved amounts. Unsaved edits stay in their rows.'}</div>
      <div className="actions history-actions"><span className="quiet" role="status">{blocked ? 'Unsaved changes' : available ? 'Saved' : 'Unavailable'}</span>{(['undo', 'redo'] as const).map(action => <button key={action} className="history-button" aria-label={action === 'undo' ? 'Undo' : 'Redo'} title={action === 'undo' ? 'Undo' : 'Redo'} disabled={blocked || !available || !sheet?.undo_token || (action === 'redo' ? redoToken !== sheet.undo_token : redoToken === sheet.undo_token)} onClick={() => void perform(action, latest => api.command<'MonthlySheet'>(`/monthly-sheets/${month}/undo`, { undo_token: latest.undo_token }, { version: latest.version }))}><svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true" style={{ transform: action === 'redo' ? 'scaleX(-1)' : undefined }}><path d="M9 5 4 10l5 5M4 10h9a6 6 0 0 1 6 6v3" strokeLinecap="round" strokeLinejoin="round" /></svg></button>)}</div>
      {jobs.undo?.status !== 'saved' && feedback('undo')}{jobs.redo?.status !== 'saved' && feedback('redo')}
      {notice && <p className="notice" role="status">{notice}</p>}
      {loadError != null && <div className="notice error" role="alert">{sheet ? 'Saved data may be stale. ' : ''}{errorMessage(loadError)} <button onClick={() => void load(loadedMonth.current !== month)}>Retry loading</button></div>}
      {sheet && <>
        {sheet.currency !== "SGD" && <p className="notice">Historical {sheet.currency} amounts are read-only and preserved in export. No currency conversion has been applied.</p>}
        <section className="remainder" aria-label="Monthly summary"><h1>Monthly remainder</h1><output className="hero-amount">{money(sheet.remainder_minor)}</output>
          <dl className="totals"><div><dt>Money in</dt><dd>{money(sheet.money_in_minor)}</dd></div><div><dt>Money out</dt><dd>{money(sheet.money_out_minor)}</dd></div></dl>
          <p className="coverage">Based on entered amounts.{sheet.missing_count > 0 ? ` ${sheet.missing_count} ${sheet.missing_count === 1 ? 'item still blank' : 'items still blank'}.` : ' All listed amounts entered.'}</p>
        </section>
        {sheet.has_legacy_data && <p className="notice">Your earlier financial data is retained separately and included in export. It has not been converted into monthly amounts.</p>}
        {blocked && <p className="quiet" role="status">Unsaved edits stay in this month until saved or discarded.</p>}

        <div className="sheet-columns">{(['in', 'out'] as const).map(direction => <div className="sheet-column" key={direction}><h2>Money {direction}</h2>
          {sheet.sections.filter(section => section.direction === direction).map(section => <section className="sheet-section" key={section.id} aria-label={section.name}>
            <div className="section-heading"><h3>{section.name}</h3><output aria-label={`${section.name} subtotal`}>{money(section.total_minor)}</output></div>
            {section.amount_label === 'Bill amount' && <p className="amount-caption">Bill amount</p>}
            {itemsIn(section).map(item => <ItemRow key={`${month}-${item.id}`} item={item} section={section} sheet={sheet} api={api} available={Boolean(available)} locked={locked(item.id) || locked(`${item.id}-restore`)} feedback={<>{feedback(item.id)}{feedback(`${item.id}-restore`)}</>} perform={perform} markDirty={markDirty} />)}
            {!itemsIn(section).length && <p className="empty">Add your first item.</p>}
            <div className="section-actions">{section.items.some(item => item.repeat_monthly && item.amount_minor == null && item.usual_amount_minor != null) && <button className="quiet-button" disabled={blocked || !available} onClick={() => void perform(`usual-${section.id}`, latest => api.command<'MonthlySheet'>(`/monthly-sheets/${month}/usual-amounts`, { section_id: section.id }, { version: latest.version }))}>Use usual amounts</button>}
              <button id={`add-${section.id}`} className="quiet-button" disabled={!available || addSection !== null} onClick={() => { setAddSection(section.id); markDirty('add', true); }}>Add item<span className="sr-only"> to {section.name}</span></button></div>
            {feedback(`usual-${section.id}`)}
            {addSection === section.id && <AddItem section={section} sheet={sheet} api={api} signedIn={signedIn} perform={perform} locked={locked('add')} feedback={feedback('add')} focusItem={focusItem} close={() => { setAddSection(null); markDirty('add', false); requestAnimationFrame(() => document.getElementById(`add-${section.id}`)?.focus()); }} />}
          </section>)}
        </div>)}</div>
        <details className="settings"><summary>Sheet options</summary>
          <Form submit="Add section" disabled={blocked || !available} fields={[{ name: 'name', label: 'Section name', required: true }, { name: 'direction', label: 'Side', type: 'select', required: true, options: [{ value: 'in', label: 'Money in' }, { value: 'out', label: 'Money out' }] }]} onSubmit={async values => {
            await perform('section-add', latest => api.command<'MonthlySheet'>(`/monthly-sheets/${month}/sections`, { name: text(values, 'name'), direction: text(values, 'direction') }, { version: latest.version }));
          }} />{feedback('section-add')}
          <div className="section-settings"><h3>Rename a section</h3>{sheet.sections.map(section => <SectionRename key={section.id} section={section} disabled={blocked || !available} perform={perform} api={api} month={month} feedback={feedback(`section-${section.id}`)} />)}</div>
        </details>
      </>}
      <footer>Folio · Monthly amounts, one sheet. The remainder is not a bank balance.</footer>
    </main>
    <TransactionChat api={api} signedIn={signedIn} month={month} sheet={sheet} available={Boolean(available)} saving={queue.current.length > 0} onDirty={setChatDirty} feedback={feedback} locked={locked} onAdd={async (key, input, done) => {
      const latest = sheetRef.current;
      if (!latest || !available || queue.current.length > 0) return false;
      const direction = latest.sections.find(section => section.id === input.section_id)?.direction;
      if (latest.sections.some(section => section.direction === direction && section.items.some(item => normalize(item.name) === normalize(input.name)))) throw new Error('An item with this name already exists on this side of the sheet. Edit that item, or use a different name.');
      return perform(key, current => api.command<'MonthlySheet'>(`/monthly-sheets/${month}/items`, input, { version: current.version }), done);
    }} />
  </div>;
}

function SectionRename({ section, disabled, perform, api, month, feedback }: { section: Section; disabled: boolean; perform: Perform; api: ApiClient; month: string; feedback: React.ReactNode }) {
  const [name, setName] = useState(section.name);
  return <div><form className="section-rename" onSubmit={event => { event.preventDefault(); void perform(`section-${section.id}`, latest => api.command<'MonthlySheet'>(`/monthly-sheets/${month}/sections/${section.id}`, { name }, { method: 'PUT', version: latest.version })); }}>
    <div className="field"><label htmlFor={`section-${section.id}`}>{section.name}</label><input id={`section-${section.id}`} value={name} onChange={event => setName(event.target.value)} required disabled={disabled} /></div><button disabled={disabled || name === section.name}>Rename</button>
  </form>{feedback}</div>;
}

function ItemRow({ item, section, sheet, api, available, locked, feedback, perform, markDirty }: {
  item: Item; section: Section; sheet: Sheet; api: ApiClient; available: boolean; locked: boolean; feedback: React.ReactNode; perform: Perform; markDirty(key: string, value: boolean): void;
}) {
  const [value, setValue] = useState(moneyInput(item.amount_minor, sheet.currency_exponent));
  const edited = useRef(false);
  const [error, setError] = useState('');
  const [details, setDetails] = useState(false);
  const current = findItem(sheet, item.id);
  const placementChanged = !current || current.section_id !== section.id;
  const saveEdit = (build: Job['build'], done: Job['done']) => {
    if (!current) {
      // Restore only after explicit reapply, then merge the draft into the
      // restored server row so concurrent saved notes remain intact.
      void perform(`${item.id}-restore`, latest => api.command<'MonthlySheet'>(`/monthly-sheets/${sheet.month}/items/${item.id}/restore`, {}, { version: latest.version }), () => { void perform(item.id, build, done); });
    } else void perform(item.id, build, done);
  };
  useEffect(() => { if (!edited.current) setValue(moneyInput(item.amount_minor, sheet.currency_exponent)); }, [item.amount_minor, sheet.currency_exponent]);
  const clean = (result: Sheet) => {
    edited.current = false; markDirty(item.id, false); setError('');
    const saved = findItem(result, item.id);
    if (saved) setValue(moneyInput(saved.amount_minor, sheet.currency_exponent));
  };
  const save = (explicit = false) => {
    if (!edited.current || locked || !available || placementChanged && !explicit) return;
    let minor: string | null;
    try { minor = value.trim() === '' ? null : amount(value, sheet.currency_exponent); }
    catch (error) { setError(errorMessage(error)); return; }
    saveEdit(latest => api.command<'MonthlySheet'>(`/monthly-sheets/${sheet.month}/items/${item.id}`, { ...updateInput(findItem(latest, item.id)), amount_minor: minor }, { method: 'PUT', version: latest.version }), clean);
  };
  return <div className="sheet-row" data-item={item.id}>
    <button id={`item-${item.id}`} className="item-name" aria-expanded={details} onClick={() => setDetails(true)}>{item.name}{item.repeat_monthly && <span className="monthly-label">Monthly</span>}</button>
    <div><label className="sr-only" htmlFor={`amount-${item.id}`}>{section.amount_label} for {item.name}</label><input id={`amount-${item.id}`} className="amount-field" inputMode="decimal" autoComplete="off" value={value} placeholder={item.usual_amount_minor != null && item.repeat_monthly ? `Usual ${moneyInput(item.usual_amount_minor, sheet.currency_exponent)}` : 'Not entered'} disabled={locked} aria-invalid={Boolean(error) || undefined} aria-describedby={error ? `error-${item.id}` : undefined} onChange={event => {
      edited.current = true; markDirty(item.id, true); setValue(event.target.value); setError('');
    }} onBlur={event => { if (!(event.relatedTarget instanceof HTMLElement && event.relatedTarget.dataset.discardAmount === item.id)) save(); }} onKeyDown={event => {
      if (event.key === 'Escape') {
        event.preventDefault();
        const target = edited.current ? document.querySelector<HTMLElement>(`[data-discard-amount="${item.id}"]`) : document.getElementById(`item-${item.id}`);
        target?.focus(); return;
      }
      if (event.key !== 'Enter' && event.key !== 'Tab') return;
      const fields = Array.from(document.querySelectorAll<HTMLInputElement>('.amount-field:not(:disabled)'));
      const next = fields[fields.indexOf(event.currentTarget) + (event.shiftKey ? -1 : 1)];
      if (next) { event.preventDefault(); next.focus(); }
      else if (event.key === 'Enter') { event.preventDefault(); save(true); }
    }} /></div>
    {error && <div className="row-status error" id={`error-${item.id}`} role="alert">{error}</div>}
    {feedback}
    {placementChanged && <p className="row-status">{current ? `Moved to ${sheet.sections.find(section => section.id === current.section_id)?.name} in another tab.` : 'Skipped in another tab.'} Your unsaved edits remain here. Save to reapply or discard them.</p>}
    {edited.current && !locked && <div className="row-status">Saved amount: {!current || item.amount_minor == null ? 'Not entered' : formatMoney(item.amount_minor, sheet.currency, sheet.currency_exponent)}<div><button onClick={() => save(true)} disabled={!available}>{current ? 'Save amount' : 'Restore and reapply amount'}</button><button data-discard-amount={item.id} onClick={() => { edited.current = false; markDirty(item.id, false); setValue(moneyInput(item.amount_minor, sheet.currency_exponent)); setError(''); requestAnimationFrame(() => document.getElementById(`item-${item.id}`)?.focus()); }}>Discard edit</button></div></div>}
    {details && <ItemDetails item={item} sheet={sheet} api={api} perform={perform} saveEdit={saveEdit} disabled={locked || !available || edited.current} markDirty={markDirty} onClose={() => { setDetails(false); requestAnimationFrame(() => { if (document.activeElement === document.body) document.getElementById(`item-${item.id}`)?.focus(); }); }} clean={clean} />}
  </div>;
}
function ItemDetails({ item, sheet, api, perform, saveEdit, disabled, markDirty, onClose, clean }: {
  item: Item; sheet: Sheet; api: ApiClient; perform: Perform; saveEdit(build: Job['build'], done: Job['done']): void; disabled: boolean; markDirty(key: string, value: boolean): void; onClose(): void; clean(sheet: Sheet): void;
}) {
  const [name, setName] = useState(item.name);
  const [note, setNote] = useState(item.note);
  const [repeat, setRepeat] = useState(item.repeat_monthly);
  const [usual, setUsual] = useState(moneyInput(item.usual_amount_minor, sheet.currency_exponent));
  const [error, setError] = useState('');
  const dirtyKey = `${item.id}-details`;
  const changedFields = useRef(new Set<keyof S['MonthlyItemUpdate']>());
  const changed = (field: keyof S['MonthlyItemUpdate']) => { changedFields.current.add(field); markDirty(dirtyKey, true); };
  const done = (result: Sheet) => { clean(result); markDirty(dirtyKey, false); onClose(); };
  const inputId = useId();
  return <div className="item-details"><form onSubmit={event => {
    event.preventDefault(); setError('');
    try {
      const usualMinor = repeat && usual.trim() ? amount(usual, sheet.currency_exponent) : null;
      const edits = { name, note, repeat_monthly: repeat, usual_amount_minor: usualMinor };
      saveEdit(latest => {
        const input = updateInput(findItem(latest, item.id));
        for (const field of Object.keys(edits) as (keyof typeof edits)[]) if (changedFields.current.has(field)) Object.assign(input, { [field]: edits[field] });
        return api.command<'MonthlySheet'>(`/monthly-sheets/${sheet.month}/items/${item.id}`, input, { method: 'PUT', version: latest.version });
      }, done);
    } catch (error) { setError(errorMessage(error)); }
  }}><fieldset disabled={disabled}>
    <div className="field"><label htmlFor={`${inputId}-name`}>Name</label><input id={`${inputId}-name`} autoFocus value={name} required onChange={event => { setName(event.target.value); changed('name'); }} /></div>
    <div className="field"><label htmlFor={`${inputId}-note`}>Notes</label><textarea id={`${inputId}-note`} rows={2} value={note} onChange={event => { setNote(event.target.value); changed('note'); }} /></div>
    <div className="check-field"><input id={`${inputId}-repeat`} type="checkbox" role="switch" checked={repeat} onChange={event => { setRepeat(event.target.checked); changed('repeat_monthly'); changed('usual_amount_minor'); }} /><label htmlFor={`${inputId}-repeat`}>Repeat monthly</label></div>
    {repeat && <><p className="quiet">Appears each month. Enter the amount when you know it.</p><div className="field"><label htmlFor={`${inputId}-usual`}>Usual amount</label><input id={`${inputId}-usual`} inputMode="decimal" value={usual} onChange={event => { setUsual(event.target.value); changed('usual_amount_minor'); }} /><small>Optional suggestion. Existing monthly amounts stay as entered.</small></div></>}
    {error && <p className="error" role="alert">{error}</p>}
    <div className="actions"><button className="primary">{findItem(sheet, item.id) ? 'Save details' : 'Restore and reapply details'}</button><button type="button" onClick={() => { markDirty(dirtyKey, false); onClose(); }}>Cancel</button></div>
    <div className="actions"><button type="button" disabled={!findItem(sheet, item.id) || item.amount_minor == null} onClick={() => void perform(item.id, latest => api.command<'MonthlySheet'>(`/monthly-sheets/${sheet.month}/items/${item.id}`, { ...updateInput(findItem(latest, item.id)), amount_minor: null }, { method: 'PUT', version: latest.version }), done)}>Clear amount</button>
      <button type="button" disabled={!findItem(sheet, item.id)} onClick={() => void perform(item.id, latest => api.command<'MonthlySheet'>(`/monthly-sheets/${sheet.month}/items/${item.id}/remove`, {}, { version: latest.version }), done)}>Skip this month</button>
      {item.repeat_monthly && <button type="button" disabled={!findItem(sheet, item.id)} onClick={() => void perform(item.id, latest => api.command<'MonthlySheet'>(`/monthly-sheets/${sheet.month}/items/${item.id}`, { ...updateInput(findItem(latest, item.id)), repeat_monthly: false, usual_amount_minor: null }, { method: 'PUT', version: latest.version }), done)}>Stop repeating</button>}
    </div><p className="quiet">Skipping removes this month's row. History stays saved. Stop repeating ends future automatic rows.</p>
  </fieldset></form></div>;
}
function AddItem({ section, sheet, api, signedIn, perform, locked, feedback, close, focusItem }: {
  section: Section; sheet: Sheet; api: ApiClient; signedIn: boolean; perform: Perform; locked: boolean; feedback: React.ReactNode; close(): void; focusItem(id: string): void;
}) {
  const id = useId();
  const [name, setName] = useState('');
  const [value, setValue] = useState('');
  const [repeat, setRepeat] = useState(false);
  const [suggestUsual, setSuggestUsual] = useState(false);
  const [usualEdited, setUsualEdited] = useState(false);
  const [preferenceEdited, setPreferenceEdited] = useState(false);
  const [selected, setSelected] = useState<S['MonthlyItemSuggestion']>();
  const [showSuggestions, setShowSuggestions] = useState(true);
  const [active, setActive] = useState(-1);
  const [error, setError] = useState('');
  const suggestions = useQuery({ queryKey: ['monthly-suggestions', sheet.month, section.id, name, sheet.version], queryFn: () => api.get<'MonthlyItemSuggestions'>(`/monthly-item-suggestions?month=${sheet.month}&section_id=${section.id}&q=${encodeURIComponent(name)}`), enabled: signedIn && showSuggestions && !selected && !locked });
  const matches = showSuggestions && !selected ? suggestions.data?.items ?? [] : [];
  const select = (item: S['MonthlyItemSuggestion']) => {
    if (item.current_month_item_id) { close(); focusItem(item.current_month_item_id); return; }
    setSelected(item); setName(item.name); setRepeat(item.repeat_monthly); setSuggestUsual(item.usual_amount_minor != null); setPreferenceEdited(false); setUsualEdited(false); setActive(-1); setShowSuggestions(false);
    document.getElementById(`${id}-amount`)?.focus();
  };
  return <form className="inline-form" aria-label={`Add item to ${section.name}`} onSubmit={event => {
    event.preventDefault(); setError('');
    const existing = sheet.sections.filter(candidate => candidate.direction === section.direction).flatMap(candidate => candidate.items).find(item => normalize(item.name) === normalize(name));
    if (existing) { close(); focusItem(existing.id); return; }
    try {
      const minor = amount(value, sheet.currency_exponent);
      const input: S['MonthlyItemInput'] = { section_id: section.id, name, amount_minor: minor };
      if (selected) input.item_id = selected.id;
      // Omit preferences for a typed historical match unless the user chose them.
      if (preferenceEdited || !selected && repeat) {
        input.repeat_monthly = repeat;
        input.usual_amount_minor = repeat && suggestUsual ? usualEdited ? minor : selected?.usual_amount_minor ?? minor : null;
      }
      void perform('add', latest => api.command<'MonthlySheet'>(`/monthly-sheets/${sheet.month}/items`, input, { version: latest.version }), result => {
        close(); const saved = result.sections.flatMap(section => section.items).find(item => item.id === selected?.id || normalize(item.name) === normalize(name)); if (saved) focusItem(saved.id);
      });
    } catch (error) { setError(errorMessage(error)); }
  }}><h4>Add item to {section.name}</h4><fieldset disabled={locked}>
    <div className="field"><label htmlFor={`${id}-name`}>Name</label><input id={`${id}-name`} autoFocus required role="combobox" aria-autocomplete="list" aria-expanded={matches.length > 0} aria-controls={`${id}-suggestions`} aria-activedescendant={active >= 0 && matches[active] ? `${id}-option-${active}` : undefined} autoComplete="off" value={name} onChange={event => {
      setName(event.target.value); setShowSuggestions(true); setActive(-1);
      if (selected) { setSelected(undefined); setRepeat(false); setSuggestUsual(false); setPreferenceEdited(false); setUsualEdited(false); }
    }} onKeyDown={event => {
      if (event.key === 'Escape') { event.preventDefault(); setShowSuggestions(false); setActive(-1); }
      if (event.key === 'ArrowDown' && matches.length) { event.preventDefault(); setActive(index => (index + 1) % matches.length); }
      if (event.key === 'ArrowUp' && matches.length) { event.preventDefault(); setActive(index => (index + matches.length - 1) % matches.length); }
      if (event.key === 'Enter' && active >= 0 && matches[active]) { event.preventDefault(); select(matches[active]); }
    }} />
      {matches.length > 0 && <ul className="suggestions" role="listbox" id={`${id}-suggestions`} aria-label="Saved items">{matches.map((item, index) => <li key={item.id} role="option" id={`${id}-option-${index}`} aria-selected={active === index} onPointerDown={event => event.preventDefault()} onClick={() => select(item)}>{item.name}<small>{item.section_name}{item.current_month_item_id ? ' · Already in this month' : item.repeat_monthly ? ' · Monthly' : ''}</small></li>)}</ul>}
      {matches.length > 0 && <button className="quiet-button" type="button" onClick={() => setShowSuggestions(false)}>Dismiss suggestions</button>}
      {suggestions.isError && <small>Saved names could not be loaded. You can still type a name.</small>}
    </div>
    <div className="field"><label htmlFor={`${id}-amount`}>{section.amount_label}</label><input id={`${id}-amount`} inputMode="decimal" required value={value} onChange={event => setValue(event.target.value)} />{section.amount_label === 'Bill amount' && <small>The bill amount accounted for this month.</small>}</div>
    <div className="check-field"><input id={`${id}-repeat`} role="switch" type="checkbox" checked={repeat} onChange={event => { setRepeat(event.target.checked); setPreferenceEdited(true); }} /><label htmlFor={`${id}-repeat`}>Repeat monthly</label></div>
    {repeat && <><p className="quiet">Appears each month. Enter the amount when you know it.</p><div className="check-field"><input id={`${id}-usual`} type="checkbox" checked={suggestUsual} onChange={event => { setSuggestUsual(event.target.checked); setPreferenceEdited(true); setUsualEdited(true); }} /><label htmlFor={`${id}-usual`}>Suggest this amount each month</label></div>{selected?.usual_amount_minor != null && suggestUsual && !usualEdited && <small>Saved usual amount: {formatMoney(selected.usual_amount_minor, sheet.currency, sheet.currency_exponent)}</small>}</>}
    {error && <p className="error" role="alert">{error}</p>}
    <div className="actions"><button className="primary">Save item</button><button type="button" onClick={close}>Cancel</button></div>
  </fieldset>{feedback}</form>;
}

function MonthPicker({ value, disabled, onChange }: { value: string; disabled: boolean; onChange(value: string): void }) {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const names = Array.from({ length: 12 }, (_, i) => new Intl.DateTimeFormat('en', { month: 'long' }).format(new Date(2020, i, 1)));
  useEffect(() => {
    const outside = (event: PointerEvent) => { if (!root.current?.contains(event.target as Node)) setOpen(false); };
    document.addEventListener('pointerdown', outside);
    return () => document.removeEventListener('pointerdown', outside);
  }, []);
  useEffect(() => { if (open) root.current?.querySelector<HTMLButtonElement>('[aria-pressed="true"]')?.focus(); }, [open]);
  return <div className="month-picker" ref={root} onBlur={event => { if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false); }} onKeyDown={event => {
    if (event.key === 'Escape') { setOpen(false); trigger.current?.focus(); }
    if (open && ['ArrowRight', 'ArrowLeft', 'ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
      event.preventDefault(); const buttons = Array.from(root.current!.querySelectorAll<HTMLButtonElement>('.month-popup button')); const index = buttons.indexOf(document.activeElement as HTMLButtonElement);
      const next = event.key === 'Home' ? 0 : event.key === 'End' ? 11 : (index + (event.key === 'ArrowRight' ? 1 : event.key === 'ArrowLeft' ? -1 : event.key === 'ArrowDown' ? 3 : -3) + 12) % 12; buttons[next].focus();
    }
  }}><button id="month" ref={trigger} disabled={disabled} aria-label="Month" aria-expanded={open} aria-controls="month-popup" onClick={() => setOpen(!open)}>{names[Number(value) - 1]} <svg aria-hidden="true" width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor"><path d="m4 6 4 4 4-4" /></svg></button>
    {open && <div id="month-popup" className="month-popup" role="group" aria-label="Choose month">{names.map((name, i) => <button key={name} disabled={disabled} aria-pressed={Number(value) === i + 1} onClick={() => { onChange(String(i + 1).padStart(2, '0')); setOpen(false); trigger.current?.focus(); }}>{name}</button>)}</div>}
  </div>;
}
