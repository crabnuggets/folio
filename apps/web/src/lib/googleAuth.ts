import { AuthClient, type GoTrueClient } from '@supabase/auth-js';

export type BrowserAuth = { development_key: boolean; supabase_url?: string; publishable_key?: string };
export const authStorageKey = 'folio-auth';
let initialization: Promise<{ config: BrowserAuth; client: GoTrueClient | null }> | undefined;

export function loadBrowserAuth() {
  return initialization ??= (async () => {
    const response = await fetch('/auth/config', { credentials: 'omit', cache: 'no-store', redirect: 'error' });
    if (!response.ok) throw new Error('Sign-in is unavailable. Reload to try again.');
    const config: BrowserAuth = await response.json();
    const client = config.supabase_url && config.publishable_key ? new AuthClient({
      url: `${config.supabase_url}/auth/v1`, headers: { apikey: config.publishable_key, Authorization: `Bearer ${config.publishable_key}` },
      flowType: 'pkce', detectSessionInUrl: false, persistSession: true, autoRefreshToken: true, storage: sessionStorage, storageKey: authStorageKey,
    }) : null;
    return { config, client };
  })().catch(error => { initialization = undefined; throw error; });
}

// The opener keeps its financial edits and PKCE verifier while Google uses a popup.
export async function signInWithGoogle(client: GoTrueClient) {
  const popup = window.open('about:blank', 'folio-google-sign-in', 'popup,width=500,height=700');
  if (!popup) throw new Error('Allow popups to sign in with Google.');
  try {
    const { data, error } = await client.signInWithOAuth({ provider: 'google', options: {
      redirectTo: `${window.location.origin}/auth/callback`, skipBrowserRedirect: true, queryParams: { prompt: 'select_account' },
    } });
    if (error || !data.url) throw new Error('Google sign-in could not start. Try again.');
    const code = await new Promise<string>((resolve, reject) => {
      const finish = (error?: Error, code?: string) => {
        window.removeEventListener('message', receive); clearInterval(closed); clearTimeout(timeout);
        if (error) reject(error); else resolve(code!);
      };
      const receive = (event: MessageEvent) => {
        if (event.origin !== window.location.origin || event.source !== popup || event.data?.type !== 'folio-google-callback') return;
        if (typeof event.data.code === 'string' && event.data.code) finish(undefined, event.data.code);
        else finish(new Error('Google sign-in was cancelled or failed. Try again.'));
      };
      const closed = setInterval(() => { if (popup.closed) finish(new Error('Google sign-in was cancelled.')); }, 500);
      const timeout = setTimeout(() => finish(new Error('Google sign-in timed out. Try again.')), 5 * 60 * 1000);
      window.addEventListener('message', receive);
      popup.location.href = data.url;
    });
    const result = await client.exchangeCodeForSession(code, data.flowId ? { flowId: data.flowId } : undefined);
    if (result.error) throw new Error('Google sign-in could not be completed. Try again.');
  } finally { popup.close(); }
}
