import { useEffect, useId, useRef, useState, type ReactNode } from 'react';
import { ApiClient, errorMessage, type S } from './lib/api';
import { Attach, Close, Send, Sliders } from './components/Brand';
import { formatMonth, groupAmount, moneyInput, parseMoney } from './lib/money';

type Proposal = { key: string; name: string; section_id: string; amount: string; note: string; repeat: boolean; editing: boolean };
type Attachment = NonNullable<S['ChatCaptureInput']['attachment']>;
type Props = {
  api: ApiClient; signedIn: boolean; month: string; sheet?: S['MonthlySheet']; available: boolean; unavailable: string; saving: boolean;
  onDirty(value: boolean): void; locked(key: string): boolean; feedback(key: string): ReactNode;
  onAdd(key: string, input: S['MonthlyItemInput'], done: () => void): Promise<boolean>;
};
const fileTypes: Attachment['media_type'][] = ['image/png', 'image/jpeg', 'image/webp', 'application/pdf', 'text/plain'];
const normalized = (value: string) => value.trim().replace(/\s+/g, ' ').toLowerCase();

export function TransactionChat({ api, signedIn, month, sheet, available, unavailable, saving, onDirty, locked, feedback, onAdd }: Props) {
  const id = useId();
  const [expanded, setExpanded] = useState(false);
  const [connection, setConnection] = useState<S['ChatGPTConnection']>();
  const [connectionError, setConnectionError] = useState('');
  const [connecting, setConnecting] = useState(false);
  const [model, setModel] = useState('');
  const [text, setText] = useState('');
  const [attachment, setAttachment] = useState<Attachment>();
  const [reading, setReading] = useState(false);
  const [processing, setProcessing] = useState(false);
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');
  const [proposals, setProposals] = useState<Proposal[]>([]);
  const [saved, setSaved] = useState(0);
  const [adding, setAdding] = useState(false);
  const [composing, setComposing] = useState(false);
  useEffect(() => { setSaved(0); setMessage(''); setError(''); }, [month]);
  const fileInput = useRef<HTMLInputElement>(null);
  const root = useRef<HTMLElement>(null);
  const popup = useRef<Window | null>(null);
  const active = useRef(signedIn);
  active.current = signedIn;
  const busy = reading || processing;
  const dirty = Boolean(text.trim() || attachment || proposals.length || busy);
  useEffect(() => { onDirty(dirty); }, [dirty, onDirty]);
  // Keep keyboard focus in the review: first suggestion on arrival, the neighbour after one is resolved, the composer at the end.
  const resolvedIndex = useRef(0);
  const proposalCount = useRef(0);
  useEffect(() => {
    const before = proposalCount.current; proposalCount.current = proposals.length;
    if (before === proposals.length || !root.current) return;
    if (document.activeElement !== document.body && !root.current.contains(document.activeElement)) return;
    const forms = root.current.querySelectorAll<HTMLElement>('.chat-proposal');
    const target = proposals.length === 0 ? document.getElementById(`${id}-text`) : forms[before === 0 ? 0 : Math.min(resolvedIndex.current, forms.length - 1)]?.querySelector<HTMLElement>('button:not(:disabled)');
    target?.focus();
  }, [proposals.length]);
  // Focus that was inside the chat (or dropped to the page) lands on the control that now leads the review.
  const focusReview = () => {
    if (!root.current || document.activeElement !== document.body && !root.current.contains(document.activeElement)) return;
    const forms = root.current.querySelectorAll<HTMLElement>('.chat-proposal');
    const target = forms[Math.min(resolvedIndex.current, forms.length - 1)]?.querySelector<HTMLElement>('button:not(:disabled)')
      ?? root.current.querySelector<HTMLElement>('.chat-review-bar button:not(:disabled)') ?? root.current.querySelector<HTMLElement>('.chat-panel-heading button') ?? document.getElementById(`${id}-text`);
    target?.focus();
  };
  // Collapsing a pending review hands focus to Review; reopening returns it to the suggestion last in use.
  const wasExpanded = useRef(expanded);
  useEffect(() => {
    if (wasExpanded.current === expanded) return;
    wasExpanded.current = expanded;
    if (proposals.length) focusReview();
  }, [expanded]);
  const refresh = async () => {
    try {
      const result = await api.get<'ChatGPTConnection'>('/chatgpt/connection');
      if (!active.current) return;
      setConnection(result); setConnectionError('');
      setModel(previous => result.models?.some(choice => choice.id === previous) ? previous : result.models?.[0]?.id ?? '');
    } catch (error) { if (active.current) setConnectionError(errorMessage(error)); }
  };
  useEffect(() => {
    active.current = signedIn;
    if (!signedIn) { popup.current?.close(); popup.current = null; setConnecting(false); return; }
    void refresh();
    const focus = () => { if (popup.current) void refresh(); };
    window.addEventListener('focus', focus);
    return () => { window.removeEventListener('focus', focus); };
  }, [signedIn]);
  useEffect(() => {
    if (!connecting || !popup.current) return;
    const timer = window.setInterval(() => {
      if (popup.current?.closed) { popup.current = null; setConnecting(false); void refresh(); }
    }, 700);
    const timeout = window.setTimeout(() => {
      popup.current?.close(); popup.current = null; setConnecting(false);
      setConnectionError('Sign-in timed out. Try connecting again.');
    }, 10 * 60 * 1000);
    return () => { window.clearInterval(timer); window.clearTimeout(timeout); };
  }, [connecting]);
  useEffect(() => () => { active.current = false; popup.current?.close(); }, []);
  const connect = async () => {
    setConnectionError('');
    const windowRef = window.open('', 'folio-chatgpt', 'popup,width=560,height=760');
    if (!windowRef) { setConnectionError('Allow pop-ups for Folio, then try connecting again.'); return; }
    popup.current = windowRef; setConnecting(true);
    try {
      const result = await api.command<'ChatGPTConnectResponse'>('/chatgpt/connect', {}).run();
      if (active.current && !windowRef.closed) windowRef.location.href = result.authorization_url;
    } catch (error) { windowRef.close(); popup.current = null; setConnecting(false); if (active.current) setConnectionError(errorMessage(error)); }
  };
  const chooseFile = async (file?: File) => {
    if (!file) return;
    setError(''); setExpanded(true);
    const mediaType = fileTypes.find(type => type === file.type);
    if (!mediaType) { setError('Choose a PNG, JPEG, WebP, PDF, or plain text document.'); return; }
    if (file.size === 0 || file.size > 8 * 1024 * 1024) { setError('Choose a nonempty file up to 8 MB.'); return; }
    setReading(true);
    try {
      const data = await new Promise<string>((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(String(reader.result).split(',')[1]);
        reader.onerror = () => reject(new Error('The file could not be read. Choose it again.'));
        reader.readAsDataURL(file);
      });
      if (active.current) setAttachment({ name: file.name, media_type: mediaType, data });
    } catch (error) { setError(errorMessage(error)); }
    finally { setReading(false); }
  };
  const send = async () => {
    if (!sheet || busy || proposals.length || !available || !connection?.sharing || !model || (!text.trim() && !attachment)) return;
    setExpanded(true); setProcessing(true); setError(''); setMessage(''); setSaved(0);
    try {
      const result = await api.command<'ChatCaptureResponse', S['ChatCaptureInput']>(`/monthly-sheets/${month}/capture`, { text, model, ...(attachment ? { attachment } : {}) }).run();
      if (!active.current) return;
      setMessage(result.message);
      setProposals(result.items.map(item => ({ key: `chat-${crypto.randomUUID()}`, name: item.name, section_id: item.section_id, amount: moneyInput(item.amount_minor, sheet.currency_exponent), note: item.note, repeat: false, editing: false })));
      if (result.items.length > 0) { setText(''); setAttachment(undefined); }
    } catch (error) { if (active.current) setError(`No items were added. ${errorMessage(error)}`); }
    finally { setProcessing(false); }
  };
  useEffect(() => {
    const outside = (event: PointerEvent) => { if (!root.current?.contains(event.target as Node)) setComposing(false); };
    document.addEventListener('pointerdown', outside);
    return () => document.removeEventListener('pointerdown', outside);
  }, []);
  // Keep the sheet reachable: an idle panel steps aside, pending suggestions stay.
  useEffect(() => {
    if (!expanded || proposals.length || busy || connecting) return;
    const outside = (event: PointerEvent) => { if (!root.current?.contains(event.target as Node)) setExpanded(false); };
    document.addEventListener('pointerdown', outside);
    return () => document.removeEventListener('pointerdown', outside);
  }, [expanded, proposals.length, busy, connecting]);
  const addProposal = async (item: Proposal) => {
    if (!item.name.trim() || new TextEncoder().encode(item.name.trim()).length > 120) throw new Error('Enter an item name of 1 to 120 bytes.');
    if (new TextEncoder().encode(item.note).length > 2000) throw new Error('Keep the note within 2000 bytes.');
    const amount = item.amount.trim() ? parseMoney(item.amount, sheet!.currency_exponent) : null;
    if (amount != null && BigInt(amount) < 0n) throw new Error('Enter zero or a positive amount.');
    return onAdd(item.key, { name: item.name.trim(), section_id: item.section_id, amount_minor: amount, note: item.note, item_id: null, repeat_monthly: item.repeat, usual_amount_minor: null }, () => {
      setProposals(previous => previous.filter(value => value.key !== item.key)); setSaved(previous => previous + 1);
    });
  };
  const isDuplicate = (item: Proposal) => {
    const direction = sheet?.sections.find(section => section.id === item.section_id)?.direction;
    return Boolean(sheet?.sections.some(candidate => candidate.direction === direction && candidate.items.some(existing => normalized(existing.name) === normalized(item.name))));
  };
  const edit = (key: string, values: Partial<Proposal>) => setProposals(previous => previous.map(item => item.key === key ? { ...item, ...values } : item));
  const ready = available && !saving && Boolean(connection?.sharing && model);
  const status = connection?.connected ? connection.sharing ? `Connected${connection.email ? ` as ${connection.email}` : ''}` : 'ChatGPT plan access needed' : 'ChatGPT is not connected';
  const label = formatMonth(month);
  const reviewing = proposals.length > 0 || processing || Boolean(message) || saved > 0;
  return <aside className="transaction-chat" aria-label="Add items with ChatGPT" ref={root}>
    {expanded && proposals.length > 0 && <div className="chat-scrim" aria-hidden="true" />}
    {expanded && <section id={`${id}-panel`} className="chat-panel" aria-labelledby={`${id}-title`}>
      <div className="chat-panel-heading"><div><h2 id={`${id}-title`}>{reviewing ? `Add to ${label}` : connection?.sharing ? 'ChatGPT settings' : 'Add items with ChatGPT'}</h2>{!reviewing && !connection?.sharing && <p className="quiet">Describe items or attach a bill. Nothing is added until you review it.</p>}</div><button type="button" className="icon-button" aria-label="Collapse chat" onClick={() => setExpanded(false)}><Close /></button></div>
      {!proposals.length && <><div className="chat-connection">
        <span className={`chat-connection-status ${connection?.sharing ? 'is-connected' : ''}`}>{status}</span>
        {connection?.connected ? <button type="button" className="quiet-button" disabled={connecting || busy || saving} onClick={async () => {
          setConnecting(true); setConnectionError('');
          try { const result = await api.command<'ChatGPTConnection'>('/chatgpt/disconnect', {}).run(); if (active.current) { setConnection(result); setModel(''); } }
          catch (error) { setConnectionError(errorMessage(error)); }
          finally { setConnecting(false); }
        }}>Disconnect</button> : <>{!connection?.available && <button type="button" className="quiet-button" disabled={!signedIn || busy} onClick={() => void refresh()}>Refresh connection</button>}<button type="button" className="primary chat-connect" data-oai-siwc-button="" disabled={!connection?.available || connecting || !signedIn} onClick={() => void connect()}>{connecting ? 'Waiting for sign-in...' : 'Continue with ChatGPT'}</button></>}
      </div>
      {connection?.reason && <p className="quiet">{connection.reason}</p>}
      {connectionError && <p className="notice error" role="alert">{connectionError}</p>}
      {connection?.connected && !connection.sharing && <p className="notice">This connection has no ChatGPT plan access. Disconnect and reconnect to approve plan usage.</p>}
      {connection?.sharing && <div className="chat-model field"><label htmlFor={`${id}-model`}>ChatGPT model</label><select id={`${id}-model`} value={model} disabled={busy || saving} onChange={event => setModel(event.target.value)}>{!connection.models.length && <option value="">No models available</option>}{connection.models.map(choice => <option key={choice.id} value={choice.id}>{choice.name}</option>)}</select><a href="https://chatgpt.com/settings/usage" target="_blank" rel="noopener noreferrer">Manage usage</a></div>}</>}
      {processing && <p className="notice" role="status">Reading your input with ChatGPT. Nothing has been added yet.</p>}
      {message && <p className="chat-response" role="status">{message}</p>}
      {saved > 0 && <p className="quiet" role="status">{saved} {saved === 1 ? 'item added' : 'items added'} to {label}.</p>}
      {proposals.length > 0 && <div className="chat-proposals"><h3>Review suggested items</h3><p className="quiet">Edit anything that looks wrong. A blank amount stays blank.</p>{proposals.map((item, index) => {
        const section = sheet?.sections.find(section => section.id === item.section_id);
        const duplicate = isDuplicate(item);
        const itemLocked = locked(item.key);
        return <form key={item.key} className="chat-proposal" onFocusCapture={() => { resolvedIndex.current = index; }} aria-label={`Suggested item ${index + 1}`} onSubmit={async event => {
          event.preventDefault(); setError('');
          if (!sheet || duplicate || !ready || itemLocked) return;
          resolvedIndex.current = index;
          try { await addProposal(item); }
          catch (error) { setError(errorMessage(error)); }
        }}><fieldset disabled={itemLocked || busy || adding}>
          {!item.editing && !duplicate && <div className="chat-proposal-summary"><div><strong>{item.name}</strong><small className="chat-proposal-meta"><span>{section?.name}</span>{item.repeat && <span>Monthly</span>}{item.note && <span>{item.note}</span>}</small></div><span className="money">{item.amount.trim() ? groupAmount(item.amount.trim()) : 'Blank'}</span></div>}
          {(item.editing || duplicate) && <><div className="chat-proposal-fields"><div className="field"><label htmlFor={`${item.key}-name`}>Name</label><input id={`${item.key}-name`} value={item.name} required maxLength={120} onChange={event => edit(item.key, { name: event.target.value })} /></div>
          <div className="field"><label htmlFor={`${item.key}-amount`}>{section?.amount_label === 'Bill amount' ? 'Bill amount' : 'Amount'}</label><input id={`${item.key}-amount`} className="amount-input" inputMode="decimal" placeholder="Leave blank" value={item.amount} onChange={event => edit(item.key, { amount: event.target.value })} /></div></div>
          <div className="chat-proposal-fields is-even"><div className="field"><label htmlFor={`${item.key}-section`}>Section</label><select id={`${item.key}-section`} value={item.section_id} required onChange={event => edit(item.key, { section_id: event.target.value })}>{(['in', 'out'] as const).map(direction => <optgroup key={direction} label={`Money ${direction}`}>{sheet?.sections.filter(section => section.direction === direction).map(section => <option key={section.id} value={section.id}>{section.name}</option>)}</optgroup>)}</select></div>
          <div className="field"><label htmlFor={`${item.key}-note`}>Notes</label><input id={`${item.key}-note`} value={item.note} maxLength={2000} onChange={event => edit(item.key, { note: event.target.value })} /></div>
          </div><div className="check-field"><input id={`${item.key}-repeat`} type="checkbox" role="switch" checked={item.repeat} onChange={event => edit(item.key, { repeat: event.target.checked })} /><label htmlFor={`${item.key}-repeat`}>Repeat monthly</label></div></>}
          {duplicate && <p className="notice">An item with this name already exists on this side of the sheet. Edit it in the sheet, or choose a different name here.</p>}
          <div className="actions"><button type="submit" className={proposals.length === 1 ? 'primary' : undefined} disabled={!ready || duplicate}>Add item<span className="sr-only"> to {label}</span></button>{!duplicate && <button type="button" className="quiet-button" onClick={() => edit(item.key, { editing: !item.editing })}>{item.editing ? 'Done' : 'Edit'}<span className="sr-only"> suggestion</span></button>}<button type="button" className="quiet-button" onClick={() => { resolvedIndex.current = index; setProposals(previous => previous.filter(value => value.key !== item.key)); }}>Discard<span className="sr-only"> suggestion</span></button></div>
        </fieldset>{feedback(item.key)}</form>;
      })}</div>}
      {error && <p className="notice error" role="alert">{error}</p>}
    </section>}
    <div className="chat-dock">
      {proposals.length > 0 ? <div className="chat-review-bar"><span>{proposals.length} to review</span>
        {!expanded && <button type="button" className="primary" onClick={() => setExpanded(true)}>Review</button>}
        {expanded && proposals.length > 1 && <><button type="button" className="quiet-button" disabled={busy || adding || proposals.some(item => locked(item.key))} onClick={() => setProposals([])}>Discard all</button>{(adding || proposals.some(item => !isDuplicate(item))) && <button type="button" className="primary" disabled={!ready || busy || adding || proposals.some(item => locked(item.key))} onClick={async () => {
          setError(''); setAdding(true);
          let kept = 0; // Position of the item being added among those still listed, so a failure keeps focus on it.
          try { for (const item of proposals) { if (isDuplicate(item)) { kept++; continue; } resolvedIndex.current = kept; if (!await addProposal(item)) break; } }
          catch (error) { setError(errorMessage(error)); }
          finally { setAdding(false); requestAnimationFrame(focusReview); }
        }}>{adding ? 'Adding…' : (count => count === 1 ? 'Add 1 item' : `Add ${count} items`)(proposals.filter(item => !isDuplicate(item)).length)}</button>}</>}</div> : <>
      {attachment && <div className="chat-attachment"><span>{attachment.name}</span><button type="button" className="quiet-button" aria-label={`Remove ${attachment.name}`} disabled={busy} onClick={() => setAttachment(undefined)}>Remove</button></div>}
      {/* Above the composer: revealing it on focus must not move Send under a pressed pointer. */}
      <p className="chat-disclosure" id={`${id}-hint`}>Sending shares your text and files with OpenAI.</p>
      <form className={`chat-composer ${composing || text || attachment ? '' : 'is-idle'}`} onSubmit={event => { event.preventDefault(); void send(); }}>
        <input ref={fileInput} className="sr-only" tabIndex={-1} aria-label="Choose screenshot or document" type="file" accept="image/png,image/jpeg,image/webp,application/pdf,text/plain" disabled={busy || !signedIn || !available} onChange={event => { void chooseFile(event.target.files?.[0]); event.target.value = ''; }} />
        <button type="button" className="chat-upload" aria-label="ChatGPT settings" title="ChatGPT settings" aria-expanded={expanded} disabled={!signedIn || busy} onClick={() => setExpanded(!expanded)}><Sliders /></button>
        <button type="button" className="chat-upload" aria-label="Upload screenshot or document" disabled={busy || !signedIn || !available} onClick={() => fileInput.current?.click()} title="Attach a screenshot, receipt, bill or document"><Attach /></button>
        <label className="sr-only" htmlFor={`${id}-text`}>Describe items to add</label><textarea id={`${id}-text`} rows={1} maxLength={12000} value={text} disabled={busy || !signedIn || !available} placeholder={available ? 'Add items by chat' : unavailable} aria-describedby={`${id}-hint`} onFocus={() => { setComposing(true); if (!connection?.sharing) setExpanded(true); }} onChange={event => setText(event.target.value)} onKeyDown={event => { if (event.key === 'Escape') setExpanded(false); if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) { event.preventDefault(); void send(); } }} />
        {(text || attachment) && !busy && <button type="button" className="quiet-button chat-clear" onClick={() => { setText(''); setAttachment(undefined); setError(''); }}>Clear</button>}
        <button className="primary chat-send" aria-label={processing ? 'Reading your input' : 'Send'} title={ready ? 'Send' : 'Connect ChatGPT to send'} disabled={!ready || busy || (!text.trim() && !attachment)} type="submit"><Send /></button>
      </form></>}
    </div>
  </aside>;
}
