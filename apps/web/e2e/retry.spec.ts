import { test, expect, bearer, month, date, accountInput, statementInput, signIn, noHorizontalOverflow } from './fixtures';

test('repeated submit and a lost committed response recover with one posting and the same retry identity', async ({ page, backend }) => {
  const funding = await backend.post<'Account'>('/accounts', accountInput('Funding'));
  await signIn(page);
  await page.getByRole('button', { name: 'Add entry', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Entry kind', { exact: false }).selectOption('income');
  await dialog.getByLabel('Amount', { exact: false }).fill('5000');
  await dialog.getByLabel('Effective date', { exact: false }).fill(date);
  await dialog.getByLabel('Reporting month', { exact: false }).first().fill(month);
  await dialog.getByLabel('Destination account', { exact: false }).selectOption(funding.id);
  await noHorizontalOverflow(page);

  const attempts: { key: string; body: string | null; precondition: string | undefined }[] = [];
  const statuses: number[] = [];
  const replayHeaders: (string | undefined)[] = [];
  await page.route('**/api/v1/entries', async route => {
    const request = route.request();
    if (request.method() !== 'POST') return route.continue();
    attempts.push({ key: request.headers()['idempotency-key'], body: request.postData(), precondition: request.headers()['if-match'] });
    const response = await route.fetch(); // Real Go commits before the response is lost.
    statuses.push(response.status());
    replayHeaders.push(response.headers()['idempotency-replayed']);
    if (attempts.length === 1) await route.abort('connectionfailed');
    else await route.fulfill({ response });
  });
  await dialog.getByRole('button', { name: 'Save entry', exact: true }).dblclick();
  const retry = page.getByRole('button', { name: 'Retry same action', exact: true });
  await expect(retry).toBeVisible();
  expect(attempts).toHaveLength(1);
  expect(statuses).toEqual([201]);
  const beforeRetry = await backend.get<'EntryPage'>('/entries');
  expect(beforeRetry.items).toHaveLength(1);
  expect(beforeRetry.items[0]).toMatchObject({ kind: 'income', amount_minor: '500000' });

  // This must be an actionable control even when the failed form used showModal.
  await retry.click();
  await expect(retry).toHaveCount(0);
  await expect(dialog).toHaveCount(0);
  expect(attempts).toHaveLength(2);
  expect(attempts[1]).toEqual(attempts[0]);
  expect(attempts[0].key).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i);
  expect(statuses).toEqual([201, 201]);
  expect(replayHeaders[1]).toBe('true');
  const afterRetry = await backend.get<'EntryPage'>('/entries');
  expect(afterRetry.items.map(item => item.id)).toEqual(beforeRetry.items.map(item => item.id));
  const summary = await backend.get<'Summary'>(`/months/${month}/summary`);
  expect(summary.income_minor).toBe('500000');
});

test('an expired bearer returns to sign-in and recovers against the real auth middleware', async ({ page }) => {
  await signIn(page);
  await page.route('**/api/v1/months/*/summary', route => route.continue({
    headers: { ...route.request().headers(), authorization: 'Bearer expired-synthetic-token-only' },
  }));
  await page.getByRole('button', { name: 'Refresh', exact: true }).click();
  await expect(page.getByLabel('Access token', { exact: false })).toBeVisible();
  await expect(page.getByRole('status')).toContainText('session expired');
  await expect(page.getByRole('button', { name: 'Export JSON', exact: true })).toHaveCount(0);
  await page.unroute('**/api/v1/months/*/summary');
  await page.getByLabel('Access token', { exact: false }).fill(bearer);
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Add entry', exact: true })).toBeEnabled();
});


test('a retained inline draft form locks after a lost response until its original action is recovered', async ({ page, backend }) => {
  const funding = await backend.post<'Account'>('/accounts', accountInput('Funding'));
  const draft = await backend.post<'Draft'>('/drafts', {
    source_id: 'manual-synthetic', external_id: 'inline-retry-synthetic', evidence: 'Synthetic income evidence',
    entry: { kind: 'income', amount_minor: '10000', currency: 'SGD', effective_date: date, destination_account_id: funding.id },
  });
  await signIn(page);
  await page.getByRole('link', { name: 'Drafts', exact: false }).click();
  await page.getByRole('button', { name: 'Review and accept', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('I reviewed the candidate and evidence', { exact: false }).check();
  const attempts: { key: string; body: string | null; precondition: string | undefined }[] = [];
  await page.route(`**/api/v1/drafts/${draft.id}/accept`, async route => {
    const request = route.request();
    attempts.push({ key: request.headers()['idempotency-key'], body: request.postData(), precondition: request.headers()['if-match'] });
    const response = await route.fetch();
    expect(response.status()).toBe(200);
    if (attempts.length === 1) await route.abort('connectionfailed');
    else await route.fulfill({ response });
  });
  const submit = dialog.getByRole('button', { name: 'Accept and post draft', exact: true });
  await submit.click();
  const retry = dialog.getByRole('button', { name: 'Retry same action', exact: true });
  await expect(retry).toBeVisible();
  await expect(submit).toBeDisabled();
  await expect(dialog.getByLabel('I reviewed the candidate and evidence', { exact: false })).toBeDisabled();
  expect(attempts).toHaveLength(1);
  await retry.click();
  await expect(dialog).toHaveCount(0);
  expect(attempts).toHaveLength(2);
  expect(attempts[1]).toEqual(attempts[0]);
  expect((await backend.get<'EntryPage'>('/entries')).items).toHaveLength(1);
  expect((await backend.get<'Draft'>(`/drafts/${draft.id}`)).state).toBe('accepted');
});


test('a lost committed response remains uncertain across a rejected retry and renewed sign-in', async ({ page, backend }) => {
  const funding = await backend.post<'Account'>('/accounts', accountInput('Funding'));
  await signIn(page);
  await page.getByRole('button', { name: 'Add entry', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Amount', { exact: false }).fill('100');
  await dialog.getByLabel('Destination account', { exact: false }).selectOption(funding.id);
  const attempts: { key: string; body: string | null }[] = [];
  await page.route('**/api/v1/entries', async route => {
    const request = route.request();
    if (request.method() !== 'POST') return route.continue();
    attempts.push({ key: request.headers()['idempotency-key'], body: request.postData() });
    const response = await route.fetch(attempts.length === 2 ? {
      headers: { ...request.headers(), authorization: 'Bearer expired-synthetic-token-only' },
    } : {});
    expect(response.status()).toBe(attempts.length === 2 ? 401 : 201);
    if (attempts.length === 1) await route.abort('connectionfailed');
    else await route.fulfill({ response });
  });
  await dialog.getByRole('button', { name: 'Save entry', exact: true }).click();
  await page.getByRole('button', { name: 'Retry same action', exact: true }).click();
  await expect(page.getByLabel('Access token', { exact: false })).toBeVisible();
  await page.getByLabel('Access token', { exact: false }).fill(bearer);
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  const retry = page.getByRole('button', { name: 'Retry same action', exact: true });
  await expect(retry).toBeVisible();
  await expect(page.getByRole('button', { name: 'Return to review', exact: true })).toHaveCount(0);
  await retry.click();
  await expect(retry).toHaveCount(0);
  expect(attempts).toHaveLength(3);
  expect(attempts[1]).toEqual(attempts[0]);
  expect(attempts[2]).toEqual(attempts[0]);
  expect((await backend.get<'EntryPage'>('/entries')).items).toHaveLength(1);
});


test('a recovered payment with stale linkage returns to review and links the existing cash movement', async ({ page, backend }) => {
  const funding = await backend.post<'Account'>('/accounts', accountInput('Settlement funding'));
  const card = await backend.post<'Account'>('/accounts', accountInput('Settlement card', 'credit_card'));
  const statement = await backend.post<'Statement'>('/statements', statementInput(card.id, '120000'));
  await signIn(page);
  await page.getByRole('link', { name: 'Statements', exact: true }).click();
  await page.getByRole('button', { name: `Record payment ${card.alias}`, exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Paying account', { exact: false }).selectOption(funding.id);
  await dialog.getByLabel('Payment date', { exact: false }).fill(date);
  const attempts: { key: string; body: string | null }[] = [];
  await page.route('**/api/v1/entries', async route => {
    const request = route.request();
    if (request.method() !== 'POST') return route.continue();
    attempts.push({ key: request.headers()['idempotency-key'], body: request.postData() });
    const response = await route.fetch();
    expect(response.status()).toBe(201);
    if (attempts.length === 1) await route.abort('connectionfailed');
    else await route.fulfill({ response });
  });
  await dialog.getByRole('button', { name: 'Save payment', exact: true }).click();
  const retry = dialog.getByRole('button', { name: 'Retry same action', exact: true });
  await expect(retry).toBeVisible();
  await expect(dialog.getByRole('button', { name: 'Return to review', exact: true })).toHaveCount(0);
  const revised = await backend.post<'Statement'>(`/statements/${statement.id}/revisions`, {
    reason: 'Concurrent statement evidence update', replacement: statementInput(card.id, '120000'),
  }, { version: statement.version });
  const rejectedLink = page.waitForResponse(response => response.url().endsWith(`/statements/${statement.id}/settlements`) && response.request().method() === 'POST');
  await retry.click();
  expect((await rejectedLink).status()).toBe(412);
  const returnToReview = dialog.getByRole('button', { name: 'Return to review', exact: true });
  await expect(returnToReview).toBeVisible();
  expect(attempts).toHaveLength(2);
  expect(attempts[1]).toEqual(attempts[0]);
  const payments = (await backend.get<'EntryPage'>('/entries')).items;
  expect(payments).toHaveLength(1);
  await expect(dialog).toContainText(payments[0].id);
  await returnToReview.click();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole('button', { name: `Record payment ${card.alias}`, exact: true })).toBeEnabled();
  await page.getByRole('button', { name: `Record payment ${card.alias}`, exact: true }).click();
  await dialog.getByLabel('Payment source', { exact: false }).selectOption('existing');
  await dialog.getByLabel('Existing payment', { exact: false }).selectOption(payments[0].id);
  await dialog.getByRole('button', { name: 'Save payment', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect((await backend.get<'EntryPage'>('/entries')).items).toHaveLength(1);
  expect((await backend.get<'Summary'>(`/months/${month}/summary`)).statements.find(item => item.statement_id === revised.id)?.remaining_minor).toBe('0');
});
