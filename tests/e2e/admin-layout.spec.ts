import {expect, test, type Page} from '@playwright/test';

const adminURL = process.env.CLASSDING_ADMIN_TEST_URL ?? 'http://127.0.0.1:15174/admin/';
const settings = {site_name:'课堂叮咚',icp_number:'京ICP备12345678号-1',theme:'green'};
const administrator = {id:'admin-1',username:'admin',display_name:'管理员',role:'super_admin',status:'active'};

async function mockAdmin(page: Page, siteName = settings.site_name, icpNumber = settings.icp_number) {
  const siteSettings = {...settings,site_name:siteName,icp_number:icpNumber};
  const responses: Record<string, unknown> = {
    '/api/v1/config': siteSettings,
    '/api/v1/auth/refresh': {access_token:'test-token',user:administrator},
    '/api/v1/admin/settings': siteSettings,
    '/api/v1/admin/users': [{id:'teacher-1',username:'teacher',display_name:'林老师',role:'user',status:'active',org_id:'school-1'}],
    '/api/v1/admin/overview': {online_devices:1,total_devices:2,today_messages:3,delivery_rate:.9,avg_delivery_ms:300,total_users:1,total_classes:1,recent_messages:[],activity:[]},
  };
  await page.route('**/api/v1/**', route => {
    const path = new URL(route.request().url()).pathname;
    return route.fulfill(path in responses ? {json:responses[path]} : {status:404,json:{message:'未模拟的请求'}});
  });
}

async function contrast(page: Page, textSelector: string, backgroundSelector: string) {
  return page.evaluate(({textSelector,backgroundSelector}) => {
    const sample = (value: string) => {
      const canvas = document.createElement('canvas'); canvas.width = canvas.height = 1;
      const context = canvas.getContext('2d')!;
      context.fillStyle = value; context.fillRect(0,0,1,1);
      return Array.from(context.getImageData(0,0,1,1).data).slice(0,3);
    };
    const luminance = (rgb: number[]) => rgb.map(value => value / 255).map(value => value <= .04045 ? value / 12.92 : ((value + .055) / 1.055) ** 2.4).reduce((sum,value,index) => sum + value * [.2126,.7152,.0722][index],0);
    const foreground = luminance(sample(getComputedStyle(document.querySelector(textSelector)!).color));
    const background = luminance(sample(getComputedStyle(document.querySelector(backgroundSelector)!).backgroundColor));
    return (Math.max(foreground,background) + .05) / (Math.min(foreground,background) + .05);
  }, {textSelector,backgroundSelector});
}

test('管理员登录后窄屏当前导航和恢复弹窗保持可见', async ({page}) => {
  await mockAdmin(page);
  for (const width of [320,390,820]) {
    await page.setViewportSize({width,height:844});
    await page.goto(`${adminURL}settings`);
    await expect(page.getByRole('heading',{name:'系统配置',exact:true})).toBeVisible();
    const nav = page.locator('.mobile-nav');
    const active = nav.locator('[aria-current="page"]');
    await expect.poll(async () => active.evaluate(element => {
      const container = element.parentElement!.getBoundingClientRect();
      const item = element.getBoundingClientRect();
      return item.left >= container.left && item.right <= container.right;
    })).toBe(true);
    await page.getByRole('button',{name:'导入并恢复',exact:true}).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading',{name:'确认覆盖恢复全站数据？'})).toBeVisible();
    for (const label of ['取消','确认覆盖恢复']) {
      const bounds = await dialog.getByRole('button',{name:label,exact:true}).boundingBox();
      expect(bounds).not.toBeNull();
      expect(bounds!.x).toBeGreaterThanOrEqual(0);
      expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(width);
      expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(844);
    }
    await dialog.getByRole('button',{name:'取消',exact:true}).click();
  }
});

test('管理员登录后深色强调内容清晰，窄屏表格可用键盘横向查看', async ({page}) => {
  await mockAdmin(page);
  await page.setViewportSize({width:320,height:844});
  await page.emulateMedia({colorScheme:'dark'});
  await page.goto(adminURL);
  await expect(page.getByRole('heading',{name:'工作概览',exact:true})).toBeVisible();
  for (const theme of ['green','blue','slate']) {
    await page.evaluate(value => { document.documentElement.dataset.cdTheme = value; },theme);
    expect(await contrast(page,'.welcome-copy h2','.welcome-banner')).toBeGreaterThanOrEqual(4.5);
  }
  await page.goto(`${adminURL}settings`);
  await expect(page.getByRole('button',{name:'保存站点配置'})).toBeVisible();
  for (const theme of ['green','blue','slate']) {
    await page.evaluate(value => { document.documentElement.dataset.cdTheme = value; },theme);
    expect(await contrast(page,'.settings-content .cd-button-primary','.settings-content .cd-button-primary')).toBeGreaterThanOrEqual(4.5);
  }
  await page.goto(`${adminURL}users`);
  const table = page.getByRole('region',{name:'数据表格'});
  await expect(table).toBeVisible();
  await expect(page.getByText('左右滑动查看更多')).toBeVisible();
  await table.focus();
  for (let i=0;i<8;i++) await page.keyboard.press('ArrowRight');
  await expect.poll(() => table.evaluate(element => element.scrollLeft)).toBeGreaterThan(0);
  await page.setViewportSize({width:1440,height:844});
  await expect(page.getByText('左右滑动查看更多')).toHaveCount(0);
});

