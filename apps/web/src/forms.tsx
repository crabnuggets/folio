import { Form, nullable, text, type Field, type Values } from './components/Form';
import { entryKinds, planKinds, titleCase, useReview } from './context';
import { type S } from './lib/api';
import { moneyInput, formatMoney } from './lib/money';
import { settlementWorkflow } from './lib/settlement';

const field = (name: string, label: string, type: Field['type'] = 'text', required = false, hint?: string): Field => ({ name, label, type, required, hint });
const options = (values: readonly string[]) => values.map(value => ({ value, label: titleCase(value) }));
export function useOptions() {
  const { data } = useReview();
  const account = (kind?: string) => data.accounts.filter(a => a.active && (!kind || a.kind === kind)).map(a => ({ value: a.id, label: `${a.alias} · ${titleCase(a.kind)}` }));
  const plans = (funding = false) => data.plan.lines.filter(l => funding === l.kind.startsWith('funding_')).map(l => ({ value: l.id, label: `${l.description || titleCase(l.kind)} · ${titleCase(l.kind)}` }));
  return { account, plans };
}

export function EntryForm({ original, initialKind }: { original?: S['Entry']; initialKind?: S['EntryKind'] }) {
  const { api, data, month, perform, blocked, money } = useReview();
  const { account, plans } = useOptions();
  const exponent = data.settings.currency_exponent;
  const receipt = (v: Values) => ['income', 'refund', 'reimbursement_receipt'].includes(text(v, 'kind'));
  const fields: Field[] = [
    { ...field('kind', 'Entry kind', 'select', true), options: options(entryKinds) },
    field('amount', 'Amount', 'money', true, `${data.settings.currency}. Use a decimal point without grouping separators.`),
    field('effective_date', 'Effective date', 'date', true), field('reporting_month', 'Reporting month', 'month', true),
    field('reporting_month_reason', 'Reporting month reason', 'text', false, 'Required when the reporting month differs from the effective date.'),
    { ...field('source_account_id', 'Source account', 'select', true), options: account(), visible: v => !receipt(v) },
    { ...field('destination_account_id', 'Destination account', 'select', true), options: account(), visible: v => text(v, 'kind') !== 'direct_expense' },
    field('category', 'Description / category'),
    { ...field('related_entry_id', 'Original entry', 'select', true), options: data.entries.filter(e => e.active && ['direct_expense', 'allocation'].includes(e.kind)).map(e => ({ value: e.id, label: `${e.category || titleCase(e.kind)} · ${e.effective_date} · ${formatMoney(e.amount_minor, e.currency, exponent)}` })), visible: v => ['refund', 'allocation_withdrawal'].includes(text(v, 'kind')) },
    { ...field('plan_line_id', 'Recognized plan match', 'select'), options: plans(), hint: 'Optional. Match income, direct expense/refund or allocation to its expectation.' },
    { ...field('funding_plan_line_id', 'Funding plan match', 'select'), options: plans(true), hint: 'Separate from recognized spending. Use for a movement crossing your funding boundary.' },
    ...(original ? [field('reason', 'Correction reason', 'textarea', true)] : []),
  ];
  return <><p className="form-intro">{original ? 'The original stays in history. A replacement changes the server totals for the affected open months.' : 'Record a confirmed movement. Folio records what happened; it does not send money.'}</p>
    <Form fields={fields} defaults={{ kind: initialKind ?? original?.kind ?? 'income', amount: moneyInput(original?.amount_minor, exponent), effective_date: original?.effective_date ?? `${month}-01`, reporting_month: original?.reporting_month ?? month,
      source_account_id: original?.source_account_id ?? '', destination_account_id: original?.destination_account_id ?? '', category: original?.category ?? '', related_entry_id: original?.related_entry_id ?? '', plan_line_id: original?.plan_line_id ?? '', funding_plan_line_id: original?.funding_plan_line_id ?? '', reporting_month_reason: original?.reporting_month_reason ?? '',
    }} disabled={blocked || data.month.state === 'closed'} submit={original ? 'Save correction' : 'Save entry'} onSubmit={async values => {
      const kind = text(values, 'kind') as S['EntryKind'];
      const input: S['EntryInput'] = { kind, amount_minor: money(values, 'amount', true), currency: data.settings.currency,
        effective_date: text(values, 'effective_date'), reporting_month: text(values, 'reporting_month'), reporting_month_reason: text(values, 'reporting_month_reason'),
        source_account_id: receipt(values) ? null : nullable(values, 'source_account_id'), destination_account_id: kind === 'direct_expense' ? null : nullable(values, 'destination_account_id'),
        category: text(values, 'category'), related_entry_id: ['refund', 'allocation_withdrawal'].includes(kind) ? nullable(values, 'related_entry_id') : null,
        plan_line_id: nullable(values, 'plan_line_id'), funding_plan_line_id: nullable(values, 'funding_plan_line_id'),
      };
      if (original) await perform('Entry correction', api.command<'Entry', S['EntryCorrection']>(`/entries/${original.id}/corrections`, { reason: text(values, 'reason'), replacement: input }, { version: original.version }));
      else await perform('Entry', api.command<'Entry', S['EntryInput']>('/entries', input));
    }} /></>;
}

