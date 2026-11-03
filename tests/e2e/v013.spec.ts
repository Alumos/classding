import {test,expect,type Page} from '@playwright/test';
const adminURL='http://127.0.0.1:15174/admin/';
const teacherURL='http://127.0.0.1:15173';
async function adminLogin(page:Page){
  await page.goto(adminURL);await page.getByLabel('管理员账号').fill('admin');
  await page.getByLabel('密码',{exact:true}).fill('AdminDing2026!');
  await page.getByRole('button',{name:'登录管理后台'}).click();
  await expect(page.getByRole('heading',{name:'工作概览',exact:true})).toBeVisible();
}
async function teacherLogin(page:Page){
  await page.goto(teacherURL+'/login');await page.getByLabel('用户名',{exact:true}).fill('teacher');
  await page.getByLabel('密码',{exact:true}).fill('ClassDing2026!');
  await page.getByRole('button',{name:'进入工作台'}).click();
  await expect(page.getByRole('heading',{name:'选择接收班级'})).toBeVisible();
}
async function noOverflow(page:Page){expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);}

test('教师可直接选择时长，紧急提示音需手动打开且切换样式会复位',async({page},info)=>{
  await teacherLogin(page);
  await page.getByLabel('通知显示时长').selectOption('600');
  await expect(page.getByRole('switch',{name:'紧急事项提示音'})).toHaveCount(0);
  await page.getByRole('button',{name:'紧急提醒',exact:true}).click();
  const chime=page.getByRole('switch',{name:'紧急事项提示音'});
  await expect(chime).not.toBeChecked();await chime.click();await expect(chime).toBeChecked();
  await page.getByRole('button',{name:'日常通知',exact:true}).click();
  await page.getByRole('button',{name:'紧急提醒',exact:true}).click();await expect(chime).not.toBeChecked();
  await chime.click();
  await page.getByLabel('通知内容').fill(`时长与提示音验收 ${info.project.name}`);
  const sent=page.waitForResponse(r=>r.url().endsWith('/api/v1/messages')&&r.request().method()==='POST');
  await page.getByRole('button',{name:'发送到 1 个班级',exact:true}).click();
  const response=await sent;expect(response.status()).toBe(200);
  expect(response.request().postDataJSON()).toMatchObject({display_sec:600,chime:true,style:'urgent'});
  expect(await response.json()).toMatchObject({display_sec:600,chime:true});
  expect(await page.getByLabel('通知内容').evaluate(e=>parseFloat(getComputedStyle(e).fontSize))).toBeGreaterThanOrEqual(16);
  await noOverflow(page);await page.screenshot({path:info.outputPath('teacher-readable.png'),fullPage:true});
});

