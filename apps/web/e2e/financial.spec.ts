import { randomUUID } from 'node:crypto';
import { test, expect, month, date, seedWorkedMonth, signIn, noHorizontalOverflow, statementInput, accountInput } from './fixtures';

const checklist = {
  income_reviewed: true, cards_reviewed: true, expenses_reviewed: true,
  allocations_reviewed: true, no_statement_reasons: {},
};

// Targeted display tests seed through the real API; worked-month.spec.ts covers
// account setup, entries and statement creation through browser forms.
test('Go financial summaries remain authoritative after settlement and statement revision', async ({ page, backend }) => {
  const { funding, cardA, statementA } = await seedWorkedMonth(backend);
  let review = await backend.get<'Month'>(`/months/${month}`);
  review = await backend.post<'Month'>(`/months/${month}/close`, { checklist }, { status: 200, version: review.version });
  const summary = await backend.get<'Summary'>(`/months/${month}/summary`);
  expect(summary).toMatchObject({
    income_minor: '500000', known_spending_minor: '250000', allocation_minor: '80000',
    monthly_surplus_minor: '170000', is_provisional: false,
  });
  await signIn(page);
  await expect(page.getByText('SGD 2,500.00', { exact: true }).first()).toBeVisible();
  await expect(page.getByText('SGD 1,700.00', { exact: true }).first()).toBeVisible();
  await expect(page.getByRole('button', { name: 'Add entry', exact: true })).toBeDisabled();
  await noHorizontalOverflow(page);

  const forbidden = await backend.request.post('/api/v1/entries', {
    headers: { 'Idempotency-Key': randomUUID() },
    data: { kind: 'income', currency: 'SGD', effective_date: date, amount_minor: '100', destination_account_id: funding.id },
  });
  expect(forbidden.status()).toBe(409);
  expect((await forbidden.json()).error.code).toBe('MONTH_CLOSED');

  await backend.post<'Month'>(`/months/${month}/reopen`, { reason: 'Synthetic settlement and statement review' }, { status: 200, version: review.version });
  const payment = await backend.post<'Entry'>('/entries', {
    kind: 'card_settlement', currency: 'SGD', effective_date: date, amount_minor: '120000',
    source_account_id: funding.id, destination_account_id: cardA.id,
  });
  const settlementKey = randomUUID();
  const settlement = { entry_id: payment.id, amount_minor: '120000' };
  const first = await backend.post<'Settlement'>(`/statements/${statementA.id}/settlements`, settlement, { key: settlementKey, version: statementA.version });
  const replay = await backend.post<'Settlement'>(`/statements/${statementA.id}/settlements`, settlement, { key: settlementKey, version: statementA.version });
  expect(replay).toEqual(first);
  const settled = await backend.get<'Summary'>(`/months/${month}/summary`);
  expect(settled.known_spending_minor).toBe('250000');
  expect(settled.statements.find(row => row.statement_id === statementA.id)?.remaining_minor).toBe('0');
  await page.getByRole('button', { name: 'Refresh', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Add entry', exact: true })).toBeEnabled();
  await expect(page.getByText('SGD 2,500.00', { exact: true }).first()).toBeVisible();

  const currentStatement = await backend.get<'Statement'>(`/statements/${statementA.id}`);
  const revised = await backend.post<'Statement'>(`/statements/${statementA.id}/revisions`, {
    reason: 'Synthetic corrected net new cycle spending', replacement: statementInput(cardA.id, '110000'),
  }, { version: currentStatement.version });
  expect(revised.supersedes_id).toBe(statementA.id);
  const corrected = await backend.get<'Summary'>(`/months/${month}/summary`);
  expect(corrected.known_spending_minor).toBe('240000');
  expect(corrected.monthly_surplus_minor).toBe('180000');
  await page.getByRole('button', { name: 'Refresh', exact: true }).click();
  await expect(page.getByText('SGD 2,400.00', { exact: true }).first()).toBeVisible();
  await expect(page.getByText('SGD 1,800.00', { exact: true }).first()).toBeVisible();
  await noHorizontalOverflow(page);
});

test('missing statement and unknown cycle spending keep reviewed figures provisional', async ({ page, backend }) => {
  await seedWorkedMonth(backend);
  const missing = await backend.post<'Account'>('/accounts', accountInput('Missing statement card', 'credit_card'));
  let summary = await backend.get<'Summary'>(`/months/${month}/summary`);
  expect(summary.is_provisional).toBe(true);
  expect(summary.monthly_surplus_minor).toBeNull();
  expect(summary.missing_items.join(' ')).toContain(missing.id);
  await signIn(page);
  await expect(page.getByText('Needs review', { exact: true }).first()).toBeVisible();
  await expect(page.getByText('Unknown', { exact: true }).first()).toBeVisible();

  // A bill due is evidence of an obligation, never a substitute for spending.
  const unknownStatement = await backend.post<'Statement'>('/statements', statementInput(missing.id, null));
  const review = await backend.get<'Month'>(`/months/${month}`);
  await backend.post<'Month'>(`/months/${month}/close`, { checklist }, { status: 200, version: review.version });
  summary = await backend.get<'Summary'>(`/months/${month}/summary`);
  expect(summary).toMatchObject({ is_provisional: true, spending_complete: false, monthly_surplus_minor: null });
  expect(summary.statements.find(row => row.statement_id === unknownStatement.id)).toMatchObject({ amount_due_minor: '120000', spending_minor: null });
  await page.getByRole('button', { name: 'Refresh', exact: true }).click();
  await expect(page.getByText('Unknown', { exact: true }).first()).toBeVisible();
  await noHorizontalOverflow(page);
});