export function VoidEntry({ entry }: { entry: S['Entry'] }) {
  const { api, perform, blocked } = useReview();
  return <Form fields={[field('reason', 'Void reason', 'textarea', true)]} submit="Void entry" disabled={blocked} onSubmit={async values => {
    await perform('Entry void', api.command<'Entry', S['EntryCorrection']>(`/entries/${entry.id}/corrections`, { reason: text(values, 'reason'), replacement: null }, { version: entry.version }));
  }}><p>The original record stays in history and stops contributing to totals. Linked records may need reconciliation first.</p></Form>;
}

export function StatementForm({ original }: { original?: S['Statement'] }) {
  const { api, data, month, perform, blocked, money } = useReview();
  const { account, plans } = useOptions();
  const exponent = data.settings.currency_exponent;
  const monetary = ['amount_due_minor', 'minimum_due_minor', 'net_spending_minor', 'new_charges_minor', 'fees_minor', 'interest_minor', 'refunds_minor'] as const;
  const defaults: Values = { account_id: original?.account_id ?? '', cycle_start: original?.cycle_start ?? `${month}-01`, cycle_end: original?.cycle_end ?? `${month}-01`, issue_date: original?.issue_date ?? `${month}-01`, due_date: original?.due_date ?? `${month}-28`, reporting_month: original?.reporting_month ?? month,
    reporting_month_reason: original?.reporting_month_reason ?? '', spending_mode: original?.net_spending_minor != null ? 'net' : original?.new_charges_minor != null ? 'components' : 'unknown', spending_evidence: original?.spending_evidence ?? '', carries_statement_id: original?.carries_statement_id ?? '', plan_line_id: original?.plan_line_id ?? '',
  };
  for (const name of monetary) defaults[name] = moneyInput(original?.[name], exponent);
  const fields: Field[] = [
    { ...field('account_id', 'Card account', 'select', true), options: account('credit_card') },
    field('cycle_start', 'Cycle start', 'date', true), field('cycle_end', 'Cycle end / cutoff', 'date', true), field('issue_date', 'Issue date', 'date', true), field('due_date', 'Due date', 'date', true),
    field('reporting_month', 'Reporting month', 'month', true), field('reporting_month_reason', 'Reporting month reason', 'text', false, 'Required when different from the due month.'),
    field('amount_due_minor', 'Amount due', 'money', true), field('minimum_due_minor', 'Minimum due', 'money'),
    { ...field('spending_mode', 'Spending basis', 'select', true), options: [{ value: 'unknown', label: 'Unknown — needs review' }, { value: 'net', label: 'Known net spending with evidence' }, { value: 'components', label: 'All four cycle components' }] },
    { ...field('net_spending_minor', 'New spending', 'money', true), visible: v => text(v, 'spending_mode') === 'net' },
    ...[['new_charges_minor', 'New charges'], ['fees_minor', 'Fees'], ['interest_minor', 'Interest'], ['refunds_minor', 'Cycle refunds']].map(([name, label]) => ({ ...field(name, label, 'money', true), visible: (v: Values) => text(v, 'spending_mode') === 'components' })),
    { ...field('spending_evidence', 'Spending evidence', 'textarea'), visible: v => text(v, 'spending_mode') !== 'unknown', hint: 'Required for net spending. For example, the statement page and labelled cycle total.' },
    { ...field('plan_line_id', 'Spending plan match', 'select'), options: plans().filter(p => data.plan.lines.find(l => l.id === p.value)?.kind === 'spending') },
    { ...field('carries_statement_id', 'Carried prior statement', 'select'), options: data.statements.filter(s => s.active && s.id !== original?.id).map(s => ({ value: s.id, label: `${data.accounts.find(a => a.id === s.account_id)?.alias ?? 'Card'} · ${s.cycle_end}` })), hint: 'Only when the printed amount due carries a prior payable. Historical spending is retained.' },
    ...(original ? [field('reason', 'Revision reason', 'textarea', true)] : []),
  ];
  return <Form fields={fields} defaults={defaults} disabled={blocked || data.month.state === 'closed'} submit={original ? 'Save revision' : 'Save statement'} onSubmit={async values => {
    const mode = text(values, 'spending_mode');
    const input: S['StatementInput'] = { account_id: text(values, 'account_id'), currency: data.settings.currency, cycle_start: text(values, 'cycle_start'), cycle_end: text(values, 'cycle_end'), issue_date: text(values, 'issue_date'), due_date: text(values, 'due_date'),
      reporting_month: text(values, 'reporting_month'), reporting_month_reason: text(values, 'reporting_month_reason'), amount_due_minor: money(values, 'amount_due_minor'),
      minimum_due_minor: text(values, 'minimum_due_minor') ? money(values, 'minimum_due_minor') : null, net_spending_minor: mode === 'net' ? money(values, 'net_spending_minor') : null,
      new_charges_minor: mode === 'components' ? money(values, 'new_charges_minor') : null, fees_minor: mode === 'components' ? money(values, 'fees_minor') : null, interest_minor: mode === 'components' ? money(values, 'interest_minor') : null, refunds_minor: mode === 'components' ? money(values, 'refunds_minor') : null,
      spending_evidence: text(values, 'spending_evidence'), plan_line_id: nullable(values, 'plan_line_id'), carries_statement_id: nullable(values, 'carries_statement_id'),
    };
    if (original) await perform('Statement revision', api.command<'Statement', S['StatementRevision']>(`/statements/${original.id}/revisions`, { reason: text(values, 'reason'), replacement: input }, { version: original.version }));
    else await perform('Statement', api.command<'Statement', S['StatementInput']>('/statements', input));
  }}><p className="form-intro">Amount due is the payable, not new spending. Leave spending unknown until the cycle components or an evidenced net amount are known.</p></Form>;
}

