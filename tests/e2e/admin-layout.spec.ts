import { expect, test, type Page } from '@playwright/test';

const adminURL = process.env.CLASSDING_ADMIN_TEST_URL ?? 'http://127.0.0.1:15174/admin/';
const settings = { site_name: '课堂叮咚', icp_number: '京ICP备12345678号-1', theme: 'green' };
const administrator = {
  id: 'admin-1',
  username: 'admin',
  display_name: '管理员',
  role: 'super_admin',
  status: 'active',
};

async function mockAdmin(page: Page, siteName = settings.site_name, icpNumber = settings.icp_number, uiScale = 100) {
  const siteSettings = { ...settings, site_name: siteName, icp_number: icpNumber };
  const responses: Record<string, unknown> = {
    '/api/v1/config': siteSettings,
    '/api/v1/auth/refresh': { access_token: 'test-token', user: { ...administrator, ui_scale: uiScale } },
    '/api/v1/admin/settings': siteSettings,
    '/api/v1/admin/schools': [],
    '/api/v1/admin/classes': [],
    '/api/v1/admin/users': [
      {
        id: 'teacher-1',
        username: 'teacher',
        display_name: '林老师',
        role: 'user',
        status: 'active',
        org_id: 'school-1',
      },
    ],
    '/api/v1/admin/overview': {
      online_devices: 1,
      total_devices: 2,
      today_messages: 3,
      delivery_rate: 0.9,
      avg_delivery_ms: 300,
      total_users: 1,
      total_classes: 1,
      recent_messages: [],
      activity: [
        { date: '2026-11-01', count: 2 },
        { date: '2026-11-02', count: 5 },
      ],
    },
  };
  await page.route('**/api/v1/**', (route) => {
    const path = new URL(route.request().url()).pathname;
    return route.fulfill(
      path in responses ? { json: responses[path] } : { status: 404, json: { message: '未模拟的请求' } },
    );
  });
}

test('管理员成功提示显示时窄屏导航仍可点击', async ({ page }) => {
  for (const width of [320, 390]) {
    for (const scale of [100, 150]) {
      await mockAdmin(page, settings.site_name, settings.icp_number, scale);
      await page.route('**/api/v1/admin/schools', (route) =>
        route.fulfill({
          json: route.request().method() === 'POST' ? { id: 'school-toast', name: '提示验收学校' } : [],
        }),
      );
      await page.setViewportSize({ width, height: 844 });
      await page.goto(adminURL + 'schools');
      await page.getByRole('button', { name: '新建学校' }).click();
      const dialog = page.getByRole('dialog');
      await dialog.getByLabel('学校名称').fill('提示验收学校');
      await dialog.getByRole('button', { name: '创建学校' }).click();
      await expect(dialog).toBeHidden();
      const toast = page.locator('[data-sonner-toast]').filter({ hasText: '学校已创建' });
      await expect(toast).toBeVisible();
      // Hold the real toast open instead of waiting for it to expire or forcing the click.
      await toast.hover();
      const navigation = page.getByRole('navigation', { name: '管理后台导航' });
      const toastBox = await toast.boundingBox();
      const navigationBox = await navigation.boundingBox();
      expect(toastBox!.y).toBeGreaterThan(navigationBox!.y + navigationBox!.height);
      await navigation.getByRole('link', { name: '班级管理' }).click({ timeout: 2000 });
      await expect(page.getByRole('heading', { name: '班级管理', exact: true })).toBeVisible();
      await expect(toast).toBeVisible();
      await noOverflow(page);
    }
  }
});

async function noOverflow(page: Page) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
}

test('管理员登录后窄屏当前导航和恢复弹窗保持可见', async ({ page }) => {
  await mockAdmin(page);
  for (const width of [320, 390]) {
    await page.setViewportSize({ width, height: 844 });
    await page.goto(`${adminURL}settings`);
    await expect(page.getByRole('heading', { name: '系统配置', exact: true })).toBeVisible();
    const active = page.locator('.mobile-nav [aria-current="page"]');
    await expect(active).toBeInViewport();
    await page.getByRole('button', { name: '导入并恢复', exact: true }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: '确认覆盖恢复全站数据？' })).toBeVisible();
    for (const label of ['取消', '确认覆盖恢复'])
      await expect(dialog.getByRole('button', { name: label, exact: true })).toBeInViewport();
    await dialog.getByRole('button', { name: '取消', exact: true }).click();
    await noOverflow(page);
  }
});

