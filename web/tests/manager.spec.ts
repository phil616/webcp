import { type Page, type APIRequestContext } from '@playwright/test';
import { test, expect } from './runtime-fixture';

test.setTimeout(90_000);
const suffix = () => Date.now().toString(36);
async function navigate(page: Page, label: string) {
  await page
    .locator('.app-sidebar .el-menu-item')
    .filter({ hasText: new RegExp(`^${label}$`) })
    .click();
}
async function code(page: Page, label: string, value: string) {
  const editor = page.getByRole('textbox', { name: label, exact: true });
  await expect(editor).toBeVisible();
  await editor.fill(value);
}
async function state(request: APIRequestContext) {
  return (await request.get('/api/overview')).json();
}
async function seedSite(request: APIRequestContext, name: string, domain: string) {
  const snapshot = await state(request);
  const response = await request.post('/api/sites', {
    headers: { 'If-Match': String(snapshot.state.revision) },
    data: { name, domains: [domain], kind: 'static', enabled: true },
  });
  expect(response.ok()).toBeTruthy();
  return response.json();
}
async function choose(page: Page, label: string, option: string) {
  await page
    .getByRole('combobox', {
      name: new RegExp('^[*\\s]*' + label.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '$'),
    })
    .press('ArrowDown');
  await page.getByRole('option', { name: option, exact: true }).click();
}
async function saveSetting(page: Page, label = '保存') {
  const button = page.getByRole('button', { name: label, exact: true });
  const response = page.waitForResponse(
    (r) =>
      /\/api\/(sites\/|panel\/websites\/\d+\/https)/.test(r.url()) &&
      ['PUT', 'POST'].includes(r.request().method()),
  );
  await button.click();
  expect((await response).ok()).toBeTruthy();
  await expect(button).not.toHaveClass(/is-loading/);
}
const drawer = (page: Page) => page.locator('.el-drawer[role="dialog"]:visible');
async function closeDrawer(page: Page) {
  await drawer(page)
    .locator('.el-drawer__header')
    .getByRole('button', { name: '关闭', exact: true })
    .click();
  await expect(drawer(page)).not.toBeVisible();
}

test('1Panel table, create drawer, directory editor and website settings', async ({
  page,
  request,
}) => {
  const id = suffix(),
    name = `网站验证 ${id}`,
    domain = `browser-${id}.example.test`;
  const errors: string[] = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.goto('/');
  await expect(page.getByRole('heading', { name: '网站', exact: true })).toBeVisible();
  expect(
    await page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue('--panel-color-primary').trim(),
    ),
  ).toBe('#005eeb');
  await page.getByRole('button', { name: '创建网站', exact: true }).click();
  await expect(drawer(page)).toBeVisible();
  const bounds = await drawer(page).boundingBox();
  expect(bounds!.x).toBeGreaterThan(700);
  await drawer(page).getByRole('button', { name: '确认', exact: true }).click();
  await expect(page.getByText('请输入网站名称', { exact: true })).toBeVisible();
  await page.getByLabel('网站名称', { exact: true }).fill(name);
  await page.getByLabel('域名 / IP', { exact: true }).fill(domain);
  await drawer(page).getByRole('button', { name: '确认', exact: true }).click();
  await expect(drawer(page)).not.toBeVisible();
  let row = page.getByRole('row').filter({ has: page.getByRole('button', { name, exact: true }) });
  await expect(row).toBeVisible();
  await row.getByRole('button', { name: '网站文件', exact: true }).click();
  await code(page, '文件内容', '<h1>Browser file content</h1>');
  await drawer(page).getByRole('button', { name: '保存文件', exact: true }).click();
  await expect(drawer(page).getByRole('button', { name: '保存文件', exact: true })).toBeEnabled();
  await page.getByLabel('上传文件', { exact: true }).setInputFiles({
    name: 'pixel.bin',
    mimeType: 'application/octet-stream',
    buffer: Buffer.from([0, 255, 128, 42]),
  });
  await expect(drawer(page).getByText('pixel.bin', { exact: true })).toBeVisible();
  await closeDrawer(page);
  await row.getByRole('button', { name: '配置', exact: true }).click();
  await expect(page).toHaveURL(/site=/);
  await expect(page.getByRole('tab', { name: 'HTTPS', exact: true })).toBeVisible();
  await page.getByRole('button', { name: '修改名称', exact: true }).click();
  await page.getByLabel('网站名称', { exact: true }).fill(`${name} 已编辑`);
  await page.getByRole('button', { name: '保存', exact: true }).click();
  await expect
    .poll(async () => {
      const s = await state(request);
      return s.state.sites.some((site: { name: string }) => site.name === `${name} 已编辑`);
    })
    .toBeTruthy();
  await page.locator('.content-container__title').getByRole('button', { name: /^返回/ }).click();
  await expect(page).toHaveURL(/#\/?sites$/);
  row = page
    .getByRole('row')
    .filter({ has: page.getByRole('button', { name: `${name} 已编辑`, exact: true }) });
  await row.getByRole('button', { name: '已启用', exact: false }).click();
  await expect(page.getByRole('dialog', { name: '停止网站', exact: true })).toBeVisible();
  await page
    .getByRole('dialog', { name: '停止网站', exact: true })
    .getByRole('button', { name: '取消', exact: true })
    .click();
  await expect(row.getByRole('button', { name: /已启用/ })).toBeVisible();
  await page.getByRole('button', { name: '高级设置', exact: true }).click();
  await expect(drawer(page)).toBeVisible();
  await expect(page.getByRole('textbox', { name: '错误页面 HTML', exact: true })).toBeVisible();
  await closeDrawer(page);
  await expect(page.locator('.el-message')).toHaveCount(0, { timeout: 15000 });
  await page.screenshot({ path: '../docs/workspace-desktop.png', fullPage: true });
  expect(errors).toEqual([]);
});

