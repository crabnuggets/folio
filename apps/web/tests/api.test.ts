import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiClient, ApiError, UncertainError, errorMessage, type S } from '../src/lib/api';
import { tokenExpiry } from '../src/lib/session';

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
const settings: S['Settings'] = { currency: 'SGD', currency_exponent: 2, timezone: 'Asia/Singapore', version: 1 };
afterEach(() => vi.useRealTimers());

describe('financial request safety', () => {
  it('replays immutable body, UUID and precondition after a lost response, with no automatic retry', async () => {
    const transport = vi.fn<typeof fetch>().mockRejectedValueOnce(new TypeError('network disconnected')).mockResolvedValue(json(settings));
    const api = new ApiClient(transport); api.setToken('opaque-owner-token');
    const input: S['SettingsInput'] = { currency: 'SGD', timezone: 'Asia/Singapore' };
    const command = api.command<'Settings', S['SettingsInput']>('/settings', input, { method: 'PATCH', version: 7 });
    input.currency = 'USD';
    await expect(command.run()).rejects.toBeInstanceOf(UncertainError);
    expect(transport).toHaveBeenCalledTimes(1);
    await expect(command.run()).resolves.toEqual(settings);
    const first = transport.mock.calls[0][1]; const retry = transport.mock.calls[1][1];
    expect(first?.body).toBe('{"currency":"SGD","timezone":"Asia/Singapore"}');
    expect(retry?.body).toBe(first?.body);
    expect(retry?.headers).toEqual(first?.headers);
    expect(new Headers(first?.headers).get('If-Match')).toBe('"7"');
    expect(new Headers(first?.headers).get('Idempotency-Key')).toMatch(/^[0-9a-f-]{36}$/);
    await command.run(); expect(transport).toHaveBeenCalledTimes(2);
  });
  it('deduplicates rapid submit while in flight', async () => {
    let respond: (response: Response) => void = () => {};
    const transport = vi.fn<typeof fetch>(() => new Promise(resolve => { respond = resolve; }));
    const api = new ApiClient(transport); api.setToken('owner');
    const command = api.command<'Settings'>('/settings', {});
    const first = command.run(); const second = command.run();
    expect(second).toBe(first); expect(transport).toHaveBeenCalledTimes(1);
    respond(json(settings)); await first;
  });
  it('preserves a stale write as a conflict without rebasing or retrying it', async () => {
    const transport = vi.fn<typeof fetch>().mockResolvedValue(json({ error: { code: 'STALE_VERSION', message: 'stale', fields: {}, request_id: 'trace' } }, 412));
    const api = new ApiClient(transport); api.setToken('owner');
    const action = api.command<'Settings'>('/settings', {}, { version: 1, method: 'PATCH' });
    const failure = await action.run().catch(error => error);
    expect(failure).toBeInstanceOf(ApiError); expect(failure.status).toBe(412);
    expect(errorMessage(failure)).toContain('compare'); expect(transport).toHaveBeenCalledTimes(1);
  });
  it('treats malformed successful JSON as uncertain and reuses the same key', async () => {
    const transport = vi.fn<typeof fetch>().mockResolvedValueOnce(new Response('{', { status: 201 })).mockResolvedValue(json(settings));
    const api = new ApiClient(transport); api.setToken('owner');
    const action = api.command<'Settings'>('/settings', {});
    await expect(action.run()).rejects.toBeInstanceOf(UncertainError);
    await action.run();
    expect(new Headers(transport.mock.calls[0][1]?.headers).get('Idempotency-Key')).toBe(new Headers(transport.mock.calls[1][1]?.headers).get('Idempotency-Key'));
  });
  it('times out without implying a financial command was rejected', async () => {
    vi.useFakeTimers();
    const transport = vi.fn<typeof fetch>((_url, init) => new Promise((_resolve, reject) => {
      init?.signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')));
    }));
    const api = new ApiClient(transport, 20); api.setToken('owner');
    const outcome = expect(api.command<'Settings'>('/settings', {}).run()).rejects.toBeInstanceOf(UncertainError);
    await vi.advanceTimersByTimeAsync(21); await outcome;
    expect(transport).toHaveBeenCalledTimes(1);
  });
});

