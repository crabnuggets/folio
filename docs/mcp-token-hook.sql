-- Provider setup only. This file is not an application migration and is not run
-- on startup. Replace the client ID below with the dedicated Supabase OAuth
-- client's UUID, then install and enable this Custom Access Token hook.
-- Merge this branch into an existing hook if the project already uses one.
CREATE OR REPLACE FUNCTION public.folio_mcp_access_token_hook(event jsonb)
RETURNS jsonb
LANGUAGE plpgsql STABLE
SET search_path = ''
AS $$
DECLARE
 claims jsonb := event->'claims';
BEGIN
 IF claims->>'client_id' = 'REPLACE_WITH_MCP_CLIENT_ID' THEN
  claims := jsonb_set(claims, '{aud}', '"https://folio-j4md.onrender.com/mcp"'::jsonb);
 END IF;
 RETURN jsonb_set(event, '{claims}', claims);
END;
$$;

REVOKE ALL ON FUNCTION public.folio_mcp_access_token_hook(jsonb) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.folio_mcp_access_token_hook(jsonb) FROM anon, authenticated;
GRANT USAGE ON SCHEMA public TO supabase_auth_admin;
GRANT EXECUTE ON FUNCTION public.folio_mcp_access_token_hook(jsonb) TO supabase_auth_admin;