test('official SSL pages, Lua reuse and independent mTLS lifecycle', async ({ page, request }) => {
  const id = suffix(),
    domain = `tls-${id}.example.test`,
    siteName = `mTLS 验证 ${id}`,
    caName = `Root${id}`,
    clientName = `Client ${id}`,
    moduleName = `utils_${id}`,
    blockName = `access_${id}`;
  const seeded = await seedSite(request, siteName, domain);
  const errors: string[] = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.goto('/#lua');
  await page.getByRole('button', { name: '创建模块', exact: true }).click();
  await page.getByLabel('资源名称', { exact: true }).fill(moduleName);
  await drawer(page).getByRole('button', { name: '保存', exact: true }).click();
  await expect(drawer(page)).not.toBeVisible();
  await page.getByRole('button', { name: '创建代码块', exact: true }).click();
  await page.getByLabel('资源名称', { exact: true }).fill(blockName);
  await code(
    page,
    'Lua 代码',
    `local helper = require("${moduleName}")\nngx.header["X-Test"] = helper.hello()\n`,
  );
  await drawer(page).getByRole('button', { name: '保存', exact: true }).click();
  await expect(drawer(page)).not.toBeVisible();

  await createOfficialCA(page, caName, '/#/websites/ssl/ca');
  await page.goto('/#/websites/ssl');
  await page.getByRole('button', { name: '申请证书', exact: true }).click();
  await page.getByRole('radio', { name: '自签证书', exact: true }).locator('..').click();
  await choose(page, '证书颁发机构', caName);
  await drawer(page).getByRole('textbox', { name: /域名/ }).fill(domain);
  await drawer(page).getByRole('button', { name: '确认', exact: true }).click();
  await expect(drawer(page)).not.toBeVisible();
  await expect(page.getByRole('row').filter({ hasText: domain })).toContainText('正常');
  await page.screenshot({ path: '../docs/ssl-certificates.png', fullPage: true });
  const clientCA = `ClientRoot${id}`;
  await createOfficialCA(page, clientCA, '/#/mtls');
  await page.goto('/#/mtls/clients');
  await page.getByRole('button', { name: '签发证书', exact: true }).click();
  await page.getByLabel('名称（Common Name）', { exact: true }).fill(clientName);
  await choose(page, '签发 CA', clientCA);
  await drawer(page).getByRole('button', { name: '确认', exact: true }).click();
  await expect(drawer(page)).not.toBeVisible();
  await expect(page.getByRole('row').filter({ hasText: clientName })).toContainText(clientCA);
  const before = await state(request);
  expect(before.state.certificates.every((c: { kind: string }) => c.kind !== 'client')).toBe(true);
  expect(before.state.mtls.clients.some((c: { name: string }) => c.name === clientName)).toBe(true);
  await page.goto(`/#sites?site=${seeded.id}&tab=tls`);
  await switchTo(page, '启用 HTTPS');
  await page.getByRole('combobox', { name: /^[*\s]*证书$/ }).press('ArrowDown');
  await page.getByRole('option').filter({ hasText: domain }).click();
  await expect(page.getByRole('option', { name: clientCA, exact: true })).not.toBeVisible();
  await saveSetting(page);
  await expect
    .poll(
      async () =>
        (await state(request)).state.sites.find((s: { id: string }) => s.id === seeded.id).tlsId,
    )
    .toBeTruthy();
  await expect(page.getByText('客户端证书验证（mTLS）', { exact: true })).not.toBeVisible();
  await page.getByRole('tab', { name: 'Lua 扩展', exact: true }).click();
  await choose(page, '访问阶段（access）', blockName);
  await saveSetting(page);
  await page.goto('/#/mtls/bindings');
  const bindingRow = page.getByRole('row').filter({ hasText: siteName });
  await bindingRow.getByRole('button', { name: '设置', exact: true }).click();
  await choose(page, '信任 CA', clientCA);
  await drawer(page).getByRole('button', { name: '确认', exact: true }).click();
  await expect(drawer(page)).not.toBeVisible();
  await expect(bindingRow).toContainText(clientCA);
  await page.screenshot({ path: '../docs/mtls-bindings.png', fullPage: true });
  await page.goto('/#/mtls/clients');
  const clientRow = page.getByRole('row').filter({ hasText: clientName });
  await clientRow.getByRole('button', { name: '吊销', exact: true }).click();
  const revoke = page.getByRole('dialog', { name: '吊销客户端证书', exact: true });
  await revoke.getByRole('button', { name: '确认', exact: true }).click();
  await expect(clientRow).toContainText('已吊销');
  await page.screenshot({ path: '../docs/mtls-clients.png', fullPage: true });
  const saved = await state(request);
  expect(
    saved.state.mtls.bindings.some(
      (b: { siteId: string; enabled: boolean }) => b.siteId === seeded.id && b.enabled,
    ),
  ).toBe(true);
  expect(
    saved.state.sites.find((s: { id: string }) => s.id === seeded.id).clientCaId,
  ).toBeUndefined();
  expect(errors).toEqual([]);
});

