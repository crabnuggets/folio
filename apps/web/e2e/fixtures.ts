import { randomUUID } from 'node:crypto';
import { test as base, expect, type APIRequestContext, type Page } from '@playwright/test';
import type { components } from '../src/lib/schema';
type S = components['schemas'];
export const bearer = 'folio-browser-e2e-synthetic-bearer-only';
export const month = '2026-10';
export const sectionIds = { income: '00000000-0000-4000-8000-000000000001', otherIn: '00000000-0000-4000-8000-000000000002', recurring: '00000000-0000-4000-8000-000000000003', cards: '00000000-0000-4000-8000-000000000004', otherOut: '00000000-0000-4000-8000-000000000005' };
export class Backend {
  constructor(readonly request: APIRequestContext) {}
  async get<K extends keyof S>(path: string): Promise<S[K]> {
    const response = await this.request.get(`/api/v1${path}`);
    expect(response.status(), await response.text()).toBe(200); return response.json();
  }
  async post<K extends keyof S>(path: string, data: unknown, options: { status?: number; version?: number; key?: string; method?: 'POST' | 'PUT' } = {}): Promise<S[K]> {
    const response = await this.request.fetch(`/api/v1${path}`, { method: options.method ?? 'POST', data, headers: { 'Idempotency-Key': options.key ?? randomUUID(), ...(options.version == null ? {} : { 'If-Match': `"${options.version}"` }) } });
    expect(response.status(), await response.text()).toBe(options.status ?? 200); return response.json();
  }
  async open(selectedMonth = month) { return this.post<'MonthlySheet'>(`/monthly-sheets/${selectedMonth}/open`, {}); }
  async add(name: string, amount: string, section = sectionIds.income, prefs: { repeat_monthly?: boolean; usual_amount_minor?: string | null } = {}, selectedMonth = month) {
    const sheet = await this.open(selectedMonth);
    return this.post<'MonthlySheet'>(`/monthly-sheets/${selectedMonth}/items`, { name, amount_minor: amount, section_id: section, ...prefs }, { version: sheet.version });
  }
}
export const test = base.extend<{ backend: Backend }>({ backend: [async ({ playwright }, use) => {
  const request = await playwright.request.newContext({ baseURL: 'http://127.0.0.1:18080', extraHTTPHeaders: { Authorization: `Bearer ${bearer}` } });
  const reset = await request.post('/__e2e/reset'); expect(reset.status()).toBe(204);
  await use(new Backend(request)); await request.dispose();
}, { auto: true }] });
export { expect };
export async function signIn(page: Page) {
  await page.goto('/'); await page.getByLabel('Access token', { exact: false }).fill(bearer);
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Add item to Income', exact: true })).toBeEnabled();
  const year = await page.getByRole('spinbutton', { name: 'Year', exact: true }).inputValue();
  const selectedMonth = await page.getByRole('button', { name: 'Month', exact: true }).textContent();
  const monthName = new Intl.DateTimeFormat('en', { month: 'long' }).format(new Date(2020, Number(month.slice(5)) - 1, 1));
  const loaded = year === month.slice(0, 4) && selectedMonth?.trim() === monthName ? undefined
    : page.waitForResponse(response => response.url().endsWith(`/monthly-sheets/${month}/open`));
  await selectMonth(page, month);
  await loaded;
  await expect(page.getByRole('button', { name: 'Add item to Income', exact: true })).toBeEnabled();
}
export async function selectMonth(page: Page, selectedMonth: string) {
  const year = page.getByRole('spinbutton', { name: 'Year', exact: true });
  await year.fill(selectedMonth.slice(0, 4)); await year.press('Enter');
  await expect(year).toBeEnabled();
  await page.getByRole('button', { name: 'Month', exact: true }).click();
  await page.getByRole('button', { name: new Intl.DateTimeFormat('en', { month: 'long' }).format(new Date(2020, Number(selectedMonth.slice(5)) - 1, 1)), exact: true }).click();
  await expect(page.getByRole('button', { name: 'Month', exact: true })).toBeEnabled();
}
export async function addItem(page: Page, section: string, name: string, amount: string, repeat = false, usual = false) {
  const region = page.getByRole('region', { name: section, exact: true });
  await region.getByRole('button', { name: `Add item to ${section}`, exact: true }).click();
  const form = region.getByRole('form', { name: `Add item to ${section}` });
  await form.getByRole('combobox', { name: 'Name', exact: true }).fill(name);
  await form.getByLabel(section === 'Credit cards' ? 'Bill amount' : 'Amount', { exact: true }).fill(amount);
  if (repeat) await form.getByRole('switch', { name: 'Repeat monthly' }).check();
  if (usual) await form.getByLabel('Suggest this amount each month').check();
  await form.getByRole('button', { name: 'Save item', exact: true }).click();
  await expect(form).toHaveCount(0);
}
export async function noHorizontalOverflow(page: Page) {
  const dimensions = await page.evaluate(() => ({ viewport: document.documentElement.clientWidth, document: document.documentElement.scrollWidth, body: document.body.scrollWidth }));
  expect(dimensions.document, JSON.stringify(dimensions)).toBeLessThanOrEqual(dimensions.viewport + 1);
  expect(dimensions.body, JSON.stringify(dimensions)).toBeLessThanOrEqual(dimensions.viewport + 1);
}
