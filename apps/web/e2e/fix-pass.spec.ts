import { randomUUID } from 'node:crypto';
import { test, expect, bearer, month, sectionIds, signIn, selectMonth } from './fixtures';

for (const keyboard of [false, true]) test(`discard amount without a mutation using ${keyboard ? 'keyboard' : 'pointer'}`, async ({ page, backend }) => {
  await backend.add('Salary', '10000'); await signIn(page);
  const writes: string[] = [];
  page.on('request', request => { if (request.method() === 'PUT') writes.push(request.url()); });
  const field = page.getByLabel('Amount for Salary', { exact: true });
  await field.fill('123');
  const discard = page.getByRole('button', { name: 'Discard edit', exact: true });
  if (keyboard) { await field.press('Escape'); await expect(discard).toBeFocused(); await page.keyboard.press('Enter'); } else await discard.click();
  await expect(field).toHaveValue('100.00'); await expect(page.getByLabel('Month', { exact: true })).toBeEnabled();
  expect(writes).toEqual([]);
  await field.fill('124'); await page.getByRole('heading', { name: 'Monthly remainder' }).click();
  await expect(page.getByLabel('Month', { exact: true })).toBeEnabled();
  expect((await backend.get<'MonthlySheet'>(`/monthly-sheets/${month}`)).money_in_minor).toBe('12400');
});

