import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ApiClient, type S } from '../src/lib/api';
import { MonthlyDraftReview } from '../src/MonthlyDraftReview';

beforeEach(() => {
  Object.defineProperty(HTMLDialogElement.prototype, 'showModal', { configurable: true, value: function(this: HTMLDialogElement) { this.setAttribute('open', ''); } });
  Object.defineProperty(HTMLDialogElement.prototype, 'close', { configurable: true, value: function(this: HTMLDialogElement) { this.removeAttribute('open'); } });
});
afterEach(cleanup);
const section = '00000000-0000-4000-8000-000000000004';
const sheet: S['MonthlySheet'] = { month: '2026-11', version: 1, currency: 'SGD', currency_exponent: 2, currency_locked: false, undo_token: null, money_in_minor: '0', money_out_minor: '0', remainder_minor: '0', missing_count: 0, sections: [{ id: section, name: 'Credit cards', direction: 'out', amount_label: 'Bill amount', total_minor: '0', missing_count: 0, items: [] }] };
const draft: S['MonthlyDraft'] = { id: '11111111-1111-4111-8111-111111111111', version: 1, created_at: '2026-11-01T00:00:00Z', updated_at: '2026-11-01T00:00:00Z', month: sheet.month, section_id: section, name: 'Sample card', amount_minor: '1250', note: 'Proposed bill', evidence: 'Synthetic statement page 1', source_id: 'test:statement', external_id: 'one', sheet_version: 1, amendment: null, state: 'pending', accepted_item_id: null, proposal_hash: 'synthetic', explanations: [] };

function mount(transport: typeof fetch) {
  const api = new ApiClient(transport); api.setToken('synthetic-owner');
  const accept = vi.fn(async (_draft: S['MonthlyDraft'], done: () => void) => { done(); return true; });
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MonthlyDraftReview api={api} sheet={sheet} signedIn disabled={false} onDirty={() => {}} onAccept={accept} feedback={() => null} /></QueryClientProvider>);
  return accept;
}

it('shows evidence and requires saved edits before explicit acceptance', async () => {
  let current = { ...draft };
  const writes: RequestInit[] = [];
  const accept = mount(async (_input, init) => {
    if (init?.method === 'PUT') {
      writes.push(init); current = { ...current, ...JSON.parse(String(init.body)), version: 2 };
      return new Response(JSON.stringify(current));
    }
    return new Response(JSON.stringify({ items: [current], next_cursor: null }));
  });
  await screen.findByRole('button', { name: 'Review Sample card' });
  expect(accept).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: 'Review Sample card' }));
  expect(screen.getByText(draft.evidence)).toBeTruthy();
  fireEvent.change(screen.getByLabelText('Proposed amount (SGD)'), { target: { value: '20.00' } });
  expect(screen.queryByRole('button', { name: 'Accept proposal' })).toBeNull();
  fireEvent.click(screen.getByRole('button', { name: 'Save proposal edits' }));
  await waitFor(() => expect(screen.getByRole('button', { name: 'Accept proposal' })).toHaveProperty('disabled', false));
  expect(screen.getByRole('dialog')).toBeTruthy();
  expect(JSON.parse(String(writes[0].body)).amount_minor).toBe('2000');
  expect(writes[0].headers).toMatchObject({ 'If-Match': '"1"' });
  fireEvent.click(screen.getByRole('button', { name: 'Accept proposal' }));
  await waitFor(() => expect(accept).toHaveBeenCalledOnce());
  expect(accept.mock.calls[0][0]).toMatchObject({ amount_minor: '2000', version: 2 });
});

