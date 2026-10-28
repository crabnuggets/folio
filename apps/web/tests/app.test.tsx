import { afterEach, beforeAll, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { App } from '../src/App';
import type { S } from '../src/lib/api';

beforeAll(() => {
  // jsdom does not implement native dialog modal behaviour. Real focus/inertness
  // is covered by the separate Playwright suite; here we verify recovery ancestry.
  HTMLDialogElement.prototype.showModal = function () { this.setAttribute('open', ''); };
  HTMLDialogElement.prototype.close = function () { this.removeAttribute('open'); };
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); location.hash = ''; });
const summary: S['Summary'] = {
  month: '2026-10', currency: 'SGD', revision: 0, income_minor: '0', gross_spending_minor: '0', confirmed_recoveries_minor: '0', known_spending_minor: '0', net_personal_cost_minor: '0', allocation_minor: '0', provisional_surplus_minor: '0', funding_inflows_minor: '0', funding_outflows_minor: '0', funding_remainder_minor: '0', unmatched_claims_minor: '0', unmatched_receipts_minor: '0', monthly_surplus_minor: null, consumption_savings_minor: null, projected_surplus_minor: null, projected_funding_remainder_minor: null, savings_rate: null, is_provisional: true, spending_complete: false, funding_complete: false, missing_items: ['income_not_reviewed'], statements: [], forecast_lines: [], snapshots: [],
};
const settings: S['Settings'] = { currency: 'SGD', currency_exponent: 2, timezone: 'Asia/Singapore', version: 1 };
function mount() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={client}><App /></QueryClientProvider>);
}
function fakeServer(mutate?: (url: string, init?: RequestInit) => Promise<Response>, accounts: S['Account'][] = []) {
  return vi.fn<typeof fetch>(async (input, init) => {
    const url = String(input);
    const send = (value: unknown) => new Response(JSON.stringify(value), { status: 200 });
    if (init?.method !== 'GET' && mutate) return mutate(url, init);
    if (url.endsWith('/settings')) return send(settings);
    if (url.includes('/accounts?')) return send({ items: accounts, next_cursor: null });
    if (url.endsWith('/summary')) return send(summary);
    if (url.endsWith('/plan')) return send({ version: 1, lines: [] });
    if (/\/months\/\d{4}-\d{2}$/.test(url)) return send({ month: '2026-10', version: 1, revision: 0, state: 'open', reviewed_card_ids: [], closed_at: null, checklist: {} });
    return send({ items: [], next_cursor: null });
  });
}
async function signIn() {
  fireEvent.change(screen.getByLabelText('Access token', { exact: false }), { target: { value: 'synthetic-test-only-bearer' } });
  fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));
  await screen.findByRole('heading', { name: 'A clearer view of your month.' });
}

it('keeps safe retry controls inside the modal after a lost financial response', async () => {
  const submissions: RequestInit[] = [];
  const transport = fakeServer(async (_url, init) => {
    submissions.push(init!);
    if (submissions.length === 1) throw new TypeError('response lost');
    return new Response(JSON.stringify({ id: 'confirmed-income' }), { status: 201 });
  }, [{ id: 'funding-account', version: 1, created_at: '', updated_at: '', alias: 'Funding', kind: 'bank', currency: 'SGD', include_in_liquid_cash: true, active: true, funding_boundary: [{ effective_month: '2026-01', included: true }] }]);
  vi.stubGlobal('fetch', transport); mount(); await signIn();
  fireEvent.click(screen.getByRole('button', { name: 'Add entry' }));
  const dialog = screen.getByRole('dialog', { name: 'Add entry' });
  fireEvent.change(within(dialog).getByLabelText('Amount', { exact: false }), { target: { value: '5000' } });
  const destination = within(dialog).getByLabelText('Destination account', { exact: false });
  fireEvent.change(destination, { target: { value: 'funding-account' } });
  fireEvent.click(within(dialog).getByRole('button', { name: 'Save entry' }));
  const retry = await within(dialog).findByRole('button', { name: 'Retry same action' });
  expect(retry.closest('dialog')).toBe(dialog);
  expect(within(dialog).getByRole('button', { name: 'Close dialog' })).toHaveProperty('disabled', true);
  expect(within(dialog).getByLabelText('Amount', { exact: false })).toHaveProperty('value', '5000');
  fireEvent.click(retry);
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  expect(submissions).toHaveLength(2);
  expect(submissions[1].body).toBe(submissions[0].body);
  expect(submissions[1].headers).toEqual(submissions[0].headers);
});

it('clears private query state on sign out and never persists the token', async () => {
  const local = vi.spyOn(Storage.prototype, 'setItem');
  vi.stubGlobal('fetch', fakeServer()); mount(); await signIn();
  fireEvent.click(screen.getByRole('button', { name: 'Sign out' }));
  await screen.findByRole('button', { name: 'Sign in' });
  expect(screen.queryByLabelText('Monthly funding summary')).toBeNull();
  expect(local).not.toHaveBeenCalled(); local.mockRestore();
});

it('tolerates malformed navigation fragments', async () => {
  location.hash = '#%bad'; vi.stubGlobal('fetch', fakeServer()); mount(); await signIn();
  expect(screen.getByRole('heading', { name: 'A clearer view of your month.' })).toBeTruthy();
});
