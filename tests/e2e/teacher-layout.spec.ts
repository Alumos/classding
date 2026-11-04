import { expect, test } from '@playwright/test';

const teacherURL = 'http://127.0.0.1:15173';

test('教师登录后桌面切换页面时侧栏固定于视口，内容宽度保持一致', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 720 });
  await page.goto(`${teacherURL}/login`);
  await page.getByLabel('用户名', { exact: true }).fill('teacher');
  await page.getByLabel('密码', { exact: true }).fill('ClassDing2026!');
  await page.getByRole('button', { name: '进入工作台' }).click();
  await expect(page.getByRole('heading', { name: '选择接收班级' })).toBeVisible();
  const pages = [
    ['/send', '把提醒，送进教室'],
    ['/classes', '我的班级'],
    ['/homework', '布置作业'],
    ['/history', '发送记录'],
    ['/me', '你好'],
  ] as const;
  const layouts = [];
  for (const [path, heading] of pages) {
    await page.locator(`.side-nav a[href="${path}"]`).click();
    await expect(page.locator('.main-content h1').first()).toContainText(heading);
    await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight));
    layouts.push(
      await page.evaluate(() => {
        const sidebar = document.querySelector('.sidebar')!.getBoundingClientRect();
        const profile = document.querySelector('.side-profile')!.getBoundingClientRect();
        const main = document.querySelector('.main-content')!.getBoundingClientRect();
        const footer = document
          .querySelector('.workspace-footer a, .workspace-footer span')!
          .getBoundingClientRect();
        return {
          sidebarTop: sidebar.top,
          sidebarHeight: sidebar.height,
          profileBottom: profile.bottom,
          mainLeft: main.left,
          mainWidth: main.width,
          footerCenter: (footer.left + footer.right) / 2,
          mainCenter: (main.left + main.right) / 2,
          viewportHeight: innerHeight,
        };
      }),
    );
  }
  for (const layout of layouts) {
    expect(Math.abs(layout.sidebarTop)).toBeLessThanOrEqual(1);
    expect(Math.abs(layout.sidebarHeight - layout.viewportHeight)).toBeLessThanOrEqual(1);
    expect(layout.profileBottom).toBeLessThanOrEqual(layout.viewportHeight);
    expect(Math.abs(layout.mainLeft - layouts[0].mainLeft)).toBeLessThanOrEqual(1);
    expect(Math.abs(layout.mainWidth - layouts[0].mainWidth)).toBeLessThanOrEqual(1);
    expect(Math.abs(layout.footerCenter - layout.mainCenter)).toBeLessThanOrEqual(1);
  }
  await expect(page.locator('.sidebar').getByRole('button', { name: '退出登录' })).toBeInViewport();
});

test('教师调整的字号会被服务器记住，刷新后保持', async ({ page }) => {
  await page.goto(`${teacherURL}/login`);
  await page.getByLabel('用户名', { exact: true }).fill('teacher');
  await page.getByLabel('密码', { exact: true }).fill('ClassDing2026!');
  await page.getByRole('button', { name: '进入工作台' }).click();
  await expect(page.getByRole('heading', { name: '选择接收班级' })).toBeVisible();
  const saved = page.waitForResponse(
    (response) => response.url().endsWith('/api/v1/me') && response.request().method() === 'PATCH',
  );
  await page.getByRole('button', { name: '调整字号' }).click();
  await page.getByRole('menuitemradio', { name: /特大/ }).click();
  expect((await saved).status()).toBe(200);
  await expect(page.locator('html')).toHaveCSS('font-size', '20.8px');
  await page.reload();
  await expect(page.getByRole('heading', { name: '选择接收班级' })).toBeVisible();
  await expect(page.locator('html')).toHaveCSS('font-size', '20.8px');
  await page.getByRole('button', { name: '调整字号' }).click();
  await page.getByRole('menuitemradio', { name: /标准/ }).click();
  await expect(page.locator('html')).toHaveCSS('font-size', '16px');
});

test('手机上班级设置的标签完整显示，倒计时可自定义时长', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 360, height: 760 });
  await page.goto(`${teacherURL}/login`);
  await page.getByLabel('用户名', { exact: true }).fill('teacher');
  await page.getByLabel('密码', { exact: true }).fill('ClassDing2026!');
  await page.getByRole('button', { name: '进入工作台' }).click();
  await expect(page.getByRole('heading', { name: '选择接收班级' })).toBeVisible();
  await page.goto(`${teacherURL}/classes`);
  await page.locator('.class-management-card').first().getByRole('button', { name: '连接大屏' }).click();
  const dialog = page.getByRole('dialog');
  const tabs = dialog.getByRole('tab');
  await expect(tabs).toHaveCount(4);
  const overflow = await tabs.evaluateAll((items) =>
    items.some((item) => {
      const tab = item.getBoundingClientRect();
      const list = item.parentElement!.getBoundingClientRect();
      return (
        item.scrollWidth > item.clientWidth + 1 || tab.left < list.left - 1 || tab.right > list.right + 1
      );
    }),
  );
  expect(overflow).toBe(false);
  await dialog.screenshot({ path: testInfo.outputPath('class-tabs-mobile.png') });

  await dialog.getByRole('tab', { name: '课堂工具' }).click();
  await dialog.getByLabel('分钟').fill('2');
  await dialog.getByLabel('秒', { exact: true }).fill('30');
  await dialog.screenshot({ path: testInfo.outputPath('class-countdown-mobile.png') });
  const sent = page.waitForRequest((request) => request.url().endsWith('/tools/countdown'));
  await dialog.getByRole('button', { name: '开始倒计时' }).click();
  expect((await sent).postDataJSON()).toEqual({ seconds: 150 });
  await dialog.getByLabel('分钟').fill('121');
  await expect(dialog.getByRole('button', { name: '开始倒计时' })).toBeDisabled();
});
