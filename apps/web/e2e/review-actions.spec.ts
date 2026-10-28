import { randomUUID } from 'node:crypto';
import { readFile } from 'node:fs/promises';
import type { Page } from '@playwright/test';
import type { components } from '../src/lib/schema';
import { test, expect, month, date, bearer, accountInput, noHorizontalOverflow, seedWorkedMonth, signIn } from './fixtures';

type S = components['schemas'];

const checklist: S['Checklist'] = {
  income_reviewed: true, cards_reviewed: true, expenses_reviewed: true,
  allocations_reviewed: true, no_statement_reasons: {},
};

function writeResponse(page: Page, path: string, method = 'POST') {
  return page.waitForResponse(response => response.url().endsWith(`/api/v1${path}`) && response.request().method() === method);
}

async function saveDialog(page: Page, name: string) {
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('button', { name, exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Refresh', exact: true })).toBeEnabled();
}

test('checklist closes the month with a missing-statement reason and reopens with an audited reason', async ({ page, backend }) => {
  await seedWorkedMonth(backend);
  const missingCard = await backend.post<'Account'>('/accounts', accountInput('Unused synthetic card', 'credit_card'));
  const original = await backend.get<'Month'>(`/months/${month}`);
  const before = await backend.get<'Summary'>(`/months/${month}/summary`);
  const noStatementReason = 'Synthetic card had no billing cycle this month';
  const reopenReason = 'Review a late synthetic expense';
  await signIn(page);
  await page.getByRole('link', { name: 'Plan & review', exact: true }).click();
  await page.getByRole('button', { name: 'Close month', exact: true }).click();
  await expect(page.getByText('Income reviewed is required.', { exact: true })).toBeVisible();
  expect((await backend.get<'Month'>(`/months/${month}`)).state).toBe('open');
  for (const label of ['Income reviewed', 'Cards reviewed', 'Major expenses reviewed', 'Allocations reviewed']) {
    await page.getByRole('checkbox', { name: label, exact: false }).check();
  }
  await page.getByLabel(`No statement reason for ${missingCard.alias}`, { exact: false }).fill(noStatementReason);
  const closeResponse = writeResponse(page, `/months/${month}/close`);
  await page.getByRole('button', { name: 'Close month', exact: true }).click();
  const closedResponse = await closeResponse;
  expect(closedResponse.status()).toBe(200);
  expect(closedResponse.request().headers()['if-match']).toBe(`"${original.version}"`);
  expect(closedResponse.request().postDataJSON()).toEqual({
    checklist: { ...checklist, no_statement_reasons: { [missingCard.id]: noStatementReason } },
  });
  const closed = await backend.get<'Month'>(`/months/${month}`);
  expect(closed).toMatchObject({ state: 'closed', checklist: { ...checklist, no_statement_reasons: { [missingCard.id]: noStatementReason } } });
  expect(closed.version).toBeGreaterThan(original.version);
  expect(closed.closed_at).not.toBeNull();
  await expect(page.getByText('Month closed', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Add entry', exact: true })).toBeDisabled();
  const reviewed = await backend.get<'Summary'>(`/months/${month}/summary`);
  expect(reviewed).toMatchObject({ income_minor: '500000', gross_spending_minor: '250000', allocation_minor: '80000', monthly_surplus_minor: '170000', is_provisional: false });
  expect(reviewed.funding_remainder_minor).toBe(before.funding_remainder_minor);

  await page.getByRole('button', { name: 'Reopen month', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Reopen month', exact: true });
  await dialog.getByRole('button', { name: 'Reopen month', exact: true }).click();
  await expect(dialog.getByText('Reopen reason is required.', { exact: true })).toBeVisible();
  expect((await backend.get<'Month'>(`/months/${month}`)).state).toBe('closed');
  await dialog.getByLabel('Reopen reason', { exact: false }).fill(reopenReason);
  const reopenResponse = writeResponse(page, `/months/${month}/reopen`);
  await saveDialog(page, 'Reopen month');
  const reopenedResponse = await reopenResponse;
  expect(reopenedResponse.status()).toBe(200);
  expect(reopenedResponse.request().headers()['if-match']).toBe(`"${closed.version}"`);
  expect(reopenedResponse.request().postDataJSON()).toEqual({ reason: reopenReason });
  const reopened = await backend.get<'Month'>(`/months/${month}`);
  expect(reopened).toMatchObject({ state: 'open', closed_at: null, revision: closed.revision + 1 });
  expect(reopened.version).toBeGreaterThan(closed.version);
  await expect(page.getByRole('button', { name: 'Add entry', exact: true })).toBeEnabled();
  await page.getByRole('link', { name: 'Records', exact: true }).click();
  await expect(page.getByText(reopenReason, { exact: true })).toBeVisible();
  const audit = await backend.get<'AuditPage'>('/audit-events');
  expect(audit.items).toEqual(expect.arrayContaining([expect.objectContaining({ action: 'month.reopened', entity_id: month, reason: reopenReason })]));
});

test('a real stale account write returns 412 and recovers through Return to review inside the modal', async ({ page, backend }) => {
  const input = accountInput('Synthetic original funding');
  const account = await backend.post<'Account'>('/accounts', input);
  await signIn(page);
  await page.getByRole('link', { name: 'Accounts & settings', exact: true }).click();
  await page.getByRole('button', { name: `Edit account ${account.alias}`, exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Edit account', exact: true });
  await dialog.getByLabel('Account alias', { exact: false }).fill('Synthetic stale browser edit');

  const concurrentInput = { ...input, alias: 'Synthetic concurrent funding' };
  const concurrentResponse = await backend.request.patch(`/api/v1/accounts/${account.id}`, {
    data: concurrentInput, headers: { 'Idempotency-Key': randomUUID(), 'If-Match': `"${account.version}"` },
  });
  expect(concurrentResponse.status(), await concurrentResponse.text()).toBe(200);
  const concurrent: S['Account'] = await concurrentResponse.json();
  expect(concurrent.version).toBeGreaterThan(account.version);
  const staleResponse = writeResponse(page, `/accounts/${account.id}`, 'PATCH');
  await dialog.getByRole('button', { name: 'Save account', exact: true }).click();
  const rejected = await staleResponse;
  expect(rejected.status()).toBe(412);
  expect(rejected.request().headers()['if-match']).toBe(`"${account.version}"`);
  await expect(dialog.getByText('This record changed in another review.', { exact: false }).first()).toBeVisible();
  const returnToReview = dialog.getByRole('button', { name: 'Return to review', exact: true });
  await expect(returnToReview).toBeVisible();
  await expect(returnToReview).toBeEnabled();
  await returnToReview.click();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole('heading', { name: concurrent.alias, exact: true })).toBeVisible();
  expect(await backend.get<'Account'>(`/accounts/${account.id}`)).toEqual(concurrent);

  await page.getByRole('button', { name: `Edit account ${concurrent.alias}`, exact: true }).click();
  await expect(dialog.getByLabel('Account alias', { exact: false })).toHaveValue(concurrent.alias);
  await dialog.getByLabel('Account alias', { exact: false }).fill('Synthetic reconciled funding');
  const reconciledResponse = writeResponse(page, `/accounts/${account.id}`, 'PATCH');
  await saveDialog(page, 'Save account');
  const savedResponse = await reconciledResponse;
  expect(savedResponse.status()).toBe(200);
  expect(savedResponse.request().headers()['if-match']).toBe(`"${concurrent.version}"`);
  const saved = await backend.get<'Account'>(`/accounts/${account.id}`);
  expect(saved).toMatchObject({ ...concurrentInput, id: account.id, alias: 'Synthetic reconciled funding' });
  expect(saved.version).toBeGreaterThan(concurrent.version);
  await expect(page.getByRole('heading', { name: saved.alias, exact: true })).toBeVisible();
});

test('a browser claim and matched receipt reduce personal cost once without becoming salary or duplicate cash', async ({ page, backend }) => {
  const funding = await backend.post<'Account'>('/accounts', accountInput('Synthetic reimbursement funding'));
  const expense = await backend.post<'Entry'>('/entries', {
    kind: 'direct_expense', currency: 'SGD', amount_minor: '12550', effective_date: date,
    source_account_id: funding.id, category: 'Synthetic shared meal',
  });
  const description = 'Synthetic meal reimbursement';
  await signIn(page);
  await page.getByRole('link', { name: 'Claims', exact: true }).click();
  await page.getByRole('button', { name: 'Add claim', exact: true }).click();
  const claimDialog = page.getByRole('dialog', { name: 'Add claim', exact: true });
  await claimDialog.getByLabel('Claim description', { exact: false }).fill(description);
  await claimDialog.getByLabel('Claim amount', { exact: false }).fill('75.25');
  await claimDialog.getByLabel('Expected receipt date', { exact: false }).fill(date);
  await claimDialog.getByLabel('Linked expense', { exact: false }).selectOption(expense.id);
  const claimResponse = writeResponse(page, '/claims');
  await saveDialog(page, 'Save claim');
  const createdResponse = await claimResponse;
  expect(createdResponse.status()).toBe(201);
  expect(createdResponse.request().postDataJSON()).toMatchObject({ amount_minor: '7525', currency: 'SGD', expense_entry_id: expense.id, statement_id: null, state: 'pending' });
  const claim: S['Claim'] = await createdResponse.json();
  const row = page.getByRole('article').filter({ has: page.getByRole('heading', { name: description, exact: true }) });
  await expect(row).toContainText('Pending');
  expect(await backend.get<'Summary'>(`/months/${month}/summary`)).toMatchObject({
    income_minor: '0', gross_spending_minor: '12550', known_spending_minor: '12550', confirmed_recoveries_minor: '0',
    unmatched_claims_minor: '7525', unmatched_receipts_minor: '0', funding_inflows_minor: '0', funding_outflows_minor: '12550',
  });
  expect((await backend.get<'EntryPage'>('/entries')).items).toHaveLength(1);

  await page.getByRole('button', { name: 'Record reimbursement receipt', exact: true }).click();
  const receiptDialog = page.getByRole('dialog', { name: 'Record reimbursement receipt', exact: true });
  await expect(receiptDialog.getByLabel('Entry kind', { exact: false })).toHaveValue('reimbursement_receipt');
  await receiptDialog.getByLabel('Amount', { exact: false }).fill('75.25');
  await receiptDialog.getByLabel('Effective date', { exact: false }).fill(date);
  await receiptDialog.getByLabel('Reporting month', { exact: false }).first().fill(month);
  await receiptDialog.getByLabel('Destination account', { exact: false }).selectOption(funding.id);
  await receiptDialog.getByLabel('Description / category', { exact: false }).fill(description);
  const receiptResponse = writeResponse(page, '/entries');
  await saveDialog(page, 'Save entry');
  const recordedResponse = await receiptResponse;
  expect(recordedResponse.status()).toBe(201);
  const receipt: S['Entry'] = await recordedResponse.json();
  expect(recordedResponse.request().postDataJSON()).toMatchObject({ kind: 'reimbursement_receipt', amount_minor: '7525', source_account_id: null, destination_account_id: funding.id, reporting_month: month });
  expect(await backend.get<'Summary'>(`/months/${month}/summary`)).toMatchObject({
    income_minor: '0', known_spending_minor: '12550', confirmed_recoveries_minor: '0', unmatched_claims_minor: '7525',
    unmatched_receipts_minor: '7525', funding_inflows_minor: '7525', funding_remainder_minor: '-5025',
  });
  await row.getByRole('button', { name: `Match receipt ${description}`, exact: true }).click();
  const matchDialog = page.getByRole('dialog', { name: 'Match reimbursement receipt', exact: true });
  await matchDialog.getByLabel('Reimbursement receipt', { exact: false }).selectOption(receipt.id);
  await matchDialog.getByLabel('Matched amount', { exact: false }).fill('75.25');
  await matchDialog.getByLabel('Recovery month', { exact: false }).fill(month);
  const matchResponse = writeResponse(page, `/claims/${claim.id}/receipt-matches`);
  await saveDialog(page, 'Match receipt');
  const matchedResponse = await matchResponse;
  expect(matchedResponse.status()).toBe(201);
  expect(matchedResponse.request().headers()['if-match']).toBe(`"${claim.version}"`);
  expect(matchedResponse.request().postDataJSON()).toEqual({ receipt_entry_id: receipt.id, amount_minor: '7525', recovery_month: month });
  const match: S['ReceiptMatch'] = await matchedResponse.json();
  expect(match).toMatchObject({ claim_id: claim.id, receipt_entry_id: receipt.id, amount_minor: '7525', recovery_month: month });
  await expect(row).toContainText('Received');
  expect(await backend.get<'Claim'>(`/claims/${claim.id}`)).toMatchObject({ state: 'received' });
  expect(await backend.get<'Summary'>(`/months/${month}/summary`)).toMatchObject({
    income_minor: '0', gross_spending_minor: '12550', confirmed_recoveries_minor: '7525', known_spending_minor: '5025',
    net_personal_cost_minor: '5025', unmatched_claims_minor: '0', unmatched_receipts_minor: '0',
    funding_inflows_minor: '7525', funding_outflows_minor: '12550', funding_remainder_minor: '-5025',
  });
  const entries = (await backend.get<'EntryPage'>('/entries')).items;
  expect(entries.map(entry => entry.id).sort()).toEqual([expense.id, receipt.id].sort());
  const exported = await backend.get<'Export'>('/exports');
  expect(Object.values(exported.state.receipt_matches)).toEqual([match]);
});


test('same-month funding boundary edits replace the dated setting before financial history exists', async ({ page, backend }) => {
  const account = await backend.post<'Account'>('/accounts', accountInput('Synthetic setup funding'));
  await signIn(page);
  await page.getByRole('link', { name: 'Accounts & settings', exact: true }).click();
  await page.getByRole('button', { name: `Edit account ${account.alias}`, exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Set a funding boundary change', { exact: true }).check();
  await dialog.getByLabel('Boundary effective month', { exact: false }).fill('2026-01');
  await dialog.getByLabel('Include in funding boundary', { exact: true }).uncheck();
  await saveDialog(page, 'Save account');
  await expect(page.getByRole('button', { name: `Edit account ${account.alias}`, exact: true })).toBeFocused();
  expect((await backend.get<'Account'>(`/accounts/${account.id}`)).funding_boundary).toEqual([
    { effective_month: '2026-01', included: false },
  ]);
});

test('structured drafts can be staged while closed, accepted after reopen, and exported with lossless histories', async ({ page, backend }) => {
  const funding = await backend.post<'Account'>('/accounts', accountInput('Synthetic draft funding'));
  const review = await backend.get<'Month'>(`/months/${month}`);
  const closed = await backend.post<'Month'>(`/months/${month}/close`, { checklist }, { version: review.version, status: 200 });
  await signIn(page);
  await page.getByRole('link', { name: 'Drafts', exact: false }).click();
  await page.getByRole('button', { name: 'Stage draft', exact: true }).click();
  const dialog = page.getByRole('dialog');
  const candidate = { kind: 'income', amount_minor: '12345', currency: 'SGD', effective_date: date, destination_account_id: funding.id };
  await dialog.getByLabel('External identity', { exact: false }).fill('synthetic-manual-income');
  await dialog.getByLabel('Candidate JSON', { exact: false }).fill(JSON.stringify(candidate));
  await dialog.getByLabel('Source evidence', { exact: false }).fill('Synthetic manually verified income');
  await saveDialog(page, 'Stage draft');
  const staged = (await backend.get<'DraftPage'>('/drafts')).items[0];
  expect(staged.state).toBe('pending');
  expect((await backend.get<'EntryPage'>('/entries')).items).toHaveLength(0);
  await page.getByRole('button', { name: 'Review and accept', exact: true }).click();
  await dialog.getByLabel('I reviewed the candidate and evidence', { exact: false }).check();
  await dialog.getByRole('button', { name: 'Accept and post draft', exact: true }).click();
  await expect(dialog.getByRole('button', { name: 'Return to review', exact: true })).toBeVisible();
  expect((await backend.get<'EntryPage'>('/entries')).items).toHaveLength(0);
  expect((await backend.get<'Draft'>(`/drafts/${staged.id}`)).state).toBe('pending');
  await dialog.getByRole('button', { name: 'Return to review', exact: true }).click();
  await backend.post<'Month'>(`/months/${month}/reopen`, { reason: 'Review synthetic staged income' }, { version: closed.version, status: 200 });
  await page.getByRole('button', { name: 'Refresh', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Refresh', exact: true })).toBeEnabled();
  await page.getByRole('button', { name: 'Review and accept', exact: true }).click();
  await dialog.getByLabel('I reviewed the candidate and evidence', { exact: false }).check();
  await saveDialog(page, 'Accept and post draft');
  const entry = (await backend.get<'EntryPage'>('/entries')).items[0];
  expect(entry).toMatchObject(candidate);
  expect((await backend.get<'Draft'>(`/drafts/${staged.id}`)).accepted_record_id).toBe(entry.id);
  const downloadReady = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Export JSON', exact: true }).click();
  const download = await downloadReady;
  expect(download.suggestedFilename()).toMatch(/^folio-export-\d{4}-\d{2}-\d{2}\.json$/);
  const exportPath = await download.path();
  expect(exportPath).not.toBeNull();
  const contents = await readFile(exportPath!, 'utf8');
  expect(contents).not.toContain(bearer);
  const exported = JSON.parse(contents) as S['Export'];
  expect(exported.schema_version).toBeDefined();
  expect(Object.values(exported.state.entries)).toEqual([entry]);
  expect(Object.values(exported.state.drafts)).toEqual(expect.arrayContaining([
    expect.objectContaining({ id: staged.id, state: 'accepted', accepted_record_id: entry.id }),
  ]));
});


test('plan matching follows corrected and voided income without counting the original twice', async ({ page, backend }) => {
  const funding = await backend.post<'Account'>('/accounts', accountInput('Synthetic planning funding'));
  await signIn(page);
  await page.getByRole('link', { name: 'Plan & review', exact: true }).click();
  await page.getByRole('button', { name: 'Add plan line', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Plan description', { exact: false }).fill('Synthetic planned salary');
  await dialog.getByLabel('Plan kind', { exact: false }).selectOption('income');
  await dialog.getByLabel('Expected amount', { exact: false }).fill('1000');
  await saveDialog(page, 'Save plan');
  const plan = await backend.get<'Plan'>(`/months/${month}/plan`);
  expect(plan.lines).toHaveLength(1);
  await noHorizontalOverflow(page);
  await page.getByRole('button', { name: 'Add entry', exact: true }).click();
  await dialog.getByLabel('Amount', { exact: false }).fill('1000');
  await dialog.getByLabel('Effective date', { exact: false }).fill(date);
  await dialog.getByLabel('Destination account', { exact: false }).selectOption(funding.id);
  await dialog.getByLabel('Description / category', { exact: false }).fill('Synthetic planned salary');
  await dialog.getByLabel('Recognized plan match', { exact: false }).selectOption(plan.lines[0].id);
  await saveDialog(page, 'Save entry');
  expect((await backend.get<'Summary'>(`/months/${month}/summary`)).forecast_lines[0]).toMatchObject({ actual_minor: '100000', remaining_minor: '0', forecast_minor: '100000' });
  await page.getByRole('link', { name: 'Records', exact: true }).click();
  await page.getByRole('button', { name: `View entry Synthetic planned salary ${date}`, exact: true }).click();
  await dialog.getByRole('button', { name: 'Correct entry', exact: true }).click();
  await dialog.getByLabel('Amount', { exact: false }).fill('900');
  await dialog.getByLabel('Correction reason', { exact: false }).fill('Corrected synthetic net salary');
  await saveDialog(page, 'Save correction');
  const corrected = (await backend.get<'EntryPage'>('/entries')).items;
  expect(corrected).toHaveLength(2);
  expect(corrected.filter(entry => entry.active)).toHaveLength(1);
  expect((await backend.get<'Summary'>(`/months/${month}/summary`))).toMatchObject({ income_minor: '90000', forecast_lines: [expect.objectContaining({ actual_minor: '90000', remaining_minor: '10000', forecast_minor: '100000' })] });
  await page.getByRole('button', { name: `View entry Synthetic planned salary ${date}`, exact: true }).click();
  await dialog.getByRole('button', { name: 'Void entry', exact: true }).click();
  await dialog.getByLabel('Void reason', { exact: false }).fill('Synthetic duplicate evidence discovered');
  await saveDialog(page, 'Void entry');
  expect((await backend.get<'Summary'>(`/months/${month}/summary`))).toMatchObject({ income_minor: '0', forecast_lines: [expect.objectContaining({ actual_minor: '0', remaining_minor: '100000' })] });
  await page.getByLabel('Show correction history', { exact: true }).check();
  await expect(page.getByRole('button', { name: `View entry Synthetic planned salary ${date}`, exact: true })).toHaveCount(2);
});
