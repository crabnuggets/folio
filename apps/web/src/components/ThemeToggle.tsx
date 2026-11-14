import { useSyncExternalStore } from 'react';

function subscribe(onChange: () => void) {
  const observer = new MutationObserver(onChange);
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] });
  return () => observer.disconnect();
}

export function ThemeToggle({ showLabel = false }: { showLabel?: boolean }) {
  const dark = useSyncExternalStore(subscribe, () => document.documentElement.dataset.theme === 'dark');
  const label = dark ? 'Switch to light mode' : 'Switch to dark mode';
  return <button className={showLabel ? undefined : 'icon-button'} aria-label={label} title={label} onClick={() => {
    const theme = dark ? 'light' : 'dark';
    document.documentElement.dataset.theme = theme;
    const color = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]');
    if (color) color.content = dark ? '#EEF5F4' : '#0D1312';
    try { localStorage.setItem('folio-theme', theme); } catch { /* Keep the choice for this page when storage is unavailable. */ }
  }}><svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
    {dark ? <><circle cx="12" cy="12" r="4" /><path d="M12 2v2M12 20v2M2 12h2M20 12h2M5 5l1.5 1.5M17.5 17.5 19 19M5 19l1.5-1.5M17.5 6.5 19 5" /></> : <path d="M20.8 13A9 9 0 0 1 11 3.2 9 9 0 1 0 20.8 13Z" />}
  </svg>{showLabel && label}</button>;
}
