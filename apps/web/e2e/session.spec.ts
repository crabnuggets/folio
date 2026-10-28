import { test, expect, bearer, signIn, noHorizontalOverflow } from './fixtures';

test('compiled app boots under Go CSP and preserves API/health routing', async ({ page, request }) => {
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  const response = await page.goto('/monthly/review');
  expect(response?.status()).toBe(200);
  expect(response?.headers()['content-security-policy']).toContain("script-src 'self'");
  await expect(page.getByLabel('Access token', { exact: false })).toBeVisible();
  await noHorizontalOverflow(page);
  expect(errors).toEqual([]);
  for (const path of ['/healthz', '/readyz']) {
    const health = await request.get(path);
    expect(health.status()).toBe(200);
    expect(health.headers()['content-type']).toContain('application/json');
  }
  const protectedRoute = await request.get('/api/v1/settings');
  expect(protectedRoute.status()).toBe(401);
  expect((await protectedRoute.json()).error.code).toBe('UNAUTHENTICATED');
});

test('invalid bearer is recoverable; sign out clears financial UI and token persistence', async ({ page }) => {
  await page.goto('/');
  const token = page.getByLabel('Access token', { exact: false });
  await token.fill('invalid-synthetic-bearer-only');
  await token.press('Enter');
  await expect(page.getByRole('alert')).toContainText('valid bearer access token');
  await token.fill(bearer);
  await token.press('Enter');
  await expect(page.getByRole('button', { name: 'Add entry', exact: true })).toBeEnabled();
  const persisted = await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }));
  expect(persisted).not.toContain(bearer);
  await page.getByRole('button', { name: 'Sign out', exact: true }).click();
  await expect(page.getByLabel('Access token', { exact: false })).toHaveValue('');
  await expect(page.getByLabel('Review month', { exact: true })).toHaveCount(0);
  await page.reload();
  await expect(page.getByLabel('Access token', { exact: false })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Export JSON', exact: true })).toHaveCount(0);
});

test('offline and failed read states recover without invented success', async ({ page, context }) => {
  await signIn(page);
  await context.setOffline(true);
  await expect(page.getByRole('status').filter({ hasText: 'Offline' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Add entry', exact: true })).toBeDisabled();
  await context.setOffline(false);
  await expect(page.getByRole('button', { name: 'Add entry', exact: true })).toBeEnabled();

  await page.route('**/api/v1/months/*/summary', route => route.abort('connectionfailed'));
  await page.getByRole('button', { name: 'Refresh', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Retry loading', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Add entry', exact: true })).toBeDisabled();
  await page.unroute('**/api/v1/months/*/summary');
  await page.getByRole('button', { name: 'Retry loading', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Add entry', exact: true })).toBeEnabled();
  await expect(page.getByRole('button', { name: 'Retry loading', exact: true })).toHaveCount(0);
});

test('320 CSS pixel reflow preserves navigation and labelled entry controls', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'phone-chromium', 'Smallest reflow is checked once in the phone project.');
  await page.setViewportSize({ width: 320, height: 740 });
  await signIn(page);
  for (const name of ['Overview', 'Records', 'Statements', 'Plan & review', 'Claims', 'Drafts', 'Accounts & settings']) {
    await page.getByRole('link', { name, exact: true }).click();
    await noHorizontalOverflow(page);
  }
  await page.getByRole('button', { name: 'Add entry', exact: true }).click();
  await expect(page.getByRole('dialog').getByLabel('Entry kind', { exact: false })).toBeVisible();
  await noHorizontalOverflow(page);
  const dialogDimensions = await page.getByRole('dialog').evaluate(dialog => ({ width: dialog.clientWidth, content: dialog.scrollWidth }));
  expect(dialogDimensions.content).toBeLessThanOrEqual(dialogDimensions.width + 1);
  await page.getByRole('button', { name: 'Close dialog', exact: true }).click();
});


test('keyboard navigation keeps page controls inert and returns to the dialog trigger on Escape', async ({ page }) => {
  await signIn(page);
  const trigger = page.getByRole('button', { name: 'Add entry', exact: true });
  await trigger.focus();
  await page.keyboard.press('Enter');
  const dialog = page.getByRole('dialog');
  await expect(dialog).toBeVisible();
  for (let index = 0; index < 14; index++) {
    await page.keyboard.press('Tab');
    // Native dialogs may tab into browser chrome (activeElement becomes body),
    // but must never focus an underlying page control.
    expect(await dialog.evaluate(element => document.activeElement === document.body || element.contains(document.activeElement))).toBe(true);
  }
  await page.keyboard.press('Escape');
  await expect(dialog).toHaveCount(0);
  await expect(trigger).toBeFocused();
});