test('系统配置同步两后台，备案固定链接，备份可下载并恢复',async({page,context},info)=>{
  await adminLogin(page);
  await page.getByRole('link',{name:'系统配置',exact:true}).filter({visible:true}).first().click();
  await expect(page.getByRole('heading',{name:'系统配置',exact:true})).toBeVisible();
  await page.getByLabel('站点名称').fill('阳光学校课堂通知');
  await page.getByLabel('备案号').fill('京ICP备12345678号-1');
  await page.getByLabel('主题',{exact:true}).selectOption('blue');
  await page.getByRole('button',{name:'保存站点配置'}).click();
  await expect(page.locator('.sidebar-brand')).toContainText('阳光学校课堂通知');
  const icp=page.getByRole('link',{name:'京ICP备12345678号-1',exact:true});
  await expect(icp).toHaveAttribute('href','https://beian.miit.gov.cn/');
  await expect(page.locator('html')).toHaveAttribute('data-cd-theme','blue');
  expect(await page.getByLabel('站点名称').evaluate(e=>parseFloat(getComputedStyle(e).fontSize))).toBeGreaterThanOrEqual(16);
  await noOverflow(page);await page.screenshot({path:info.outputPath('system-settings.png'),fullPage:true});
  const teacher=await context.newPage();await teacherLogin(teacher);
  await expect(teacher.locator('.sidebar-brand')).toContainText('阳光学校课堂通知');
  await expect(teacher.getByRole('link',{name:'京ICP备12345678号-1'})).toHaveAttribute('href','https://beian.miit.gov.cn/');
  await teacher.close();
  const download=page.waitForEvent('download');await page.getByRole('button',{name:'下载备份'}).click();
  const backup=await download;const backupPath=info.outputPath('site-backup.zip');await backup.saveAs(backupPath);
  await page.getByLabel('站点名称').fill('恢复前临时站点');await page.getByRole('button',{name:'保存站点配置'}).click();
  await expect(page.locator('.sidebar-brand')).toContainText('恢复前临时站点');
  await page.getByRole('button',{name:'导入并恢复',exact:true}).click();
  const dialog=page.getByRole('dialog');await dialog.getByLabel('选择备份文件').setInputFiles(backupPath);
  await expect(dialog.getByRole('button',{name:'确认覆盖恢复',exact:true})).toBeDisabled();
  await dialog.getByLabel('输入“覆盖恢复”确认').fill('覆盖恢复');
  await dialog.getByRole('button',{name:'确认覆盖恢复',exact:true}).click();
  await expect(page.getByLabel('管理员账号')).toBeVisible();
  await adminLogin(page);await expect(page.locator('.sidebar-brand')).toContainText('阳光学校课堂通知');
  await page.getByRole('link',{name:'系统配置',exact:true}).filter({visible:true}).first().click();
  await expect(page.getByLabel('站点名称')).toHaveValue('阳光学校课堂通知');
  // Keep the shared temporary test server in its original appearance for later projects.
  await page.getByLabel('站点名称').fill('课堂叮咚');await page.getByLabel('备案号').fill('');
  await page.getByLabel('主题',{exact:true}).selectOption('green');await page.getByRole('button',{name:'保存站点配置'}).click();
  await expect(page.locator('.sidebar-brand')).toContainText('课堂叮咚');
});

test('设备管理可保存和取消指定设备的服务器切换计划',async({page},info)=>{
  await adminLogin(page);await page.getByRole('link',{name:'设备管理',exact:true}).filter({visible:true}).first().click();
  const row=page.locator('tbody tr').first();await row.getByRole('button',{name:'配置服务器'}).click();
  let dialog=page.getByRole('dialog');
  await dialog.getByLabel('服务器地址').fill(`https://migration-${info.project.name}.example`);
  await dialog.getByRole('button',{name:'保存配置'}).click();
  await expect(row).toContainText(`https://migration-${info.project.name}.example`);
  await row.getByRole('button',{name:'配置服务器'}).click();dialog=page.getByRole('dialog');
  await dialog.getByLabel('服务器地址').fill('');await dialog.getByRole('button',{name:'保存配置'}).click();
  await expect(row).toContainText('未设置');await noOverflow(page);
});

test('两后台在明暗模式下保持清晰文字和可用布局',async({page},info)=>{
  for(const scheme of ['light','dark'] as const){
    await page.emulateMedia({colorScheme:scheme});
    for(const entry of [{url:teacherURL+'/login',text:'.login-form-wrap>p'},{url:adminURL,text:'.login-card>.muted'}]){
      await page.goto(entry.url);const text=page.locator(entry.text);await expect(text).toBeVisible();
      const contrast=await text.evaluate(element=>{
        const rgb=(color:string)=>color.match(/[\d.]+/g)!.slice(0,3).map(Number);
        const lum=(c:number[])=>c.map(v=>v/255).map(v=>v<=.04045?v/12.92:((v+.055)/1.055)**2.4).reduce((a,v,i)=>a+v*[.2126,.7152,.0722][i],0);
        let parent:Element|null=element;let bg='';
        while(parent){const color=getComputedStyle(parent).backgroundColor;if(color!=='rgba(0, 0, 0, 0)'&&color!=='transparent'){bg=color;break;}parent=parent.parentElement;}
        const a=lum(rgb(getComputedStyle(element).color)),b=lum(rgb(bg));
        return (Math.max(a,b)+.05)/(Math.min(a,b)+.05);
      });
      expect(contrast).toBeGreaterThanOrEqual(4.5);await noOverflow(page);
    }
    await page.screenshot({path:info.outputPath(`admin-login-${scheme}.png`),fullPage:true});
  }
});
