import { test, expect } from './runtime-fixture';
import type { APIRequestContext, Page } from '@playwright/test';
const overview = async (request: APIRequestContext) => (await request.get('/api/overview')).json();
async function choose(page: Page, label: string, option: string) {
  await page.getByRole('combobox', { name: new RegExp(label + '$') }).press('ArrowDown');
  await page.getByRole('option', { name: option, exact: true }).click();
}
async function code(page: Page, label: string, value: string) {
  await page.getByRole('textbox', { name: label, exact: true }).fill(value);
}

test('website and mTLS navigation selection follows route history and aliases', async ({
  page,
}) => {
  await page.goto('/#/sites');
  await page.getByRole('radio', { name: '四层代理', exact: true }).locator('..').click();
  await expect(page).toHaveURL(/\/streams$/);
  await expect(page.getByRole('radio', { name: '四层代理', exact: true })).toBeChecked();
  await page.goBack();
  await expect(page.getByRole('radio', { name: '网站', exact: true })).toBeChecked();
  await page.locator('.app-sidebar').getByRole('menuitem', { name: 'mTLS', exact: true }).click();
  const active = async (label: string) => {
    await expect(page.getByRole('radio', { name: label, exact: true })).toBeChecked();
    await expect(page.locator('.router_card input:checked')).toHaveCount(1);
  };
  await active('CA 管理');
  await page.getByRole('radio', { name: '客户端证书', exact: true }).locator('..').click();
  await active('客户端证书');
  await expect(page.getByRole('radio', { name: 'CA 管理', exact: true })).not.toBeChecked();
  await page.getByRole('radio', { name: '网站绑定', exact: true }).locator('..').click();
  await active('网站绑定');
  await page.goBack();
  await active('客户端证书');
  await page.reload();
  await active('客户端证书');
  await page.goto('/#/mtls/ca');
  await active('CA 管理');
});

test('original OpenResty form saves actual values, reloads and rejects stale drafts', async ({
  page,
  request,
}) => {
  await page.goto('/#/runtime');
  await page.getByRole('button', { name: '性能调节', exact: true }).click();
  await expect(page.getByRole('button', { name: '保存', exact: true })).toBeEnabled();
  const keepalive = page.getByRole('textbox', { name: /keepalive_timeout$/ });
  await expect(keepalive).toHaveValue('75s');
  await keepalive.fill('1s1m');
  await page.getByRole('button', { name: '保存', exact: true }).click();
  await expect(page.locator('.el-form-item__error')).toBeVisible();
  await keepalive.fill('1m30s 90');
  await page.getByRole('spinbutton', { name: /client_max_body_size$/ }).fill('32');
  await choose(page, 'gzip', 'on');
  await page.getByRole('textbox', { name: /gzip_comp_level$/ }).fill('3');
  await page.getByRole('button', { name: '保存', exact: true }).click();
  await expect
    .poll(async () => (await overview(request)).state.openresty?.http.keepalive_timeout)
    .toBe('1m30s 90');
  await expect(keepalive).toHaveValue('1m30s 90');
  const generated = await (await request.get('/api/config')).json();
  expect(generated.config).toContain('client_max_body_size 32m;');
  expect(generated.config).toContain('gzip on;');
  const previous = await overview(request);
  const response = await request.post('/api/panel/openresty/update', {
    headers: { 'If-Match': String(previous.state.revision) },
    data: { scope: 'http-per', operate: 'update', params: { keepalive_timeout: '2m' } },
  });
  expect(response.ok()).toBeTruthy();
  await page.getByRole('button', { name: '刷新', exact: true }).click();
  await page.getByRole('textbox', { name: /gzip_comp_level$/ }).fill('4');
  await page.getByRole('button', { name: '保存', exact: true }).click();
  await expect(page.locator('.el-message--error')).toContainText('配置版本已变化');
  expect((await overview(request)).state.openresty.http.keepalive_timeout).toBe('2m');
  await page.getByRole('button', { name: '刷新内容', exact: true }).click();
  await expect(keepalive).toHaveValue('2m');
  // Restore the fixture defaults so this suite may run against the same isolated backend again.
  const current = await overview(request);
  await request.post('/api/panel/openresty/update', {
    headers: { 'If-Match': String(current.state.revision) },
    data: { scope: 'http-per', operate: 'update', params: { keepalive_timeout: '75s' } },
  });
});

test('global certificate error pages and website overrides round trip', async ({
  page,
  request,
}) => {
  const value = `<h1>Missing client ${Date.now()}</h1>`;
  await page.goto('/#/pages');
  await choose(page, '选择错误页面', '496 - 未提供客户端证书');
  await code(page, '错误页面 HTML', value);
  await page.getByRole('button', { name: '保存', exact: true }).click();
  await expect
    .poll(async () => (await (await request.get('/api/pages')).json()).pages['496'])
    .toBe(value);
  const snapshot = await overview(request),
    name = `错误页${Date.now()}`;
  const response = await request.post('/api/sites', {
    headers: { 'If-Match': String(snapshot.state.revision) },
    data: { name, domains: [`errors-${Date.now()}.test`], kind: 'static', enabled: true },
  });
  expect(response.ok()).toBeTruthy();
  const site = await response.json();
  await page.goto(`/#/sites?site=${site.id}&tab=pages`);
  await page.reload();
  await choose(page, '错误页面', '495 - 客户端证书校验失败');
  await page.getByRole('switch', { name: '覆盖全局页面', exact: true }).locator('..').click();
  await code(page, '网站错误页面 HTML', '<h1>Site invalid client</h1>');
  await page.getByRole('button', { name: '保存', exact: true }).click();
  await expect
    .poll(
      async () => (await (await request.get(`/api/sites/${site.id}/pages`)).json()).pages['495'],
    )
    .toBe('<h1>Site invalid client</h1>');
  expect((await (await request.get('/api/pages')).json()).pages['496']).toBe(value);
});