async function createOfficialCA(page: Page, name: string, path: string) {
  await page.goto(path);
  await page.getByRole('button', { name: '创建', exact: true }).click();
  await page.getByLabel('机构名称', { exact: true }).fill(name);
  await page.getByLabel('证书主体名称(CN)', { exact: true }).fill(name);
  await page.getByLabel('公司/组织', { exact: true }).fill('Example');
  await drawer(page).getByRole('button', { name: '确认', exact: true }).click();
  await expect(drawer(page)).not.toBeVisible();
  await expect(page.getByRole('row').filter({ hasText: name })).toBeVisible();
}

test('default page editor and stale-version protection after refresh', async ({
  page,
  request,
}) => {
  await page.goto('/#pages');
  await code(page, '错误页面 HTML', '<h1>stale local edit</h1>');
  const snapshot = await state(request);
  const document = await (await request.get('/api/pages')).json();
  const externalPages = { ...document.pages, '404': '<h1>external edit</h1>' };
  const response = await request.put('/api/pages', {
    headers: { 'If-Match': String(snapshot.state.revision) },
    data: { pages: externalPages, etag: document.etag },
  });
  expect(response.ok()).toBeTruthy();
  await page.getByRole('button', { name: '刷新', exact: true }).click();
  await expect(page.locator('.revision')).toContainText(`草稿 v${snapshot.state.revision}`);
  await page.getByRole('button', { name: '保存', exact: true }).click();
  await expect(page.locator('.el-message--error')).toContainText('页面文件已变化');
  expect((await (await request.get('/api/pages')).json()).pages['404']).toBe(externalPages['404']);
  await page.getByRole('button', { name: '重置', exact: true }).click();
  await choose(page, '选择错误页面', '502 - 网关错误');
  await code(page, '错误页面 HTML', `<h1>Custom 502 ${suffix()}</h1>`);
  await page.getByRole('button', { name: '保存', exact: true }).click();
  await expect(page.getByRole('button', { name: '保存', exact: true })).toBeDisabled();
});

test('backup drawer and 1Panel typed restore confirmation', async ({ page }) => {
  await page.goto('/#backups');
  await page.getByRole('button', { name: '创建备份', exact: true }).click();
  await drawer(page).getByRole('button', { name: '确认', exact: true }).click();
  await expect(drawer(page)).not.toBeVisible();
  await page.getByRole('button', { name: '恢复', exact: true }).first().click();
  const dialog = page.getByRole('dialog', { name: '恢复备份', exact: true });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole('button', { name: '确认', exact: true })).toBeDisabled();
  await dialog.getByLabel('确认内容', { exact: true }).fill('恢复');
  await expect(dialog.getByRole('button', { name: '确认', exact: true })).toBeEnabled();
  await dialog.getByRole('button', { name: '取消', exact: true }).click();
  await expect(dialog).not.toBeVisible();
});

