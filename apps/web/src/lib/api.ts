import type { components } from './schema';

export type S = components['schemas'];
export class ApiError extends Error {
  constructor(public status: number, public code: string, message: string, public fields: Record<string, string> = {}, public requestId = '') {
    super(message);
  }
}
export class UncertainError extends Error {
  constructor() { super('The server outcome is unknown. Keep this page open and retry the same action to safely recover its result.'); }
}
export const isUncertain = (error: unknown) => error instanceof UncertainError || (error instanceof ApiError && (error.status >= 500 || error.code === 'SESSION_ENDED'));

type ActionOptions = { method?: 'POST' | 'PATCH' | 'PUT'; version?: number };
export type Command<T> = {
  readonly key: string;
  readonly path: string;
  readonly body: string;
  readonly method: string;
  readonly version?: number;
  run(): Promise<T>;
};

/** An action is immutable and single-flight. Repeated runs replay the exact UUID,
 * body and If-Match, including after a lost response or authentication expiry. */
export class ApiClient {
  private token: string | null = null;
  private generation = 0;
  onUnauthorized: () => void = () => {};
  constructor(private transport: typeof fetch = (...args) => fetch(...args), private timeoutMs = 25000) {}

  setToken(token: string | null) { this.token = token; this.generation++; }

  // A refreshed token represents the same verified session. Keep in-flight commands valid.
  refreshToken(token: string) { this.token = token; }

  private async request<T>(path: string, init: RequestInit): Promise<T> {
    if (!path.startsWith('/') || path.startsWith('//') || path.includes('..')) throw new Error('Invalid API path');
    if (!this.token) throw new ApiError(401, 'UNAUTHENTICATED', 'Sign in to continue.');
    const generation = this.generation;
    const token = this.token;
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeoutMs);
    try {
      const response = await this.transport(`/api/v1${path}`, {
        ...init, credentials: 'omit', cache: 'no-store', redirect: 'error', signal: controller.signal,
        headers: { 'Content-Type': 'application/json', ...init.headers, Authorization: `Bearer ${token}` },
      });
      if (generation !== this.generation) throw new ApiError(401, 'SESSION_ENDED', 'The session ended. Sign in again.');
      let data: unknown;
      try { data = await response.json(); } catch {
        if (response.status !== 401) throw new UncertainError();
      }
      // Check for a newer session before invalidating this one. A 401 invalidates
      // the current generation itself, so preserve its actual authentication error.
      if (generation !== this.generation) throw new ApiError(401, 'SESSION_ENDED', 'The session ended. Sign in again.');
      if (response.status === 401) {
        if (token !== this.token) throw new UncertainError();
        this.setToken(null);
        this.onUnauthorized();
      }
      if (!response.ok) {
        const error = (data as S['ErrorEnvelope'])?.error;
        throw new ApiError(response.status, error?.code ?? 'REQUEST_FAILED', error?.message ?? 'The request failed.', error?.fields ?? {}, error?.request_id ?? response.headers.get('X-Request-ID') ?? '');
      }
      return data as T;
    } catch (error) {
      if (error instanceof ApiError || error instanceof UncertainError) throw error;
      throw new UncertainError();
    } finally { clearTimeout(timer); }
  }

  get<K extends keyof S>(path: string): Promise<S[K]> { return this.request(path, { method: 'GET' }); }

  async list<K extends keyof S>(path: string): Promise<S[K][]> {
    const result: S[K][] = [];
    const seen = new Set<string>();
    let cursor: string | null = null;
    do {
      const params = new URLSearchParams({ limit: '100' });
      if (cursor) params.set('cursor', cursor);
      const page: { items: S[K][]; next_cursor: string | null } = await this.request(`${path}${path.includes('?') ? '&' : '?'}${params}`, { method: 'GET' });
      result.push(...page.items);
      cursor = page.next_cursor;
      if (cursor && seen.has(cursor)) throw new Error('The server repeated a pagination cursor. Refresh to retry.');
      if (cursor) seen.add(cursor);
    } while (cursor);
    return result;
  }

  command<K extends keyof S, B = unknown>(path: string, input: B, options: ActionOptions = {}): Command<S[K]> {
    const key = crypto.randomUUID();
    const body = JSON.stringify(input);
    const method = options.method ?? 'POST';
    const version = options.version;
    let completed = false;
    let result: S[K];
    let running: Promise<S[K]> | undefined;
    const run = (): Promise<S[K]> => {
      if (completed) return Promise.resolve(result);
      if (running) return running;
      running = this.request<S[K]>(path, {
        method, body,
        headers: { 'Idempotency-Key': key, ...(version == null ? {} : { 'If-Match': `"${version}"` }) },
      }).then(value => { completed = true; result = value; return value; }).finally(() => { running = undefined; });
      return running;
    };
    return Object.freeze({ key, path, body, method, version, run });
  }
}

export function errorMessage(error: unknown): string {
  if (error instanceof ApiError && error.status === 412) return 'This record changed in another review. Refresh, compare the current record, then submit your changes again.';
  if (error instanceof ApiError && error.code === 'MONTH_CLOSED') return 'This affects a closed month. Reopen that month with a reason before changing it.';
  return error instanceof Error ? error.message : 'The request could not be completed.';
}