export function PaymentForm({ statement }: { statement: S['Statement'] }) {
  const { api, data, month, perform, blocked, money } = useReview();
  const { account, plans } = useOptions();
  const balance = data.summary.statements.find(s => s.statement_id === statement.id);
  const existing = data.entries.filter(e => e.active && e.kind === 'card_settlement' && e.destination_account_id === statement.account_id);
  return <Form disabled={blocked || data.month.state === 'closed'} fields={[
    { ...field('mode', 'Payment source', 'select', true), options: [{ value: 'new', label: 'Record a new confirmed payment' }, { value: 'existing', label: 'Link an existing recorded payment' }] },
    { ...field('entry_id', 'Existing payment', 'select', true), options: existing.map(e => ({ value: e.id, label: `${e.effective_date} · ${formatMoney(e.amount_minor, e.currency, data.settings.currency_exponent)} · ${e.id.slice(0, 8)}` })), visible: v => text(v, 'mode') === 'existing' },
    field('amount', 'Payment amount', 'money', true),
    { ...field('source_account_id', 'Paying account', 'select', true), options: account().filter(a => data.accounts.find(item => item.id === a.value)?.kind !== 'credit_card'), visible: v => text(v, 'mode') === 'new' },
    { ...field('effective_date', 'Payment date', 'date', true), visible: v => text(v, 'mode') === 'new', hint: `Must be after the cycle cutoff ${statement.cycle_end}.` },
    { ...field('funding_plan_line_id', 'Funding plan match', 'select'), options: plans(true), visible: v => text(v, 'mode') === 'new' },
  ]} defaults={{ mode: 'new', amount: moneyInput(balance?.remaining_minor, data.settings.currency_exponent), effective_date: `${month}-28` }} submit="Save payment" onSubmit={async values => {
    const amount = money(values, 'amount', true);
    const entry = text(values, 'mode') === 'existing' ? existing.find(e => e.id === text(values, 'entry_id')) : undefined;
    if (text(values, 'mode') === 'existing' && !entry) throw new Error('Select an existing payment.');
    const input: S['EntryInput'] = { kind: 'card_settlement', amount_minor: amount, currency: data.settings.currency, source_account_id: text(values, 'source_account_id'), destination_account_id: statement.account_id, effective_date: text(values, 'effective_date'), category: 'Card settlement', funding_plan_line_id: nullable(values, 'funding_plan_line_id') };
    await perform('Statement payment', settlementWorkflow(api, statement, amount, entry ?? input));
  }}><p className="form-intro">Record a payment already made. Settlement changes funding and the obligation; recognized spending stays unchanged. If linking fails, the recorded payment is retained for retry.</p></Form>;
}

