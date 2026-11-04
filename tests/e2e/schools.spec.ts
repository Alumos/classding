import { expect, request, test, type Page } from '@playwright/test';

const adminURL = 'http://127.0.0.1:15174/admin/';
const apiURL = 'http://127.0.0.1:18080';

async function adminLogin(page: Page) {
  await page.goto(adminURL);
  await page.getByLabel('管理员账号').fill('admin');
  await page.getByLabel('密码', { exact: true }).fill('AdminDing2026!');
  await page.getByRole('button', { name: '登录管理后台' }).click();
  await expect(page.getByRole('heading', { name: '工作概览', exact: true })).toBeVisible();
}

async function choose(page: Page, label: string, option: string) {
  await page.getByRole('combobox', { name: label }).click();
  await page.getByRole('option', { name: option, exact: true }).click();
}

test('运营管理员建立学校，教师兑换该校激活码后自动加入学校', async ({ page }, testInfo) => {
  const suffix = `${testInfo.project.name}-${Date.now()}`;
  const school = `验收学校${suffix}`;
  await adminLogin(page);
  await page.getByRole('link', { name: '学校管理' }).filter({ visible: true }).first().click();
  await page.getByRole('button', { name: '新建学校' }).click();
  await page.getByRole('dialog').getByLabel('学校名称').fill(school);
  await page.getByRole('dialog').getByRole('button', { name: '创建学校' }).click();
  await expect(page.getByRole('cell', { name: new RegExp(school) })).toBeVisible();

  await page.getByRole('link', { name: '激活码管理' }).filter({ visible: true }).first().click();
  await page.getByRole('button', { name: '生成激活码' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('生成数量').fill('1');
  await choose(page, '所属学校', school);
  await dialog.getByLabel('备注').fill(`学校验收-${suffix}`);
  await dialog.getByRole('button', { name: '生成激活码', exact: true }).click();
  await page.getByPlaceholder('搜索激活码、备注或批次').fill(`学校验收-${suffix}`);
  const row = page.locator('tbody tr');
  await expect(row).toHaveCount(1);
  await expect(row).toContainText(school);
  const code = (await row.locator('.font-mono').innerText()).trim();

  // A teacher who registered on their own belongs to no school until they redeem the code.
  const teacher = await request.newContext({ baseURL: apiURL });
  const username = `school.${testInfo.project.name}.${Date.now()}`.slice(0, 32);
  const registered = await teacher.post('/api/v1/auth/register', {
    data: { username, password: 'SchoolTeacher2026', display_name: '验收老师' },
    headers: { 'X-ClassDing-Auth-Scope': 'teacher' },
  });
  expect(registered.ok()).toBe(true);
  const { access_token } = await registered.json();
  const redeemed = await teacher.post('/api/v1/classes', {
    data: { name: '验收班级', activation_code: code },
    headers: { Authorization: `Bearer ${access_token}` },
  });
  expect(redeemed.ok()).toBe(true);
  await teacher.dispose();

  await page.getByRole('link', { name: '教师管理' }).filter({ visible: true }).first().click();
  await page.getByPlaceholder('搜索姓名、账号或学校').fill(username);
  await expect(page.locator('tbody tr')).toContainText(school);
});

test('批量删除未使用的激活码，使用中的激活码不可选', async ({ page }, testInfo) => {
  const note = `批量删除-${testInfo.project.name}-${Date.now()}`;
  await adminLogin(page);
  await page.getByRole('link', { name: '激活码管理' }).filter({ visible: true }).first().click();
  await page.getByRole('button', { name: '生成激活码' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('生成数量').fill('3');
  await dialog.getByLabel('备注').fill(note);
  await dialog.getByRole('button', { name: '生成激活码', exact: true }).click();
  await expect(page.getByText('成功生成 3 个激活码')).toBeVisible();
  await page.getByPlaceholder('搜索激活码、备注或批次').fill(note);
  await expect(page.locator('tbody tr')).toHaveCount(3);
  await page.getByRole('checkbox', { name: '选择本页可删除的激活码' }).click();
  await page.getByRole('button', { name: '删除所选（3）' }).click();
  await page.getByRole('alertdialog').getByRole('button', { name: '确认删除' }).click();
  await expect(page.getByText('已删除 3 个激活码')).toBeVisible();
  await expect(page.getByText('没有找到激活码')).toBeVisible();

  await page.getByPlaceholder('搜索激活码、备注或批次').fill('DEMO-A234');
  await expect(page.getByRole('checkbox', { name: '选择激活码 DEMO-A234' })).toBeDisabled();
});
