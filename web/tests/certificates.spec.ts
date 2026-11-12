import { type Page } from '@playwright/test';
import { test, expect } from './runtime-fixture';
const drawer = (page: Page) => page.locator('.el-drawer[role="dialog"]:visible');
const choose = async (page: Page, label: string, option: string) => {
  await drawer(page)
    .getByRole('combobox', { name: new RegExp(label + '$') })
    .press('ArrowDown');
  await page.getByRole('option', { name: option, exact: true }).click();
};
test('original upload, details, download, DNS credentials and ACME form', async ({
  page,
  request,
}) => {
  const id = Date.now().toString(36),
    domain = `upload-${id}.test`,
    description = `Imported${id}`;
  const errors: string[] = [];
  page.on('pageerror', (e) => errors.push(e.message));
  let response = await request.post('/api/panel/websites/ca', {
    data: {
      name: `UploadCA${id}`,
      commonName: `UploadCA${id}`,
      country: 'CN',
      organization: 'Example',
      keyType: 'EC256',
    },
  });
  expect(response.ok()).toBeTruthy();
  const snapshot = await (await request.get('/api/overview')).json();
  const ca = snapshot.state.ssl.cas.find((r: { name: string }) => r.name === `UploadCA${id}`);
  response = await request.post('/api/panel/websites/ca/obtain', {
    data: { id: ca.id, domains: domain, keyType: 'EC256', time: 30, unit: 'day' },
  });
  expect(response.ok()).toBeTruthy();
  const sslID = (await response.json()).id;
  const certificate = await (await request.get(`/api/panel/websites/ssl/${sslID}`)).json();
  await page.goto('/#/websites/ssl');
  await page.getByRole('button', { name: '上传证书', exact: true }).click();
  await drawer(page).getByLabel('证书(PEM格式)', { exact: true }).fill(certificate.pem);
  await drawer(page).getByLabel('私钥(KEY)', { exact: true }).fill(certificate.privateKey);
  await drawer(page).getByLabel('备注', { exact: true }).fill(description);
  await drawer(page).getByRole('button', { name: '确认', exact: true }).click();
  await expect(drawer(page)).not.toBeVisible();
  const row = page.getByRole('row').filter({ hasText: description });
  await expect(row).toContainText('手动创建');
  await row.getByRole('button', { name: '详情', exact: true }).click();
  await expect(drawer(page)).toContainText(domain);
  await drawer(page).getByRole('radio', { name: '私钥', exact: true }).locator('..').click();
  await expect(drawer(page).locator('textarea')).toHaveValue(certificate.privateKey);
  await drawer(page)
    .getByRole('button', { name: '取消', exact: true })
    .count()
    .then(async (n) => {
      if (n) await drawer(page).getByRole('button', { name: '取消', exact: true }).click();
      else await drawer(page).getByRole('button', { name: /关闭/ }).click();
    });
  const download = page.waitForEvent('download');
  await row.getByRole('button', { name: '下载', exact: true }).click();
  expect((await download).suggestedFilename()).toBe(`${domain}.zip`);
  await page.getByRole('button', { name: 'DNS 账户', exact: true }).click();
  await expect(page).toHaveURL(/dns-account/);
  await page.getByRole('button', { name: '创建', exact: true }).click();
  await drawer(page).getByLabel('名称', { exact: true }).fill(`Cloudflare${id}`);
  await choose(page, '类型', 'Cloudflare');
  await drawer(page)
    .getByLabel('API Token', { exact: true })
    .fill('fixture-token-no-external-DNS-writes');
  await drawer(page).getByRole('button', { name: '确认', exact: true }).click();
  await expect(drawer(page)).not.toBeVisible();
  await expect(page.getByRole('row').filter({ hasText: `Cloudflare${id}` })).toBeVisible();
  await page.goto('/#/websites/ssl/acme-account');
  await page.getByRole('button', { name: '创建', exact: true }).click();
  await choose(page, '账号类型', '自定义 ACME 服务');
  await expect(drawer(page).getByRole('textbox', { name: /ACME 服务 URL/ })).toBeVisible();
  await page.screenshot({
    path: '../docs/ssl-acme-account.png',
    fullPage: true,
    animations: 'disabled',
  });
  await drawer(page).getByRole('button', { name: '取消', exact: true }).click();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/#/websites/ssl');
  await page.getByRole('button', { name: '上传证书', exact: true }).click();
  await expect(drawer(page)).toHaveCSS('width', '390px');
  expect(errors).toEqual([]);
});