test('running OpenResty serves page file edits without reload and keeps old generation on invalid Lua', async ({
  page,
  request,
}) => {
  const id = suffix(),
    name = `自动重载 ${id}`,
    domain = `reload-${id}.example.test`,
    badBlock = `invalid_${id}`;
  await seedSite(request, name, domain);
  await page.goto('/#sites');
  try {
    await page.getByRole('button', { name: '启动', exact: true }).click();
    await expect(page.locator('.engine-row')).toContainText('已启动', { timeout: 45000 });
    const pageRevision = (await state(request)).state.revision;
    await navigate(page, '默认页面');
    await code(page, '错误页面 HTML', `<h1>reload-${id}</h1>`);
    await page.getByRole('button', { name: '保存', exact: true }).click();
    await expect
      .poll(async () => {
        const s = await state(request);
        return (
          (await (await request.get('/api/pages')).json()).pages['404'] ===
            `<h1>reload-${id}</h1>` &&
          s.state.revision === pageRevision &&
          s.runtime.appliedRevision === pageRevision
        );
      })
      .toBeTruthy();
    const websiteURL = process.env.RESTY_TEST_WEBSITE_URL || 'http://127.0.0.1:18080';
    await expect
      .poll(async () => {
        const missing = await request.get(`${websiteURL}/missing`, { headers: { Host: domain } });
        return `${missing.status()} ${await missing.text()}`;
      })
      .toContain(`404 <h1>reload-${id}</h1>`);
    const before = (await state(request)).runtime.appliedRevision;
    await navigate(page, 'Lua 模块');
    await page.getByRole('button', { name: '创建代码块', exact: true }).click();
    await page.getByLabel('资源名称', { exact: true }).fill(badBlock);
    await code(page, 'Lua 代码', 'function !!! invalid lua');
    await drawer(page).getByRole('button', { name: '保存', exact: true }).click();
    await expect(drawer(page)).not.toBeVisible({ timeout: 30000 });
    await expect(page.locator('.el-message--error')).toContainText('配置已保存，但应用失败');
    expect((await state(request)).runtime.appliedRevision).toBe(before);
    expect((await request.get(`${websiteURL}/`, { headers: { Host: domain } })).status()).toBe(200);
    const row = page
      .getByRole('row')
      .filter({ has: page.getByRole('button', { name: badBlock, exact: true }) });
    await row.getByRole('button', { name: '编辑', exact: true }).click();
    await code(page, 'Lua 代码', 'ngx.header["X-Test"] = "ok"');
    await drawer(page).getByRole('button', { name: '保存', exact: true }).click();
    await expect(drawer(page)).not.toBeVisible({ timeout: 30000 });
    await expect
      .poll(async () => {
        const s = await state(request);
        return s.runtime.appliedRevision === s.state.revision;
      })
      .toBeTruthy();
  } finally {
    await request.post('/api/runtime/stop');
  }
});

test('collapsed menu, dark theme and mobile drawer keep upstream layout', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: '收起导航', exact: true }).click();
  await expect(page.locator('.app-sidebar')).toHaveCSS('width', '75px');
  await page.getByRole('button', { name: '展开导航', exact: true }).click();
  await page.getByRole('button', { name: '切换深色主题', exact: true }).click();
  await expect(page.locator('html')).toHaveClass('dark');
  expect(
    await page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue('--panel-color-primary').trim(),
    ),
  ).toBe('#3d8eff');
  await page.getByRole('button', { name: '切换浅色主题', exact: true }).click();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole('button', { name: '打开导航', exact: true }).click();
  await navigate(page, '默认页面');
  await expect(page.locator('.app-sidebar')).toHaveCSS('pointer-events', 'none');
  await expect(page.locator('.app-sidebar')).toHaveCSS('transform', 'matrix(1, 0, 0, 1, -180, 0)');
  await page.locator('iframe').scrollIntoViewIfNeeded();
  await expect(page.frameLocator('iframe').locator('body')).not.toBeEmpty();
  await page.screenshot({ path: '../docs/workspace-mobile.png', fullPage: true });
  await page.getByRole('button', { name: '打开导航', exact: true }).click();
  await navigate(page, '网站');
  await page.getByRole('button', { name: '创建网站', exact: true }).click();
  await expect(drawer(page)).toHaveCSS('width', '390px');
  await closeDrawer(page);
});

