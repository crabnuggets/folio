import { test, expect, bearer, sectionIds, signIn } from './fixtures';

test('changing month cannot reuse a late load from the previous month', async ({ page, backend }) => {
  await page.clock.setFixedTime(new Date('2026-11-01T04:00:00Z'));
  await backend.add('Salary', '10000');
  let changingMonth = false;
  let settingsCount = 0;
  let releasePrevious: (() => void) | undefined;
  let releaseCurrent = () => {};
  let currentStarted = () => {};
  const current = new Promise<void>(resolve => { currentStarted = resolve; });
  await page.route('**/api/v1/settings', async route => {
    settingsCount++;
    if (changingMonth) {
      await new Promise<void>(resolve => { releaseCurrent = resolve; currentStarted(); });
    } else if (settingsCount === 3) {
      await new Promise<void>(resolve => { releasePrevious = resolve; });
    }
    await route.continue();
  });
  try {
    await page.goto('/');
    await page.getByLabel('Access token', { exact: false }).fill(bearer);
    await page.getByRole('button', { name: 'Sign in', exact: true }).click();
    await expect(page.getByRole('region', { name: 'Income', exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Month', exact: true })).toBeEnabled();
    changingMonth = true;
    await page.getByRole('button', { name: 'Month', exact: true }).click();
    await page.getByRole('button', { name: 'October', exact: true }).click();
    await current;
    if (releasePrevious) {
      const previousLoaded = page.waitForResponse(response => response.url().endsWith('/monthly-sheets/2026-11/open'));
      releasePrevious();
      await previousLoaded;
    }
    releaseCurrent();
    await expect(page.getByLabel('Amount for Salary', { exact: true })).toHaveValue('100.00');
    await expect(page.getByRole('button', { name: 'Month', exact: true })).toHaveText('October');
  } finally { releasePrevious?.(); releaseCurrent(); }
});

test('closing details preserves focus moved to another amount before the next frame', async ({ page, backend }) => {
  await page.clock.setFixedTime(new Date('2026-10-15T04:00:00Z'));
  await backend.add('Salary', '10000');
  await backend.add('Refund', '2000', sectionIds.otherIn);
  await signIn(page);
  await page.getByRole('button', { name: 'Salary', exact: true }).click();
  await expect(page.getByLabel('Notes', { exact: true })).toBeVisible();
  const nextId = await page.getByLabel('Amount for Refund', { exact: true }).getAttribute('id');
  const focused = await page.evaluate(async target => {
    const cancel = Array.from(document.querySelectorAll<HTMLButtonElement>('.item-details button')).find(button => button.textContent === 'Cancel')!;
    cancel.click();
    document.getElementById(target!)!.focus();
    await new Promise<void>(resolve => requestAnimationFrame(() => resolve()));
    return document.activeElement?.id;
  }, nextId);
  expect(focused).toBe(nextId);
});
