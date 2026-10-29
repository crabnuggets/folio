import { test, expect, signIn, sectionIds } from './fixtures';

test('history suggestions retain typed amount and saved preferences, support keyboard and avoid duplicate rows', async ({ page, backend }) => {
  await backend.add('House savings', '90000', sectionIds.recurring, { repeat_monthly: true, usual_amount_minor: '90000' }, '2026-09');
  await backend.add('Car service', '18000', sectionIds.otherOut, {}, '2026-09');
  await signIn(page);
  const section = page.getByRole('region', { name: 'Other money out', exact: true });
  await section.getByRole('button', { name: 'Add item to Other money out' }).click();
  const form = section.getByRole('form');
  await form.getByLabel('Amount', { exact: true }).fill('195.25');
  const name = form.getByRole('combobox', { name: 'Name' }); await name.fill('car');
  await expect(form.getByRole('option', { name: /Car service/ })).toBeVisible();
  await name.press('ArrowDown'); await name.press('Enter');
  await expect(form.getByLabel('Amount', { exact: true })).toHaveValue('195.25');
  await expect(form.getByRole('switch', { name: 'Repeat monthly' })).not.toBeChecked();
  await form.getByRole('button', { name: 'Save item' }).click(); await expect(form).toHaveCount(0);
  const current = page.getByLabel('Amount for Car service', { exact: true }); await expect(current).toHaveValue('195.25');
  await section.getByRole('button', { name: 'Add item to Other money out' }).click();
  await form.getByRole('combobox', { name: 'Name' }).fill('  CAR   service ');
  await form.getByLabel('Amount', { exact: true }).fill('999');
  await form.getByRole('button', { name: 'Save item' }).click();
  await expect(current).toBeFocused(); await expect(current).toHaveValue('195.25');
  const sheet = await backend.get<'MonthlySheet'>('/monthly-sheets/2026-10');
  expect(sheet.sections.flatMap(section => section.items).filter(item => item.name === 'Car service')).toHaveLength(1);
  // Reuse a repeating identity that was skipped this month. History keeps its preferences.
  const recurring = sheet.sections.flatMap(section => section.items).find(item => item.name === 'House savings')!;
  await backend.post<'MonthlySheet'>(`/monthly-sheets/2026-10/items/${recurring.id}/remove`, {}, { version: sheet.version });
  await page.reload(); await signIn(page);
  await section.getByRole('button', { name: 'Add item to Other money out' }).click();
  await form.getByRole('combobox', { name: 'Name' }).fill('house');
  await form.getByRole('option', { name: /House savings/ }).click();
  await expect(form.getByRole('switch', { name: 'Repeat monthly' })).toBeChecked();
  await expect(form.getByLabel('Suggest this amount each month')).toBeChecked();
  await expect(form.getByLabel('Amount', { exact: true })).toHaveValue('');
  await form.getByLabel('Amount', { exact: true }).fill('925');
  await form.getByRole('button', { name: 'Save item' }).click(); await expect(form).toHaveCount(0);
  const final = await backend.get<'MonthlySheet'>('/monthly-sheets/2026-10');
  const reused = final.sections.flatMap(section => section.items).find(item => item.id === recurring.id)!;
  expect(reused).toMatchObject({ amount_minor: '92500', repeat_monthly: true, usual_amount_minor: '90000', section_id: sectionIds.otherOut });
});

test('suggestions can be dismissed while free typing a new card with recurrence off', async ({ page, backend }) => {
  await backend.add('Travel card', '10000', sectionIds.cards, {}, '2026-09');
  await signIn(page);
  const region = page.getByRole('region', { name: 'Credit cards', exact: true });
  await region.getByRole('button', { name: 'Add item to Credit cards' }).click();
  const form = region.getByRole('form'); const name = form.getByRole('combobox', { name: 'Name' });
  await name.fill('Travel'); await expect(form.getByRole('listbox')).toBeVisible();
  await name.press('Escape'); await expect(form.getByRole('listbox')).toHaveCount(0);
  await expect(name).toHaveValue('Travel'); await name.fill('New card');
  await expect(form.getByRole('switch', { name: 'Repeat monthly' })).not.toBeChecked();
  await form.getByLabel('Bill amount', { exact: true }).fill('0');
  await form.getByRole('button', { name: 'Save item' }).click();
  await expect(page.getByLabel('Bill amount for New card', { exact: true })).toHaveValue('0.00');
  const sheet = await backend.get<'MonthlySheet'>('/monthly-sheets/2026-10');
  expect(sheet.sections.flatMap(section => section.items).find(item => item.name === 'New card')?.repeat_monthly).toBe(false);
});

test('explicitly suggesting the entered amount replaces a historical usual amount', async ({ page, backend }) => {
  await backend.add('Allowance', '10000', sectionIds.recurring, { repeat_monthly: true, usual_amount_minor: '10000' }, '2026-09');
  const sheet = await backend.open(); const item = sheet.sections.flatMap(section => section.items)[0];
  await backend.post<'MonthlySheet'>(`/monthly-sheets/2026-10/items/${item.id}/remove`, {}, { version: sheet.version });
  await signIn(page);
  const section = page.getByRole('region', { name: 'Recurring expenses', exact: true });
  await section.getByRole('button', { name: 'Add item to Recurring expenses' }).click();
  const form = section.getByRole('form'); await form.getByRole('combobox', { name: 'Name' }).fill('Allow');
  await form.getByRole('option', { name: /Allowance/ }).click(); await form.getByLabel('Amount', { exact: true }).fill('135');
  const usual = form.getByLabel('Suggest this amount each month'); await expect(usual).toBeChecked(); await usual.uncheck(); await usual.check();
  await form.getByRole('button', { name: 'Save item' }).click(); await expect(form).toHaveCount(0);
  const saved = await backend.get<'MonthlySheet'>('/monthly-sheets/2026-10');
  expect(saved.sections.flatMap(section => section.items)[0]).toMatchObject({ amount_minor: '13500', usual_amount_minor: '13500', repeat_monthly: true });
});