test('L4 TCP/UDP rules use 1Panel drawers, validation, toggles and deletion', async ({
  page,
  request,
}) => {
  const name = `四层代理 ${suffix()}`;
  try {
    await page.goto('/#streams');
    await expect(page.locator('.page-header-bar h1')).toHaveText('四层代理');
    await page.getByRole('button', { name: '创建四层代理', exact: true }).click();
    await drawer(page).getByRole('button', { name: '保存', exact: true }).click();
    await expect(drawer(page)).toBeVisible();
    await page.getByLabel('名称', { exact: true }).fill(name);
    await page.getByLabel('监听地址', { exact: true }).fill('127.0.0.1');
    await page.getByLabel('监听端口', { exact: true }).fill('29091');
    await page.getByLabel('目标地址', { exact: true }).fill('127.0.0.1;bad');
    await drawer(page).getByRole('button', { name: '保存', exact: true }).click();
    await expect(drawer(page).locator('.el-alert--error')).toContainText('目标地址无效');
    await page.getByLabel('目标地址', { exact: true }).fill('::1');
    await page.getByLabel('目标端口', { exact: true }).fill('5353');
    await drawer(page).getByRole('button', { name: '保存', exact: true }).click();
    await expect(drawer(page)).not.toBeVisible();
    const row = page
      .getByRole('row')
      .filter({ has: page.getByRole('button', { name, exact: true }) });
    await expect(row).toContainText('[::1]:5353');
    await expect(row).toContainText('TCP');
    await row.getByRole('button', { name: '编辑', exact: true }).click();
    await page.getByRole('radio', { name: 'UDP', exact: true }).locator('..').click();
    await page.getByRole('button', { name: '高级设置', exact: true }).click();
    await expect(page.getByLabel('每个 UDP 请求预期响应数', { exact: true })).toBeVisible();
    await expect(page.getByText('发送 PROXY protocol', { exact: true })).not.toBeVisible();
    await page.getByLabel('每个 UDP 请求预期响应数', { exact: true }).scrollIntoViewIfNeeded();
    await page.screenshot({
      path: '../docs/l4-proxy-drawer.png',
      fullPage: true,
      animations: 'disabled',
    });
    await drawer(page).getByRole('button', { name: '保存', exact: true }).click();
    await expect(drawer(page)).not.toBeVisible();
    await expect(row).toContainText('UDP');
    const saved = (await state(request)).state.streams.find(
      (item: { name: string }) => item.name === name,
    );
    expect(saved.protocol).toBe('udp');
    expect(saved.proxyProtocol).toBe(false);
    const config = await (await request.get('/api/config')).json();
    expect(config.config).toContain('proxy_pass [::1]:5353;');
    expect(config.config).toContain('udp reuseport;');
    await page.getByRole('button', { name: '启动', exact: true }).click();
    await expect(page.locator('.engine-row')).toContainText('已启动', { timeout: 45000 });
    await expect
      .poll(async () =>
        (await state(request)).runtime.publishedPorts.some(
          (port: { hostPort: number; protocol: string }) =>
            port.hostPort === 29091 && port.protocol === 'udp',
        ),
      )
      .toBeTruthy();
    await row.getByRole('button', { name: '已启用', exact: true }).click();
    await expect(row.getByRole('button', { name: '已停止', exact: true })).toBeVisible();
    await expect
      .poll(async () => {
        const current = await state(request);
        return (
          current.runtime.appliedRevision === current.state.revision &&
          !current.runtime.publishedPorts.some(
            (port: { hostPort: number }) => port.hostPort === 29091,
          )
        );
      })
      .toBeTruthy();
    await page.reload();
    await expect(row.getByRole('button', { name: '已停止', exact: true })).toBeVisible();
    await page.screenshot({ path: '../docs/l4-proxy.png', fullPage: true, animations: 'disabled' });
    await row.getByRole('button', { name: '删除', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '删除四层代理', exact: true });
    await expect(dialog).toContainText(name);
    await dialog.getByRole('button', { name: '确认', exact: true }).click();
    await expect(row).not.toBeVisible();
  } finally {
    await request.post('/api/runtime/stop');
  }
});