describe('authentication and private reads', () => {
  it('uses a same-origin cookie only when starting ChatGPT authorization', async () => {
    const transport = vi.fn<typeof fetch>().mockResolvedValue(json({ authorization_url: 'https://auth.openai.com/' }));
    const api = new ApiClient(transport); api.setToken('owner');
    await api.command<'ChatGPTConnectResponse'>('/chatgpt/connect', {}).run();
    expect(transport.mock.calls[0][1]?.credentials).toBe('same-origin');
  });
  it('uses an in-memory bearer with no cookies, redirects or browser cache', async () => {
    const transport = vi.fn<typeof fetch>().mockResolvedValue(json(settings));
    const api = new ApiClient(transport);
    await expect(api.get<'Settings'>('/settings')).rejects.toMatchObject({ status: 401 });
    expect(transport).not.toHaveBeenCalled();
    api.setToken('private-access'); await api.get<'Settings'>('/settings');
    const init = transport.mock.calls[0][1];
    expect(init).toMatchObject({ credentials: 'omit', cache: 'no-store', redirect: 'error' });
    expect(new Headers(init?.headers).get('Authorization')).toBe('Bearer private-access');
    api.setToken(null); await expect(api.get<'Settings'>('/settings')).rejects.toMatchObject({ status: 401 });
    expect(transport).toHaveBeenCalledTimes(1);
  });
  it('expires on 401 and permits an original action to resume after same-owner sign-in', async () => {
    const transport = vi.fn<typeof fetch>().mockResolvedValueOnce(json({ error: { code: 'UNAUTHENTICATED', message: 'expired' } }, 401)).mockResolvedValue(json(settings));
    const api = new ApiClient(transport); const expire = vi.fn(); api.onUnauthorized = expire; api.setToken('expired');
    const command = api.command<'Settings'>('/settings', {});
    await expect(command.run()).rejects.toMatchObject({ status: 401, code: 'UNAUTHENTICATED', message: 'expired' }); expect(expire).toHaveBeenCalledOnce();
    await expect(api.get<'Settings'>('/settings')).rejects.toMatchObject({ status: 401 });
    api.setToken('renewed'); await command.run();
    expect(new Headers(transport.mock.calls[0][1]?.headers).get('Idempotency-Key')).toBe(new Headers(transport.mock.calls[1][1]?.headers).get('Idempotency-Key'));
    expect(new Headers(transport.mock.calls[1][1]?.headers).get('Authorization')).toBe('Bearer renewed');
  });
  it('prevents an in-flight read from repopulating cache after logout or replacement login', async () => {
    let finish: (response: Response) => void = () => {};
    const api = new ApiClient(() => new Promise(resolve => { finish = resolve; })); api.setToken('owner');
    const read = api.get<'Settings'>('/settings'); api.setToken(null); api.setToken('new-session');
    finish(json(settings)); await expect(read).rejects.toMatchObject({ code: 'SESSION_ENDED' });
  });
  it('also guards logout while the response body is being parsed', async () => {
    let finish: (body: unknown) => void = () => {};
    const response = json(settings);
    response.json = () => new Promise(resolve => { finish = resolve; });
    const api = new ApiClient(async () => response); api.setToken('owner');
    const read = api.get<'Settings'>('/settings'); await Promise.resolve(); api.setToken(null); finish(settings);
    await expect(read).rejects.toMatchObject({ code: 'SESSION_ENDED' });
  });
  it('does not follow an arbitrary external URL', async () => {
    const transport = vi.fn<typeof fetch>(); const api = new ApiClient(transport); api.setToken('owner');
    await expect(api.get<'Settings'>('//example.com')).rejects.toThrow('Invalid API path'); expect(transport).not.toHaveBeenCalled();
  });
  it('uses a JWT exp only as a UI expiry hint', () => {
    expect(tokenExpiry(`header.${btoa(JSON.stringify({ exp: 1770000000 }))}.signature`)).toBe(1770000000000);
    expect(tokenExpiry('opaque-development-token')).toBeNull(); expect(tokenExpiry('bad.%bad.bad')).toBeNull();
  });
});

describe('complete list pagination', () => {
  it('reads every page and preserves the month filter', async () => {
    const transport = vi.fn<typeof fetch>().mockResolvedValueOnce(json({ items: [{ id: 'a' }], next_cursor: 'cursor-1' })).mockResolvedValue(json({ items: [{ id: 'b' }], next_cursor: null }));
    const api = new ApiClient(transport); api.setToken('owner');
    expect(await api.list<'Entry'>('/entries?month=2026-10')).toEqual([{ id: 'a' }, { id: 'b' }]);
    expect(String(transport.mock.calls[1][0])).toContain('month=2026-10&limit=100&cursor=cursor-1');
  });
  it('fails explicitly if a server cursor loops', async () => {
    const transport = vi.fn<typeof fetch>().mockImplementation(async () => json({ items: [], next_cursor: 'same' }));
    const api = new ApiClient(transport); api.setToken('owner');
    await expect(api.list<'Account'>('/accounts')).rejects.toThrow('repeated a pagination cursor');
    expect(transport).toHaveBeenCalledTimes(2);
  });
});
