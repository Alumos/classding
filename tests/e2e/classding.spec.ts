import {test,expect,type Page} from '@playwright/test';
const adminURL='http://127.0.0.1:15174/admin/';
const teacherURL='http://127.0.0.1:15173';
const screenURL='http://127.0.0.1:15175';
async function adminLogin(page:Page){
  await page.goto(adminURL);
  await page.getByLabel('管理员账号').fill('admin');
  await page.getByLabel('密码',{exact:true}).fill('AdminDing2026!');
  await page.getByRole('button',{name:'登录管理后台'}).click();
  await expect(page.getByRole('heading',{name:'工作概览',exact:true})).toBeVisible();
}
async function teacherLogin(page:Page){
  await page.goto(teacherURL+'/login');
  await page.getByLabel('用户名',{exact:true}).fill('teacher');
  await page.getByLabel('密码',{exact:true}).fill('ClassDing2026!');
  await page.getByRole('button',{name:'进入工作台'}).click();
  await expect(page.getByRole('heading',{name:'选择接收班级'})).toBeVisible();
}
async function navigate(page:Page,path:string,heading:string){
  await page.locator(`a[href="${path}"]`).filter({visible:true}).first().click();
  await expect(page.getByRole('heading',{name:heading})).toBeVisible();
}
async function noOverflow(page:Page){
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth+1)).toBeTruthy();
}
test('管理员登录、生成激活码、搜索与查看真实数据',async({page},testInfo)=>{
  await adminLogin(page);
  await noOverflow(page);
  await expect(page.getByText('正在加载图表…',{exact:true})).not.toBeVisible();
  await expect(page.locator('.recharts-surface')).toBeVisible();
  await page.screenshot({path:testInfo.outputPath('admin-overview.png'),fullPage:true});
  await page.getByRole('link',{name:'激活码管理'}).click();
  await page.getByRole('button',{name:'生成激活码'}).click();
  const dialog=page.getByRole('dialog');
  await expect(dialog).toBeVisible();
  await dialog.getByLabel('生成数量').fill('2');
  await dialog.getByLabel('备注').fill(`浏览器验收-${testInfo.project.name}`);
  await dialog.getByRole('button',{name:'生成激活码',exact:true}).click();
  await expect(page.getByText('成功生成 2 个激活码')).toBeVisible();
  await page.getByPlaceholder('搜索激活码、备注或批次').fill(`浏览器验收-${testInfo.project.name}`);
  await expect(page.locator('tbody tr')).toHaveCount(2);
  await page.screenshot({path:testInfo.outputPath('activation-codes.png'),fullPage:true});
  await noOverflow(page);
  await page.reload();
  await expect(page.getByRole('heading',{name:'激活码管理',exact:true})).toBeVisible();
});
test('教师登录、切换页面并保持移动端布局',async({page},testInfo)=>{
  await teacherLogin(page);
  await noOverflow(page);
  await page.screenshot({path:testInfo.outputPath('teacher-send.png'),fullPage:true});
  await navigate(page,'/classes','我的班级');
  await expect(page.getByRole('heading',{name:'三年级（1）班',exact:true})).toBeVisible();
  await noOverflow(page);
  await navigate(page,'/history','发送记录');
  await noOverflow(page);
});
test('真实绑定大屏、发送200字通知并更新送达回执',async({page,context},testInfo)=>{
  await teacherLogin(page);
  await navigate(page,'/classes','我的班级');
  const classCard=page.locator('.class-management-card').filter({has:page.getByRole('heading',{name:'三年级（1）班',exact:true})});
  await classCard.getByRole('button',{name:'连接大屏'}).click();
  await page.getByRole('button',{name:'获取绑定码',exact:true}).click();
  await expect(page.locator('.bind-digits')).toBeVisible();
  const code=(await page.locator('.bind-digits').innerText()).replace(/\D/g,'');
  expect(code).toMatch(/^\d{6}$/);
  const screen=await context.newPage();
  await screen.goto(screenURL);
  await screen.getByLabel('6 位班级绑定码').fill(code);
  await screen.getByRole('button',{name:'连接班级',exact:true}).click();
  await expect(screen.getByText('连接正常',{exact:true})).toBeVisible();
  await screen.screenshot({path:testInfo.outputPath('screen-ready.png'),fullPage:true});
  await page.getByRole('button',{name:'关闭',exact:true}).first().click();
  await navigate(page,'/send','选择接收班级');
  const prefix=`验收${testInfo.project.name}${Date.now()}：`;
  const message=(prefix+'请同学们带上作业到办公室，认真检查书包并保持教室整洁。'.repeat(10)).slice(0,200);
  await page.getByLabel('通知内容').fill(message);
  await page.getByPlaceholder('例如：李沐阳').fill('李沐阳');
  await page.getByRole('button',{name:'发送到 1 个班级',exact:true}).click();
  await expect(screen.locator('.cd-notice-body p')).toHaveText(message);
  expect(await screen.locator('.cd-notice-body').evaluate(element=>{
    const body=element.getBoundingClientRect();
    const content=element.querySelector('p')!.getBoundingClientRect();
    const name=element.querySelector('strong')!.getBoundingClientRect();
    return content.bottom<=body.bottom+1&&name.top>=body.top-1;
  })).toBeTruthy();
  await screen.screenshot({path:testInfo.outputPath('screen-notice.png'),fullPage:true});
  await navigate(page,'/history','发送记录');
  const entry=page.locator('.history-row').filter({hasText:message});
  await expect(entry).toBeVisible();
  await expect(entry.getByText(/已送达|正在播放|已播放|投递失败/).filter({visible:true}).first()).toBeVisible();
  await screen.getByRole('button',{name:'关闭通知'}).click();
  await screen.close();
});
test('教师在开发模式下录制合成音源并上传原声',async({page})=>{
  // Keep MediaRecorder and the upload real, with a deterministic audio source
  // that does not depend on a CI machine's microphone or operating system permission.
  await page.addInitScript(()=>{
    navigator.mediaDevices.getUserMedia=async()=>{
      const audio=new AudioContext();
      const oscillator=audio.createOscillator();
      const destination=audio.createMediaStreamDestination();
      oscillator.connect(destination);
      oscillator.start();
      for(const track of destination.stream.getTracks()){
        const stop=track.stop.bind(track);
        track.stop=()=>{stop();oscillator.stop();void audio.close();};
      }
      return destination.stream;
    };
  });
  await teacherLogin(page);
  await page.getByRole('button',{name:'语音',exact:true}).click();
  await page.getByRole('button',{name:'点击开始录音',exact:true}).click();
  await expect(page.getByRole('button',{name:'结束录音',exact:true})).toBeVisible();
  // Record an actual short media clip from the browser's synthetic source.
  await page.waitForTimeout(700);
  const upload=page.waitForResponse(response=>response.url().endsWith('/api/v1/audio')&&response.request().method()==='POST');
  await page.getByRole('button',{name:'结束录音',exact:true}).click();
  expect((await upload).status()).toBe(200);
  await expect(page.getByText('原声已准备好',{exact:true})).toBeVisible();
  await expect(page.getByRole('button',{name:'发送到 1 个班级',exact:true})).toBeEnabled();
});