export function AdjustmentForm({ statement }: { statement: S['Statement'] }) {
  const { api, perform, blocked, money, month } = useReview();
  return <Form disabled={blocked} fields={[field('amount', 'Adjustment amount', 'money', true), field('effective_date', 'Adjustment date', 'date', true), field('reason', 'Adjustment reason', 'textarea', true)]} defaults={{ effective_date: `${month}-28` }} submit="Save obligation adjustment" onSubmit={async values => {
    await perform('Obligation adjustment', api.command<'ObligationAdjustment', S['ObligationAdjustmentInput']>(`/statements/${statement.id}/obligation-adjustments`, { amount_minor: money(values, 'amount', true), effective_date: text(values, 'effective_date'), reason: text(values, 'reason') }, { version: statement.version }));
  }}><p>Post-cutoff credits reduce the remaining payable once. This does not change recognized spending. Revise the statement if cycle spending itself is incorrect.</p></Form>;
}

export function AccountForm({ original }: { original?: S['Account'] }) {
  const { api, data, month, perform, blocked } = useReview();
  return <Form disabled={blocked} submit={original ? 'Save account' : 'Save account'} defaults={{ alias: original?.alias ?? '', kind: original?.kind ?? 'bank', active: original?.active ?? true, liquid: original?.include_in_liquid_cash ?? false, boundary_change: !original, boundary_included: false, effective_month: month }} fields={[
    field('alias', 'Account alias', 'text', true), { ...field('kind', 'Account kind', 'select', true), options: options(['bank', 'cash', 'credit_card', 'investment', 'savings']) },
    field('active', 'Active account', 'checkbox'), field('liquid', 'Include in observed spendable cash', 'checkbox'),
    ...(original ? [field('boundary_change', 'Set a funding boundary change', 'checkbox')] : []),
    { ...field('boundary_included', 'Include in funding boundary', 'checkbox'), visible: v => !original || Boolean(v.boundary_change) },
    { ...field('effective_month', 'Boundary effective month', 'month', true), visible: v => !original || Boolean(v.boundary_change), hint: 'Effective-dated. Replaces any change for the same month. The server protects existing financial history.' },
  ]} onSubmit={async values => {
    const boundary = original?.funding_boundary ?? [];
    const input: S['AccountInput'] = { alias: text(values, 'alias'), kind: text(values, 'kind') as S['Account']['kind'], currency: data.settings.currency, active: Boolean(values.active), include_in_liquid_cash: Boolean(values.liquid),
      funding_boundary: !original || values.boundary_change ? [...boundary.filter(change => change.effective_month !== text(values, 'effective_month')), { effective_month: text(values, 'effective_month'), included: Boolean(values.boundary_included) }] : boundary,
    };
    await perform('Account', api.command<'Account', S['AccountInput']>(original ? `/accounts/${original.id}` : '/accounts', input, original ? { method: 'PATCH', version: original.version } : {}));
  }}><p>Use aliases, not account credentials. Designate your main income/bill-paying account as the funding boundary; personal spending envelopes stay outside it.</p></Form>;
}

