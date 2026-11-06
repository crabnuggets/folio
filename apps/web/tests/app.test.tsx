import { afterEach, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { StrictMode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { App } from '../src/App';
import { ApiClient, type S } from '../src/lib/api';
import { MonthlySheet } from '../src/MonthlySheet';
afterEach(() => { cleanup(); vi.unstubAllGlobals(); vi.useRealTimers(); });
const settings: S['Settings'] = { currency: 'SGD', currency_exponent: 2, timezone: 'Asia/Singapore', version: 1 };
const emptySheet: S['MonthlySheet'] = { month: '2026-10', version: 1, currency: 'SGD', currency_exponent: 2, currency_locked: false, has_legacy_data: false, undo_token: null, money_in_minor: '0', money_out_minor: '0', remainder_minor: '0', missing_count: 0, sections: [{ id: 'income', name: 'Income', direction: 'in', amount_label: 'Amount', total_minor: '0', missing_count: 0, items: [] }] };
function mount() { render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } } })}><App /></QueryClientProvider>); }
function transport() { return vi.fn<typeof fetch>(async (input, init) => {
  const url = String(input); if (url.endsWith('/settings')) return new Response(JSON.stringify(settings));
  if (url === '/auth/config') return new Response('{"development_key":true}');
  if (url.endsWith('/open')) return new Response(JSON.stringify(emptySheet)); if (url.includes('monthly-item-suggestions')) return new Response('{"items":[]}');
  if (init?.method !== 'GET') throw new TypeError('Synthetic connection loss'); return new Response(JSON.stringify(emptySheet));
}); }
async function signIn() { fireEvent.change(await screen.findByLabelText('Access token', { exact: false }), { target: { value: 'synthetic-test-only-bearer' } }); fireEvent.click(screen.getByRole('button', { name: 'Sign in' })); await screen.findByRole('heading', { name: 'Monthly remainder' }); }
it('keeps month controls disabled until the latest load finishes', async () => {
  let settingsReads = 0;
  let releaseOld = () => {};
  let releaseNew = () => {};
  const oldSettings = new Promise<void>(resolve => { releaseOld = resolve; });
  const newSettings = new Promise<void>(resolve => { releaseNew = resolve; });
  const opened: string[] = [];
  const api = new ApiClient(async input => {
    const path = String(input);
    if (path.endsWith('/settings')) {
      const read = ++settingsReads;
      if (read === 2) await oldSettings;
      if (read === 3) await newSettings;
      return new Response(JSON.stringify(settings));
    }
    const selectedMonth = path.match(/monthly-sheets\/(\d{4}-\d{2})\/open$/)?.[1];
    if (selectedMonth) {
      opened.push(selectedMonth);
      return new Response(JSON.stringify({ ...emptySheet, month: selectedMonth }));
    }
    return new Response(JSON.stringify({ available: false, connected: false, sharing: false, models: [], reason: '' }));
  });
  api.setToken('synthetic-test-only-bearer');
  render(<StrictMode><QueryClientProvider client={new QueryClient()}><MonthlySheet api={api} signedIn onDirty={() => {}} onLogout={() => {}} /></QueryClientProvider></StrictMode>);
  await waitFor(() => expect(settingsReads).toBe(2));
  await act(async () => {}); // Settle the earlier response while the latest stays blocked.
  const next = screen.getByRole('button', { name: 'Next month' });
  expect(next).toHaveProperty('disabled', true);
  await act(async () => { releaseOld(); });
  await screen.findByRole('heading', { name: 'Monthly remainder' });
  const index = Number(opened[0].slice(0, 4)) * 12 + Number(opened[0].slice(5));
  const nextMonth = `${Math.floor(index / 12)}-${String(index % 12 + 1).padStart(2, '0')}`;
  fireEvent.click(next);
  await waitFor(() => expect(settingsReads).toBe(3));
  expect(screen.getByRole('button', { name: 'Next month' })).toHaveProperty('disabled', true);
  await act(async () => { releaseNew(); });
  expect(opened).toContain(nextMonth);
});
it('preserves add values and blocks month change after an uncertain write', async () => {
  vi.stubGlobal('fetch', transport()); mount(); await signIn(); fireEvent.click(screen.getByRole('button', { name: 'Add item to Income' }));
  fireEvent.change(screen.getByRole('combobox', { name: 'Name' }), { target: { value: 'Salary' } }); fireEvent.change(screen.getByLabelText('Amount', { exact: true }), { target: { value: '6400' } }); fireEvent.click(screen.getByRole('button', { name: 'Save item' }));
  await screen.findByRole('button', { name: 'Try again' }); expect(screen.getByLabelText('Amount', { exact: true })).toHaveProperty('value', '6400'); expect(screen.getByRole('button', { name: /^Month:/ })).toHaveProperty('disabled', true); expect(screen.getByRole('button', { name: 'Next month' })).toHaveProperty('disabled', true); fireEvent.click(screen.getByRole('button', { name: 'Menu' })); expect(screen.getByRole('button', { name: 'Sign out' })).toHaveProperty('disabled', true); expect(screen.queryByRole('navigation')).toBeNull();
});
it('clears private state on sign out and never persists the bearer', async () => {
  const storage = vi.spyOn(Storage.prototype, 'setItem'); vi.stubGlobal('fetch', transport()); mount(); await signIn();
  expect(screen.queryByRole('button', { name: 'Settings' })).toBeNull();
  fireEvent.click(screen.getByRole('button', { name: 'Menu' }));
  const header = screen.getByRole('banner'); expect(header.contains(screen.getByRole('button', { name: 'Export JSON' }))).toBe(true); expect(header.contains(screen.getByRole('button', { name: 'Sign out' }))).toBe(true);
  fireEvent.click(screen.getByRole('button', { name: 'Sign out' }));
  await screen.findByRole('button', { name: 'Sign in' }); expect(screen.queryByRole('heading', { name: 'Monthly remainder' })).toBeNull(); expect(storage).not.toHaveBeenCalled(); storage.mockRestore();
});

