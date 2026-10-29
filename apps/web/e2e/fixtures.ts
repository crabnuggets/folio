import { randomUUID } from 'node:crypto';
import { test as base, expect, type APIRequestContext, type Page } from '@playwright/test';
import type { components } from '../src/lib/schema';

type S = components['schemas'];
export const bearer = 'folio-browser-e2e-synthetic-bearer-only';
export const month = '2026-10';
export const date = '2026-10-25';

export class Backend {
  constructor(readonly request: APIRequestContext) {}

  async get<K extends keyof S>(path: string): Promise<S[K]> {
    const response = await this.request.get(`/api/v1${path}`);
    expect(response.status(), await response.text()).toBe(200);
    return response.json();
  }

  async post<K extends keyof S>(path: string, data: unknown, options: { status?: number; version?: number; key?: string } = {}): Promise<S[K]> {
    const response = await this.request.post(`/api/v1${path}`, {
      data,
      headers: {
        'Idempotency-Key': options.key ?? randomUUID(),
        ...(options.version == null ? {} : { 'If-Match': `"${options.version}"` }),
      },
    });
    expect(response.status(), await response.text()).toBe(options.status ?? 201);
    return response.json();
  }
}

export const test = base.extend<{ backend: Backend }>({
  backend: [async ({ playwright }, use) => {
    const request = await playwright.request.newContext({
      baseURL: 'http://127.0.0.1:18080',
      extraHTTPHeaders: { Authorization: `Bearer ${bearer}` },
    });
    const reset = await request.post('/__e2e/reset');
    expect(reset.status()).toBe(204);
    await use(new Backend(request));
    await request.dispose();
  }, { auto: true }],
});
export { expect };

export async function signIn(page: Page) {
  await page.goto('/');
  await page.getByLabel('Access token', { exact: false }).fill(bearer);
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await expect(page.getByLabel('Review month', { exact: true })).toBeVisible();
  await page.getByLabel('Review month', { exact: true }).fill(month);
  await expect(page.getByRole('button', { name: 'Refresh', exact: true })).toBeEnabled();
}

export function accountInput(alias: string, kind: S['AccountInput']['kind'] = 'bank'): S['AccountInput'] {
  return {
    alias, kind, currency: 'SGD', active: true,
    include_in_liquid_cash: kind === 'bank',
    funding_boundary: kind === 'bank' ? [{ effective_month: '2026-01', included: true }] : [],
  };
}

export function statementInput(accountId: string, spending: string | null): S['StatementInput'] {
  return {
    account_id: accountId, currency: 'SGD', cycle_start: '2026-09-01', cycle_end: '2026-09-30',
    issue_date: '2026-10-01', due_date: '2026-10-28', amount_due_minor: spending ?? '120000',
    net_spending_minor: spending, spending_evidence: spending == null ? '' : 'Synthetic statement net cycle spending',
  };
}

export async function seedWorkedMonth(backend: Backend) {
  const funding = await backend.post<'Account'>('/accounts', accountInput('Funding'));
  const investments = await backend.post<'Account'>('/accounts', accountInput('Investments', 'investment'));
  const cardA = await backend.post<'Account'>('/accounts', accountInput('Card A', 'credit_card'));
  const cardB = await backend.post<'Account'>('/accounts', accountInput('Card B', 'credit_card'));
  await backend.post<'Entry'>('/entries', {
    kind: 'income', amount_minor: '500000', currency: 'SGD', effective_date: date, destination_account_id: funding.id,
  });
  await backend.post<'Entry'>('/entries', {
    kind: 'direct_expense', amount_minor: '100000', currency: 'SGD', effective_date: date, source_account_id: funding.id,
  });
  await backend.post<'Entry'>('/entries', {
    kind: 'allocation', amount_minor: '80000', currency: 'SGD', effective_date: date,
    source_account_id: funding.id, destination_account_id: investments.id,
  });
  const statementA = await backend.post<'Statement'>('/statements', statementInput(cardA.id, '120000'));
  const statementB = await backend.post<'Statement'>('/statements', statementInput(cardB.id, '30000'));
  return { funding, investments, cardA, cardB, statementA, statementB };
}

export async function noHorizontalOverflow(page: Page) {
  const dimensions = await page.evaluate(() => ({
    viewport: document.documentElement.clientWidth,
    document: document.documentElement.scrollWidth,
    body: document.body.scrollWidth,
  }));
  expect(dimensions.document, JSON.stringify(dimensions)).toBeLessThanOrEqual(dimensions.viewport + 1);
  expect(dimensions.body, JSON.stringify(dimensions)).toBeLessThanOrEqual(dimensions.viewport + 1);
}
