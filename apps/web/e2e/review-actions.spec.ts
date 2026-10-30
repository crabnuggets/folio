import { test, expect, signIn, addItem } from './fixtures';

test('clear and remove offer Undo, details preserve notes and rename, custom sections stay simple', async ({ page, backend }) => {
  await signIn(page); await addItem(page, 'Credit cards', 'Everyday card', '120.25');
  const row = page.getByLabel('Bill amount for Everyday card', { exact: true });
  await page.getByRole('button', { name: 'Everyday card', exact: true }).click();
  await page.getByLabel('Notes', { exact: true }).fill('Includes the refund already.');
  await page.getByLabel('Name', { exact: true }).fill('Daily card');
  await page.getByRole('button', { name: 'Save details' }).click();
  const renamed = page.getByLabel('Bill amount for Daily card', { exact: true }); await expect(renamed).toHaveValue('120.25');
  await page.getByRole('button', { name: 'Daily card', exact: true }).click(); await expect(page.getByLabel('Notes', { exact: true })).toHaveValue('Includes the refund already.');
  await page.getByRole('button', { name: 'Clear amount', exact: true }).click(); await expect(renamed).toHaveValue('');
  await page.getByRole('button', { name: 'Undo last change' }).click(); await expect(renamed).toHaveValue('120.25');
  await page.getByRole('button', { name: 'Daily card', exact: true }).click(); await page.getByRole('button', { name: 'Skip this month' }).click(); await expect(renamed).toHaveCount(0);
  await page.getByRole('button', { name: 'Undo last change' }).click(); await expect(renamed).toHaveValue('120.25');
  await page.getByText('Sheet options', { exact: true }).click();
  await page.getByRole('textbox', { name: 'Section name' }).fill('Family'); await page.getByRole('combobox', { name: 'Side' }).selectOption('out');
  await page.getByRole('button', { name: 'Add section', exact: true }).click(); await expect(page.getByRole('region', { name: 'Family', exact: true })).toBeVisible();
  const cardsRename = page.getByRole('textbox', { name: 'Credit cards', exact: true }); await cardsRename.fill('Card bills');
  await cardsRename.locator('..').locator('..').getByRole('button', { name: 'Rename' }).click();
  await expect(page.getByRole('region', { name: 'Card bills', exact: true })).toBeVisible();
  await expect(page.getByLabel('Bill amount for Daily card', { exact: true })).toBeVisible(); await expect(row).toHaveCount(0);
  const sheet = await backend.get<'MonthlySheet'>('/monthly-sheets/2026-10'); expect(sheet.sections.flatMap(section => section.items)[0].note).toBe('Includes the refund already.');
});
