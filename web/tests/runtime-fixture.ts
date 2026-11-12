import { test as base, expect } from '@playwright/test';
// Production strips Vue warnings. Run the same flows through Vite development
// as well, and fail on component registration/injection/prop/render problems.
export const test = base.extend<{ runtimeAudit: void; autoLogin: boolean; accessSession: void }>({
  autoLogin: [true, { option: true }],
  accessSession: [
    async ({ page, request, autoLogin }, use) => {
      if (autoLogin) {
        const key = process.env.WEBCP_TEST_ACCESS_KEY;
        if (!key) throw new Error('设置 WEBCP_TEST_ACCESS_KEY 后运行浏览器测试');
        for (const client of [request, page.context().request]) {
          const result = await client.post('/api/auth/login', { data: { key } });
          expect(result.ok(), '测试密钥登录失败').toBeTruthy();
        }
      }
      await use();
    },
    { auto: true },
  ],
  runtimeAudit: [
    async ({ page }, use) => {
      const errors: string[] = [];
      page.on('pageerror', (error) => errors.push(error.message));
      page.on('console', (message) => {
        if (message.type() === 'warning' && /\[Vue warn\]|\[Vue Router warn\]/.test(message.text()))
          errors.push(message.text());
      });
      await use();
      expect(errors, 'Unexpected browser or Vue runtime errors').toEqual([]);
    },
    { auto: true },
  ],
});
export { expect };