test('管理员登录后长站点名称不会挤占导航，页脚保留完整名称', async ({page}) => {
  const siteName = '校园课堂通知管理服务平台'.repeat(6).slice(0,60);
  await mockAdmin(page,siteName);
  for (const width of [320,820,1440]) {
    await page.setViewportSize({width,height:844});
    await page.goto(`${adminURL}settings`);
    await expect(page.locator('.app-footer')).toContainText(siteName);
    const layout = await page.locator('.sidebar-brand').evaluate(element => {
      const title = element.querySelector('.cd-brand strong')!.getBoundingClientRect();
      const brand = element.querySelector('.cd-brand')!.getBoundingClientRect();
      const label = element.querySelector(':scope > span')!.getBoundingClientRect();
      return {titleHeight:title.height,brandRight:brand.right,labelLeft:label.left,labelRight:label.right};
    });
    expect(layout.titleHeight).toBeLessThan(50);
    if (width <= 900) {
      expect(layout.labelLeft).toBeGreaterThanOrEqual(layout.brandRight);
      expect(layout.labelRight).toBeLessThanOrEqual(width);
    }
  }
});

test('管理员登录后桌面切换页面时侧栏固定于视口，内容宽度保持一致', async ({page}) => {
  await mockAdmin(page);
  for (const width of [1000,1440]) {
    await page.setViewportSize({width,height:720});
    for (const colorScheme of ['light','dark'] as const) {
      await page.emulateMedia({colorScheme});
      await page.goto(adminURL);
      await expect(page.getByRole('heading',{name:'工作概览',exact:true})).toBeVisible();
      await expect.poll(() => page.evaluate(() => document.documentElement.scrollHeight)).toBeGreaterThan(720);
      const caption = await page.locator('.sidebar-caption').evaluate(element => {
        const english = document.createRange();
        english.selectNodeContents(element.firstChild!);
        const first = english.getBoundingClientRect();
        const second = element.querySelector('span')!.getBoundingClientRect();
        return {firstTop:first.top,secondTop:second.top,secondRight:second.right,containerRight:element.getBoundingClientRect().right};
      });
      expect(Math.abs(caption.firstTop - caption.secondTop)).toBeLessThan(2);
      expect(caption.secondRight).toBeLessThanOrEqual(caption.containerRight);
      const measure = () => page.evaluate(() => {
        const sidebar = document.querySelector('.sidebar')!.getBoundingClientRect();
        const account = document.querySelector('.sidebar-user')!.getBoundingClientRect();
        const main = document.querySelector('.main-content')!.getBoundingClientRect();
        return {sidebarTop:sidebar.top,sidebarHeight:sidebar.height,viewportHeight:innerHeight,accountGap:sidebar.bottom-account.bottom,padding:parseFloat(getComputedStyle(document.querySelector('.sidebar')!).paddingBottom),mainLeft:main.left,mainWidth:main.width};
      });
      await page.evaluate(() => window.scrollTo(0,document.documentElement.scrollHeight));
      const longPage = await measure();
      expect(Math.abs(longPage.sidebarTop)).toBeLessThanOrEqual(1);
      expect(Math.abs(longPage.sidebarHeight-longPage.viewportHeight)).toBeLessThanOrEqual(1);
      expect(longPage.accountGap).toBeLessThanOrEqual(longPage.padding+1);
      await expect(page.locator('.sidebar').getByRole('button',{name:'退出登录'})).toBeInViewport();
      await page.locator('.side-nav').getByRole('link',{name:'教师管理'}).click();
      await expect(page.getByRole('heading',{name:'教师管理',exact:true})).toBeVisible();
      const shortPage = await measure();
      expect(shortPage.sidebarHeight).toBeCloseTo(longPage.sidebarHeight,0);
      expect(shortPage.mainLeft).toBeCloseTo(longPage.mainLeft,0);
      expect(shortPage.mainWidth).toBeCloseTo(longPage.mainWidth,0);
    }
  }
});


