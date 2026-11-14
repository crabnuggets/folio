import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { AuthClient, AuthApiError } from '@supabase/auth-js';
import { OAuthConsent, oauthCallback } from '../src/OAuthConsent';

afterEach(() => { cleanup(); window.history.replaceState(null, '', '/'); });

it('allows only provider-returned Claude and ChatGPT callback URLs', () => {
  expect(oauthCallback('https://claude.ai/api/mcp/auth_callback?code=one&state=two')).toContain('code=one');
  expect(oauthCallback('https://claude.com/api/mcp/auth_callback?code=one')).toContain('claude.com');
  for (const value of ['https://claude.ai/api/mcp/other', 'https://claude.ai.evil.example/api/mcp/auth_callback', 'http://claude.ai/api/mcp/auth_callback']) expect(() => oauthCallback(value)).toThrow();
  expect(oauthCallback('https://chatgpt.com/connector/oauth/generated-id?state=one')).toContain('generated-id');
  expect(oauthCallback('https://chatgpt.com/connector_platform_oauth_redirect?code=one')).toContain('code=one');
  for (const value of ['javascript:alert(1)', 'https://chatgpt.com.evil.example/connector/oauth/id', 'https://evil.example/callback', 'http://chatgpt.com/connector/oauth/id', 'https://chatgpt.com/other', 'https://user@chatgpt.com/connector/oauth/id']) expect(() => oauthCallback(value)).toThrow();
});

it('loads provider details and requires explicit consent for the pinned client', async () => {
  const client = new AuthClient({ url: 'https://auth.example/auth/v1', persistSession: false, autoRefreshToken: false, detectSessionInUrl: false });
  const details = vi.spyOn(client.oauth, 'getAuthorizationDetails').mockResolvedValue({ data: { authorization_id: 'test-request', client: { id: 'registered-client', name: 'ChatGPT', uri: '', logo_uri: '' }, redirect_uri: 'https://chatgpt.com/connector/oauth/generated-id', user: { id: 'owner', email: 'synthetic@example.test' }, scope: 'openid' }, error: null });
  const approve = vi.spyOn(client.oauth, 'approveAuthorization').mockResolvedValue({ data: null, error: new AuthApiError('Synthetic expired request', 400, 'invalid_request') });
  window.history.replaceState(null, '', '/oauth/consent?authorization_id=test-request');
  render(<OAuthConsent client={client} clientID="registered-client" onLogout={() => {}} />);
  await screen.findByRole('button', { name: 'Allow monthly sheets and proposals' });
  expect(details).toHaveBeenCalledWith('test-request'); expect(approve).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: 'Allow monthly sheets and proposals' }));
  await waitFor(() => expect(approve).toHaveBeenCalledWith('test-request', { skipBrowserRedirect: true }));
  await screen.findByRole('alert');
});

it('refuses to approve an unregistered OAuth client', async () => {
  const client = new AuthClient({ url: 'https://auth.example/auth/v1', persistSession: false, autoRefreshToken: false, detectSessionInUrl: false });
  vi.spyOn(client.oauth, 'getAuthorizationDetails').mockResolvedValue({ data: { authorization_id: 'test', client: { id: 'other-client', name: 'Other', uri: '', logo_uri: '' }, redirect_uri: 'https://chatgpt.com/connector/oauth/generated-id', user: { id: 'owner', email: 'synthetic@example.test' }, scope: 'openid' }, error: null });
  const approve = vi.spyOn(client.oauth, 'approveAuthorization');
  window.history.replaceState(null, '', '/oauth/consent?authorization_id=test');
  render(<OAuthConsent client={client} clientID="registered-client" onLogout={() => {}} />);
  await screen.findByRole('alert');
  expect(screen.queryByRole('button', { name: 'Allow monthly sheets and proposals' })).toBeNull();
  expect(approve).not.toHaveBeenCalled();
});

it('shows the provider error when an authorization request cannot be loaded', async () => {
  const client = new AuthClient({ url: 'https://auth.example/auth/v1', persistSession: false, autoRefreshToken: false, detectSessionInUrl: false });
  vi.spyOn(client.oauth, 'getAuthorizationDetails').mockResolvedValue({ data: null, error: new AuthApiError('authorization request cannot be processed', 400, 'validation_failed') });
  window.history.replaceState(null, '', '/oauth/consent?authorization_id=used');
  render(<OAuthConsent client={client} clientID="registered-client" onLogout={() => {}} />);
  expect((await screen.findByRole('alert')).textContent).toContain('authorization request cannot be processed');
});
