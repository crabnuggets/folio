import { test, expect, bearer, month, sectionIds, signIn } from './fixtures';
test('lost committed add retries the same command across renewed sign-in', async ({ page, backend }) => {
  await signIn(page); await page.getByRole('button', { name: 'Add item to Income', exact: true }).click();
  const form = page.getByRole('form', { name: 'Add item to Income' });
  await form.getByRole('combobox', { name: 'Name' }).fill('Salary'); await form.getByLabel('Amount', { exact: true }).fill('6400');
  const attempts: { key: string; body: string | null; version: string | undefined }[] = []; const statuses: number[] = [];
  await page.route('**/api/v1/monthly-sheets/*/items', async route => {
    const request = route.request(); if (request.method() !== 'POST') return route.continue();
    attempts.push({ key: request.headers()['idempotency-key'], body: request.postData(), version: request.headers()['if-match'] });
    const response = await route.fetch(attempts.length === 2 ? { headers: { ...request.headers(), authorization: 'Bearer expired-synthetic-token-only' } } : {});
    statuses.push(response.status()); if (attempts.length === 1) await route.abort('connectionfailed'); else await route.fulfill({ response });
  });
  await form.getByRole('button', { name: 'Save item' }).click(); const retry = page.getByRole('button', { name: 'Retry same action' }); await expect(retry).toBeVisible();
  await expect(form.getByLabel('Amount', { exact: true })).toHaveValue('6400'); await expect(page.getByLabel('Month', { exact: true })).toBeDisabled(); await expect(form.getByRole('button', { name: 'Save item' })).toBeDisabled();
  expect((await backend.get<'MonthlySheet'>(`/monthly-sheets/${month}`)).sections.flatMap(section => section.items)).toHaveLength(1);
  await retry.click(); await expect(page.getByLabel('Access token', { exact: false })).toBeVisible();
  await page.getByLabel('Access token', { exact: false }).fill(bearer); await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await expect(retry).toBeVisible(); await expect(form.getByLabel('Amount', { exact: true })).toHaveValue('6400'); await expect(page.getByRole('button', { name: 'Compare latest' })).toHaveCount(0);
  await retry.click(); await expect(form).toHaveCount(0); expect(attempts).toHaveLength(3); expect(attempts[1]).toEqual(attempts[0]); expect(attempts[2]).toEqual(attempts[0]); expect(statuses).toEqual([200, 401, 200]);
  expect((await backend.get<'MonthlySheet'>(`/monthly-sheets/${month}`)).sections.flatMap(section => section.items)).toHaveLength(1);
});
test('stale row edits compare latest before a fresh explicit save', async ({ page, backend }) => {
  await backend.add('Salary', '640000'); await signIn(page);
  const sheet = await backend.get<'MonthlySheet'>(`/monthly-sheets/${month}`); const item = sheet.sections.flatMap(section => section.items)[0];
  const { id: _id, ...input } = item;
  await backend.post<'MonthlySheet'>(`/monthly-sheets/${month}/items/${item.id}`, { ...input, amount_minor: '650000', note: 'Concurrent saved note' }, { method: 'PUT', version: sheet.version });
  const field = page.getByLabel('Amount for Salary', { exact: true }); await field.fill('6600'); await field.press('Tab');
  await expect(page.getByRole('button', { name: 'Compare latest' })).toBeVisible(); await expect(field).toHaveValue('6600');
  await page.getByRole('button', { name: 'Compare latest' }).click(); await expect(field).toHaveValue('6600');
  await page.getByRole('button', { name: 'Save amount' }).click(); await expect(page.getByLabel('Month', { exact: true })).toBeEnabled();
  const saved = await backend.get<'MonthlySheet'>(`/monthly-sheets/${month}`); expect(saved.sections.flatMap(section => section.items)[0]).toMatchObject({ amount_minor: '660000', note: 'Concurrent saved note' });
});
test('queued row saves use fresh versions and preserve another typed row during failure', async ({ page, backend }) => {
  await backend.add('Salary', '640000'); await backend.add('Reimbursement', '5000', sectionIds.otherIn); await signIn(page);
  const salary = page.getByLabel('Amount for Salary', { exact: true }); const reimbursement = page.getByLabel('Amount for Reimbursement', { exact: true });
  const attempts: { url: string; key: string; body: string | null; version: string }[] = [];
  await page.route('**/api/v1/monthly-sheets/*/items/*', async route => {
    const request = route.request(); if (request.method() !== 'PUT') return route.continue();
    attempts.push({ url: request.url(), key: request.headers()['idempotency-key'], body: request.postData(), version: request.headers()['if-match'] });
    const response = await route.fetch(); if (attempts.length === 1) await route.abort('connectionfailed'); else await route.fulfill({ response });
  });
  await salary.fill('6500'); await reimbursement.focus(); await reimbursement.fill('85'); await reimbursement.press('Tab');
  await expect(page.getByRole('button', { name: 'Retry same action' })).toBeVisible(); await expect(reimbursement).toHaveValue('85'); expect(attempts).toHaveLength(1);
  await page.getByRole('button', { name: 'Retry same action' }).click(); await expect(page.getByLabel('Month', { exact: true })).toBeEnabled();
  expect(attempts).toHaveLength(3); expect(attempts[1]).toEqual(attempts[0]); expect(Number(attempts[2].version.replaceAll('"', ''))).toBeGreaterThan(Number(attempts[0].version.replaceAll('"', '')));
  expect((await backend.get<'MonthlySheet'>(`/monthly-sheets/${month}`)).money_in_minor).toBe('658500');
});
