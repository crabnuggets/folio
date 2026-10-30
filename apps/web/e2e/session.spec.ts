import { test, expect, bearer, signIn, noHorizontalOverflow } from './fixtures';

test('compiled app boots under Go CSP and preserves API/health routing', async ({ page, request }) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  const response = await page.goto('/monthly/sheet'); expect(response?.status()).toBe(200);
  expect(response?.headers()['content-security-policy']).toContain("script-src 'self'");
  await expect(page.getByLabel('Access token', { exact: false })).toBeVisible(); await noHorizontalOverflow(page); expect(errors).toEqual([]);
  for (const path of ['/healthz', '/readyz']) { const health = await request.get(path); expect(health.status()).toBe(200); expect(health.headers()['content-type']).toContain('application/json'); }
  expect((await request.get('/api/v1/settings')).status()).toBe(401);
});

test('invalid bearer is recoverable; sign out clears financial UI and token persistence', async ({ page }) => {
  await page.goto('/'); const token = page.getByLabel('Access token', { exact: false });
  await token.fill('invalid-synthetic-bearer-only'); await token.press('Enter'); await expect(page.getByRole('alert')).toContainText('valid bearer access token');
  await token.fill(bearer); await token.press('Enter'); await expect(page.getByRole('region', { name: 'Monthly summary' })).toBeVisible();
  expect(await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }))).not.toContain(bearer);
  await page.getByRole('button', { name: 'Settings', exact: true }).click(); await page.getByRole('button', { name: 'Sign out', exact: true }).click();
  await expect(page.getByLabel('Access token', { exact: false })).toHaveValue(''); await expect(page.getByRole('region', { name: 'Monthly summary' })).toHaveCount(0);
  await page.reload(); await expect(page.getByLabel('Access token', { exact: false })).toBeVisible();
});

test('offline and failed read states preserve the sheet and recover without invented success', async ({ page, context }) => {
  await signIn(page); await context.setOffline(true); await expect(page.getByRole('status').filter({ hasText: 'Offline' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Add item to Income', exact: true })).toBeDisabled(); await context.setOffline(false);
  await expect(page.getByRole('button', { name: 'Add item to Income', exact: true })).toBeEnabled();
  await page.route('**/api/v1/monthly-sheets/2026-11/open', route => route.abort('connectionfailed'));
  await page.getByLabel('Month', { exact: true }).fill('2026-11'); await expect(page.getByRole('button', { name: 'Retry loading' })).toBeVisible();
  await page.unroute('**/api/v1/monthly-sheets/2026-11/open'); await page.getByRole('button', { name: 'Retry loading' }).click();
  await expect(page.getByRole('region', { name: 'Income', exact: true })).toBeVisible();
});

test('320 CSS pixel reflow and keyboard controls preserve labelled card entry and focus', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 740 }); await signIn(page); await noHorizontalOverflow(page);
  const trigger = page.getByRole('button', { name: 'Add item to Credit cards' }); await trigger.focus(); await page.keyboard.press('Enter');
  const form = page.getByRole('form', { name: 'Add item to Credit cards' }); await expect(form.getByRole('combobox', { name: 'Name' })).toBeFocused();
  await expect(form.getByLabel('Bill amount', { exact: true })).toBeVisible(); await noHorizontalOverflow(page);
  await form.getByRole('combobox', { name: 'Name' }).fill('Phone card'); await page.keyboard.press('Tab');
  // History is empty, so the next input is the monthly bill amount.
  await expect(form.getByLabel('Bill amount', { exact: true })).toBeFocused(); await page.keyboard.type('12.34');
  await form.getByRole('button', { name: 'Save item' }).click();
  await expect(page.getByLabel('Bill amount for Phone card', { exact: true })).toBeFocused(); await noHorizontalOverflow(page);
});
