import { type APIRequestContext } from '@playwright/test';
import { test, expect } from './runtime-fixture';
async function snapshot(request: APIRequestContext) {
  return (await request.get('/api/overview')).json();
}
async function seed(request: APIRequestContext, https = false) {
  const id = Date.now().toString(36),
    domain = `framework-${id}.test`;
  const st = await snapshot(request);
  let response = await request.post('/api/sites', {
    headers: { 'If-Match': String(st.state.revision) },
    data: { name: `框架验证${id}`, domains: [domain], kind: 'static', enabled: true },
  });
  expect(response.ok()).toBeTruthy();
  const site = await response.json();
  if (https) {
    response = await request.post('/api/panel/websites/ca', {
      data: {
        name: `Framework${id}`,
        commonName: `Framework${id}`,
        country: 'CN',
        organization: 'Test',
        keyType: 'EC256',
      },
    });
    expect(response.ok()).toBeTruthy();
    const state = await snapshot(request),
      ca = state.state.ssl.cas.find((c: { name: string }) => c.name === `Framework${id}`);
    response = await request.post('/api/panel/websites/ca/obtain', {
      data: { id: ca.id, domains: domain, keyType: 'EC256', time: 30, unit: 'day' },
    });
    expect(response.ok()).toBeTruthy();
    const ssl = (await response.json()).id;
    response = await request.post(`/api/panel/websites/${site.panelId}/https`, {
      data: {
        enable: true,
        type: 'existed',
        websiteSSLId: ssl,
        httpConfig: 'HTTPToHTTPS',
        SSLProtocol: ['TLSv1.2', 'TLSv1.3'],
        algorithm: 'HIGH:!aNULL',
      },
    });
    expect(response.ok()).toBeTruthy();
  }
  return site;
}
test('upstream navigation, automatic layout, transitions and system theme', async ({
  page,
  request,
}) => {
  const site = await seed(request);
  await page.goto(`/#/sites?site=${site.id}&tab=basic`);
  await page.getByRole('tab', { name: '真实 IP', exact: true }).click();
  await page.getByRole('tab', { name: 'HTTPS', exact: true }).click();
  await expect(page).toHaveURL(/tab=tls/);
  await page.goBack();
  await expect(page.getByRole('tab', { name: '真实 IP', exact: true })).toHaveAttribute(
    'aria-selected',
    'true',
  );
  await page.goForward();
  await expect(page.getByRole('tab', { name: 'HTTPS', exact: true })).toHaveAttribute(
    'aria-selected',
    'true',
  );
  const sidebar = page.locator('.app-sidebar');
  await expect(sidebar).toHaveCSS('transition-duration', '0.3s');
  await expect(page.locator('.main-container')).toHaveCSS('transition-duration', '0.3s');
  await expect(page.locator('.menu-background svg linearGradient')).toHaveCount(2);
  await page.evaluate(() => {
    (window as any).__transitionSeen = false;
    new MutationObserver((records) => {
      for (const record of records) {
        const element = record.target as Element;
        if (element.classList?.contains('fade-transform-enter-active'))
          (window as any).__transitionSeen = true;
      }
    }).observe(document.querySelector('.app-main')!, {
      subtree: true,
      attributes: true,
      attributeFilter: ['class'],
    });
  });
  await sidebar.getByRole('menuitem', { name: 'SSL 证书', exact: true }).click();
  await expect(page).toHaveURL(/websites\/ssl$/);
  await expect(sidebar.getByRole('menuitem', { name: 'SSL 证书', exact: true })).toHaveClass(
    /is-active/,
  );
  await expect.poll(() => page.evaluate(() => (window as any).__transitionSeen)).toBe(true);
  await page.getByRole('button', { name: '主题模式', exact: true }).click();
  await page.getByRole('menuitem', { name: '跟随系统', exact: true }).click();
  await page.emulateMedia({ colorScheme: 'dark' });
  await expect(page.locator('html')).toHaveClass('dark');
  await page.emulateMedia({ colorScheme: 'light' });
  await expect(page.locator('html')).toHaveClass('light');
  await page.screenshot({
    path: '../docs/framework-desktop.png',
    fullPage: true,
    animations: 'disabled',
  });
  await page.setViewportSize({ width: 1000, height: 900 });
  await expect(sidebar).toHaveCSS('width', '75px');
  await expect(
    sidebar.getByRole('menuitem', { name: 'OpenResty', exact: true }).locator('svg'),
  ).toBeVisible();
  await page.setViewportSize({ width: 1400, height: 900 });
  await expect(sidebar).toHaveCSS('width', '180px');
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(sidebar).toHaveCSS('transform', 'matrix(1, 0, 0, 1, -180, 0)');
  await page.getByRole('button', { name: '打开导航', exact: true }).click();
  await sidebar
    .locator('.el-menu-item')
    .filter({ hasText: /^网站$/ })
    .click();
  await expect(page.locator('.complex-table__card').filter({ hasText: site.name })).toBeVisible();
  await page
    .locator('.complex-table__card')
    .filter({ hasText: site.name })
    .getByRole('button', { name: '配置', exact: true })
    .click();
  await expect(page).toHaveURL(new RegExp('site=' + site.id));
  await page.getByRole('button', { name: '打开导航', exact: true }).click();
  await sidebar.getByRole('menuitem', { name: '默认页面', exact: true }).click();
  await expect(page).toHaveURL(/\/pages$/);
  await expect(page.locator('.drawer-bg')).not.toBeVisible();
  await expect(page.getByRole('combobox', { name: '选择错误页面' })).toBeInViewport();
  await expect
    .poll(() => page.locator('main.app-main').evaluate((element) => element.scrollTop))
    .toBe(0);
  await expect(page.locator('.fade-transform-enter-active')).toHaveCount(0);
  await page.screenshot({
    path: '../docs/framework-mobile.png',
    fullPage: true,
    animations: 'disabled',
  });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});