test('amount Tab traverses both ways, exits boundaries, and returns to item options', async ({ page, backend }) => {
  await backend.add('Salary', '10000'); await backend.add('Refund', '2000', sectionIds.otherIn); await signIn(page);
  const first = page.getByLabel('Amount for Salary', { exact: true }); const last = page.getByLabel('Amount for Refund', { exact: true });
  await first.focus(); await first.press('Shift+Tab'); await expect(page.getByRole('button', { name: 'Salary', exact: true })).toBeFocused();
  await first.fill('101'); await first.press('Tab'); await expect(last).toBeFocused();
  await expect(first).toBeEnabled(); await last.fill('21'); await last.press('Shift+Tab'); await expect(first).toBeFocused();
  await expect(last).toBeEnabled(); await first.press('Escape'); await expect(page.getByRole('button', { name: 'Salary', exact: true })).toBeFocused();
  await page.keyboard.press('Enter'); await expect(page.getByLabel('Name', { exact: true })).toBeFocused();
  await page.keyboard.press('Tab'); await expect(page.getByLabel('Notes', { exact: true })).toBeFocused();
  await page.keyboard.type('Keyboard note'); await page.keyboard.press('Tab'); await expect(page.getByRole('switch', { name: 'Repeat monthly' })).toBeFocused();
  await page.keyboard.press('Tab'); await expect(page.getByRole('button', { name: 'Save details', exact: true })).toBeFocused();
  await page.keyboard.press('Tab'); await expect(page.getByRole('button', { name: 'Cancel', exact: true })).toBeFocused();
  await page.getByRole('button', { name: 'Cancel', exact: true }).click();
  await last.focus(); await last.press('Escape'); await page.keyboard.press('Enter'); await expect(page.getByLabel('Notes', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Cancel', exact: true }).click();
  await last.focus(); await last.press('Tab'); await expect(page.getByRole('button', { name: 'Add item to Other money in', exact: true })).toBeFocused();
  expect((await backend.get<'MonthlySheet'>(`/monthly-sheets/${month}`)).money_in_minor).toBe('12200');
});

test('dirty first amount keeps focus on item options while Shift+Tab autosaves', async ({ page, backend }) => {
  await backend.add('Salary', '10000'); await signIn(page);
  let releaseSave = () => {};
  let writeStarted = () => {};
  const started = new Promise<void>(resolve => { writeStarted = resolve; });
  await page.route('**/api/v1/monthly-sheets/*/items/*', async route => {
    if (route.request().method() !== 'PUT') return route.continue();
    await new Promise<void>(resolve => { releaseSave = resolve; writeStarted(); });
    await route.continue();
  });
  const field = page.getByLabel('Amount for Salary', { exact: true });
  const options = page.getByRole('button', { name: 'Salary', exact: true });
  try {
    await field.fill('101'); await field.press('Shift+Tab'); await started;
    await expect(field).toBeDisabled(); await expect(options).toBeFocused();
  } finally { releaseSave(); }
  await expect(field).toBeEnabled(); await expect(options).toBeFocused();
  await expect(field).toHaveValue('101.00');
  expect((await backend.get<'MonthlySheet'>(`/monthly-sheets/${month}`)).money_in_minor).toBe('10100');
  await options.press('Enter'); await expect(page.getByLabel('Name', { exact: true })).toBeFocused();
});

for (const changed of ['removed', 'relocated'] as const) test(`compare retains ${changed} details, affected queued amount, and unrelated details`, async ({ page, backend }) => {
  await backend.add('Salary', '10000'); await backend.add('Refund', '2000', sectionIds.otherIn); await backend.add('Other', '100', sectionIds.otherIn); await signIn(page);
  await page.getByRole('button', { name: 'Salary', exact: true }).click();
  const salaryRow = page.locator('.sheet-row').filter({ has: page.getByLabel('Amount for Salary', { exact: true }) });
  await salaryRow.getByLabel('Name', { exact: true }).fill('Salary renamed');
  await page.getByRole('button', { name: 'Other', exact: true }).click();
  const otherRow = page.locator('.sheet-row').filter({ has: page.getByLabel('Amount for Other', { exact: true }) });
  await otherRow.getByLabel('Notes', { exact: true }).fill('Other unsaved draft');
  let latest = await backend.get<'MonthlySheet'>(`/monthly-sheets/${month}`);
  for (const name of ['Salary', 'Refund']) {
    const item = latest.sections.flatMap(section => section.items).find(item => item.name === name)!; const { id, ...input } = item;
    latest = await backend.post<'MonthlySheet'>(`/monthly-sheets/${month}/items/${id}`, { ...input, note: 'Concurrent note', section_id: changed === 'relocated' ? (item.section_id === sectionIds.income ? sectionIds.otherIn : sectionIds.income) : item.section_id }, { method: 'PUT', version: latest.version });
    if (changed === 'removed') latest = await backend.post<'MonthlySheet'>(`/monthly-sheets/${month}/items/${id}/remove`, {}, { version: latest.version });
  }
  await salaryRow.getByRole('button', { name: 'Save details', exact: true }).click();
  const refund = page.getByLabel('Amount for Refund', { exact: true }); await refund.fill('22'); await refund.press('Tab');
  await page.getByRole('button', { name: 'Compare latest' }).click(); await expect(page.getByText('Latest saved values loaded.', { exact: false })).toBeVisible();
  await expect(salaryRow.getByLabel('Name', { exact: true })).toHaveValue('Salary renamed'); await expect(refund).toHaveValue('22');
  await expect(otherRow.getByLabel('Notes', { exact: true })).toHaveValue('Other unsaved draft');
  await salaryRow.getByRole('button', { name: changed === 'removed' ? 'Restore and reapply details' : 'Save details', exact: true }).click();
  const refundRow = page.locator('.sheet-row').filter({ has: refund }); await refundRow.getByRole('button', { name: 'Discard edit', exact: true }).click();
  await otherRow.getByRole('button', { name: 'Save details', exact: true }).click(); await expect(page.getByLabel('Month', { exact: true })).toBeEnabled();
  const saved = await backend.get<'MonthlySheet'>(`/monthly-sheets/${month}`);
  expect(saved.sections.flatMap(section => section.items).find(item => item.name === 'Salary renamed')).toMatchObject({ note: 'Concurrent note', amount_minor: '10000' });
  expect(saved.sections.flatMap(section => section.items).find(item => item.name === 'Other')).toMatchObject({ note: 'Other unsaved draft' });
});

test('SGD is fixed before amounts and in empty later months with separate month and year controls', async ({ page, backend }) => {
  await signIn(page);
  const change = page.getByRole('button', { name: 'Change currency', exact: true }); await expect(change).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Settings', exact: true })).toHaveCount(0);
  await expect(page.getByRole('region', { name: 'Monthly summary' })).toContainText('SGD 0.00');
  const monthSelect = page.getByRole('button', { name: 'Month', exact: true });
  await expect(monthSelect).toContainText('October'); await monthSelect.click(); await expect(page.getByRole('group', { name: 'Choose month' }).getByRole('button')).toHaveCount(12); await monthSelect.press('Escape');
  const year = page.getByRole('spinbutton', { name: 'Year', exact: true }); await expect(year).toHaveValue('2026');
  await year.fill('1899'); await year.press('Enter'); await expect(year).toHaveValue('2026');
  await backend.add('Salary', '0'); await page.reload(); await signIn(page);
  await selectMonth(page, '2027-01'); await expect(monthSelect).toContainText('January'); await expect(year).toHaveValue('2027');
  await expect(page.getByRole('region', { name: 'Income', exact: true })).toBeVisible(); await expect(change).toHaveCount(0);
  const later = await backend.get<'MonthlySheet'>('/monthly-sheets/2027-01'); expect(later.currency).toBe('SGD'); expect(later.currency_locked).toBe(true); expect(later.sections.flatMap(section => section.items)).toHaveLength(0);
});

for (const changed of ['removed', 'relocated'] as const) for (const action of ['discard', 'reapply'] as const) test(`compare retains ${changed} row and other drafts for explicit ${action}`, async ({ page, context, backend }) => {
  await backend.add('Salary', '10000'); await backend.add('Refund', '2000', sectionIds.otherIn); await signIn(page);
  const other = await context.newPage(); await signIn(other);
  const old = await backend.get<'MonthlySheet'>(`/monthly-sheets/${month}`); const item = old.sections[0].items[0]; const { id, ...input } = item;
  await other.request.put(`/api/v1/monthly-sheets/${month}/items/${id}`, { headers: { Authorization: `Bearer ${bearer}`, 'Idempotency-Key': randomUUID(), 'If-Match': `"${old.version}"` }, data: { ...input, note: 'Concurrent note', section_id: changed === 'relocated' ? sectionIds.otherIn : item.section_id } }).then(response => expect(response.status()).toBe(200));
  if (changed === 'removed') { await other.reload(); await signIn(other); await other.getByRole('button', { name: 'Salary', exact: true }).click(); await other.getByRole('button', { name: 'Skip this month', exact: true }).click(); await expect(other.getByLabel('Amount for Salary', { exact: true })).toHaveCount(0); }
  const salary = page.getByLabel('Amount for Salary', { exact: true }); const refund = page.getByLabel('Amount for Refund', { exact: true });
  await salary.fill('111'); await refund.focus(); await refund.fill('22'); await refund.press('Tab');
  await expect(page.getByRole('button', { name: 'Compare latest' })).toBeVisible(); await page.getByRole('button', { name: 'Compare latest' }).click();
  await expect(page.getByText('Latest saved values loaded.', { exact: false })).toBeVisible();
  await expect(salary).toHaveValue('111'); await expect(refund).toHaveValue(/^22(?:\.00)?$/);
  const row = page.locator(`[data-item="${id}"]`);
  if (action === 'discard') await row.getByRole('button', { name: 'Discard edit', exact: true }).click();
  else await row.getByRole('button', { name: changed === 'removed' ? 'Restore and reapply amount' : 'Save amount', exact: true }).click();
  await expect(page.getByLabel('Month', { exact: true })).toBeEnabled();
  const saved = await backend.get<'MonthlySheet'>(`/monthly-sheets/${month}`); const savedItem = saved.sections.flatMap(section => section.items).find(row => row.id === id);
  if (changed === 'removed' && action === 'discard') expect(savedItem).toBeUndefined();
  else expect(savedItem).toMatchObject({ amount_minor: action === 'reapply' ? '11100' : '10000', note: 'Concurrent note', section_id: changed === 'relocated' ? sectionIds.otherIn : item.section_id });
  await selectMonth(page, '2026-11'); await expect(page.getByRole('region', { name: 'Income', exact: true })).toBeVisible();
  await other.close();
});

test('noncommitted 503 then exact retry 412 releases uncertainty for comparison', async ({ page, backend }) => {
  await backend.add('Salary', '10000'); await signIn(page);
  const attempts: { key: string; body: string | null; version: string }[] = [];
  await page.route('**/api/v1/monthly-sheets/*/items/*', async route => {
    const request = route.request(); if (request.method() !== 'PUT') return route.continue();
    attempts.push({ key: request.headers()['idempotency-key'], body: request.postData(), version: request.headers()['if-match'] });
    if (attempts.length === 1) return route.fulfill({ status: 503, json: { error: { code: 'UNAVAILABLE', message: 'Synthetic noncommit' } } });
    await route.continue();
  });
  const field = page.getByLabel('Amount for Salary', { exact: true }); await field.fill('111'); await field.press('Tab');
  await expect(page.getByRole('button', { name: 'Retry same action' })).toBeVisible(); await expect(page.getByRole('button', { name: 'Compare latest' })).toHaveCount(0);
  await backend.add('Concurrent', '100'); await page.getByRole('button', { name: 'Retry same action' }).click();
  await expect(page.getByRole('button', { name: 'Compare latest' })).toBeVisible(); expect(attempts[1]).toEqual(attempts[0]);
  await page.getByRole('button', { name: 'Compare latest' }).click(); await expect(field).toHaveValue('111');
  await page.getByRole('button', { name: 'Save amount', exact: true }).click(); await expect(page.getByLabel('Month', { exact: true })).toBeEnabled();
  expect(attempts[2].key).not.toBe(attempts[0].key);
});

test('stale timezone settings require a fresh version and command before an explicit update', async ({ backend }) => {
  const settings = await backend.get<'Settings'>('/settings');
  const response = await backend.request.patch('/api/v1/settings', { headers: { 'Idempotency-Key': randomUUID(), 'If-Match': `"${settings.version}"` }, data: { currency: 'SGD', timezone: 'UTC' } }); expect(response.status()).toBe(200);
  const stale = { headers: { 'Idempotency-Key': randomUUID(), 'If-Match': `"${settings.version}"` }, data: { currency: 'SGD', timezone: 'Asia/Singapore' } };
  for (let attempt = 0; attempt < 2; attempt++) {
    const rejected = await backend.request.patch('/api/v1/settings', stale); expect(rejected.status()).toBe(412); expect((await rejected.json()).error.code).toBe('STALE_VERSION');
  }
  const latest = await backend.get<'Settings'>('/settings'); expect(latest.timezone).toBe('UTC');
  const corrected = await backend.request.patch('/api/v1/settings', { ...stale, headers: { 'Idempotency-Key': randomUUID(), 'If-Match': `"${latest.version}"` } }); expect(corrected.status()).toBe(200);
  expect(await backend.get<'Settings'>('/settings')).toMatchObject({ currency: 'SGD', timezone: 'Asia/Singapore', version: latest.version + 1 });
});

test('non-SGD settings fail validation before and after amounts while SGD timezone updates succeed', async ({ backend }) => {
  const settings = await backend.get<'Settings'>('/settings');
  for (const hasAmount of [false, true]) {
    if (hasAmount) await backend.add('Salary', '0');
    for (const currency of ['USD', 'JPY', 'KWD', 'XYZ']) {
      const response = await backend.request.patch('/api/v1/settings', { headers: { 'Idempotency-Key': randomUUID(), 'If-Match': `"${settings.version}"` }, data: { currency, timezone: 'UTC' } });
      expect(response.status()).toBe(422); expect((await response.json()).error.code).toBe('VALIDATION_FAILED');
      expect(await backend.get<'Settings'>('/settings')).toEqual(settings);
    }
  }
  const corrected = await backend.request.patch('/api/v1/settings', { headers: { 'Idempotency-Key': randomUUID(), 'If-Match': `"${settings.version}"` }, data: { currency: 'SGD', timezone: 'UTC' } }); expect(corrected.status()).toBe(200);
  expect(await backend.get<'Settings'>('/settings')).toMatchObject({ currency: 'SGD', timezone: 'UTC', version: settings.version + 1 });
});

test('committed timezone settings replay exactly after an expired bearer without duplicate audit', async ({ backend }) => {
  const settings = await backend.get<'Settings'>('/settings');
  const command = { headers: { 'Idempotency-Key': randomUUID(), 'If-Match': `"${settings.version}"` }, data: { currency: 'SGD', timezone: 'UTC' } };
  const committed = await backend.request.patch('/api/v1/settings', command); expect(committed.status()).toBe(200);
  const expired = await backend.request.patch('/api/v1/settings', { ...command, headers: { ...command.headers, Authorization: 'Bearer expired-synthetic-token-only' } }); expect(expired.status()).toBe(401);
  const replay = await backend.request.patch('/api/v1/settings', command); expect(replay.status()).toBe(200); expect(replay.headers()['idempotency-replayed']).toBe('true'); expect(await replay.json()).toEqual(await committed.json());
  expect(await backend.get<'Settings'>('/settings')).toMatchObject({ currency: 'SGD', timezone: 'UTC', version: settings.version + 1 });
  const exported = await backend.get<'Export'>('/exports'); expect(exported.state.audit_events.filter(event => event.action === 'settings.updated')).toHaveLength(1);
});
