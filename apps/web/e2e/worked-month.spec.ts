import type { Page } from '@playwright/test';
import { test, expect, month, date, signIn, noHorizontalOverflow } from './fixtures';

async function saveDialog(page: Page, name: string) {
  const dialog = page.getByRole('dialog');
  await noHorizontalOverflow(page);
  await dialog.getByRole('button', { name, exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Refresh', exact: true })).toBeEnabled();
}

async function addAccount(page: Page, alias: string, kind: string, funding = false) {
  await page.getByRole('link', { name: 'Accounts & settings', exact: true }).click();
  await page.getByRole('button', { name: 'Add account', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Account alias', { exact: false }).fill(alias);
  await dialog.getByLabel('Account kind', { exact: false }).selectOption(kind);
  await dialog.getByLabel('Boundary effective month', { exact: false }).fill('2026-01');
  if (funding) {
    await dialog.getByLabel('Include in funding boundary', { exact: true }).check();
    await dialog.getByLabel('Include in observed spendable cash', { exact: true }).check();
  }
  await saveDialog(page, 'Save account');
}

async function addEntry(page: Page, kind: string, amount: string, source: string | null, destination: string | null) {
  await page.getByRole('button', { name: 'Add entry', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Entry kind', { exact: false }).selectOption(kind);
  await dialog.getByLabel('Amount', { exact: false }).fill(amount);
  await dialog.getByLabel('Effective date', { exact: false }).fill(date);
  await dialog.getByLabel('Reporting month', { exact: false }).first().fill(month);
  if (source) await dialog.getByLabel('Source account', { exact: false }).selectOption(source);
  if (destination) await dialog.getByLabel('Destination account', { exact: false }).selectOption(destination);
  await saveDialog(page, 'Save entry');
}

async function addStatement(page: Page, card: string, amount: string) {
  await page.getByRole('link', { name: 'Statements', exact: true }).click();
  await page.getByRole('button', { name: 'Add statement', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Card account', { exact: false }).selectOption(card);
  await dialog.getByLabel('Cycle start', { exact: false }).fill('2026-09-01');
  await dialog.getByLabel('Cycle end / cutoff', { exact: false }).fill('2026-09-30');
  await dialog.getByLabel('Issue date', { exact: false }).fill('2026-10-01');
  await dialog.getByLabel('Due date', { exact: false }).fill('2026-10-28');
  await dialog.getByLabel('Amount due', { exact: false }).fill(amount);
  await dialog.getByLabel('Spending basis', { exact: false }).selectOption('net');
  await dialog.getByLabel('New spending', { exact: false }).fill(amount);
  await dialog.getByLabel('Spending evidence', { exact: false }).fill('Synthetic statement labelled net cycle spending');
  await saveDialog(page, 'Save statement');
}

test('browser records, pays and revises the worked month with authoritative Go totals', async ({ page, backend }, testInfo) => {
  test.setTimeout(90_000);
  await signIn(page);
  await addAccount(page, 'Funding', 'bank', true);
  await addAccount(page, 'Investments', 'investment');
  await addAccount(page, 'Card A', 'credit_card');
  await addAccount(page, 'Card B', 'credit_card');
  const accounts = (await backend.get<'AccountPage'>('/accounts')).items;
  expect(accounts).toHaveLength(4);
  const id = (alias: string) => {
    const account = accounts.find(value => value.alias === alias);
    expect(account, `created account ${alias}`).toBeDefined();
    return account!.id;
  };
  await addEntry(page, 'income', '5000', null, id('Funding'));
  await addEntry(page, 'direct_expense', '1000', id('Funding'), null);
  await addEntry(page, 'allocation', '800', id('Funding'), id('Investments'));
  await addStatement(page, id('Card A'), '1200');
  await addStatement(page, id('Card B'), '300');

  // Assert the wire data separately from formatted UI so a browser-side subtotal
  // cannot conceal a failed or duplicated financial posting.
  const entries = await backend.get<'EntryPage'>('/entries');
  expect(entries.items).toHaveLength(3);
  expect(entries.items.map(entry => entry.amount_minor).sort()).toEqual(['100000', '500000', '80000']);
  expect((await backend.get<'StatementPage'>('/statements')).items).toHaveLength(2);
  async function closeAndShowOverview() {
    await page.getByRole('link', { name: 'Plan & review', exact: true }).click();
    for (const label of ['Income reviewed', 'Cards reviewed', 'Major expenses reviewed', 'Allocations reviewed']) {
      await page.getByRole('checkbox', { name: label, exact: false }).check();
    }
    await page.getByRole('button', { name: 'Close month', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Reopen month', exact: true })).toBeEnabled();
    await page.getByRole('link', { name: 'Overview', exact: true }).click();
  }
  async function reopenForCorrection(reason: string) {
    await page.getByRole('link', { name: 'Plan & review', exact: true }).click();
    await page.getByRole('button', { name: 'Reopen month', exact: true }).click();
    await page.getByRole('dialog').getByLabel('Reopen reason', { exact: false }).fill(reason);
    await saveDialog(page, 'Reopen month');
  }
  await closeAndShowOverview();
  const summary = await backend.get<'Summary'>(`/months/${month}/summary`);
  expect(summary).toMatchObject({ income_minor: '500000', known_spending_minor: '250000', allocation_minor: '80000', monthly_surplus_minor: '170000', is_provisional: false });
  await expect(page.getByText('SGD 2,500.00', { exact: true }).first()).toBeVisible();
  await expect(page.getByText('SGD 1,700.00', { exact: true }).first()).toBeVisible();
  await expect(page.getByRole('button', { name: 'Add entry', exact: true })).toBeDisabled();
  await noHorizontalOverflow(page);
  await page.screenshot({ path: testInfo.outputPath('overview-reviewed.png'), fullPage: true });

  await reopenForCorrection('Record a confirmed synthetic card payment');
  await page.getByRole('link', { name: 'Statements', exact: true }).click();
  await page.getByRole('button', { name: 'Record payment Card A', exact: true }).click();
  const payment = page.getByRole('dialog');
  await payment.getByLabel('Payment amount', { exact: false }).fill('1200');
  await payment.getByLabel('Paying account', { exact: false }).selectOption(id('Funding'));
  await payment.getByLabel('Payment date', { exact: false }).fill(date);
  await saveDialog(page, 'Save payment');
  expect((await backend.get<'EntryPage'>('/entries')).items).toHaveLength(4);
  expect((await backend.get<'Summary'>(`/months/${month}/summary`)).gross_spending_minor).toBe('250000');
  await closeAndShowOverview();
  await expect(page.getByText('SGD 1,700.00', { exact: true }).first()).toBeVisible();

  await reopenForCorrection('Correct a synthetic statement cycle spending total');
  await page.getByRole('link', { name: 'Statements', exact: true }).click();
  await page.getByRole('button', { name: 'View statement Card A', exact: true }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Revise statement', exact: true }).click();
  const revision = page.getByRole('dialog');
  await revision.getByLabel('New spending', { exact: false }).fill('1100');
  await revision.getByLabel('Revision reason', { exact: false }).fill('Corrected synthetic cycle total');
  await saveDialog(page, 'Save revision');
  await closeAndShowOverview();
  expect(await backend.get<'Summary'>(`/months/${month}/summary`)).toMatchObject({
    income_minor: '500000', gross_spending_minor: '240000', allocation_minor: '80000', monthly_surplus_minor: '180000',
  });
  await expect(page.getByText('SGD 2,400.00', { exact: true }).first()).toBeVisible();
  await expect(page.getByText('SGD 1,800.00', { exact: true }).first()).toBeVisible();
  await noHorizontalOverflow(page);
  await page.screenshot({ path: testInfo.outputPath('overview-revised.png'), fullPage: true });
});
