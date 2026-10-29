import { spawn, spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const web = fileURLToPath(new URL('..', import.meta.url));
const api = fileURLToPath(new URL('../../../services/api', import.meta.url));
const dist = fileURLToPath(new URL('../dist', import.meta.url));
const windows = process.platform === 'win32';

// Static arguments only: Windows requires cmd.exe to launch npm.cmd.
const build = spawnSync(windows ? 'npm.cmd' : 'npm', ['run', 'build'], {
  cwd: web, stdio: 'inherit', shell: windows,
});
if (build.error) throw build.error;
if (build.status !== 0) process.exit(build.status ?? 1);

const server = spawn('go', [
  'test', './internal/httpapi', '-run', '^TestBrowserServer$', '-count=1', '-timeout=30m',
], {
  cwd: api, stdio: 'inherit',
  env: { ...process.env, FOLIO_BROWSER_E2E: '1', FOLIO_BROWSER_WEB_DIST_DIR: dist },
});
server.on('error', error => { console.error(error.message); process.exit(1); });
server.on('exit', code => process.exit(code ?? 1));
for (const signal of ['SIGINT', 'SIGTERM']) process.once(signal, () => server.kill(signal));
