import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { StrictMode } from 'react';
import { App } from '../src/App';
import type { S } from '../src/lib/api';

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
const settings: S['Settings'] = { currency: 'SGD', currency_exponent: 2, timezone: 'Asia/Singapore', version: 1 };
const connection: S['ChatGPTConnection'] = { available: true, connected: true, sharing: true, email: 'owner@example.test', reason: '', models: [{ id: 'test-model', name: 'Test model' }] };
const capture: S['ChatCaptureResponse'] = { message: 'Review your coffee amount.', items: [{ name: 'Coffee', section_id: 'out', amount_minor: '450', note: 'From your description' }] };
const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status });
function setup(options: { lostWrite?: boolean; expiredCapture?: boolean; connected?: boolean } = {}) {
  const sheet: S['MonthlySheet'] = { month: '2026-10', version: 1, currency: 'SGD', currency_exponent: 2, currency_locked: false, has_legacy_data: false, undo_token: null, money_in_minor: '0', money_out_minor: '0', remainder_minor: '0', missing_count: 0, sections: [{ id: 'in', name: 'Income', direction: 'in', amount_label: 'Amount', total_minor: '0', missing_count: 0, items: [] }, { id: 'out', name: 'Other money out', direction: 'out', amount_label: 'Amount', total_minor: '0', missing_count: 0, items: [] }] };
  let writes = 0;
  const transport = vi.fn<typeof fetch>(async (input, init) => {
    const path = String(input);
    if (path.endsWith('/settings')) return json(settings);
    if (path.endsWith('/chatgpt/connection')) return json(options.connected === false ? { ...connection, available: false, connected: false, sharing: false, email: '', models: [], reason: 'ChatGPT is not configured for this server.' } : connection);
    if (path.endsWith('/capture')) return options.expiredCapture ? json({ error: { code: 'UNAUTHENTICATED', message: 'Session expired.' } }, 401) : json(capture);
    if (path.endsWith('/items') && init?.method === 'POST') {
      writes++;
      if (options.lostWrite && writes === 1) throw new TypeError('Synthetic lost response');
      const item: S['MonthlyItemInput'] = JSON.parse(String(init.body));
      sheet.version++;
      sheet.sections[1].items.push({ id: 'coffee', name: item.name, section_id: item.section_id, amount_minor: item.amount_minor, note: item.note ?? '', repeat_monthly: false, usual_amount_minor: null });
      return json(sheet);
    }
    if (path.includes('monthly-item-suggestions')) return json({ items: [] });
    return json(sheet);
  });
  vi.stubGlobal('fetch', transport);
  render(<StrictMode><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } } })}><App /></QueryClientProvider></StrictMode>);
  return transport;
}
async function signIn() {
  fireEvent.change(screen.getByLabelText('Access token', { exact: false }), { target: { value: 'synthetic-chat-owner' } });
  fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));
  await screen.findByRole('heading', { name: 'Monthly remainder' });
}
async function propose() {
  fireEvent.change(screen.getByLabelText('Describe items to add'), { target: { value: 'Coffee 4.50' } });
  await waitFor(() => expect(screen.getByRole('button', { name: 'Send' })).toHaveProperty('disabled', false));
  fireEvent.click(screen.getByRole('button', { name: 'Send' }));
  return screen.findByRole('form', { name: 'Suggested item 1' });
}
it('keeps chat available and requires reviewed confirmation before a sheet write', async () => {
  const transport = setup(); await signIn();
  expect(screen.getByRole('complementary', { name: 'Add items with ChatGPT' })).toBeTruthy();
  const draft = await propose();
  expect(transport.mock.calls.filter(([path]) => String(path).endsWith('/items'))).toHaveLength(0);
  expect(screen.getByRole('button', { name: 'Sign out' })).toHaveProperty('disabled', true);
  expect(screen.getByLabelText('Month', { exact: true })).toHaveProperty('disabled', true);
  fireEvent.change(within(draft).getByLabelText('Amount (SGD)'), { target: { value: '5.25' } });
  fireEvent.click(within(draft).getByRole('button', { name: /^Add item to/ }));
  await screen.findByRole('button', { name: 'Coffee' });
  const write = transport.mock.calls.find(([path]) => String(path).endsWith('/items'));
  expect(JSON.parse(String(write?.[1]?.body))).toMatchObject({ name: 'Coffee', amount_minor: '525', section_id: 'out' });
  expect(screen.queryByRole('form', { name: 'Suggested item 1' })).toBeNull();
  await waitFor(() => expect(screen.getByRole('button', { name: 'Sign out' })).toHaveProperty('disabled', false));
});
it('retains the reviewed draft and immutable retry after an uncertain sheet write', async () => {
  const transport = setup({ lostWrite: true }); await signIn(); const draft = await propose();
  fireEvent.click(within(draft).getByRole('button', { name: /^Add item to/ }));
  const retry = await screen.findByRole('button', { name: 'Retry same action' });
  expect(within(draft).getByLabelText('Item name').matches(':disabled')).toBe(true);
  expect(within(draft).getByRole('button', { name: 'Discard suggestion' }).matches(':disabled')).toBe(true);
  fireEvent.click(retry); await screen.findByRole('button', { name: 'Coffee' });
  const writes = transport.mock.calls.filter(([path]) => String(path).endsWith('/items'));
  expect(writes).toHaveLength(2);
  expect(writes[1][1]?.body).toBe(writes[0][1]?.body);
  expect(writes[1][1]?.headers).toEqual(writes[0][1]?.headers);
});
it('clears completed chat feedback when switching reporting months', async () => {
  setup(); await signIn(); const draft = await propose();
  fireEvent.click(within(draft).getByRole('button', { name: /^Add item to/ }));
  await screen.findByText(/1 item added to/);
  await waitFor(() => expect(screen.getByLabelText('Year')).toHaveProperty('disabled', false));
  const year = screen.getByLabelText('Year');
  fireEvent.change(year, { target: { value: '2027' } }); fireEvent.blur(year);
  await waitFor(() => expect(screen.queryByText(/1 item added to/)).toBeNull());
  expect(screen.queryByText(capture.message)).toBeNull();
});
it('rejects an overlong UTF-8 item name before creating a save command', async () => {
  const transport = setup(); await signIn(); const draft = await propose();
  fireEvent.change(within(draft).getByLabelText('Item name'), { target: { value: '名'.repeat(41) } });
  fireEvent.click(within(draft).getByRole('button', { name: /^Add item to/ }));
  await screen.findByText('Enter an item name of 1 to 120 bytes.');
  expect(within(draft).getByLabelText('Item name').matches(':disabled')).toBe(false);
  expect(transport.mock.calls.filter(([path]) => String(path).endsWith('/items'))).toHaveLength(0);
});
it('holds an uploaded document locally until Send and lets the user remove it', async () => {
  const transport = setup(); await signIn();
  const input = screen.getByLabelText('Choose screenshot or document');
  fireEvent.change(input, { target: { files: [new File(['Coffee 4.50'], 'receipt.txt', { type: 'text/plain' })] } });
  await screen.findByRole('button', { name: 'Remove receipt.txt' });
  expect(transport.mock.calls.filter(([path]) => String(path).endsWith('/capture'))).toHaveLength(0);
  fireEvent.click(screen.getByRole('button', { name: 'Send' }));
  await screen.findByRole('form', { name: 'Suggested item 1' });
  const request = transport.mock.calls.find(([path]) => String(path).endsWith('/capture'));
  expect(JSON.parse(String(request?.[1]?.body))).toMatchObject({ text: '', model: 'test-model', attachment: { name: 'receipt.txt', media_type: 'text/plain', data: btoa('Coffee 4.50') } });
});
it('preserves unsent text across an expired owner session', async () => {
  setup({ expiredCapture: true }); await signIn();
  fireEvent.change(screen.getByLabelText('Describe items to add'), { target: { value: 'Coffee 4.50' } });
  await waitFor(() => expect(screen.getByRole('button', { name: 'Send' })).toHaveProperty('disabled', false));
  fireEvent.click(screen.getByRole('button', { name: 'Send' }));
  await screen.findByRole('button', { name: 'Sign in' });
  expect(screen.queryByRole('complementary', { name: 'Add items with ChatGPT' })).toBeNull();
  await signIn(); expect(screen.getByLabelText('Describe items to add')).toHaveProperty('value', 'Coffee 4.50');
});
it('explains unavailable connection setup and never sends to an unconnected provider', async () => {
  const transport = setup({ connected: false }); await signIn();
  fireEvent.click(screen.getByRole('button', { name: 'Chat to add items' }));
  await screen.findByText('ChatGPT is not configured for this server.');
  expect(screen.getByRole('button', { name: 'Continue with ChatGPT' })).toHaveProperty('disabled', true);
  fireEvent.change(screen.getByLabelText('Describe items to add'), { target: { value: 'Coffee 4.50' } });
  expect(screen.getByRole('button', { name: 'Send' })).toHaveProperty('disabled', true);
  expect(transport.mock.calls.filter(([path]) => String(path).endsWith('/capture'))).toHaveLength(0);
});
