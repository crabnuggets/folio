import { useState, type ReactNode } from 'react';
import { Form, text, type Field } from './components/Form';
import { titleCase, useReview } from './context';
import { AccountForm, AdjustmentForm, ClaimForm, DraftForm, EntryForm, PaymentForm, PlanLineForm, ReceiptMatchForm, SnapshotForm, StatementForm, VoidEntry } from './forms';
import type { S } from './lib/api';
import { formatMoney } from './lib/money';

function useDisplay() {
  const { data } = useReview();
  return {
    money: (amount: string | null | undefined) => formatMoney(amount, data.settings.currency, data.settings.currency_exponent),
    account: (id: string | null | undefined) => data.accounts.find(a => a.id === id)?.alias ?? (id ? 'Unknown account' : 'External'),
  };
}
function Empty({ children }: { children: ReactNode }) { return <p className="empty">{children}</p>; }
function Section({ title, children, action, className = '' }: { title: string; children: ReactNode; action?: ReactNode; className?: string }) {
  return <section className={`panel ${className}`}><div className="section-heading"><h2>{title}</h2>{action}</div>{children}</section>;
}
function Metric({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return <div className="metric"><dt>{label}</dt><dd>{value}</dd>{hint && <small>{hint}</small>}</div>;
}
function MissingItems() {
  const { data } = useReview();
  const { account } = useDisplay();
  const labels: Record<string, string> = {
    income_not_reviewed: 'Income needs review', cards_not_reviewed: 'Cards need review', expenses_not_reviewed: 'Major expenses need review', allocations_not_reviewed: 'Allocations need review',
    income_review_missing: 'Income needs review', cards_review_missing: 'Cards need review', expenses_review_missing: 'Major expenses need review', allocations_review_missing: 'Allocations need review',
    funding_boundary_missing: 'No funding accounts designated', no_funding_accounts: 'No funding accounts designated',
    statement_missing: 'Statement missing', statement_spending_unknown: 'Statement spending is unknown', obligation_funding_plan_missing: 'Unpaid statement needs a card-specific funding plan',
    planned_item_pending: 'Expected item still pending', unmatched_receipt: 'Receipt needs cost matching',
  };
  const describe = (item: string) => {
    const [code, id] = item.split(':');
    const line = data.plan.lines.find(l => l.id === id);
    const statement = data.statements.find(s => s.id === id);
    return `${labels[code] ?? titleCase(code)}${id ? ` · ${line?.description || (statement ? account(statement.account_id) : data.accounts.some(a => a.id === id) ? account(id) : id.slice(0, 8))}` : ''}`;
  };
  return data.summary.missing_items.length ? <ul className="review-list">{data.summary.missing_items.map(item => <li key={item}><span className="review-dot" aria-hidden="true" />{describe(item)}</li>)}</ul> : <p className="quiet">No coverage gaps reported by the server. Closing remains your explicit review assertion.</p>;
}

export function Overview() {
  const { data, open, blocked } = useReview();
  const { money, account } = useDisplay();
  const s = data.summary;
  const closed = data.month.state === 'closed';
  return <>
    <section className="funding-hero" aria-label="Monthly funding summary"><div className="hero-top"><span>Monthly funding remainder</span><span className="status review">{s.funding_complete ? 'Reviewed coverage' : 'Provisional coverage'}</span></div>
      <p className="hero-amount" data-testid="funding-remainder">{money(s.funding_remainder_minor)}</p>
      <p className="hero-description">Confirmed money crossing your funding boundary this month.<br />Excludes opening cash. This is not an account balance.</p>
      <dl className="hero-metrics"><Metric label="Confirmed inflows" value={money(s.funding_inflows_minor)} /><Metric label="Confirmed outflows" value={money(s.funding_outflows_minor)} /><Metric label="Projected funding remainder" value={money(s.projected_funding_remainder_minor)} hint="Actuals plus expected remaining movements" /></dl>
    </section>
    <div className="overview-grid"><Section title="The spending view" action={<a href="#Records">View records →</a>}>
      <p className="quiet">Recognized costs and allocations, independently of payment timing.</p>
      <dl className="metric-grid"><Metric label="Recorded income" value={money(s.income_minor)} /><Metric label="Recognized spending" value={s.spending_complete ? money(s.gross_spending_minor) : 'Unknown'} hint={!s.spending_complete ? `Known subtotal: ${money(s.known_spending_minor)}` : 'Gross costs before recoveries'} /><Metric label="Completed allocations" value={money(s.allocation_minor)} hint="Savings and investment contributions" /><Metric label="Monthly surplus" value={money(s.monthly_surplus_minor)} hint={s.monthly_surplus_minor == null ? `Provisional subtotal: ${money(s.provisional_surplus_minor)}` : 'Recorded income less spending and allocations'} /></dl>
      <div className="divider" /><dl className="metric-grid compact"><Metric label="Projected surplus" value={money(s.projected_surplus_minor)} /><Metric label="Confirmed recoveries" value={money(s.confirmed_recoveries_minor)} /><Metric label="Known net personal cost" value={money(s.net_personal_cost_minor)} hint="Known costs only; may be incomplete" /><Metric label="Savings rate" value={s.savings_rate == null ? 'Unknown' : `${s.savings_rate} ratio`} /></dl>
    </Section><Section title="Your review, at a glance" action={<a href={`#${encodeURIComponent('Plan & review')}`}>Review month →</a>}><p className="quiet">{s.is_provisional ? 'A few things still need your attention.' : 'Coverage has been reviewed.'}</p><MissingItems /><div className="subtle-box"><strong>{data.drafts.filter(d => d.state === 'pending').length} pending drafts</strong><p>Proposals stay outside your totals until you accept them.</p><a href="#Drafts">Open review inbox →</a></div></Section></div>
    <Section title="Card obligations" action={<a href="#Statements">View statements →</a>}><p className="quiet">Statement spending is recognized once. A recorded payment settles the payable.</p><StatementRows compact /></Section>
    <Section title="Observed spendable cash" action={<button disabled={blocked} onClick={() => open('Add balance snapshot', <SnapshotForm />)}>Add balance snapshot</button>}>
      <p className="quiet">Latest server-selected snapshots for included liquid accounts, as of this reporting month. No total is inferred from missing observations.</p>
      {s.snapshots.length ? <div className="balance-grid">{s.snapshots.map(snapshot => <div className="balance-item" key={snapshot.id}><h3>{account(snapshot.account_id)}</h3><strong>{money(snapshot.amount_minor)}</strong><small>Observed {new Date(snapshot.observed_at).toLocaleString()}<br />{snapshot.source}</small></div>)}</div> : <Empty>No included balances observed yet. Record a dated balance snapshot in Accounts & settings.</Empty>}
    </Section>
    {!data.accounts.length && <div className="notice"><strong>Start with your accounts.</strong><p>Add your funding account, cards, and savings or investment destinations to begin this month.</p><a className="button primary" href={`#${encodeURIComponent('Accounts & settings')}`}>Set up accounts</a></div>}
    {!closed && <div className="next-action"><div><strong>Ready for the next step?</strong><p>Check expectations and review coverage before closing this month.</p></div><a className="button" href={`#${encodeURIComponent('Plan & review')}`}>Plan & review →</a></div>}
  </>;
}

export function Records() {
  const { data, month, open, blocked } = useReview();
  const { money, account } = useDisplay();
  const [history, setHistory] = useState(false);
  const entries = data.entries.filter(e => e.reporting_month === month && (history || e.active)).sort((a, b) => b.effective_date.localeCompare(a.effective_date));
  const closed = data.month.state === 'closed';
  return <><Section title="Confirmed entries" action={<label className="inline-check"><input type="checkbox" checked={history} onChange={e => setHistory(e.target.checked)} />Show correction history</label>}>
    <p className="quiet">Income, direct expenses, allocations, envelope transfers, returns, refunds and settlement movements. Statement spending is listed separately.</p>
    {entries.length ? <div className="table-scroll"><table><thead><tr><th scope="col">Entry</th><th scope="col">Date / accounts</th><th scope="col" className="numeric">Amount</th><th scope="col">Review</th></tr></thead><tbody>{entries.map(entry => <tr key={entry.id} className={!entry.active ? 'historic' : ''}><th scope="row">{entry.category || titleCase(entry.kind)}<small>{titleCase(entry.kind)} · {entry.active ? 'Confirmed' : 'Superseded / voided'}</small></th><td>{entry.effective_date}<small>{account(entry.source_account_id)} → {account(entry.destination_account_id)}</small></td><td className="numeric">{money(entry.amount_minor)}</td><td><button onClick={() => open('Entry details', <EntryDetails entry={entry} />)}>View entry<span className="sr-only"> {entry.category || titleCase(entry.kind)} {entry.effective_date}</span></button></td></tr>)}</tbody></table></div> : <Empty>No confirmed entries for {month}. Add income or another confirmed movement to begin.</Empty>}
    <div className="actions"><button disabled={blocked || closed} onClick={() => open('Add entry', <EntryForm />)}>Add entry</button><button disabled={blocked || closed} onClick={() => open('Record envelope funding', <EntryForm initialKind="envelope_funding" />)}>Fund spending envelope</button><button disabled={blocked || closed} onClick={() => open('Record envelope return', <EntryForm initialKind="envelope_return" />)}>Record envelope return</button></div>
  </Section><Section title="Audit history"><p className="quiet">Server-recorded changes, including retained correction and revision reasons.</p>
    {data.audit.length ? <ol className="audit-list">{[...data.audit].sort((a, b) => b.at.localeCompare(a.at)).map(event => <li key={event.id}><div><strong>{titleCase(event.action.replaceAll('.', '_'))}</strong><small>{new Date(event.at).toLocaleString()} · {event.entity_id}</small>{event.reason && <p>{event.reason}</p>}</div><details><summary>Before / after</summary><pre>{JSON.stringify({ before: event.before, after: event.after }, null, 2)}</pre></details></li>)}</ol> : <Empty>No changes recorded.</Empty>}
  </Section></>;
}

function EntryDetails({ entry }: { entry: S['Entry'] }) {
  const { data, open, blocked } = useReview();
  const { money, account } = useDisplay();
  return <><dl className="details"><Metric label="Entry kind" value={titleCase(entry.kind)} /><Metric label="Amount" value={money(entry.amount_minor)} /><Metric label="Effective date" value={entry.effective_date} /><Metric label="Reporting month" value={entry.reporting_month} /><Metric label="Source account" value={account(entry.source_account_id)} /><Metric label="Destination account" value={account(entry.destination_account_id)} /><Metric label="Status" value={entry.active ? 'Confirmed' : 'Superseded / voided'} /><Metric label="Version" value={String(entry.version)} /></dl>
    <p>{entry.category}</p>{entry.reporting_month_reason && <p>Reporting reason: {entry.reporting_month_reason}</p>}{entry.correction_reason && <p>Correction reason: {entry.correction_reason}</p>}<p className="fine-print">Record {entry.id}{entry.supersedes_id && ` · Replaces ${entry.supersedes_id}`}</p>
    <div className="actions"><button disabled={blocked || !entry.active || data.month.state === 'closed'} onClick={() => open('Correct entry', <EntryForm original={entry} />)}>Correct entry</button><button disabled={blocked || !entry.active || data.month.state === 'closed'} onClick={() => open('Void entry', <VoidEntry entry={entry} />)}>Void entry</button></div></>;
}

function StatementRows({ compact = false }: { compact?: boolean }) {
  const { data, month, open, blocked } = useReview();
  const { money, account } = useDisplay();
  const statements = data.statements.filter(s => s.active && (s.reporting_month === month || s.due_date.startsWith(month)));
  return statements.length ? <div className="statement-list">{statements.map(statement => {
    const balance = data.summary.statements.find(s => s.statement_id === statement.id);
    return <article className="statement-row" key={statement.id}><div className="statement-identity"><span className="card-icon" aria-hidden="true">▤</span><div><h3>{account(statement.account_id)}</h3><small>Due {statement.due_date} · Cycle ends {statement.cycle_end}</small></div></div><dl><Metric label="New spending" value={money(balance?.spending_minor)} /><Metric label="Amount due" value={money(statement.amount_due_minor)} /><Metric label="Remaining obligation" value={money(balance?.remaining_minor)} hint={balance?.payable_active === false ? 'Prior payable retired' : undefined} />{!compact && <Metric label="Excess paid / credit" value={money(balance?.credit_minor)} />}</dl><div className="actions"><button onClick={() => open('Statement details', <StatementDetails statement={statement} />)}>View statement<span className="sr-only"> {account(statement.account_id)}</span></button>{!compact && <button disabled={blocked || !statement.payable_active || data.month.state === 'closed'} onClick={() => open('Record payment', <PaymentForm statement={statement} />)}>Record payment<span className="sr-only"> {account(statement.account_id)}</span></button>}</div></article>;
  })}</div> : <Empty>No active statements for this month. Missing statements stay visible in review coverage.</Empty>;
}

export function Statements() {
  const { data, month, open, blocked } = useReview();
  const { account } = useDisplay();
  return <><Section title="This month’s statements" action={<button className="primary" disabled={blocked || data.month.state === 'closed'} onClick={() => open('Add statement', <StatementForm />)}>Add statement</button>}><StatementRows /></Section>
    <Section title="Statement history"><p className="quiet">Revisions retain previous amounts and reasons. Other months remain available through the month selector.</p>{data.statements.filter(s => !s.active && s.reporting_month === month).map(statement => <div className="history-row" key={statement.id}><div><strong>{account(statement.account_id)} · {statement.cycle_end}</strong><small>Superseded · {statement.revision_reason || 'Original statement'}</small></div><button onClick={() => open('Historical statement', <StatementDetails statement={statement} />)}>View history</button></div>)}{!data.statements.some(s => !s.active && s.reporting_month === month) && <Empty>No statement revisions for this month.</Empty>}</Section></>;
}

function StatementDetails({ statement }: { statement: S['Statement'] }) {
  const { data, open, blocked } = useReview();
  const { money, account } = useDisplay();
  const balance = data.summary.statements.find(s => s.statement_id === statement.id);
  return <><h3>{account(statement.account_id)}</h3><dl className="details"><Metric label="New spending" value={money(balance?.spending_minor)} /><Metric label="Amount due" value={money(statement.amount_due_minor)} /><Metric label="Minimum due" value={money(statement.minimum_due_minor)} /><Metric label="Remaining obligation" value={money(balance?.remaining_minor)} /><Metric label="Excess paid / credit" value={money(balance?.credit_minor)} /><Metric label="Reporting month" value={statement.reporting_month} /><Metric label="Cycle" value={`${statement.cycle_start} → ${statement.cycle_end}`} /><Metric label="Issue / due dates" value={`${statement.issue_date} / ${statement.due_date}`} /></dl>
    <div className="subtle-box"><h3>Spending evidence</h3><p className="preserve-lines">{statement.spending_evidence || 'Not recorded. Amount due is not inferred to be new spending.'}</p><dl className="details"><Metric label="New charges" value={money(statement.new_charges_minor)} /><Metric label="Fees" value={money(statement.fees_minor)} /><Metric label="Interest" value={money(statement.interest_minor)} /><Metric label="Cycle refunds" value={money(statement.refunds_minor)} /><Metric label="Evidenced net spending" value={money(statement.net_spending_minor)} /></dl></div>
    {statement.revision_reason && <p>Revision reason: {statement.revision_reason}</p>}{statement.reporting_month_reason && <p>Reporting month reason: {statement.reporting_month_reason}</p>}
    <p className="fine-print">Record {statement.id} · Version {statement.version}{statement.supersedes_id && ` · Replaces ${statement.supersedes_id}`}{statement.carries_statement_id && ` · Carries ${statement.carries_statement_id}`}</p>
    <div className="actions"><button className="primary" disabled={blocked || !statement.active || !statement.payable_active || data.month.state === 'closed'} onClick={() => open('Record payment', <PaymentForm statement={statement} />)}>Record payment</button><button disabled={blocked || !statement.active || data.month.state === 'closed'} onClick={() => open('Revise statement', <StatementForm original={statement} />)}>Revise statement</button><button disabled={blocked || !statement.active || data.month.state === 'closed'} onClick={() => open('Adjust obligation', <AdjustmentForm statement={statement} />)}>Adjust obligation</button></div>
    <h3>Recorded payment movements</h3><p className="quiet">Movements to this card. A movement can exist before linkage; the remaining obligation above reflects confirmed links from the server.</p>
    {data.entries.filter(e => e.active && e.kind === 'card_settlement' && e.destination_account_id === statement.account_id).map(entry => <div className="history-row" key={entry.id}><span>{entry.effective_date} · {money(entry.amount_minor)}<small>Payment {entry.id}</small></span><button onClick={() => open('Payment movement', <EntryDetails entry={entry} />)}>View payment</button></div>)}
  </>;
}

export function PlanReview() {
  const { data, open, blocked } = useReview();
  const { money, account } = useDisplay();
  const closed = data.month.state === 'closed';
  return <><Section title="Expected versus actual" action={<button disabled={blocked || closed} onClick={() => open('Add plan line', <PlanLineForm />)}>Add plan line</button>}><p className="quiet">Recognized spending and funding expectations are separate. Only matched actuals replace an estimate. A recurrence or plan is never evidence of payment.</p>
    {data.plan.lines.length ? <div className="table-scroll"><table><thead><tr><th scope="col">Expectation</th><th scope="col" className="numeric">Expected</th><th scope="col" className="numeric">Matched actual</th><th scope="col" className="numeric">Remaining</th><th scope="col" className="numeric">Forecast</th><th scope="col">Review</th></tr></thead><tbody>{data.plan.lines.map(line => {
      const actual = data.summary.forecast_lines.find(f => f.id === line.id);
      return <tr key={line.id}><th scope="row">{line.description || titleCase(line.kind)}<small>{titleCase(line.kind)}{line.account_id && ` · ${account(line.account_id)}`}</small></th><td className="numeric">{money(line.expected_minor)}</td><td className="numeric">{money(actual?.actual_minor)}</td><td className="numeric">{money(actual?.remaining_minor)}</td><td className="numeric">{money(actual?.forecast_minor)}</td><td><button disabled={blocked || closed} onClick={() => open('Edit plan line', <PlanLineForm original={line} />)}>Edit expectation<span className="sr-only"> {line.description}</span></button><small>{line.complete ? 'Complete' : 'Pending'}</small></td></tr>;
    })}</tbody></table></div> : <Empty>No expectations yet. Add expected salary, spending, allocations and funding commitments; then link confirmed entries to them.</Empty>}
    <dl className="metric-grid compact"><Metric label="Projected funding remainder" value={money(data.summary.projected_funding_remainder_minor)} /><Metric label="Projected surplus" value={money(data.summary.projected_surplus_minor)} /></dl>
  </Section><Section title="Coverage and monthly review"><MissingItems /><div className="divider" />{closed ? <><p>Closed {data.month.closed_at ? new Date(data.month.closed_at).toLocaleString() : ''} · Revision {data.month.revision}.</p><p>Closing records your review assertion. Unknown statement spending remains unknown.</p><button disabled={blocked} onClick={() => open('Reopen month', <ReopenMonth />)}>Reopen month</button></> : <CloseMonth />}</Section></>;
}

function CloseMonth() {
  const { api, data, month, perform, blocked } = useReview();
  const missingCards = data.accounts.filter(a => a.active && a.kind === 'credit_card' && !data.statements.some(s => s.active && s.account_id === a.id && s.reporting_month === month));
  const checks: [keyof S['Checklist'], string][] = [['income_reviewed', 'Income reviewed'], ['cards_reviewed', 'Cards reviewed'], ['expenses_reviewed', 'Major expenses reviewed'], ['allocations_reviewed', 'Allocations reviewed']];
  const fields: Field[] = [...checks.map(([name, label]) => ({ name, label, type: 'checkbox' as const, required: true })), ...missingCards.map(card => ({ name: `no_statement_${card.id}`, label: `No statement reason for ${card.alias}`, type: 'text' as const, required: true, hint: 'An explicit reason is required if this active card has no statement this month.' }))];
  return <Form disabled={blocked} submit="Close month" fields={fields} defaults={Object.fromEntries([...checks.map(([name]) => [name, Boolean(data.month.checklist[name])]), ...missingCards.map(card => [`no_statement_${card.id}`, data.month.checklist.no_statement_reasons?.[card.id] ?? ''])])} onSubmit={async values => {
    const checklist: S['Checklist'] = { income_reviewed: Boolean(values.income_reviewed), cards_reviewed: Boolean(values.cards_reviewed), expenses_reviewed: Boolean(values.expenses_reviewed), allocations_reviewed: Boolean(values.allocations_reviewed), no_statement_reasons: Object.fromEntries(missingCards.map(card => [card.id, text(values, `no_statement_${card.id}`)])) };
    await perform('Month close', api.command<'Month', S['MonthTransition']>(`/months/${month}/close`, { checklist }, { version: data.month.version }));
  }}><p>Review {month} before closing. Confirm each area, then close the month to protect it from financial writes. This asserts review; it does not prove exhaustive bank coverage.</p></Form>;
}

function ReopenMonth() {
  const { api, data, month, perform, blocked } = useReview();
  return <Form disabled={blocked} fields={[{ name: 'reason', label: 'Reopen reason', type: 'textarea', required: true }]} submit="Reopen month" onSubmit={async values => {
    await perform('Month reopen', api.command<'Month', S['MonthTransition']>(`/months/${month}/reopen`, { reason: text(values, 'reason') }, { version: data.month.version }));
  }}><p>Reopening {month} allows corrections and advances the summary revision. The reason is retained in audit history.</p></Form>;
}

export function Accounts() {
  const { api, data, open, perform, blocked } = useReview();
  const { money, account } = useDisplay();
  return <><Section title="Your accounts" action={<button className="primary" disabled={blocked} onClick={() => open('Add account', <AccountForm />)}>Add account</button>}>
    {data.accounts.length ? <div className="account-grid">{data.accounts.map(item => <article className="account-card" key={item.id}><span className="account-kind">{titleCase(item.kind)}</span><h3>{item.alias}</h3><p>{item.currency} · {item.active ? 'Active' : 'Archived'}{item.include_in_liquid_cash ? ' · Observed spendable cash' : ''}</p><details><summary>Funding boundary history</summary>{item.funding_boundary.length ? <ul>{item.funding_boundary.map((boundary, index) => <li key={index}>{boundary.effective_month}: {boundary.included ? 'Included' : 'Outside funding boundary'}</li>)}</ul> : <p>Outside funding boundary</p>}</details><button disabled={blocked} onClick={() => open('Edit account', <AccountForm original={item} />)}>Edit account<span className="sr-only"> {item.alias}</span></button></article>)}</div> : <Empty>No accounts yet. Add a main funding account, your cards, and any savings or investment destinations.</Empty>}
  </Section><Section title="Balance observations" action={<button disabled={blocked} onClick={() => open('Add balance snapshot', <SnapshotForm />)}>Add balance snapshot</button>}>
    {data.snapshots.length ? <div className="table-scroll"><table><thead><tr><th scope="col">Account</th><th scope="col" className="numeric">Observed balance</th><th scope="col">Observed at</th><th scope="col">Source</th></tr></thead><tbody>{[...data.snapshots].sort((a, b) => b.observed_at.localeCompare(a.observed_at)).map(snapshot => <tr key={snapshot.id}><th scope="row">{account(snapshot.account_id)}</th><td className="numeric">{money(snapshot.amount_minor)}</td><td>{new Date(snapshot.observed_at).toLocaleString()}</td><td>{snapshot.source}</td></tr>)}</tbody></table></div> : <Empty>No balance observations recorded. A missing observation is not a zero balance.</Empty>}
  </Section><Section title="Settings"><Form disabled={blocked} fields={[{ name: 'currency', label: 'Base currency', required: true, hint: 'ISO currency code. Immutable once financial data exists.' }, { name: 'timezone', label: 'Timezone', required: true, hint: 'IANA timezone, for example Asia/Singapore.' }]} defaults={{ currency: data.settings.currency, timezone: data.settings.timezone }} submit="Save settings" onSubmit={async values => {
    await perform('Settings', api.command<'Settings', S['SettingsInput']>('/settings', { currency: text(values, 'currency').toUpperCase(), timezone: text(values, 'timezone') }, { method: 'PATCH', version: data.settings.version }));
  }} /></Section></>;
}

export function Claims() {
  const { data, open, blocked } = useReview();
  const { money } = useDisplay();
  return <><Section title="Claims & reimbursements" action={<button disabled={blocked} className="primary" onClick={() => open('Add claim', <ClaimForm />)}>Add claim</button>}><p className="quiet">Pending claims never count as income or received cash. Record a reimbursement receipt, then match it once to a claim. Claims below include the undated backlog.</p>
    <dl className="metric-grid"><Metric label="Outstanding claims this month" value={money(data.summary.unmatched_claims_minor)} /><Metric label="Unmatched receipts this month" value={money(data.summary.unmatched_receipts_minor)} /><Metric label="Confirmed recoveries this month" value={money(data.summary.confirmed_recoveries_minor)} /></dl>
    <div className="actions"><button disabled={blocked || data.month.state === 'closed'} onClick={() => open('Record reimbursement receipt', <EntryForm initialKind="reimbursement_receipt" />)}>Record reimbursement receipt</button></div>
    {data.claims.length ? data.claims.map(claim => <article className="claim-row" key={claim.id}><div><h3>{claim.description}</h3><p>{money(claim.amount_minor)} · {titleCase(claim.state)}</p><small>{claim.expected_receipt_date ? `Expected ${claim.expected_receipt_date}` : 'Receipt date unknown'} · {claim.expense_entry_id ? 'Linked expense' : claim.statement_id ? 'Linked statement' : 'Cost unclassified'}</small><small>Claim {claim.id}</small></div><div className="actions"><button disabled={blocked} onClick={() => open('Edit claim', <ClaimForm original={claim} />)}>Edit claim<span className="sr-only"> {claim.description}</span></button><button disabled={blocked} onClick={() => open('Match reimbursement receipt', <ReceiptMatchForm claim={claim} />)}>Match receipt<span className="sr-only"> {claim.description}</span></button></div></article>) : <Empty>No claims yet. Expected reimbursements can be tracked before any money arrives.</Empty>}
  </Section></>;
}

export function Drafts() {
  const { api, data, open, perform, blocked } = useReview();
  const [history, setHistory] = useState(false);
  const drafts = data.drafts.filter(d => history || d.state === 'pending');
  return <Section title="Structured review inbox" action={<button disabled={blocked} onClick={() => open('Stage structured draft', <DraftForm />)}>Stage draft</button>}><p className="quiet">Manual structured proposals with evidence. Drafts remain outside totals; acceptance validates and posts atomically. Imports, Gmail and text capture are later milestones.</p><label className="inline-check"><input type="checkbox" checked={history} onChange={e => setHistory(e.target.checked)} />Show accepted and rejected drafts</label>
    {drafts.length ? drafts.map(draft => <article className="draft-row" key={draft.id}><div className="section-heading"><h3>{draft.entry ? 'Entry proposal' : 'Statement proposal'}</h3><span className="status">{titleCase(draft.state)}</span></div><small>Source {draft.source_id} · {draft.external_id}</small><p className="preserve-lines">{draft.evidence}</p><DraftCandidate draft={draft} /><div className="actions"><button className="primary" disabled={blocked || draft.state !== 'pending'} onClick={() => open('Accept draft', <Form fields={[{ name: 'confirmed', label: 'I reviewed the candidate and evidence', type: 'checkbox', required: true }]} submit="Accept and post draft" disabled={blocked} onSubmit={async () => { await perform('Draft acceptance', api.command<'Draft', S['EmptyInput']>(`/drafts/${draft.id}/accept`, {}, { version: draft.version })); }}><p>This posts the proposal into its reporting month. Closed months must be reopened before acceptance.</p><DraftCandidate draft={draft} /></Form>)}>Review and accept</button><button disabled={blocked || draft.state !== 'pending'} onClick={() => open('Reject draft', <Form fields={[{ name: 'reason', label: 'Rejection reason', type: 'textarea', required: true }]} submit="Reject draft" disabled={blocked} onSubmit={async values => { await perform('Draft rejection', api.command<'Draft', S['ReasonInput']>(`/drafts/${draft.id}/reject`, { reason: text(values, 'reason') }, { version: draft.version })); }} />)}>Reject draft</button></div>{draft.accepted_record_id && <small>Posted record {draft.accepted_record_id}</small>}{draft.rejection_reason && <p>Reason: {draft.rejection_reason}</p>}</article>) : <Empty>No pending drafts. Add confirmed records directly or stage a structured candidate for review.</Empty>}
  </Section>;
}

function DraftCandidate({ draft }: { draft: S['Draft'] }) {
  const { money, account } = useDisplay();
  const candidate = draft.entry ?? draft.statement;
  if (!candidate) return <p className="notice warning">Candidate missing. Reject and stage a corrected draft.</p>;
  const entry = draft.entry;
  return <div className="subtle-box"><dl className="details"><Metric label="Proposal" value={entry ? titleCase(entry.kind || 'Unknown kind') : 'Statement'} /><Metric label="Amount" value={money(entry ? entry.amount_minor : draft.statement?.amount_due_minor)} /><Metric label="Reporting month" value={candidate.reporting_month || 'Not supplied — validated on acceptance'} /><Metric label="Account" value={entry ? `${account(entry.source_account_id)} → ${account(entry.destination_account_id)}` : account(draft.statement?.account_id)} /></dl><details><summary>All proposed fields</summary><pre>{JSON.stringify(candidate, null, 2)}</pre></details></div>;
}
