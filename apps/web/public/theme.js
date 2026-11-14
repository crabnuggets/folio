(() => {
  const system = window.matchMedia('(prefers-color-scheme: dark)');
  let saved;
  try { saved = localStorage.getItem('folio-theme'); } catch { /* Theme controls still work without storage. */ }
  const apply = theme => {
    document.documentElement.dataset.theme = theme;
    document.querySelector('meta[name="theme-color"]').content = theme === 'dark' ? '#0D1312' : '#EEF5F4';
  };
  apply(saved === 'light' || saved === 'dark' ? saved : system.matches ? 'dark' : 'light');
  system.addEventListener('change', () => {
    let preference;
    try { preference = localStorage.getItem('folio-theme'); } catch { return; }
    if (preference !== 'light' && preference !== 'dark') apply(system.matches ? 'dark' : 'light');
  });
})();
