import { test, expect } from './runtime-fixture';

test('website configuration editor maps GUI state and rejects unsupported directives', async ({
  page,
  request,
}, testInfo) => {
  const overview = await (await request.get('/api/overview')).json();
  const created = await request.post('/api/sites', {
    headers: { 'If-Match': String(overview.state.revision) },
    data: {
      name: '配置文件回归',
      domains: ['config-editor.example.test'],
      kind: 'static',
      enabled: true,
    },
  });
  expect(created.ok()).toBeTruthy();
  const site = await created.json();
  await page.goto(`/#/sites?site=${site.id}&tab=config`);
  const editor = page.getByRole('textbox', { name: '网站 Nginx 配置', exact: true });
  await expect(editor).toContainText('server_name config-editor.example.test');
  const initial = await editor.innerText();
  const good = initial
    .replace('server {', 'server {\n add_header X-Editor tested always;')
    .replace('disable_symlinks on;', 'disable_symlinks off;')
    .replace('  location ~ /\\. { deny all; }\n', '');
  await editor.fill(good);
  let saved = page.waitForResponse(
    (r) => r.url().endsWith(`/api/sites/${site.id}/config`) && r.request().method() === 'PUT',
  );
  await page.getByRole('button', { name: '保存并重载', exact: true }).click();
  expect((await saved).status()).toBe(200);
  await expect(editor).toContainText('add_header X-Editor "tested" always;');
  await page.getByRole('tab', { name: '响应头与安全', exact: true }).click();
  await expect(page.getByRole('textbox', { name: '响应头名称', exact: true })).toHaveValue(
    'X-Editor',
  );
  await expect(page.getByRole('switch', { name: '允许跟随符号链接', exact: true })).toBeChecked();
  await page.getByRole('textbox', { name: '响应头值', exact: true }).fill('gui-edited');
  saved = page.waitForResponse(
    (r) => r.url().endsWith(`/api/sites/${site.id}/advanced`) && r.request().method() === 'PUT',
  );
  await page.getByRole('button', { name: '保存', exact: true }).click();
  expect((await saved).status()).toBe(200);
  await expect(page.getByRole('button', { name: '保存', exact: true })).not.toHaveClass(
    /is-loading/,
  );
  await page.getByRole('tab', { name: '配置文件', exact: true }).click();
  await expect(editor).toContainText('"gui-edited"');
  const valid = await editor.innerText();
  const bad = valid.replace('server {', 'server {\n add_trailer X-Unsupported value always;');
  await editor.fill(bad);
  saved = page.waitForResponse(
    (r) => r.url().endsWith(`/api/sites/${site.id}/config`) && r.request().method() === 'PUT',
  );
  await page.getByRole('button', { name: '保存并重载', exact: true }).click();
  expect((await saved).status()).toBe(400);
  await expect(editor).toContainText('add_trailer X-Unsupported');
  const doc = await (await request.get(`/api/sites/${site.id}/config`)).json();
  expect(doc.content).not.toContain('add_trailer');
  await page.reload();
  await expect(editor).toContainText('"gui-edited"');
  await expect(editor).not.toContainText('add_trailer');
  await page.screenshot({ path: testInfo.outputPath('config-desktop.png'), fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole('tab', { name: '响应头与安全', exact: true }).click();
  const mobileValue = page.getByRole('textbox', { name: '响应头值', exact: true });
  await mobileValue.scrollIntoViewIfNeeded();
  await expect(mobileValue).toBeInViewport();
  await page.screenshot({ path: testInfo.outputPath('headers-mobile.png'), fullPage: true });
  await mobileValue.fill('mobile-edited');
  saved = page.waitForResponse(
    (r) => r.url().endsWith(`/api/sites/${site.id}/advanced`) && r.request().method() === 'PUT',
  );
  await page.getByRole('button', { name: '保存', exact: true }).click();
  expect((await saved).status()).toBe(200);
  await expect(page.getByRole('button', { name: '保存', exact: true })).not.toHaveClass(
    /is-loading/,
  );
  const mobileDoc = await (await request.get(`/api/sites/${site.id}/config`)).json();
  expect(mobileDoc.content).toContain('"mobile-edited"');
  await page.getByRole('tab', { name: '配置文件', exact: true }).click();
  await expect(editor).toContainText('"mobile-edited"');
  await editor.scrollIntoViewIfNeeded();
  await expect(editor).toBeInViewport();
});
