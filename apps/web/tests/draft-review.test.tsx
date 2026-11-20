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
const draft: S['MonthlyDraft'] = { id: '11111111-1111-4111-8111-111111111111', version: 1, created_at: '2026-11-01T00:00:00Z', updated_at: '2026-11-01T00:00:00Z', month: sheet.month, section_id: section, name: 'Sample card', amount_minor: '1250', note: 'Proposed bill', evidence: 'Synthetic statement page 1', source_id: 'test:statement', external_id: 'one', sheet_version: 1, amendment: null, recurrence_change: null, repeat_monthly: false, usual_amount_minor: null, state: 'pending', accepted_item_id: null, proposal_hash: 'synthetic', explanations: [] };

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

it('reviews a new recurring item that has no amount yet and keeps it blank when editing', async () => {
  let current: S['MonthlyDraft'] = { ...draft, name: 'Streaming', amount_minor: null, repeat_monthly: true, usual_amount_minor: '1598', source_id: 'chat', external_id: 'streaming-2026-11' };
  const writes: RequestInit[] = [];
  mount(async (_input, init) => {
    if (init?.method === 'PUT') {
      writes.push(init); current = { ...current, ...JSON.parse(String(init.body)), version: 2 };
      return new Response(JSON.stringify(current));
    }
    return new Response(JSON.stringify({ items: [current], next_cursor: null }));
  });
  expect(await screen.findByText('Repeats monthly')).toBeTruthy();
  expect(screen.getByText('Blank')).toBeTruthy();
  fireEvent.click(screen.getByRole('button', { name: 'Review Streaming' }));
  const dialog = screen.getByRole('dialog');
  expect(dialog.textContent).toContain('Blank this month');
  expect(dialog.textContent).toContain('Repeats monthly, usual SGD 15.98');
  expect(dialog.textContent).toContain('appears blank on every later monthly sheet');
  fireEvent.change(screen.getByLabelText('Note'), { target: { value: 'Owner checked' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save proposal edits' }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(JSON.parse(String(writes[0].body))).toMatchObject({ name: 'Streaming', amount_minor: null, note: 'Owner checked' });
});

it('compares a recurring item change with the current settings and only offers accept or reject', async () => {
  const change: S['MonthlyDraft'] = { ...draft, month: '2026-12', name: 'Streaming', amount_minor: null, repeat_monthly: true, usual_amount_minor: '1798', source_id: '', external_id: '', evidence: '', recurrence_change: { item_id: '22222222-2222-4222-8222-222222222222', base_section_id: section, base_repeat_monthly: true, base_usual_amount_minor: '1598' } };
  const accept = mount(async () => new Response(JSON.stringify({ items: [change], next_cursor: null })));
  expect(await screen.findByText('Recurring item change from December 2026')).toBeTruthy();
  fireEvent.click(screen.getByRole('button', { name: 'Review Streaming' }));
  expect(screen.getByRole('dialog').textContent).toContain('Recurring item change for Streaming');
  expect(screen.getByLabelText('Settings for Streaming when proposed').textContent).toContain('SGD 15.98');
  expect(screen.getByLabelText('Proposed settings for Streaming').textContent).toContain('SGD 17.98');
  expect(screen.getByRole('dialog').textContent).toContain('Earlier months and entered amounts stay as they are');
  expect(screen.getByText('Note: Proposed bill')).toBeTruthy();
  expect(screen.queryByText('Edit before accepting')).toBeNull();
  fireEvent.click(screen.getByRole('button', { name: 'Accept proposal' }));
  await waitFor(() => expect(accept).toHaveBeenCalledOnce());
  expect(accept.mock.calls[0][0]).toMatchObject({ recurrence_change: { base_usual_amount_minor: '1598' } });
});

it('describes a stop that also moves section without promising the blank row moves', async () => {
  const other = '00000000-0000-4000-8000-000000000005';
  const withOther: S['MonthlySheet'] = { ...sheet, sections: [...sheet.sections, { ...sheet.sections[0], id: other, name: 'Other money out', amount_label: 'Amount' }] };
  const stop: S['MonthlyDraft'] = { ...draft, name: 'Paper', amount_minor: null, section_id: other, source_id: '', external_id: '', evidence: '', recurrence_change: { item_id: '22222222-2222-4222-8222-222222222222', base_section_id: section, base_repeat_monthly: true, base_usual_amount_minor: null } };
  const api = new ApiClient(async () => new Response(JSON.stringify({ items: [stop], next_cursor: null }))); api.setToken('synthetic-owner');
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MonthlyDraftReview api={api} sheet={withOther} signedIn disabled={false} onDirty={() => {}} onAccept={async () => true} feedback={() => null} /></QueryClientProvider>);
  fireEvent.click(await screen.findByRole('button', { name: 'Review Paper' }));
  expect(screen.getByRole('dialog').textContent).toContain('Accepting stops Paper repeating from November 2026. Blank Paper items you haven\'t touched disappear from November 2026 on. If Paper is still on the November 2026 sheet, it moves to Other money out');
});