test('HTTPS disable confirmation recovers from cancel and failed saves', async ({
  page,
  request,
}) => {
  const site = await seed(request, true);
  await page.goto(`/#/sites?site=${site.id}&tab=tls`);
  const toggle = page.getByRole('switch', { name: '启用 HTTPS', exact: true });
  await expect(toggle).toBeChecked();
  await toggle.locator('..').click();
  const dialog = page.getByRole('dialog', { name: '禁用 HTTPS', exact: true });
  await dialog.getByRole('button', { name: '取消', exact: true }).click();
  await expect(toggle).toBeChecked();
  let rejectOnce = true;
  await page.route('**/api/panel/websites/*/https', async (route) => {
    if (route.request().method() === 'POST' && rejectOnce) {
      rejectOnce = false;
      await route.fulfill({
        status: 400,
        contentType: 'application/json',
        body: JSON.stringify({ error: '模拟保存失败，请重试' }),
      });
    } else await route.continue();
  });
  await toggle.locator('..').click();
  await dialog.getByRole('button', { name: '确认', exact: true }).click();
  await expect(page.locator('.el-message--error')).toContainText('模拟保存失败');
  await expect(dialog.getByRole('button', { name: '确认', exact: true })).not.toHaveClass(
    /is-loading/,
  );
  await expect(dialog).toBeVisible();
  await dialog.getByRole('button', { name: '确认', exact: true }).click();
  await expect(dialog).not.toBeVisible();
  await expect(toggle).not.toBeChecked();
  expect(
    (await snapshot(request)).state.sites.find((s: { id: string }) => s.id === site.id).tlsId,
  ).toBe('');
});
test('HTTPS file chooser resolves original components in the source build', async ({
  page,
  request,
}) => {
  const site = await seed(request);
  await page.goto(`/#/sites?site=${site.id}&tab=tls`);
  await page.getByRole('switch', { name: '启用 HTTPS', exact: true }).locator('..').click();
  const select = async (label: string, value: string) => {
    await page.getByRole('combobox', { name: new RegExp(label + '$') }).press('ArrowDown');
    await page.getByRole('option', { name: value, exact: true }).click();
  };
  await select('SSL 选项', '手动导入证书');
  await select('导入方式', '选择服务器文件');
  const field = page.locator('.el-form-item').filter({ hasText: '证书文件' });
  await field.getByRole('button').click();
  const chooser = page.getByRole('dialog', { name: '文件列表', exact: true });
  await expect(chooser).toBeVisible();
  await expect(chooser.locator('.el-table')).toBeVisible();
  await chooser.getByRole('button', { name: '取消', exact: true }).click();
});
