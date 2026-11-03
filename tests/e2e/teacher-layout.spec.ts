import {expect, test} from '@playwright/test';

const teacherURL = 'http://127.0.0.1:15173';

test('教师登录后桌面切换页面时侧栏固定于视口，内容宽度保持一致', async ({page}) => {
  await page.setViewportSize({width:1440,height:720});
  await page.goto(`${teacherURL}/login`);
  await page.getByLabel('用户名',{exact:true}).fill('teacher');
  await page.getByLabel('密码',{exact:true}).fill('ClassDing2026!');
  await page.getByRole('button',{name:'进入工作台'}).click();
  await expect(page.getByRole('heading',{name:'选择接收班级'})).toBeVisible();
  const pages = [['/send','把提醒，送进教室'],['/classes','我的班级'],['/homework','布置作业'],['/history','发送记录'],['/me','你好']] as const;
  const layouts = [];
  for (const [path,heading] of pages) {
    await page.locator(`.side-nav a[href="${path}"]`).click();
    await expect(page.locator('.main-content h1').first()).toContainText(heading);
    await page.evaluate(() => window.scrollTo(0,document.documentElement.scrollHeight));
    layouts.push(await page.evaluate(() => {
      const sidebar = document.querySelector('.sidebar')!.getBoundingClientRect();
      const profile = document.querySelector('.side-profile')!.getBoundingClientRect();
      const main = document.querySelector('.main-content')!.getBoundingClientRect();
      const footer = document.querySelector('.workspace-footer a, .workspace-footer span')!.getBoundingClientRect();
      return {sidebarTop:sidebar.top,sidebarHeight:sidebar.height,profileBottom:profile.bottom,mainLeft:main.left,mainWidth:main.width,footerCenter:(footer.left+footer.right)/2,mainCenter:(main.left+main.right)/2,viewportHeight:innerHeight};
    }));
  }
  for (const layout of layouts) {
    expect(Math.abs(layout.sidebarTop)).toBeLessThanOrEqual(1);
    expect(Math.abs(layout.sidebarHeight-layout.viewportHeight)).toBeLessThanOrEqual(1);
    expect(layout.profileBottom).toBeLessThanOrEqual(layout.viewportHeight);
    expect(Math.abs(layout.mainLeft-layouts[0].mainLeft)).toBeLessThanOrEqual(1);
    expect(Math.abs(layout.mainWidth-layouts[0].mainWidth)).toBeLessThanOrEqual(1);
    expect(Math.abs(layout.footerCenter-layout.mainCenter)).toBeLessThanOrEqual(1);
  }
  await expect(page.locator('.sidebar').getByRole('button',{name:'退出登录'})).toBeInViewport();
});