async function switchTo(page: Page, label: string, checked = true) {
  const input = page.getByLabel(label, { exact: true });
  if ((await input.isChecked()) !== checked) await input.locator('..').click();
}
async function mutate(
  request: APIRequestContext,
  method: 'post' | 'put',
  path: string,
  data: unknown,
) {
  const current = await state(request);
  const result = await request[method]('/api' + path, {
    headers: { 'If-Match': String(current.state.revision) },
    data,
  });
  expect(result.ok(), await result.text()).toBeTruthy();
  return result.json();
}

test('website domains, run directory, ordered documents, CORS and trusted real IP settings', async ({
  page,
  request,
}) => {
  const id = suffix(),
    domain = `settings-${id}.test`,
    alias = `alias-${id}.test`;
  const site = await seedSite(request, `网站设置 ${id}`, domain);
  await mutate(request, 'put', `/sites/${site.id}/file`, {
    path: 'public/home.html',
    content: '<h1>Home</h1>',
  });
  await page.goto(`/#sites?site=${site.id}`);
  await expect(page.getByRole('tab', { name: '域名设置', exact: true })).toHaveAttribute(
    'aria-selected',
    'true',
  );
  await expect(page.getByRole('button', { name: '删除', exact: true })).toBeDisabled();
  await page.getByRole('button', { name: '添加域名', exact: true }).click();
  await page.getByLabel('域名 / IP', { exact: true }).fill(alias);
  await page.getByLabel('端口', { exact: true }).fill('29092');
  await drawer(page).getByRole('button', { name: '确认', exact: true }).click();
  await expect(drawer(page)).not.toBeVisible();
  await expect(page.getByRole('row').filter({ hasText: alias })).toContainText('29092');
  await page.screenshot({
    path: '../docs/website-domains.png',
    fullPage: true,
    animations: 'disabled',
  });
  await page.getByRole('tab', { name: '网站目录', exact: true }).click();
  await choose(page, '运行目录', '/public');
  await saveSetting(page, '保存并重载');
  await expect
    .poll(
      async () =>
        (await state(request)).state.sites.find((s: { id: string }) => s.id === site.id)
          .runDirectory,
    )
    .toBe('/public');
  await page.screenshot({
    path: '../docs/website-directory.png',
    fullPage: true,
    animations: 'disabled',
  });
  await page.getByRole('tab', { name: '默认文档', exact: true }).click();
  await page.getByRole('textbox', { name: '默认文档', exact: true }).fill('home.html\nindex.html');
  await saveSetting(page);
  await expect
    .poll(
      async () =>
        (await state(request)).state.sites.find((s: { id: string }) => s.id === site.id).index,
    )
    .toEqual(['home.html', 'index.html']);
  await page.getByRole('tab', { name: '真实 IP', exact: true }).click();
  await switchTo(page, '启用真实 IP');
  await page.getByLabel('信任来源', { exact: true }).fill('192.0.2.0/24\n2001:db8::/32');
  await choose(page, 'IP Header', 'CF-Connecting-IP');
  await saveSetting(page);
  await expect
    .poll(
      async () =>
        (await state(request)).state.sites.find((s: { id: string }) => s.id === site.id).realIp
          .header,
    )
    .toBe('CF-Connecting-IP');
  await page.getByRole('tab', { name: '跨域设置', exact: true }).click();
  await switchTo(page, '启用跨域');
  await page.getByLabel('允许的来源', { exact: true }).fill('https://client.example.test');
  await page.getByLabel('允许的请求头', { exact: true }).fill('Content-Type,Authorization');
  await switchTo(page, '允许携带凭据');
  await saveSetting(page);
  await expect
    .poll(
      async () =>
        (await state(request)).state.sites.find((s: { id: string }) => s.id === site.id).cors
          .allowCredentials,
    )
    .toBe(true);
  await page.getByLabel('允许的来源', { exact: true }).fill('https://local-edit.example.test');
  await mutate(request, 'put', `/sites/${site.id}/index`, { index: ['home.html'] });
  await page.getByRole('button', { name: '保存', exact: true }).click();
  await expect(page.locator('.el-message--error')).toContainText('配置版本已变化');
  await expect(page.getByLabel('允许的来源', { exact: true })).toHaveValue(
    'https://local-edit.example.test',
  );
  const saved = (await state(request)).state.sites.find((s: { id: string }) => s.id === site.id);
  expect(saved.cors.allowOrigins).toBe('https://client.example.test');
  expect(saved.realIp.from).toEqual(['192.0.2.0/24', '2001:db8::/32']);
  expect(saved.bindings).toHaveLength(2);
  await page.getByRole('button', { name: '重置', exact: true }).click();
  await expect(page.getByLabel('允许的来源', { exact: true })).toHaveValue(
    'https://client.example.test',
  );
});