export function SnapshotForm() {
  const { api, data, perform, blocked, money } = useReview();
  const { account } = useOptions();
  return <Form disabled={blocked} fields={[{ ...field('account_id', 'Snapshot account', 'select', true), options: account() }, field('amount', 'Observed balance', 'money', true), field('observed_at', 'Observed at', 'text', true, 'RFC3339 with timezone, for example 2026-10-28T18:00:00+08:00.'), field('source', 'Observation source')]} defaults={{ observed_at: new Date().toISOString(), source: 'Manual observation' }} submit="Save balance snapshot" onSubmit={async values => {
    await perform('Balance snapshot', api.command<'Snapshot', S['SnapshotInput']>('/balance-snapshots', { account_id: text(values, 'account_id'), amount_minor: money(values, 'amount'), currency: data.settings.currency, observed_at: text(values, 'observed_at'), source: text(values, 'source') }));
  }}><p>A balance is an observation at a point in time. Folio does not reconstruct it from partially tracked entries.</p></Form>;
}

export function PlanLineForm({ original }: { original?: S['PlanLine'] }) {
  const { api, data, month, perform, blocked, money } = useReview();
  const { account } = useOptions();
  return <Form disabled={blocked || data.month.state === 'closed'} fields={[field('description', 'Plan description', 'text', true), { ...field('kind', 'Plan kind', 'select', true), options: options(planKinds) }, field('expected', 'Expected amount', 'money', true), field('remaining', 'Estimated remaining amount', 'money', false, 'Blank lets the server estimate ordinary positive targets. Signed targets/actuals require an explicit remainder.'), field('complete', 'Expectation complete', 'checkbox', false, 'Complete uses actuals as the forecast, including explicit zero actuals.'), { ...field('account_id', 'Plan account', 'select'), options: account(), hint: 'Optional constraint. Card funding outflows should identify the card.' }]} defaults={{ description: original?.description ?? '', kind: original?.kind ?? 'income', expected: moneyInput(original?.expected_minor, data.settings.currency_exponent), remaining: moneyInput(original?.estimated_remaining_minor, data.settings.currency_exponent), complete: original?.complete ?? false, account_id: original?.account_id ?? '' }} submit="Save plan" onSubmit={async values => {
    const line: S['PlanLine'] = { id: original?.id ?? crypto.randomUUID(), kind: text(values, 'kind') as S['PlanKind'], expected_minor: money(values, 'expected'), estimated_remaining_minor: text(values, 'remaining') ? money(values, 'remaining') : null, complete: Boolean(values.complete), account_id: nullable(values, 'account_id'), recurrence_id: original?.recurrence_id ?? null, description: text(values, 'description') };
    const lines = original ? data.plan.lines.map(l => l.id === original.id ? line : l) : [...data.plan.lines, line];
    await perform('Monthly plan', api.command<'Plan', S['PlanInput']>(`/months/${month}/plan`, { lines }, { method: 'PUT', version: data.plan.version }));
  }}><p>Expectations never create income or payments. Match confirmed entries to stable plan lines to replace estimates.</p></Form>;
}

