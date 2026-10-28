import { expect, it, vi } from 'vitest';
import { ApiClient, type S } from '../src/lib/api';
import { settlementWorkflow } from '../src/lib/settlement';

const json = (body: unknown, status = 201) => new Response(JSON.stringify(body), { status });
const statement: S['Statement'] = {
  id: 'card-statement', version: 7, created_at: '', updated_at: '', account_id: 'card', currency: 'SGD', cycle_start: '2026-09-01', cycle_end: '2026-09-30', issue_date: '2026-10-01', due_date: '2026-10-28', reporting_month: '2026-10', reporting_month_reason: '', amount_due_minor: '120000', minimum_due_minor: null, new_charges_minor: null, fees_minor: null, interest_minor: null, refunds_minor: null, net_spending_minor: '120000', spending_evidence: 'test evidence', carries_statement_id: null, plan_line_id: null, active: true, payable_active: true, supersedes_id: null, revision_reason: '',
};
const payment: S['EntryInput'] = { kind: 'card_settlement', amount_minor: '120000', currency: 'SGD', effective_date: '2026-10-25', source_account_id: 'funding', destination_account_id: 'card' };

it('retains a committed payment when linking times out, and retries only the exact link', async () => {
  const transport = vi.fn<typeof fetch>()
    .mockResolvedValueOnce(json({ id: 'retained-payment' }))
    .mockRejectedValueOnce(new TypeError('response lost after commit'))
    .mockResolvedValue(json({ id: 'settlement', entry_id: 'retained-payment' }));
  const api = new ApiClient(transport); api.setToken('owner');
  const workflow = settlementWorkflow(api, statement, '120000', payment);
  await expect(workflow.run()).rejects.toThrow('outcome is unknown');
  expect(workflow.paymentId).toBe('retained-payment');
  await workflow.run();
  expect(transport.mock.calls.map(call => call[0])).toEqual(['/api/v1/entries', '/api/v1/statements/card-statement/settlements', '/api/v1/statements/card-statement/settlements']);
  expect(transport.mock.calls[1][1]?.body).toBe('{"entry_id":"retained-payment","amount_minor":"120000"}');
  expect(transport.mock.calls[2][1]?.headers).toEqual(transport.mock.calls[1][1]?.headers);
  expect(new Headers(transport.mock.calls[2][1]?.headers).get('If-Match')).toBe('"7"');
});

it('recovers the same payment when its create response is lost before attempting a link', async () => {
  const transport = vi.fn<typeof fetch>().mockRejectedValueOnce(new TypeError('response lost')).mockResolvedValueOnce(json({ id: 'payment' })).mockResolvedValue(json({ id: 'settlement' }));
  const api = new ApiClient(transport); api.setToken('owner');
  const workflow = settlementWorkflow(api, statement, '120000', payment);
  await expect(workflow.run()).rejects.toThrow(); await workflow.run();
  expect(transport.mock.calls[1][1]?.headers).toEqual(transport.mock.calls[0][1]?.headers);
  expect(transport.mock.calls.filter(call => String(call[0]).endsWith('/settlements'))).toHaveLength(1);
});

it('leaves a committed cash payment recoverable when statement concurrency rejects linkage', async () => {
  const transport = vi.fn<typeof fetch>().mockResolvedValueOnce(json({ id: 'recoverable' })).mockImplementation(async () => json({ error: { code: 'STALE_VERSION', message: 'stale' } }, 412));
  const api = new ApiClient(transport); api.setToken('owner');
  const workflow = settlementWorkflow(api, statement, '120000', payment);
  await expect(workflow.run()).rejects.toMatchObject({ status: 412 });
  expect(workflow.paymentId).toBe('recoverable'); await expect(workflow.run()).rejects.toMatchObject({ status: 412 });
  expect(transport.mock.calls.filter(call => String(call[0]).endsWith('/entries'))).toHaveLength(1);
});


it('clears resolved payment uncertainty when recovered cash reaches a definitively rejected link', async () => {
  const transport = vi.fn<typeof fetch>()
    .mockRejectedValueOnce(new TypeError('committed payment response lost'))
    .mockResolvedValueOnce(json({ id: 'recovered-payment' }))
    .mockResolvedValueOnce(json({ error: { code: 'STALE_VERSION', message: 'stale' } }, 412));
  const api = new ApiClient(transport); api.setToken('owner');
  const workflow = settlementWorkflow(api, statement, '120000', payment);
  await expect(workflow.run()).rejects.toThrow('outcome is unknown');
  expect(workflow).toHaveProperty('uncertain', true);
  await expect(workflow.run()).rejects.toMatchObject({ status: 412 });
  expect(workflow).toHaveProperty('paymentId', 'recovered-payment');
  expect(workflow).toHaveProperty('uncertain', false);
  expect(transport.mock.calls[1][1]?.headers).toEqual(transport.mock.calls[0][1]?.headers);
});

it('keeps a lost link outcome uncertain across a later definitive rejection', async () => {
  const transport = vi.fn<typeof fetch>()
    .mockResolvedValueOnce(json({ id: 'retained-payment' }))
    .mockRejectedValueOnce(new TypeError('committed link response lost'))
    .mockResolvedValueOnce(json({ error: { code: 'STALE_VERSION', message: 'stale' } }, 412))
    .mockResolvedValueOnce(json({ id: 'recovered-settlement' }));
  const api = new ApiClient(transport); api.setToken('owner');
  const workflow = settlementWorkflow(api, statement, '120000', payment);
  await expect(workflow.run()).rejects.toThrow('outcome is unknown');
  await expect(workflow.run()).rejects.toMatchObject({ status: 412 });
  expect(workflow).toHaveProperty('uncertain', true);
  await workflow.run();
  expect(workflow).toHaveProperty('uncertain', false);
  expect(transport.mock.calls.filter(call => String(call[0]).endsWith('/entries'))).toHaveLength(1);
  expect(transport.mock.calls[2][1]?.headers).toEqual(transport.mock.calls[1][1]?.headers);
  expect(transport.mock.calls[3][1]?.headers).toEqual(transport.mock.calls[1][1]?.headers);
});