test('website reverse proxy rules, cache options, replacements, preview and status confirmations', async ({
  page,
  request,
}) => {
  const id = suffix(),
    name = `api_${id}`;
  const site = await seedSite(request, `代理设置 ${id}`, `proxy-settings-${id}.test`);
  await page.goto(`/#sites?site=${site.id}&tab=proxy`);
  await page.getByRole('button', { name: '创建反向代理', exact: true }).click();
  await page.getByLabel('名称', { exact: true }).fill(name);
  await page.getByLabel('代理目录', { exact: true }).fill('/api/');
  await page.getByLabel('代理地址', { exact: true }).fill('http://127.0.0.1:9001/backend/');
  await drawer(page).getByRole('tab', { name: '高级设置', exact: true }).click();
  await switchTo(page, '服务端缓存');
  await drawer(page).getByRole('radio', { name: '启用', exact: true }).locator('..').click();
  await switchTo(page, '启用跨域');
  await page.getByRole('button', { name: '添加替换', exact: true }).click();
  await page.getByLabel('替换原文 1', { exact: true }).fill('BEFORE');
  await page.getByLabel('替换内容 1', { exact: true }).fill('AFTER');
  await drawer(page).getByRole('button', { name: '确认', exact: true }).click();
  await expect(drawer(page)).not.toBeVisible();
  const row = page.getByRole('row').filter({ hasText: name });
  await expect(row).toContainText('/api/');
  const snapshot = (await state(request)).state.sites.find((s: { id: string }) => s.id === site.id);
  expect(snapshot.proxies[0].cache).toBe(true);
  expect(snapshot.proxies[0].cacheTime).toBe(4);
  expect(snapshot.proxies[0].cors.preflight).toBe(true);
  expect(snapshot.proxies[0].replaces).toEqual([{ from: 'BEFORE', to: 'AFTER' }]);
  await row.getByRole('button', { name: '配置预览', exact: true }).click();
  await expect(page.getByRole('textbox', { name: '反向代理配置', exact: true })).toContainText(
    'proxy_pass',
  );
  await expect(page.getByRole('textbox', { name: '反向代理配置', exact: true })).toHaveAttribute(
    'aria-readonly',
    'true',
  );
  await closeDrawer(page);
  await row.getByRole('button', { name: '已启用', exact: true }).click();
  const stop = page.getByRole('dialog', { name: '停止反向代理', exact: true });
  await expect(stop).toContainText(name);
  await stop.getByRole('button', { name: '确认', exact: true }).click();
  await expect(row.getByRole('button', { name: '已停止', exact: true })).toBeVisible();
  await expect(row.getByRole('button', { name: '编辑', exact: true })).toBeDisabled();
  await page.getByRole('button', { name: '缓存设置', exact: true }).click();
  await page.getByLabel('共享内存（MiB）', { exact: true }).fill('8');
  await drawer(page).getByRole('button', { name: '保存', exact: true }).click();
  await expect(drawer(page)).not.toBeVisible();
  await page.screenshot({
    path: '../docs/website-proxies.png',
    fullPage: true,
    animations: 'disabled',
  });
  await row.getByRole('button', { name: '删除', exact: true }).click();
  await page
    .getByRole('dialog', { name: '删除反向代理', exact: true })
    .getByRole('button', { name: '确认', exact: true })
    .click();
  await expect(row).not.toBeVisible();
});