export function ClaimForm({ original }: { original?: S['Claim'] }) {
  const { api, data, perform, blocked, money } = useReview();
  return <Form disabled={blocked} fields={[field('description', 'Claim description', 'text', true), field('amount', 'Claim amount', 'money', true), field('expected_receipt_date', 'Expected receipt date', 'date'), { ...field('expense_entry_id', 'Linked expense', 'select'), options: data.entries.filter(e => e.active && e.kind === 'direct_expense').map(e => ({ value: e.id, label: `${e.category || 'Expense'} · ${e.effective_date}` })) }, { ...field('statement_id', 'Linked statement', 'select'), options: data.statements.filter(s => s.active).map(s => ({ value: s.id, label: `${data.accounts.find(a => a.id === s.account_id)?.alias} · ${s.cycle_end}` })) }, { ...field('state', 'Claim state', 'select', true), options: options(['pending', 'disputed', 'waived']) }]} defaults={{ description: original?.description ?? '', amount: moneyInput(original?.amount_minor, data.settings.currency_exponent), expected_receipt_date: original?.expected_receipt_date ?? '', expense_entry_id: original?.expense_entry_id ?? '', statement_id: original?.statement_id ?? '', state: ['disputed', 'waived'].includes(original?.state ?? '') ? original!.state : 'pending' }} submit="Save claim" onSubmit={async values => {
    const input: S['ClaimInput'] = { description: text(values, 'description'), amount_minor: money(values, 'amount', true), currency: data.settings.currency, expected_receipt_date: nullable(values, 'expected_receipt_date'), expense_entry_id: nullable(values, 'expense_entry_id'), statement_id: nullable(values, 'statement_id'), state: text(values, 'state') as S['Claim']['state'] };
    if (input.expense_entry_id && input.statement_id) throw new Error('Link one recognized cost: an expense or a statement.');
    await perform('Claim', api.command<'Claim', S['ClaimInput']>(original ? `/claims/${original.id}` : '/claims', input, original ? { method: 'PATCH', version: original.version } : {}));
  }}><p>A pending claim is not received cash. Receipt matching determines received status and can classify recoveries against a linked cost.</p></Form>;
}

export function ReceiptMatchForm({ claim }: { claim: S['Claim'] }) {
  const { api, data, perform, blocked, money, month } = useReview();
  return <Form disabled={blocked} fields={[{ ...field('receipt_entry_id', 'Reimbursement receipt', 'select', true), options: data.entries.filter(e => e.active && e.kind === 'reimbursement_receipt').map(e => ({ value: e.id, label: `${e.effective_date} · ${formatMoney(e.amount_minor, e.currency, data.settings.currency_exponent)} · ${e.category}` })) }, field('amount', 'Matched amount', 'money', true), field('recovery_month', 'Recovery month', 'month', false, 'Separate from cash receipt month. Leave blank if the recognized cost is not yet classified. Must be open.')]} defaults={{ recovery_month: claim.expense_entry_id || claim.statement_id ? month : '' }} submit="Match receipt" onSubmit={async values => {
    await perform('Receipt match', api.command<'ReceiptMatch', S['ReceiptMatchInput']>(`/claims/${claim.id}/receipt-matches`, { receipt_entry_id: text(values, 'receipt_entry_id'), amount_minor: money(values, 'amount', true), recovery_month: nullable(values, 'recovery_month') }, { version: claim.version }));
  }}><p>Match an already recorded cash receipt. This cannot create a second receipt. To classify an earlier unrecovered match, repeat its receipt and amount with an explicit recovery month.</p></Form>;
}

export function DraftForm() {
  const { api, perform, blocked } = useReview();
  return <Form disabled={blocked} fields={[field('source_id', 'Source identity', 'text', true), field('external_id', 'External identity', 'text', true), { ...field('kind', 'Draft kind', 'select', true), options: options(['entry', 'statement']) }, field('candidate', 'Candidate JSON', 'textarea', true, 'Use API snake_case fields and integer-string minor units. Missing values remain staged; acceptance validates the full posting.'), field('evidence', 'Source evidence', 'textarea', true)]} defaults={{ source_id: 'manual', kind: 'entry' }} submit="Stage draft" onSubmit={async values => {
    const candidate: unknown = JSON.parse(text(values, 'candidate'));
    if (!candidate || typeof candidate !== 'object' || Array.isArray(candidate)) throw new Error('Candidate must be a JSON object.');
    const input: S['DraftInput'] = { source_id: text(values, 'source_id'), external_id: text(values, 'external_id'), evidence: text(values, 'evidence'),
      ...(text(values, 'kind') === 'entry' ? { entry: candidate as S['DraftEntryCandidate'] } : { statement: candidate as S['DraftStatementCandidate'] }),
    };
    await perform('Structured draft', api.command<'Draft', S['DraftInput']>('/drafts', input));
  }}><p>Structured manual staging only. No parsing or Gmail connection is available. Drafts do not affect totals until explicitly accepted.</p></Form>;
}
