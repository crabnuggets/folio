import { useEffect, useId, useRef, useState, type ReactNode } from 'react';
import { ApiClient, errorMessage, type S } from './lib/api';
import { moneyInput, parseMoney } from './lib/money';

type Proposal = { key: string; name: string; section_id: string; amount: string; note: string };
type Attachment = NonNullable<S['ChatCaptureInput']['attachment']>;
type Props = {
  api: ApiClient; signedIn: boolean; month: string; sheet?: S['MonthlySheet']; available: boolean; saving: boolean;
  onDirty(value: boolean): void; locked(key: string): boolean; feedback(key: string): ReactNode;
  onAdd(key: string, input: S['MonthlyItemInput'], done: () => void): Promise<boolean>;
};
const fileTypes: Attachment['media_type'][] = ['image/png', 'image/jpeg', 'image/webp', 'application/pdf', 'text/plain'];
const normalized = (value: string) => value.trim().replace(/\s+/g, ' ').toLowerCase();

export function TransactionChat({ api, signedIn, month, sheet, available, saving, onDirty, locked, feedback, onAdd }: Props) {
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
  useEffect(() => { setSaved(0); setMessage(''); setError(''); }, [month]);
  const fileInput = useRef<HTMLInputElement>(null);
  const popup = useRef<Window | null>(null);
  const active = useRef(signedIn);
  active.current = signedIn;
  const busy = reading || processing;
  const dirty = Boolean(text.trim() || attachment || proposals.length || busy);
  useEffect(() => { onDirty(dirty); }, [dirty, onDirty]);
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
      setProposals(result.items.map(item => ({ key: `chat-${crypto.randomUUID()}`, name: item.name, section_id: item.section_id, amount: moneyInput(item.amount_minor, sheet.currency_exponent), note: item.note })));
      setText(''); setAttachment(undefined);
    } catch (error) { if (active.current) setError(`No items were added. ${errorMessage(error)}`); }
    finally { setProcessing(false); }
  };
  const edit = (key: string, values: Partial<Proposal>) => setProposals(previous => previous.map(item => item.key === key ? { ...item, ...values } : item));
  const ready = available && !saving && Boolean(connection?.sharing && model);
  const status = connection?.connected ? connection.sharing ? 'ChatGPT connected' : 'ChatGPT plan access needed' : 'Connect ChatGPT';
  return <aside className="transaction-chat" aria-label="Add items with ChatGPT">
    {expanded && <section id={`${id}-panel`} className="chat-panel" aria-labelledby={`${id}-title`}>
      <div className="chat-panel-heading"><div><h2 id={`${id}-title`}>Add to your monthly sheet</h2><p className="quiet">Review suggestions for {month} before adding them.</p></div><button type="button" className="quiet-button" aria-label="Collapse chat" onClick={() => setExpanded(false)}>Close</button></div>
      <div className="chat-connection">
        <div><strong>{status}</strong>{connection?.email && <p className="quiet">{connection.email}</p>}</div>
        {connection?.connected ? <button type="button" disabled={connecting || busy || saving} onClick={async () => {
          setConnecting(true); setConnectionError('');
          try { const result = await api.command<'ChatGPTConnection'>('/chatgpt/disconnect', {}).run(); if (active.current) { setConnection(result); setModel(''); } }
          catch (error) { setConnectionError(errorMessage(error)); }
          finally { setConnecting(false); }
        }}>Disconnect</button> : <button type="button" data-oai-siwc-button="" disabled={!connection?.available || connecting || !signedIn} onClick={() => void connect()}>{connecting ? 'Waiting for sign-in...' : 'Continue with ChatGPT'}</button>}
        <button type="button" className="quiet-button" disabled={!signedIn || busy} onClick={() => void refresh()}>Refresh connection</button>
      </div>
      {connection?.reason && <p className="quiet">{connection.reason}</p>}
      {connectionError && <p className="notice error" role="alert">{connectionError}</p>}
      {connection?.connected && !connection.sharing && <p className="notice">This connection has no ChatGPT plan access. Disconnect and reconnect to approve plan usage.</p>}
      {connection?.sharing && <><p className="quiet">Using your ChatGPT plan for requests sent from Folio.</p><div className="chat-model field"><label htmlFor={`${id}-model`}>ChatGPT model</label><select id={`${id}-model`} value={model} disabled={busy || saving} onChange={event => setModel(event.target.value)}>{!connection.models.length && <option value="">No models available</option>}{connection.models.map(choice => <option key={choice.id} value={choice.id}>{choice.name}</option>)}</select><a href="https://chatgpt.com/settings/usage" target="_blank" rel="noopener noreferrer">Manage ChatGPT usage</a></div></>}
      {processing && <p className="notice" role="status">Reading your input with ChatGPT. Nothing has been added yet.</p>}
      {message && <p className="chat-response" role="status">{message}</p>}
      {saved > 0 && <p className="quiet" role="status">{saved} {saved === 1 ? 'item added' : 'items added'} to {month}.</p>}
      {proposals.length > 0 && <div className="chat-proposals"><h3>Review suggested items</h3><p className="quiet">Correct the name, section, and amount. A blank amount stays blank.</p>{proposals.map((item, index) => {
        const section = sheet?.sections.find(section => section.id === item.section_id);
        const duplicate = sheet?.sections.some(candidate => candidate.direction === section?.direction && candidate.items.some(existing => normalized(existing.name) === normalized(item.name)));
        const itemLocked = locked(item.key);
        return <form key={item.key} className="chat-proposal" aria-label={`Suggested item ${index + 1}`} onSubmit={async event => {
          event.preventDefault(); setError('');
          if (!sheet || duplicate || !ready || itemLocked) return;
          try {
            if (!item.name.trim() || new TextEncoder().encode(item.name.trim()).length > 120) throw new Error('Enter an item name of 1 to 120 bytes.');
            if (new TextEncoder().encode(item.note).length > 2000) throw new Error('Keep the note within 2000 bytes.');
            const amount = item.amount.trim() ? parseMoney(item.amount, sheet.currency_exponent) : null;
            if (amount != null && BigInt(amount) < 0n) throw new Error('Enter zero or a positive amount.');
            await onAdd(item.key, { name: item.name.trim(), section_id: item.section_id, amount_minor: amount, note: item.note, item_id: null, repeat_monthly: false, usual_amount_minor: null }, () => {
              setProposals(previous => previous.filter(value => value.key !== item.key)); setSaved(previous => previous + 1);
            });
          } catch (error) { setError(errorMessage(error)); }
        }}><fieldset disabled={itemLocked || busy}>
          <div className="chat-proposal-fields"><div className="field"><label htmlFor={`${item.key}-name`}>Item name</label><input id={`${item.key}-name`} value={item.name} required maxLength={120} onChange={event => edit(item.key, { name: event.target.value })} /></div>
          <div className="field"><label htmlFor={`${item.key}-section`}>Section</label><select id={`${item.key}-section`} value={item.section_id} required onChange={event => edit(item.key, { section_id: event.target.value })}>{sheet?.sections.map(section => <option key={section.id} value={section.id}>{section.direction === 'in' ? 'Money in' : 'Money out'} · {section.name}</option>)}</select></div>
          <div className="field"><label htmlFor={`${item.key}-amount`}>{section?.amount_label === 'Bill amount' ? 'Bill amount' : 'Amount'} ({sheet?.currency})</label><input id={`${item.key}-amount`} inputMode="decimal" placeholder="Not entered" value={item.amount} onChange={event => edit(item.key, { amount: event.target.value })} /></div></div>
          <div className="field"><label htmlFor={`${item.key}-note`}>Note</label><input id={`${item.key}-note`} value={item.note} maxLength={2000} onChange={event => edit(item.key, { note: event.target.value })} /></div>
          {duplicate && <p className="notice">An item with this name already exists on this side of the sheet. Edit it in the sheet, or choose a different name here.</p>}
          <div className="actions"><button type="submit" className="primary" disabled={!ready || duplicate}>Add item to {month}</button><button type="button" onClick={() => setProposals(previous => previous.filter(value => value.key !== item.key))}>Discard suggestion</button></div>
        </fieldset>{feedback(item.key)}</form>;
      })}</div>}
      {error && <p className="notice error" role="alert">{error}</p>}
    </section>}
    <div className="chat-dock">
      <div className="chat-dock-heading"><button type="button" className="chat-toggle" aria-expanded={expanded} aria-controls={`${id}-panel`} onClick={() => setExpanded(!expanded)}><svg aria-hidden="true" width="19" height="19" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8"><path d="M5 4h14a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H9l-6 4V6a2 2 0 0 1 2-2Z" /></svg>Chat to add items</button><button type="button" className="quiet-button chat-status" onClick={() => setExpanded(true)}>{status}</button></div>
      <form className="chat-composer" onSubmit={event => { event.preventDefault(); void send(); }}>
        <input ref={fileInput} className="sr-only" tabIndex={-1} aria-label="Choose screenshot or document" type="file" accept="image/png,image/jpeg,image/webp,application/pdf,text/plain" disabled={busy || !signedIn || proposals.length > 0} onChange={event => { void chooseFile(event.target.files?.[0]); event.target.value = ''; }} />
        <button type="button" className="chat-upload" aria-label="Upload screenshot or document" disabled={busy || !signedIn || proposals.length > 0} onClick={() => fileInput.current?.click()} title="Attach a screenshot, receipt, bill, or document"><svg aria-hidden="true" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8"><path d="m8 12 7-7a4 4 0 0 1 6 6L10 22a6 6 0 0 1-8-8L13 3" transform="translate(0 -1) scale(.95)" /></svg><span>Upload</span></button>
        <label className="sr-only" htmlFor={`${id}-text`}>Describe items to add</label><textarea id={`${id}-text`} rows={1} maxLength={12000} value={text} disabled={busy || !signedIn || proposals.length > 0} placeholder={'Try "Salary 5,000 and rent 1,200"'} aria-describedby={`${id}-hint`} onFocus={() => setExpanded(true)} onChange={event => setText(event.target.value)} onKeyDown={event => { if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) { event.preventDefault(); void send(); } }} />
        <button className="primary chat-send" disabled={!ready || busy || proposals.length > 0 || (!text.trim() && !attachment)} type="submit">{processing ? 'Reading...' : 'Send'}</button>
      </form>
      {attachment && <div className="chat-attachment"><span>{attachment.name}</span><button type="button" className="quiet-button" aria-label={`Remove ${attachment.name}`} disabled={busy} onClick={() => setAttachment(undefined)}>Remove</button></div>}
      <div className="chat-hint" id={`${id}-hint`}>Describe an item or upload a screenshot or document. Send shares your input with OpenAI. Review before adding.</div>
      {(text || attachment) && !busy && !proposals.length && <button type="button" className="quiet-button chat-clear" onClick={() => { setText(''); setAttachment(undefined); setError(''); }}>Clear input</button>}
    </div>
  </aside>;
}