test('website domain, path and 404 redirects follow 1Panel keep-path and confirmation flow', async ({
  page,
  request,
}) => {
  const id = suffix(),
    domain = `redirect-${id}.test`,
    alias = `alias-redirect-${id}.test`;
  const site = await seedSite(request, `重定向设置 ${id}`, domain);
  await mutate(request, 'put', `/sites/${site.id}/domains`, {
    bindings: [
      { domain, port: 80, ssl: false },
      { domain: alias, port: 80, ssl: false },
    ],
  });
  await page.goto(`/#sites?site=${site.id}&tab=redirect`);
  await page.getByRole('button', { name: '创建重定向', exact: true }).click();
  await page.getByLabel('名称', { exact: true }).fill('alias_redirect');
  await choose(page, '来源域名', alias);
  await page.getByLabel('目标 URL', { exact: true }).fill('https://destination.test');
  await drawer(page).getByRole('button', { name: '确认', exact: true }).click();
  await expect(drawer(page)).not.toBeVisible();
  await page.getByRole('button', { name: '创建重定向', exact: true }).click();
  await choose(page, '类型', '路径');
  await page.getByLabel('名称', { exact: true }).fill('old_path');
  await page.getByLabel('来源路径', { exact: true }).fill('/old/');
  await page.getByLabel('目标 URL', { exact: true }).fill('https://destination.test/new/');
  await choose(page, '重定向方式', '302 临时重定向');
  await drawer(page).getByRole('button', { name: '确认', exact: true }).click();
  await expect(drawer(page)).not.toBeVisible();
  await page.getByRole('button', { name: '创建重定向', exact: true }).click();
  await choose(page, '类型', '404');
  await switchTo(page, '重定向到首页');
  await expect(page.getByLabel('目标 URL', { exact: true })).not.toBeVisible();
  await drawer(page).getByRole('button', { name: '确认', exact: true }).click();
  await expect(drawer(page)).not.toBeVisible();
  await page.screenshot({
    path: '../docs/website-redirects.png',
    fullPage: true,
    animations: 'disabled',
  });
  const rules = (await state(request)).state.sites.find(
    (s: { id: string }) => s.id === site.id,
  ).redirects;
  expect(rules).toHaveLength(3);
  expect(rules.find((r: { type: string }) => r.type === 'path').keepPath).toBe(true);
  expect(rules.find((r: { type: string }) => r.type === '404').redirectRoot).toBe(true);
  const row = page.getByRole('row').filter({ hasText: 'old_path' });
  await row.getByRole('button', { name: '配置预览', exact: true }).click();
  await expect(page.getByRole('textbox', { name: '重定向配置', exact: true })).toContainText('$1');
  await closeDrawer(page);
  await row.getByRole('button', { name: '删除', exact: true }).click();
  await page
    .getByRole('dialog', { name: '删除重定向', exact: true })
    .getByRole('button', { name: '确认', exact: true })
    .click();
  await expect(row).not.toBeVisible();
});

test('official HTTPS modes, HSTS, HTTP3 and disable confirmation preserve SSL records', async ({
  page,
  request,
}) => {
  const id = suffix(),
    domain = `https-settings-${id}.test`;
  const site = await seedSite(request, `HTTPS 设置 ${id}`, domain);
  const caName = `HTTPSRoot${id}`;
  await createOfficialCA(page, caName, '/#/websites/ssl/ca');
  const snapshot = await state(request);
  const ca = snapshot.state.ssl.cas.find((c: { name: string }) => c.name === caName);
  const response = await request.post('/api/panel/websites/ca/obtain', {
    data: { id: ca.id, domains: domain, keyType: 'EC256', time: 30, unit: 'day' },
  });
  expect(response.ok()).toBeTruthy();
  const ssl = (await response.json()).id;
  await page.goto(`/#sites?site=${site.id}&tab=tls`);
  await switchTo(page, '启用 HTTPS');
  await choose(page, 'HTTP 选项', 'HTTP可直接访问');
  const field = (label: string) =>
    page.locator('.el-form-item').filter({
      has: page.locator('.el-form-item__label').filter({ hasText: new RegExp(`^${label}$`) }),
    });
  if (!(await field('HSTS').getByRole('checkbox').isChecked()))
    await field('HSTS').locator('.el-checkbox').click();
  await field('HSTS 子域').locator('.el-checkbox').click();
  await field('HTTP3').locator('.el-checkbox').click();
  await saveSetting(page);
  await expect
    .poll(
      async () =>
        (await state(request)).state.sites.find((s: { id: string }) => s.id === site.id).https
          .httpConfig,
    )
    .toBe('HTTPAlso');
  await page.screenshot({
    path: '../docs/website-https.png',
    fullPage: true,
    animations: 'disabled',
  });
  await choose(page, 'HTTP 选项', '禁止 HTTP');
  await saveSetting(page);
  await expect
    .poll(
      async () =>
        (await state(request)).state.sites.find((s: { id: string }) => s.id === site.id).https
          .httpConfig,
    )
    .toBe('HTTPSOnly');
  await switchTo(page, '启用 HTTPS', false);
  const dialog = page.getByRole('dialog', { name: '禁用 HTTPS', exact: true });
  await expect(dialog).toBeVisible();
  await dialog.getByRole('button', { name: '确认', exact: true }).click();
  await expect
    .poll(
      async () =>
        (await state(request)).state.sites.find((s: { id: string }) => s.id === site.id).tlsId,
    )
    .toBe('');
  expect((await state(request)).state.ssl.records.some((c: { id: number }) => c.id === ssl)).toBe(
    true,
  );
});