test('桌面短屏和长站名下可滚动访问侧栏导航与退出按钮', async ({page}) => {
  const siteName = '校园课堂通知管理服务平台'.repeat(6).slice(0,60);
  await mockAdmin(page,siteName);
  for (const width of [1000,1440]) {
    await page.setViewportSize({width,height:450});
    for (const colorScheme of ['light','dark'] as const) {
      await page.emulateMedia({colorScheme});
      await page.goto(`${adminURL}settings`);
      await expect(page.getByRole('heading',{name:'系统配置',exact:true})).toBeVisible();
      const sidebar = page.locator('.sidebar');
      const dimensions = await sidebar.evaluate(element => {
        const parts = ['.sidebar-brand','.sidebar-caption','.side-nav','.sidebar-note','.sidebar-user'].map(selector => element.querySelector(selector)!.getBoundingClientRect()).filter(rect => rect.height > 0);
        const links = [...element.querySelectorAll('.side-nav a')].map(link => link.getBoundingClientRect().height);
        return {pageHeight:document.documentElement.scrollHeight,viewportHeight:innerHeight,parts:parts.map(rect => ({top:rect.top,bottom:rect.bottom})),links};
      });
      expect(dimensions.pageHeight).toBeGreaterThan(dimensions.viewportHeight);
      for (let i=1;i<dimensions.parts.length;i++) expect(dimensions.parts[i].top).toBeGreaterThanOrEqual(dimensions.parts[i-1].bottom-1);
      expect(Math.min(...dimensions.links)).toBeGreaterThanOrEqual(44);
      const lastLink = sidebar.getByRole('link',{name:'系统配置'});
      await lastLink.scrollIntoViewIfNeeded();
      await expect(lastLink).toBeInViewport();
      const logout = sidebar.getByRole('button',{name:'退出登录'});
      await logout.scrollIntoViewIfNeeded();
      await expect(logout).toBeInViewport();
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    }
  }
});


test('登录页与管理页的备案号位于页脚底部内容区中央，长编号可换行', async ({page}) => {
  const icpNumber = `京ICP备${'1234567890'.repeat(12)}号`;
  const longName = '校园课堂通知管理服务平台'.repeat(6).slice(0,60);
  await mockAdmin(page,longName,icpNumber);
  await page.route('**/api/v1/auth/refresh',route => route.fulfill({status:401,json:{code:'UNAUTHORIZED'}}));
  for (const loggedIn of [false,true]) {
    if (loggedIn) await page.unroute('**/api/v1/auth/refresh');
    for (const width of [320,390,820,1000,1440]) {
      await page.setViewportSize({width,height:844});
      await page.goto(loggedIn ? `${adminURL}settings` : adminURL);
      if (loggedIn) await expect(page.getByRole('heading',{name:'系统配置',exact:true})).toBeVisible();
      else await expect(page.getByRole('heading',{name:'欢迎回来',exact:true})).toBeVisible();
      const footer = page.locator(loggedIn ? '.app-footer' : '.login-copyright');
      const link = footer.getByRole('link',{name:icpNumber});
      await expect(link).toHaveAttribute('href','https://beian.miit.gov.cn/');
      const geometry = await footer.evaluate(element => {
        const copyright = element.querySelector('span')!.getBoundingClientRect();
        const anchor = element.querySelector('a')!;
        const link = anchor.getBoundingClientRect();
        const style = getComputedStyle(anchor);
        const column = element.parentElement!.getBoundingClientRect();
        return {contentCenter:(column.left+column.right)/2,linkCenter:(link.left+link.right)/2,copyrightCenter:(copyright.left+copyright.right)/2,copyrightLeft:copyright.left,linkLeft:link.left,sidebarRight:document.querySelector('.sidebar')?.getBoundingClientRect().right??0,copyrightBottom:copyright.bottom,linkTop:link.top,linkHeight:link.height,lineHeight:parseFloat(style.lineHeight),textAlign:style.textAlign};
      });
      expect(Math.abs(geometry.linkCenter-geometry.contentCenter)).toBeLessThanOrEqual(1);
      expect(Math.abs(geometry.copyrightCenter-geometry.contentCenter)).toBeLessThanOrEqual(1);
      expect(geometry.textAlign).toBe('center');
      if (loggedIn && width>900) {
        expect(geometry.copyrightLeft).toBeGreaterThanOrEqual(geometry.sidebarRight);
        expect(geometry.linkLeft).toBeGreaterThanOrEqual(geometry.sidebarRight);
      }
      expect(geometry.linkTop).toBeGreaterThanOrEqual(geometry.copyrightBottom);
      if (width <= 390) expect(geometry.linkHeight).toBeGreaterThan(geometry.lineHeight*1.5);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
    }
  }
});


test('桌面短页面的侧栏账号贴近视口底部', async ({page}) => {
  await mockAdmin(page);
  await page.setViewportSize({width:1440,height:1100});
  await page.goto(`${adminURL}settings`);
  await expect(page.getByRole('heading',{name:'系统配置',exact:true})).toBeVisible();
  const alignment = await page.locator('.sidebar').evaluate(element => {
    const sidebar = element.getBoundingClientRect();
    const account = element.querySelector('.sidebar-user')!.getBoundingClientRect();
    return {pageHeight:document.documentElement.scrollHeight,sidebarBottom:sidebar.bottom,accountGap:sidebar.bottom-account.bottom,padding:parseFloat(getComputedStyle(element).paddingBottom)};
  });
  expect(alignment.pageHeight).toBeLessThanOrEqual(1101);
  expect(Math.abs(alignment.sidebarBottom-1100)).toBeLessThanOrEqual(1);
  expect(alignment.accountGap).toBeLessThanOrEqual(alignment.padding+1);
});
