import { afterEach, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { App } from '../src/App';
import type { S } from '../src/lib/api';
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
const settings: S['Settings'] = { currency: 'SGD', currency_exponent: 2, timezone: 'Asia/Singapore', version: 1 };
const emptySheet: S['MonthlySheet'] = { month: '2026-10', version: 1, currency: 'SGD', currency_exponent: 2, currency_locked: false, has_legacy_data: false, undo_token: null, money_in_minor: '0', money_out_minor: '0', remainder_minor: '0', missing_count: 0, sections: [{ id: 'income', name: 'Income', direction: 'in', amount_label: 'Amount', total_minor: '0', missing_count: 0, items: [] }] };
function mount() { render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } } })}><App /></QueryClientProvider>); }
function transport() { return vi.fn<typeof fetch>(async (input, init) => {
  const url = String(input); if (url.endsWith('/settings')) return new Response(JSON.stringify(settings));
  if (url.endsWith('/open')) return new Response(JSON.stringify(emptySheet)); if (url.includes('monthly-item-suggestions')) return new Response('{"items":[]}');
  if (init?.method !== 'GET') throw new TypeError('Synthetic connection loss'); return new Response(JSON.stringify(emptySheet));
}); }
async function signIn() { fireEvent.change(screen.getByLabelText('Access token', { exact: false }), { target: { value: 'synthetic-test-only-bearer' } }); fireEvent.click(screen.getByRole('button', { name: 'Sign in' })); await screen.findByRole('heading', { name: 'Monthly remainder' }); }
it('keeps month controls disabled until the latest load finishes', async () => {
  let settingsReads = 0;
  let releaseOld = () => {};
  let releaseNew = () => {};
  const oldSettings = new Promise<void>(resolve => { releaseOld = resolve; });
  const newSettings = new Promise<void>(resolve => { releaseNew = resolve; });
  const opened: string[] = [];
  vi.stubGlobal('fetch', vi.fn<typeof fetch>(async input => {
    const path = String(input);
    if (path.endsWith('/settings')) {
      const read = ++settingsReads;
      if (read === 3) await oldSettings;
      if (read === 4) await newSettings;
      return new Response(JSON.stringify(settings));
    }
    const selectedMonth = path.match(/monthly-sheets\/(\d{4}-\d{2})\/open$/)?.[1];
    if (selectedMonth) {
      opened.push(selectedMonth);
      return new Response(JSON.stringify({ ...emptySheet, month: selectedMonth }));
    }
    return new Response(JSON.stringify({ available: false, connected: false, sharing: false, models: [], reason: '' }));
  }));
  mount();
  fireEvent.change(screen.getByLabelText('Access token', { exact: false }), { target: { value: 'synthetic-test-only-bearer' } });
  fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));
  await waitFor(() => expect(settingsReads).toBe(3));
  await act(async () => {}); // Settle the earlier response while the latest stays blocked.
  const year = screen.getByLabelText('Year', { exact: true });
  expect(year).toHaveProperty('disabled', true);
  await act(async () => { releaseOld(); });
  await screen.findByRole('heading', { name: 'Monthly remainder' });
  const nextYear = String(Number(opened[0].slice(0, 4)) + 1);
  const nextMonth = `${nextYear}${opened[0].slice(4)}`;
  fireEvent.change(year, { target: { value: nextYear } }); fireEvent.blur(year);
  await waitFor(() => expect(settingsReads).toBe(4));
  expect(screen.getByLabelText('Year', { exact: true })).toHaveProperty('disabled', true);
  await act(async () => { releaseNew(); });
  expect(opened).toContain(nextMonth);
});
it('preserves add values and blocks month change after an uncertain write', async () => {
  vi.stubGlobal('fetch', transport()); mount(); await signIn(); fireEvent.click(screen.getByRole('button', { name: 'Add item to Income' }));
  fireEvent.change(screen.getByRole('combobox', { name: 'Name' }), { target: { value: 'Salary' } }); fireEvent.change(screen.getByLabelText('Amount', { exact: true }), { target: { value: '6400' } }); fireEvent.click(screen.getByRole('button', { name: 'Save item' }));
  await screen.findByRole('button', { name: 'Retry same action' }); expect(screen.getByLabelText('Amount', { exact: true })).toHaveProperty('value', '6400'); expect(screen.getByLabelText('Month', { exact: true })).toHaveProperty('disabled', true); expect(screen.getByLabelText('Year', { exact: true })).toHaveProperty('disabled', true); expect(screen.getByRole('button', { name: 'Sign out' })).toHaveProperty('disabled', true); expect(screen.queryByRole('navigation')).toBeNull();
});
it('clears private state on sign out and never persists the bearer', async () => {
  const storage = vi.spyOn(Storage.prototype, 'setItem'); vi.stubGlobal('fetch', transport()); mount(); await signIn();
  expect(screen.queryByRole('button', { name: 'Settings' })).toBeNull();
  const header = screen.getByRole('banner'); expect(header.contains(screen.getByRole('button', { name: 'Export JSON' }))).toBe(true); expect(header.contains(screen.getByRole('button', { name: 'Sign out' }))).toBe(true);
  fireEvent.click(screen.getByRole('button', { name: 'Sign out' }));
  await screen.findByRole('button', { name: 'Sign in' }); expect(screen.queryByRole('heading', { name: 'Monthly remainder' })).toBeNull(); expect(storage).not.toHaveBeenCalled(); storage.mockRestore();
});