it('retries a lost rejection response with the identical command and keeps acceptance blocked', async () => {
  const attempts: RequestInit[] = [];
  const accept = mount(async (_input, init) => {
    if (init?.method === 'POST') {
      attempts.push(init);
      if (attempts.length === 1) throw new TypeError('Synthetic lost response');
      return new Response(JSON.stringify({ ...draft, state: 'rejected', version: 2 }));
    }
    return new Response(JSON.stringify({ items: attempts.length > 1 ? [] : [draft], next_cursor: null }));
  });
  fireEvent.click(await screen.findByRole('button', { name: 'Review Sample card' }));
  fireEvent.click(screen.getByRole('button', { name: 'Reject proposal' }));
  fireEvent.click(await screen.findByRole('button', { name: 'Try again' }));
  await screen.findByText('No pending proposals for this month.');
  expect(attempts).toHaveLength(2);
  expect(attempts[1].headers).toEqual(attempts[0].headers);
  expect(attempts[1].body).toBe(attempts[0].body);
  expect(accept).not.toHaveBeenCalled();
});

it('shows a proposal breakdown and can drop it when saving edits', async () => {
  let current: S['MonthlyDraft'] = { ...draft, explanations: [{ name: 'Grocer', amount_minor: '700' }, { name: 'Cafe', amount_minor: '300' }] };
  const writes: RequestInit[] = [];
  mount(async (_input, init) => {
    if (init?.method === 'PUT') {
      writes.push(init); current = { ...current, ...JSON.parse(String(init.body)), version: 2 };
      return new Response(JSON.stringify(current));
    }
    return new Response(JSON.stringify({ items: [current], next_cursor: null }));
  });
  expect(await screen.findByText(/2 explanations/)).toBeTruthy();
  fireEvent.click(screen.getByRole('button', { name: 'Review Sample card' }));
  const breakdown = screen.getByLabelText('Explanations for Sample card');
  expect(breakdown.textContent).toContain('Grocer');
  expect(breakdown.textContent).toContain('Unexplained');
  fireEvent.change(screen.getByLabelText('Proposed amount (SGD)'), { target: { value: '5.00' } });
  fireEvent.click(screen.getByLabelText('Keep explanations'));
  fireEvent.click(screen.getByRole('button', { name: 'Save proposal edits' }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(JSON.parse(String(writes[0].body))).toMatchObject({ amount_minor: '500', explanations: [] });
});

it('shows an amendment against the entered amount and keeps its item fixed when editing', async () => {
  let current: S['MonthlyDraft'] = { ...draft, explanations: [{ name: 'Grocer', amount_minor: '1000' }, { name: 'Cafe', amount_minor: '250' }], amendment: { item_id: '22222222-2222-4222-8222-222222222222', base_amount_minor: '1250', base_explanations: [{ name: 'Grocer', amount_minor: '1000' }] } };
  const writes: RequestInit[] = [];
  const accept = mount(async (_input, init) => {
    if (init?.method === 'PUT') {
      writes.push(init); current = { ...current, ...JSON.parse(String(init.body)), version: 2 };
      return new Response(JSON.stringify(current));
    }
    return new Response(JSON.stringify({ items: [current], next_cursor: null }));
  });
  expect(await screen.findByText(/Changes the explanations of the entered amount/)).toBeTruthy();
  fireEvent.click(screen.getByRole('button', { name: 'Review Sample card' }));
  expect(screen.getByRole('dialog').textContent).toContain('Amendment for Sample card');
  expect(screen.getByLabelText('Entered explanations for Sample card').textContent).toContain('Unexplained');
  expect(screen.getByLabelText('Explanations for Sample card').textContent).toContain('Cafe');
  expect(screen.queryByLabelText('Item name')).toBeNull();
  fireEvent.change(screen.getByLabelText('Note'), { target: { value: 'Owner checked' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save proposal edits' }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(JSON.parse(String(writes[0].body))).toMatchObject({ name: 'Sample card', section_id: section, amount_minor: '1250', note: 'Owner checked' });
  await waitFor(() => expect(screen.getByRole('button', { name: 'Accept proposal' })).toHaveProperty('disabled', false));
  fireEvent.click(screen.getByRole('button', { name: 'Accept proposal' }));
  await waitFor(() => expect(accept).toHaveBeenCalledOnce());
});