it('ignores a delayed StrictMode startup load after changing the reporting month', async () => {
  vi.useFakeTimers({ toFake: ['Date'] }); vi.setSystemTime(new Date('2026-11-01T04:00:00Z'));
  let changingMonth = false; let settingsCount = 0;
  let releasePrevious = () => {}; let releaseCurrent = () => {}; let currentStarted = () => {};
  const current = new Promise<void>(resolve => { currentStarted = resolve; });
  const api = new ApiClient(async input => {
    const url = String(input);
    if (url.endsWith('/settings')) {
      settingsCount++;
      if (changingMonth) await new Promise<void>(resolve => { releaseCurrent = resolve; currentStarted(); });
      else if (settingsCount === 1) await new Promise<void>(resolve => { releasePrevious = resolve; });
      return Response.json(settings);
    }
    return Response.json({ ...emptySheet, month: url.includes('/2026-10/') ? '2026-10' : '2026-11', remainder_minor: url.includes('/2026-10/') ? '10000' : '0' });
  });
  api.setToken('synthetic-test-only-bearer');
  render(<StrictMode><QueryClientProvider client={new QueryClient()}><MonthlySheet api={api} signedIn onDirty={() => {}} onLogout={() => {}} /></QueryClientProvider></StrictMode>);
  try {
    await screen.findByRole('heading', { name: 'Monthly remainder' }); changingMonth = true;
    fireEvent.click(screen.getByRole('button', { name: /^Month:/ })); fireEvent.click(screen.getByRole('button', { name: 'October' }));
    await current;
    await act(async () => { releasePrevious(); });
    await act(async () => { releaseCurrent(); });
    await waitFor(() => expect(screen.getByRole('region', { name: 'Monthly summary' }).textContent).toContain('SGD 100.00'));
  } finally { releasePrevious(); releaseCurrent(); }
});
