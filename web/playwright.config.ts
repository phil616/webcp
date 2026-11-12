import { defineConfig } from '@playwright/test';
const development = process.env.RESTY_TEST_MODE === 'dev';
export default defineConfig({
  webServer: development
    ? {
        command: 'npm run dev',
        url: 'http://127.0.0.1:5173',
        reuseExistingServer: false,
        env: { RESTY_DEV_API: process.env.RESTY_DEV_API || 'http://127.0.0.1:19080' },
      }
    : undefined,
  testDir: 'tests',
  workers: 1,
  use: {
    baseURL:
      process.env.RESTY_TEST_URL ||
      (development ? 'http://127.0.0.1:5173' : 'http://127.0.0.1:19080'),
    headless: true,
    viewport: { width: 1440, height: 1050 },
    actionTimeout: 15000,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  reporter: 'list',
});
