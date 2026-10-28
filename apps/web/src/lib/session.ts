/** This is only an expiry hint for clearing UI memory; the API verifies identity. */
export function tokenExpiry(token: string): number | null {
  try {
    const part = token.split('.')[1];
    if (!part) return null;
    const payload: unknown = JSON.parse(atob(part.replaceAll('-', '+').replaceAll('_', '/')));
    if (payload && typeof payload === 'object' && 'exp' in payload && typeof payload.exp === 'number' && Number.isFinite(payload.exp)) {
      return payload.exp * 1000;
    }
  } catch { /* Opaque local development tokens have no JWT expiry. */ }
  return null;
}
