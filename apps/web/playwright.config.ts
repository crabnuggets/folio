import { defineConfig, devices } from '@playwright/test';
import { fileURLToPath } from 'node:url';
import { createServer, type AddressInfo } from 'node:net';

// Share one isolated server across this run's workers, including config reloads.
const port = process.env.FOLIO_BROWSER_PORT ?? await new Promise<string>((resolve, reject) => {
  const listener = createServer().once('error', reject);
  listener.listen(0, '127.0.0.1', () => {
    const { port } = listener.address() as AddressInfo;
    listener.close(error => error ? reject(error) : resolve(String(port)));
  });
});
process.env.FOLIO_BROWSER_PORT = port;
const baseURL = `http://127.0.0.1:${port}`;

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  // Tests reset one isolated Go store, so projects must share one worker.
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: 0,
  timeout: 45_000,
  expect: { timeout: 10_000 },
  reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [
    { name: 'desktop-chromium', use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 1000 } } },
    { name: 'phone-chromium', use: { ...devices['Desktop Chrome'], viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true } },
  ],
  webServer: {
    command: 'node e2e/start-server.mjs',
    cwd: fileURLToPath(new URL('.', import.meta.url)),
    url: `${baseURL}/healthz`,
    reuseExistingServer: false,
    timeout: 180_000,
    stdout: 'pipe',
    stderr: 'pipe',
  },
});
