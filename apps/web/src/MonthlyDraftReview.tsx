import { useEffect, useId, useRef, useState, type ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import { ApiClient, ApiError, errorMessage, isUncertain, type Command, type S } from './lib/api';
import { formatMoney, formatMonth, groupAmount, moneyInput, parseMoney } from './lib/money';
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
  const [discards, setDiscards] = useState(0);
  // Closing with unsaved edits asks first in the footer instead of discarding silently or trapping the user.
  const [confirmClose, setConfirmClose] = useState(false);
  const formId = useId();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const command = useRef<Command<Draft>>(undefined);
  // Saved edits keep the dialog open on the updated proposal; a rejection closes it.
  const keepOpen = useRef(false);
  const drafts = useQuery({ queryKey: ['monthly-drafts', sheet.month], enabled: signedIn, queryFn: () => api.list<'MonthlyDraft'>(`/monthly-drafts?month=${sheet.month}`) });
  useEffect(() => { onDirty(Boolean(selected)); setConfirmClose(false); return () => onDirty(false); }, [selected]);
  const keepEditing = useRef<HTMLButtonElement>(null);
  // autoFocus is unreliable inside an open <dialog>; move focus to the safe choice explicitly.
  useEffect(() => { if (confirmClose) keepEditing.current?.focus(); }, [confirmClose]);
  const reload = async () => { await drafts.refetch(); };
  const run = async () => {
    if (!command.current || busy) return;
    setBusy(true); setError(undefined);
    try {
      const result = await command.current.run(); command.current = undefined;
      if (keepOpen.current) { setSelected(result); setEdited(false); } else setSelected(undefined);
      await reload();
    }
    catch (failure) { setError(failure); if (!isUncertain(failure)) command.current = undefined; }
    finally { setBusy(false); }
  };
  const locked = busy || Boolean(command.current);
  const money = (value: string | null) => formatMoney(value, sheet.currency, sheet.currency_exponent);
  const sectionName = (id: string) => sheet.sections.find(section => section.id === id)?.name;
  const count = drafts.data?.length ?? 0;
  // Name the total that moves and by how much, not just the month.
  const consequence = (draft: Draft) => {
    const total = `money ${sheet.sections.find(section => section.id === draft.section_id)?.direction ?? 'out'}`;
    const month = formatMonth(sheet.month);
    if (draft.recurrence_change) {
      const from = formatMonth(draft.month);
      const start = !draft.repeat_monthly && draft.recurrence_change.base_repeat_monthly
        ? `Accepting stops ${draft.name} repeating from ${from}. Blank ${draft.name} items you haven't touched disappear from ${from} on.`
        : `Accepting applies these settings from ${from} on. Blank ${draft.name} items you haven't touched follow them.`;
      const target = sectionName(draft.section_id);
      const move = draft.section_id === draft.recurrence_change.base_section_id ? '' : ` If ${draft.name} is still on the ${from} sheet, it moves to ${target}, with any amount entered there.`;
      return `${start}${move} Earlier months and entered amounts stay as they are.`;
    }
    if (!draft.amendment) {
      // Mirrors acceptance: a same-direction item with the same name is filled if blank and refused if entered.
      // Acceptance also matches former names, which the sheet does not expose, so a miss cannot promise a new item.
      const direction = sheet.sections.find(section => section.id === draft.section_id)?.direction;
      const key = (name: string) => name.trim().replace(/\s+/g, ' ').toLowerCase();
      const match = sheet.sections.filter(section => section.direction === direction).flatMap(section => section.items).find(item => key(item.name) === key(draft.name));
      const target = sectionName(draft.section_id);
      if (match && draft.repeat_monthly) return `${match.name} is already on the sheet, so this proposal can't be accepted. Reject it, and change ${match.name} on the sheet instead.`;
      if (match?.amount_minor != null) return `${match.name} already has an amount for ${month}, so this proposal can't be accepted. Change the amount on the sheet, or reject this proposal.`;
      if (match) return `Accepting fills the blank ${match.name} item in ${target}, adding ${money(draft.amount_minor)} to ${total} for ${month}.`;
      const amount = draft.amount_minor == null ? `, blank for ${month}, so totals stay the same` : ` with ${money(draft.amount_minor)}, adding to ${total} for ${month}`;
      if (draft.repeat_monthly) return `Accepting adds ${draft.name} to ${target}${amount}. It then appears blank on every later monthly sheet${draft.usual_amount_minor == null ? '' : `, with a usual amount of ${money(draft.usual_amount_minor)}`}.`;
      return `Accepting adds ${draft.name} to ${target}${amount}. If ${draft.name} is a former name of an item on the sheet, that item is filled instead, or acceptance is refused if it already has an amount.`;
    }
    const delta = BigInt(draft.amount_minor ?? '0') - BigInt(draft.amendment.base_amount_minor);
    const change = delta === 0n ? `${total} stays the same` : `${total} ${delta > 0n ? 'goes up' : 'goes down'} by ${money((delta < 0n ? -delta : delta).toString())}`;
    return `Accepting replaces this item's amount and explanations for ${month}, so ${change}. Its name, section, note and recurrence stay as they are.`;
  };
  return <section className={`draft-review ${drafts.data?.length === 0 ? 'is-empty' : ''}`} aria-label="Pending proposals">
    <div className="draft-head"><div><h2>Pending proposals{count > 0 && <span className="draft-count"><span className="sr-only"> (</span>{count}<span className="sr-only">)</span></span>}</h2>
      {drafts.isPending && <p className="section-meta" role="status">Loading proposals…</p>}
      {drafts.data?.length === 0 && <p className="section-meta">No pending proposals for this month.</p>}
      {count > 0 && <p className="section-meta">Totals change only when you accept one.</p>}</div>
      <button className="quiet-button draft-refresh" disabled={disabled || Boolean(selected) || drafts.isFetching} onClick={() => void reload()}>Refresh<span className="sr-only"> proposals</span></button></div>
    {drafts.error && <p className="row-status error draft-error" role="alert">Could not load proposals. {errorMessage(drafts.error)}</p>}
    {count > 0 && <ul className="draft-list">{drafts.data!.map(draft => <li key={draft.id}>
      <div className="draft-main"><span className="draft-name">{draft.name}</span><span className="section-meta"><span>{sectionName(draft.section_id)}</span>{draft.recurrence_change ? <span>{`${draft.repeat_monthly ? 'Recurring item change' : 'Stops repeating'} from ${formatMonth(draft.month)}`}</span> : draft.repeat_monthly && <span>Repeats monthly</span>}{draft.amendment && <span>{draft.amendment.base_amount_minor === draft.amount_minor ? 'Changes the explanations of the entered amount' : `Replaces entered ${groupAmount(moneyInput(draft.amendment.base_amount_minor, sheet.currency_exponent))}`}</span>}{draft.explanations.length > 0 && <span>{draft.explanations.length} {draft.explanations.length === 1 ? 'explanation' : 'explanations'}</span>}</span></div>
      <span className="draft-amount money">{draft.recurrence_change ? null : draft.amount_minor == null ? <span className="draft-blank">Blank</span> : groupAmount(moneyInput(draft.amount_minor, sheet.currency_exponent))}</span>
      <button className="accent-button draft-open" disabled={disabled || Boolean(selected)} onClick={() => { setSelected(draft); setEdited(false); setError(undefined); }}>Review<span className="sr-only"> {draft.name}</span></button>
      {feedback(draft.id)}
    </li>)}</ul>}
    {selected && <Dialog title={`${selected.recurrence_change ? 'Recurring item change' : selected.amendment ? 'Amendment' : 'Proposal'} for ${selected.name}`} locked={locked} onClose={() => { if (edited) setConfirmClose(true); else setSelected(undefined); }}>
      {!selected.recurrence_change && <p className="draft-context section-meta"><span>{sectionName(selected.section_id)}</span><span>{formatMonth(sheet.month)}</span></p>}
      {selected.recurrence_change ? <div className="draft-compare">
        <section aria-label={`Settings for ${selected.name} when proposed`}><h3>When proposed</h3><Settings repeat={selected.recurrence_change.base_repeat_monthly} usual={selected.recurrence_change.base_usual_amount_minor} section={sectionName(selected.recurrence_change.base_section_id)} money={money} /></section>
        <section aria-label={`Proposed settings for ${selected.name}`}><h3>From {formatMonth(selected.month)}</h3><Settings repeat={selected.repeat_monthly} usual={selected.usual_amount_minor} section={sectionName(selected.section_id)} money={money} /></section>
      </div> : selected.amendment ? <div className="draft-compare">
        <section aria-label={`Entered ${selected.name}`}><h3>Entered when proposed</h3><p className="money">{money(selected.amendment.base_amount_minor)}</p>
          {selected.amendment.base_explanations.length > 0 && <Breakdown label={`Entered explanations for ${selected.name}`} amount={selected.amendment.base_amount_minor} parts={selected.amendment.base_explanations} sheet={sheet} />}</section>
        <section aria-label={`Proposed ${selected.name}`}><h3>Proposed</h3><p className="money">{money(selected.amount_minor)}</p>
          {selected.explanations.length > 0 && <Breakdown label={`Explanations for ${selected.name}`} amount={selected.amount_minor ?? '0'} parts={selected.explanations} sheet={sheet} />}</section>
      </div> : <div className="draft-proposed"><p className="draft-figure money">{selected.amount_minor == null ? <span className="draft-blank">Blank this month</span> : money(selected.amount_minor)}</p>
        {selected.repeat_monthly && <p className="draft-repeat">{selected.usual_amount_minor == null ? 'Repeats monthly' : `Repeats monthly, usual ${money(selected.usual_amount_minor)}`}</p>}
        {selected.explanations.length > 0 && <Breakdown label={`Explanations for ${selected.name}`} amount={selected.amount_minor ?? '0'} parts={selected.explanations} sheet={sheet} />}</div>}
      <p className="draft-consequence">{consequence(selected)}</p>
      {selected.recurrence_change && selected.note && <p className="draft-note">Note: {selected.note}</p>}
      {selected.evidence && <figure className="draft-evidence"><blockquote>{selected.evidence}</blockquote>{selected.source_id && <figcaption>Source: {selected.source_id}, {selected.external_id}</figcaption>}</figure>}
      {!selected.recurrence_change && <details className="draft-edit" open={edited || undefined}><summary>Edit before accepting</summary>
      <div onChangeCapture={() => setEdited(true)}><Form key={`${selected.id}-${selected.version}-${discards}`} formId={formId} submit="Save proposal edits" disabled={locked} defaults={{ name: selected.name, section_id: selected.section_id, amount: moneyInput(selected.amount_minor, sheet.currency_exponent), note: selected.note, keep_breakdown: true }} fields={[
        ...(selected.amendment ? [] : [
          { name: 'name', label: 'Item name', required: true },
          { name: 'section_id', label: 'Section', type: 'select' as const, required: true, options: sheet.sections.map(section => ({ value: section.id, label: `${section.name} (money ${section.direction})` })) },
        ]),
        { name: 'amount', label: 'Proposed amount (SGD)', type: 'money', required: !selected.repeat_monthly, hint: selected.repeat_monthly ? 'Optional. Leave blank to add the item without an amount this month.' : undefined },
        { name: 'note', label: 'Note', type: 'textarea' },
        ...(selected.explanations.length ? [{ name: 'keep_breakdown', label: 'Keep explanations', type: 'checkbox' as const, hint: 'Clear this to remove the explanations from this proposal, for example after lowering the amount below them.' }] : []),
      ]} onSubmit={async values => {
        const entered = text(values, 'amount').trim();
        const amount = selected.repeat_monthly && !entered ? null : parseMoney(entered, sheet.currency_exponent);
        if (amount != null && BigInt(amount) < 0n) throw new Error('Enter zero or a positive amount.');
        // An amendment stays on its item; only the amount, note and explanations can change.
        const item = selected.amendment ? { name: selected.name, section_id: selected.section_id } : { name: text(values, 'name'), section_id: text(values, 'section_id') };
        keepOpen.current = true;
        command.current = api.command<'MonthlyDraft'>(`/monthly-drafts/${selected.id}`, { ...item, amount_minor: amount, note: text(values, 'note'), ...(values.keep_breakdown === false ? { explanations: [] } : {}) }, { method: 'PUT', version: selected.version });
        await run();
      }} /></div></details>}
      {error != null && <div className="notice error" role="alert"><span>{errorMessage(error)}</span>{command.current && <button disabled={busy} onClick={() => void run()}>Try again</button>}{error instanceof ApiError && error.status === 412 && <button onClick={() => { setSelected(undefined); void reload(); }}>Reload proposals</button>}</div>}
      <p className="fine-print draft-undo">{edited ? 'Save your edits, then accept or reject the updated proposal.' : selected.recurrence_change ? 'Undo on the sheet reverses an accepted change. To adjust it, accept it and then change the item on the sheet.' : 'Undo on the sheet reverses an accepted amount.'}</p>
      {/* One decision at a time: unsaved edits swap Reject/Accept for Discard/Save. */}
      <div className="dialog-footer">{confirmClose ? <>
        <p className="dialog-footer-note" role="status">Discard your edits?</p>
        <button type="button" className="danger-button" onClick={() => setSelected(undefined)}>Discard and close</button>
        <button type="button" ref={keepEditing} className="primary" onClick={() => setConfirmClose(false)}>Keep editing</button>
      </> : edited ? <>
        <button type="button" disabled={locked} onClick={() => { setDiscards(count => count + 1); setEdited(false); }}>Discard edits</button>
        <button type="submit" form={formId} className="primary" disabled={locked}>Save proposal edits</button>
      </> : <>
        <button disabled={locked} onClick={() => {
          keepOpen.current = false;
          command.current = api.command<'MonthlyDraft'>(`/monthly-drafts/${selected.id}/reject`, {}, { version: selected.version }); void run();
        }}>Reject proposal</button>
        <button className="primary" disabled={locked} onClick={() => {
          const draft = selected;
          setSelected(undefined);
          void onAccept(draft, () => void reload());
        }}>Accept proposal</button>
      </>}</div>
    </Dialog>}
  </section>;
}

function Settings({ repeat, usual, section, money }: { repeat: boolean; usual: string | null; section?: string; money(value: string | null): string }) {
  return <dl className="recurrence-settings">
    <div><dt>Repeats monthly</dt><dd>{repeat ? 'Yes' : 'No'}</dd></div>
    <div><dt>Usual amount</dt><dd>{usual == null ? 'None' : money(usual)}</dd></div>
    <div><dt>Section</dt><dd>{section ?? 'Unknown section'}</dd></div>
  </dl>;
}

function Breakdown({ label, amount, parts, sheet }: { label: string; amount: string; parts: S['MonthlyExplanation'][]; sheet: S['MonthlySheet'] }) {
  const explained = parts.reduce((sum, part) => sum + BigInt(part.amount_minor), 0n);
  const remainder = BigInt(amount) - explained;
  const value = (minor: string) => groupAmount(moneyInput(minor, sheet.currency_exponent));
  return <dl className="explanation-list" aria-label={label}>
    {parts.map((part, index) => <div key={index}><dt>{part.name}</dt><dd>{value(part.amount_minor)}</dd></div>)}
    {remainder > 0n && <div className="unexplained"><dt>Unexplained</dt><dd>{value(remainder.toString())}</dd></div>}
  </dl>;
}