test('管理员登录后窄屏表格可用键盘横向查看', async ({ page }) => {
  await mockAdmin(page);
  await page.setViewportSize({ width: 320, height: 844 });
  await page.goto(`${adminURL}users`);
  const table = page.getByRole('region', { name: '数据表格' });
  await expect(table).toBeVisible();
  await expect(page.getByText('左右滑动查看更多')).toBeVisible();
  await table.focus();
  for (let i = 0; i < 8; i++) await page.keyboard.press('ArrowRight');
  await expect.poll(() => table.evaluate((element) => element.scrollLeft)).toBeGreaterThan(0);
  await page.setViewportSize({ width: 1440, height: 844 });
  await expect(page.getByText('左右滑动查看更多')).toHaveCount(0);
});

test('管理员登录后桌面切换页面时侧栏固定于视口，内容宽度保持一致', async ({ page }) => {
  const siteName = '校园课堂通知管理服务平台'.repeat(6).slice(0, 60);
  await mockAdmin(page, siteName);
  for (const height of [450, 720]) {
    await page.setViewportSize({ width: 1440, height });
    for (const colorScheme of ['light', 'dark'] as const) {
      await page.emulateMedia({ colorScheme });
      await page.goto(adminURL);
      await expect(page.getByRole('heading', { name: '工作概览', exact: true })).toBeVisible();
      await expect(page.locator('.app-footer')).toContainText(siteName);
      const measure = () =>
        page.evaluate(() => {
          const sidebar = document.querySelector('.sidebar')!.getBoundingClientRect();
          const main = document.querySelector('.main-content')!.getBoundingClientRect();
          const title = document.querySelector('.sidebar-brand')!.getBoundingClientRect();
          return {
            sidebarTop: sidebar.top,
            sidebarHeight: sidebar.height,
            viewportHeight: innerHeight,
            mainLeft: main.left,
            mainWidth: main.width,
            titleHeight: title.height,
          };
        });
      await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight));
      const longPage = await measure();
      expect(Math.abs(longPage.sidebarTop)).toBeLessThanOrEqual(1);
      expect(Math.abs(longPage.sidebarHeight - longPage.viewportHeight)).toBeLessThanOrEqual(1);
      expect(longPage.titleHeight).toBeLessThan(64);
      const logout = page.locator('.sidebar').getByRole('button', { name: '退出登录' });
      await logout.scrollIntoViewIfNeeded();
      await expect(logout).toBeInViewport();
      await page.locator('.side-nav').getByRole('link', { name: '教师管理' }).click();
      await expect(page.getByRole('heading', { name: '教师管理', exact: true })).toBeVisible();
      const shortPage = await measure();
      expect(shortPage.sidebarHeight).toBeCloseTo(longPage.sidebarHeight, 0);
      expect(shortPage.mainLeft).toBeCloseTo(longPage.mainLeft, 0);
      expect(shortPage.mainWidth).toBeCloseTo(longPage.mainWidth, 0);
      await noOverflow(page);
    }
  }
});

test('登录页与管理页的备案号位于页脚中央，长编号可换行', async ({ page }) => {
  const icpNumber = `京ICP备${'1234567890'.repeat(12)}号`;
  await mockAdmin(page, settings.site_name, icpNumber);
  await page.route('**/api/v1/auth/refresh', (route) =>
    route.fulfill({ status: 401, json: { code: 'UNAUTHORIZED' } }),
  );
  for (const loggedIn of [false, true]) {
    if (loggedIn) await page.unroute('**/api/v1/auth/refresh');
    for (const width of [320, 820, 1440]) {
      await page.setViewportSize({ width, height: 844 });
      await page.goto(loggedIn ? `${adminURL}settings` : adminURL);
      await expect(
        page.getByRole('heading', { name: loggedIn ? '系统配置' : '欢迎回来', exact: true }),
      ).toBeVisible();
      const footer = page.locator(loggedIn ? '.app-footer' : '.login-copyright');
      const link = footer.getByRole('link', { name: icpNumber });
      await expect(link).toHaveAttribute('href', 'https://beian.miit.gov.cn/');
      const geometry = await footer.evaluate((element) => {
        const column = element.getBoundingClientRect();
        const link = element.querySelector('a')!.getBoundingClientRect();
        return {
          center: (column.left + column.right) / 2,
          linkCenter: (link.left + link.right) / 2,
          linkHeight: link.height,
        };
      });
      expect(Math.abs(geometry.linkCenter - geometry.center)).toBeLessThanOrEqual(1);
      if (width <= 390) expect(geometry.linkHeight).toBeGreaterThan(20);
      await noOverflow(page);
    }
  }
});
