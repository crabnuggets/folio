import { useEffect, useRef, useState, type ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import { ApiClient, ApiError, errorMessage, isUncertain, type Command, type S } from './lib/api';
import { formatMoney, moneyInput, parseMoney } from './lib/money';
import { Form, text } from './components/Form';
import { Dialog } from './components/Dialog';

type Draft = S['MonthlyDraft'];
export function MonthlyDraftReview({ api, sheet, signedIn, disabled, onDirty, onAccept, feedback }: {
  api: ApiClient; sheet: S['MonthlySheet']; signedIn: boolean; disabled: boolean;
  onDirty(value: boolean): void;
  onAccept(draft: Draft, done: () => void): Promise<boolean>;
  feedback(id: string): ReactNode;
}) {
  const [selected, setSelected] = useState<Draft>();
  const [edited, setEdited] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const command = useRef<Command<Draft>>(undefined);
  const drafts = useQuery({ queryKey: ['monthly-drafts', sheet.month], enabled: signedIn, queryFn: () => api.list<'MonthlyDraft'>(`/monthly-drafts?month=${sheet.month}`) });
  useEffect(() => { onDirty(Boolean(selected)); return () => onDirty(false); }, [selected]);
  const reload = async () => { await drafts.refetch(); };
  const run = async () => {
    if (!command.current || busy) return;
    setBusy(true); setError(undefined);
    try { await command.current.run(); command.current = undefined; setSelected(undefined); await reload(); }
    catch (failure) { setError(failure); if (!isUncertain(failure)) command.current = undefined; }
    finally { setBusy(false); }
  };
  const locked = busy || Boolean(command.current);
  return <section className="draft-review" aria-label="Pending proposals">
    <div className="section-heading"><h2>Pending proposals{drafts.data?.length ? ` (${drafts.data.length})` : ''}</h2><button disabled={disabled || Boolean(selected) || drafts.isFetching} onClick={() => void reload()}>Refresh proposals</button></div>
    <p className="fine-print">Proposals from a connected AI assistant stay here until you accept them. Pending and rejected proposals are excluded from totals.</p>
    {drafts.isPending && <p role="status">Loading proposals...</p>}
    {drafts.error && <p role="alert">Could not load proposals. {errorMessage(drafts.error)}</p>}
    {drafts.data?.length === 0 && <p className="fine-print">No pending proposals for this month.</p>}
    <ul className="draft-list">{drafts.data?.map(draft => <li key={draft.id}>
      <div><strong>{draft.name}</strong> <span>{formatMoney(draft.amount_minor, sheet.currency, sheet.currency_exponent)}</span><p>{sheet.sections.find(section => section.id === draft.section_id)?.name}{draft.amendment && ` · ${draft.amendment.base_amount_minor === draft.amount_minor ? 'Changes the breakdown of the entered amount' : `Replaces entered ${formatMoney(draft.amendment.base_amount_minor, sheet.currency, sheet.currency_exponent)}`}`}{draft.explanations.length > 0 && ` · ${draft.explanations.length} explained ${draft.explanations.length === 1 ? 'part' : 'parts'}`}</p></div>
      <button disabled={disabled || Boolean(selected)} onClick={() => { setSelected(draft); setEdited(false); setError(undefined); }}>Review {draft.name}</button>
      {feedback(draft.id)}
    </li>)}</ul>
    {selected && <Dialog title={selected.amendment ? 'Review amendment' : 'Review proposal'} locked={locked} onClose={() => setSelected(undefined)}>
      <p>Source: {selected.source_id} / {selected.external_id}</p>
      {selected.evidence && <blockquote className="draft-evidence">{selected.evidence}</blockquote>}
      {selected.amendment ? <div className="draft-compare">
        <section aria-label={`Entered ${selected.name}`}><h3>Entered when proposed</h3><p className="money">{formatMoney(selected.amendment.base_amount_minor, sheet.currency, sheet.currency_exponent)}</p>
          {selected.amendment.base_explanations.length > 0 && <Breakdown label={`Entered breakdown for ${selected.name}`} amount={selected.amendment.base_amount_minor} parts={selected.amendment.base_explanations} sheet={sheet} />}</section>
        <section aria-label={`Proposed ${selected.name}`}><h3>Proposed</h3><p className="money">{formatMoney(selected.amount_minor, sheet.currency, sheet.currency_exponent)}</p>
          {selected.explanations.length > 0 && <Breakdown label={`Breakdown for ${selected.name}`} amount={selected.amount_minor} parts={selected.explanations} sheet={sheet} />}</section>
      </div> : selected.explanations.length > 0 && <Breakdown label={`Breakdown for ${selected.name}`} amount={selected.amount_minor} parts={selected.explanations} sheet={sheet} />}
      <p>{selected.amendment
        ? `Accepting replaces this item's amount and breakdown for ${sheet.month}. Its name, section, note and recurrence stay as they are. If its amount or breakdown has changed since this proposal, acceptance is refused.`
        : `Accepting adds this amount to ${sheet.month}. A matching blank item is filled without changing its recurrence. An item that already has an amount must be reviewed on the sheet.`}</p>
      <div onChangeCapture={() => setEdited(true)}><Form key={`${selected.id}-${selected.version}`} submit="Save proposal edits" disabled={locked} defaults={{ name: selected.name, section_id: selected.section_id, amount: moneyInput(selected.amount_minor, sheet.currency_exponent), note: selected.note, keep_breakdown: true }} fields={[
        ...(selected.amendment ? [] : [
          { name: 'name', label: 'Item name', required: true },
          { name: 'section_id', label: 'Section', type: 'select' as const, required: true, options: sheet.sections.map(section => ({ value: section.id, label: `${section.name} (money ${section.direction})` })) },
        ]),
        { name: 'amount', label: 'Proposed amount (SGD)', type: 'money', required: true },
        { name: 'note', label: 'Note', type: 'textarea' },
        ...(selected.explanations.length ? [{ name: 'keep_breakdown', label: 'Keep breakdown', type: 'checkbox' as const, hint: 'Clear this to drop the breakdown, for example after lowering the amount below it.' }] : []),
      ]} onSubmit={async values => {
        const amount = parseMoney(text(values, 'amount'), sheet.currency_exponent);
        if (BigInt(amount) < 0n) throw new Error('Enter zero or a positive amount.');
        // An amendment stays on its item; only the amount, note and breakdown can change.
        const item = selected.amendment ? { name: selected.name, section_id: selected.section_id } : { name: text(values, 'name'), section_id: text(values, 'section_id') };
        command.current = api.command<'MonthlyDraft'>(`/monthly-drafts/${selected.id}`, { ...item, amount_minor: amount, note: text(values, 'note'), ...(values.keep_breakdown === false ? { explanations: [] } : {}) }, { method: 'PUT', version: selected.version });
        await run();
      }} /></div>
      {error != null && <div role="alert"><p>{errorMessage(error)}</p>{command.current && <button disabled={busy} onClick={() => void run()}>Retry same review action</button>}{error instanceof ApiError && error.status === 412 && <button onClick={() => { setSelected(undefined); void reload(); }}>Reload proposals</button>}</div>}
      <div className="dialog-footer">
        <button disabled={locked} onClick={() => {
          command.current = api.command<'MonthlyDraft'>(`/monthly-drafts/${selected.id}/reject`, {}, { version: selected.version }); void run();
        }}>Reject proposal</button>
        <button className="primary" disabled={locked || edited} onClick={() => {
          const draft = selected;
          setSelected(undefined);
          void onAccept(draft, () => void reload());
        }}>{selected.amendment ? 'Accept amendment' : 'Accept proposed amount'}</button>
      </div>
      <p className="fine-print">Save any edits before accepting. Use Undo on the sheet to reverse an accepted amount. The source stays marked as reviewed.</p>
    </Dialog>}
  </section>;
}

function Breakdown({ label, amount, parts, sheet }: { label: string; amount: string; parts: S['MonthlyExplanation'][]; sheet: S['MonthlySheet'] }) {
  const explained = parts.reduce((sum, part) => sum + BigInt(part.amount_minor), 0n);
  const remainder = BigInt(amount) - explained;
  return <dl className="explanation-list" aria-label={label}>
    {parts.map((part, index) => <div key={index}><dt>{part.name}</dt><dd>{formatMoney(part.amount_minor, sheet.currency, sheet.currency_exponent)}</dd></div>)}
    {remainder > 0n && <div className="unexplained"><dt>Unexplained</dt><dd>{formatMoney(remainder.toString(), sheet.currency, sheet.currency_exponent)}</dd></div>}
  </dl>;
}
