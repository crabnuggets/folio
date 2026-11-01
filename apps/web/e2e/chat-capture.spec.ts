import { test, expect, signIn, month, sectionIds, noHorizontalOverflow } from './fixtures';

test.beforeEach(async ({ page }) => {
  await page.route('**/api/v1/chatgpt/connection', route => route.fulfill({ json: {
    available: true, connected: true, sharing: true, email: 'synthetic@example.test', reason: '',
    models: [{ id: 'synthetic-model', name: 'Synthetic model' }],
  } }));
});

test('chat proposals stay out of totals until reviewed and saved through Go', async ({ page, backend }) => {
  await page.route('**/api/v1/monthly-sheets/*/capture', route => route.fulfill({ json: {
    message: 'Review your salary before adding it.',
    items: [{ name: 'Salary', section_id: sectionIds.income, amount_minor: '500000', note: 'Monthly salary' }],
  } }));
  await signIn(page);
  const composer = page.getByRole('complementary', { name: 'Add items with ChatGPT' });
  await expect(composer.getByRole('button', { name: /^Upload/ })).toBeVisible();
  await composer.getByRole('textbox', { name: 'Describe items to add' }).fill('Add salary of 5000');
  await expect(page.getByRole('button', { name: 'Month', exact: true })).toBeDisabled();
  await composer.getByRole('button', { name: 'Send', exact: true }).click();
  const proposal = composer.getByRole('form', { name: 'Suggested item 1' });
  await expect(proposal).toBeVisible();
  expect((await backend.get<'MonthlySheet'>(`/monthly-sheets/${month}`)).money_in_minor).toBe('0');
  await proposal.getByRole('textbox', { name: 'Amount (SGD)', exact: true }).fill('6400.25');
  await proposal.getByRole('button', { name: `Add item to ${month}`, exact: true }).click();
  await expect(proposal).toHaveCount(0);
  expect((await backend.get<'MonthlySheet'>(`/monthly-sheets/${month}`)).money_in_minor).toBe('640025');
  await expect(composer.getByText(`1 item added to ${month}.`, { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Month', exact: true })).toBeEnabled();
  await composer.getByRole('textbox', { name: 'Describe items to add' }).fill('Add salary of 5000 again');
  await composer.getByRole('button', { name: 'Send', exact: true }).click();
  await expect(proposal.getByText(/An item with this name already exists/)).toBeVisible();
  await expect(proposal.getByRole('button', { name: `Add item to ${month}`, exact: true })).toBeDisabled();
  await proposal.getByRole('button', { name: 'Discard suggestion' }).click();
  await noHorizontalOverflow(page);
});

test('upload sends only the selected attachment and the dock stays visible on a narrow screen', async ({ page, backend }) => {
  await page.setViewportSize({ width: 320, height: 740 });
  let submitted: Record<string, unknown> | undefined;
  await page.route('**/api/v1/monthly-sheets/*/capture', async route => {
    submitted = route.request().postDataJSON();
    await route.fulfill({ json: { message: 'Review this bill.', items: [
      { name: 'Synthetic card', section_id: sectionIds.cards, amount_minor: '3780', note: 'Uploaded bill' },
    ] } });
  });
  await signIn(page);
  const composer = page.getByRole('complementary', { name: 'Add items with ChatGPT' });
  await page.getByLabel('Choose screenshot or document').setInputFiles({ name: 'bill.pdf', mimeType: 'application/pdf', buffer: Buffer.from('%PDF-1.4\nSynthetic test document') });
  await expect(composer.getByText('bill.pdf', { exact: true })).toBeVisible();
  await composer.getByRole('button', { name: 'Send', exact: true }).click();
  const proposal = composer.getByRole('form', { name: 'Suggested item 1' });
  await expect(proposal).toBeVisible();
  expect(submitted).toEqual({ text: '', model: 'synthetic-model', attachment: { name: 'bill.pdf', media_type: 'application/pdf', data: Buffer.from('%PDF-1.4\nSynthetic test document').toString('base64') } });
  expect((await backend.get<'MonthlySheet'>(`/monthly-sheets/${month}`)).money_out_minor).toBe('0');
  await proposal.getByRole('button', { name: 'Discard suggestion' }).click();
  await composer.getByRole('button', { name: 'Collapse chat' }).click();
  await page.locator('footer').scrollIntoViewIfNeeded();
  const dock = await page.locator('.chat-dock').boundingBox();
  expect(dock).not.toBeNull();
  expect(dock!.y).toBeGreaterThanOrEqual(0);
  expect(dock!.y + dock!.height).toBeLessThanOrEqual(740);
  await noHorizontalOverflow(page);
});
