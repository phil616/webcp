import { test, expect } from './runtime-fixture';

test.describe('WebCP access key login', () => {
  test.use({ autoLogin: false });
  test('login is required, incorrect keys are rejected and logout revokes the session', async ({
    page,
    request,
  }) => {
    expect((await request.get('/api/overview')).status()).toBe(401);
    expect((await request.get('/api/backups/unknown/download')).status()).toBe(401);
    await page.goto('/#/sites');
    await expect(page).toHaveTitle(/WebCP/);
    await expect(page.getByRole('heading', { name: 'WebCP', exact: true })).toBeVisible();
    await expect(page.locator('.sidebar-logo')).toHaveCount(0);
    await page.getByLabel('访问密钥', { exact: true }).fill('wrong-key');
    await page.getByRole('button', { name: '登录', exact: true }).click();
    await expect(page.getByRole('alert')).toContainText('访问密钥不正确');
    await page
      .getByRole('textbox', { name: '访问密钥', exact: true })
      .fill(process.env.WEBCP_TEST_ACCESS_KEY!);
    await page.getByRole('button', { name: '登录', exact: true }).click();
    await expect(page.getByRole('button', { name: '创建网站', exact: true })).toBeVisible();
    await expect(page.locator('.sidebar-logo')).toHaveCount(0);
    const storage = await page.evaluate(() => JSON.stringify({ ...localStorage }));
    expect(storage).not.toContain(process.env.WEBCP_TEST_ACCESS_KEY!);
    await page.reload();
    await expect(page.getByRole('button', { name: '创建网站', exact: true })).toBeVisible();
    await page.getByRole('button', { name: '退出登录', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'WebCP', exact: true })).toBeVisible();
    expect((await page.context().request.get('/api/overview')).status()).toBe(401);
  });
  test('a revoked session returns the UI to login', async ({ page }) => {
    await page
      .context()
      .request.post('/api/auth/login', { data: { key: process.env.WEBCP_TEST_ACCESS_KEY } });
    await page.goto('/#/sites');
    await expect(page.getByRole('button', { name: '创建网站', exact: true })).toBeVisible();
    await page.context().request.post('/api/auth/logout', { data: {} });
    await page.getByRole('button', { name: '刷新', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'WebCP', exact: true })).toBeVisible();
  });
  test('a delayed response from an old session cannot revoke a new login', async ({ page }) => {
    await page
      .context()
      .request.post('/api/auth/login', { data: { key: process.env.WEBCP_TEST_ACCESS_KEY } });
    await page.goto('/#/sites');
    await expect(page.getByRole('button', { name: '创建网站', exact: true })).toBeVisible();
    let release: () => void = () => {};
    const delayed = new Promise<void>((resolve) => (release = resolve));
    let received: () => void = () => {};
    const pending = new Promise<void>((resolve) => (received = resolve));
    let held = false;
    await page.route('**/api/overview', async (route) => {
      if (held) return route.continue();
      held = true;
      received();
      await delayed;
      await route.fulfill({ status: 401, json: { error: '登录已过期' } });
    });
    await page.getByRole('button', { name: '刷新', exact: true }).click();
    await pending;
    await page.getByRole('button', { name: '退出登录', exact: true }).click();
    await page.getByLabel('访问密钥', { exact: true }).fill(process.env.WEBCP_TEST_ACCESS_KEY!);
    await page.getByRole('button', { name: '登录', exact: true }).click();
    await expect(page.getByRole('button', { name: '创建网站', exact: true })).toBeVisible();
    const oldResponse = page.waitForResponse(
      (response) => response.url().endsWith('/api/overview') && response.status() === 401,
    );
    release();
    await oldResponse;
    await page.getByRole('button', { name: '刷新', exact: true }).click();
    await expect(page.getByRole('button', { name: '创建网站', exact: true })).toBeVisible();
  });
});

test('container selection accepts custom names and images without creating a container', async ({
  page,
  request,
}) => {
  const original = await (await request.get('/api/runtime/connection')).json();
  await page.goto('/#/runtime');
  await page.getByRole('button', { name: '容器连接', exact: true }).click();
  const name = `webcp-ui-${Date.now()}`;
  await page.getByRole('textbox', { name: '容器名称', exact: true }).fill(name);
  await page
    .getByRole('textbox', { name: 'OpenResty 镜像', exact: true })
    .fill('custom/openresty:local');
  await page.getByRole('button', { name: '保存连接', exact: true }).click();
  await expect
    .poll(async () => (await (await request.get('/api/runtime/connection')).json()).container)
    .toBe(name);
  const changed = await (await request.get('/api/runtime/connection')).json();
  expect(changed.image).toBe('custom/openresty:local');
  const overview = await (await request.get('/api/overview')).json();
  expect(overview.runtime.container).toBe(name);
  expect(overview.runtime.running).toBe(false);
  await page.getByRole('radio', { name: '连接已有容器', exact: true }).locator('..').click();
  await expect(page.getByRole('combobox', { name: '已有容器', exact: true })).toBeVisible();
  await expect(page.getByRole('alert').filter({ hasText: '首次应用前备份原配置' })).toBeVisible();
  const restored = await request.put('/api/runtime/connection', {
    headers: { 'If-Match': String(changed.version) },
    data: original,
  });
  expect(restored.ok()).toBeTruthy();
});
