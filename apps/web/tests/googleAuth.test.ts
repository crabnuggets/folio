import { afterEach, expect, it, vi } from 'vitest';
import { BroadcastChannel } from 'node:worker_threads';
import { setImmediate } from 'node:timers/promises';
import { webcrypto } from 'node:crypto';
import type { GoTrueClient } from '@supabase/auth-js';

const channels: BroadcastChannel[] = [];
const clients: GoTrueClient[] = [];
class TrackedChannel extends BroadcastChannel {
  constructor(name: string) { super(name); channels.push(this); }
}
function tabStorage() {
  const values = new Map<string, string>();
  return { values, getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => { values.set(key, value); }, removeItem: (key: string) => { values.delete(key); } };
}
function session(token: string) {
  return { access_token: token, refresh_token: `refresh-${token}`, token_type: 'bearer', expires_in: 3600, expires_at: Math.floor(Date.now() / 1000) + 3600,
    user: { id: '11111111-1111-4111-8111-111111111111', aud: 'authenticated', email: 'owner@example.test', app_metadata: {}, user_metadata: {}, created_at: '2026-01-01T00:00:00Z' } };
}
async function openTab(storage: ReturnType<typeof tabStorage>) {
  vi.resetModules();
  vi.stubGlobal('sessionStorage', storage); vi.stubGlobal('BroadcastChannel', TrackedChannel); vi.stubGlobal('crypto', webcrypto);
  vi.stubGlobal('fetch', vi.fn<typeof fetch>(async input => {
    const url = String(input);
    if (url === '/auth/config') return Response.json({ development_key: false, supabase_url: 'https://auth.example.test', publishable_key: 'sb_publishable_synthetic' });
    if (url.includes('/token?grant_type=pkce')) return Response.json(session('tab-a-token'));
    if (url.includes('/token?grant_type=refresh_token')) return Response.json(session('tab-a-refreshed-token'));
    if (url.includes('/logout')) return new Response(null, { status: 204 });
    throw new Error(`Unexpected auth request: ${url}`);
  }));
  const { loadBrowserAuth } = await import('../src/lib/googleAuth');
  const { client } = await loadBrowserAuth();
  if (!client) throw new Error('Expected Google auth client');
  clients.push(client); await client.getSession();
  return { client, channel: channels[channels.length - 1]! };
}
async function observeBroadcast(channel: BroadcastChannel, eventName: string, action: () => Promise<unknown>) {
  const witness = new BroadcastChannel(channel.name);
  const delivery = new Promise<void>(resolve => { witness.onmessage = event => { if (event.data.event === eventName) resolve(); }; });
  try { await action(); await delivery; await setImmediate(); } finally { witness.close(); }
}
afterEach(async () => {
  for (const client of clients.splice(0)) await client.stopAutoRefresh();
  for (const channel of channels.splice(0)) channel.close();
  vi.unstubAllGlobals(); vi.resetModules();
});

it('keeps SDK sign-in, refresh and local sign-out events in their own tab', async () => {
  const storageA = tabStorage(); const storageB = tabStorage();
  storageB.setItem('folio-auth', JSON.stringify(session('tab-b-token')));
  const tabA = await openTab(storageA); const tabB = await openTab(storageB);
  const otherEvents: string[] = [];
  tabB.client.onAuthStateChange(event => { if (event !== 'INITIAL_SESSION') otherEvents.push(event); });
  const oauth = await tabA.client.signInWithOAuth({ provider: 'google', options: { skipBrowserRedirect: true } });
  await observeBroadcast(tabA.channel, 'SIGNED_IN', async () => {
    const result = await tabA.client.exchangeCodeForSession('synthetic-code', oauth.data.flowId ? { flowId: oauth.data.flowId } : undefined);
    expect(result.error).toBeNull();
  });
  await observeBroadcast(tabA.channel, 'TOKEN_REFRESHED', () => tabA.client.refreshSession());
  await observeBroadcast(tabA.channel, 'SIGNED_OUT', () => tabA.client.signOut({ scope: 'local' }));
  expect(otherEvents).toEqual([]);
  expect((await tabB.client.getSession()).data.session?.access_token).toBe('tab-b-token');
  expect(storageA.getItem('folio-auth')).toBeNull();
});

it('preserves PKCE verifiers and restores a session after reload with a new channel', async () => {
  const storage = tabStorage(); const original = await openTab(storage);
  const oauth = await original.client.signInWithOAuth({ provider: 'google', options: { skipBrowserRedirect: true } });
  const pendingStorage = new Map(storage.values);
  const reloaded = await openTab(storage);
  const result = await reloaded.client.exchangeCodeForSession('synthetic-code', oauth.data.flowId ? { flowId: oauth.data.flowId } : undefined);
  expect(result.error).toBeNull();
  expect([...pendingStorage.keys()].every(key => key.startsWith('folio-auth'))).toBe(true);
  expect(JSON.parse(storage.getItem('folio-auth')!).access_token).toBe('tab-a-token');
  const restored = await openTab(storage);
  expect((await restored.client.getSession()).data.session?.access_token).toBe('tab-a-token');
  expect(restored.channel.name).not.toBe(original.channel.name);
});
