import { useEffect, useState } from 'react';
import type { GoTrueClient, OAuthAuthorizationDetails } from '@supabase/auth-js';
import { Brand } from './components/Brand';
import { errorMessage } from './lib/api';

// Only provider-returned callbacks for known MCP clients are used. Query
// parameters never supply a URL.
function knownCallback(url: URL) {
  if (url.origin === 'https://claude.ai' || url.origin === 'https://claude.com') return url.pathname === '/api/mcp/auth_callback';
  if (url.origin === 'https://chatgpt.com') return /^\/connector\/oauth\/[^/]+$/.test(url.pathname) || url.pathname === '/connector_platform_oauth_redirect';
  return false;
}
export function oauthCallback(value: string) {
  const url = new URL(value);
  if (url.username || url.password || !knownCallback(url)) throw new Error('The authorization provider returned an unexpected callback.');
  return url.href;
}

export function OAuthConsent({ client, clientID, onLogout }: { client: GoTrueClient; clientID?: string; onLogout(): void }) {
  const [details, setDetails] = useState<OAuthAuthorizationDetails>();
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  const authorizationID = new URLSearchParams(window.location.search).get('authorization_id') ?? '';
  useEffect(() => {
    let active = true;
    if (!clientID || !/^[a-zA-Z0-9_-]{1,200}$/.test(authorizationID)) { setError(new Error('This authorization request is missing or MCP is not configured.')); return; }
    void client.oauth.getAuthorizationDetails(authorizationID).then(({ data, error }) => {
      if (!active) return;
      if (error || !data) throw new Error(`The authorization request expired or could not be loaded. Start the connection again from your AI assistant.${error?.message ? ` (${error.message})` : ''}`);
      if ('redirect_url' in data) { window.location.assign(oauthCallback(data.redirect_url)); return; }
      if (data.client.id !== clientID) throw new Error(`This client (${data.client.name || data.client.id}) is not the one registered for Folio MCP.`);
      if (!data.scope.split(' ').includes('openid')) throw new Error(`Folio needs the openid scope, but this request asked for "${data.scope || 'nothing'}". Reconnect from your AI assistant.`);
      oauthCallback(data.redirect_uri);
      setDetails(data);
    }).catch(failure => { if (active) setError(failure); });
    return () => { active = false; };
  }, [client, clientID, authorizationID]);
  const decide = async (approve: boolean) => {
    if (!details || busy) return;
    setBusy(true); setError(undefined);
    try {
      const result = approve ? await client.oauth.approveAuthorization(authorizationID, { skipBrowserRedirect: true }) : await client.oauth.denyAuthorization(authorizationID, { skipBrowserRedirect: true });
      if (result.error || !result.data?.redirect_url) throw new Error(`The authorization decision could not be saved. Start the connection again from your AI assistant.${result.error?.message ? ` (${result.error.message})` : ''}`);
      window.location.assign(oauthCallback(result.data.redirect_url));
    } catch (failure) { setError(failure); setBusy(false); }
  };
  return <main className="login-page"><div className="login-card"><Brand /><h1>Connect an AI assistant to Folio</h1>
    {details && <><p>{details.client.name || 'An AI assistant'} is requesting access as {details.user.email}.</p><p>Allow it to read your monthly sheets and create pending item proposals. You review, edit, accept, or reject proposals in Folio. Only acceptance changes totals.</p><p className="fine-print">Requested permission: {details.scope}. Access is restricted to the registered Folio MCP client and resource.</p>
      <div className="actions"><button disabled={busy} onClick={() => void decide(false)}>Deny access</button><button className="primary" disabled={busy} onClick={() => void decide(true)}>Allow monthly sheets and proposals</button></div></>}
    {!details && !error && <p role="status">Loading authorization request...</p>}
    {error != null && <p role="alert">{errorMessage(error)}</p>}
    <button disabled={busy} onClick={onLogout}>Sign out</button>
  </div></main>;
}
