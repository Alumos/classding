import {expect,test,type Page} from '@playwright/test';

const adminURL='http://127.0.0.1:15174/admin';
const teacherURL='http://127.0.0.1:15173';

async function adminLogin(page:Page){
  await page.goto(`${adminURL}/`);
  await expect(page.getByLabel('管理员账号')).toBeVisible();
  await expect(page.getByText('动态验证码')).toHaveCount(0);
  await page.getByLabel('管理员账号').fill('admin');
  await page.getByLabel('密码',{exact:true}).fill('AdminDing2026!');
  await page.getByRole('button',{name:'登录管理后台'}).click();
  await expect(page.getByRole('heading',{name:'工作概览',exact:true})).toBeVisible();
}

async function teacherLogin(page:Page){
  await page.goto(`${teacherURL}/login`);
  await page.getByLabel('用户名',{exact:true}).fill('teacher');
  await page.getByLabel('密码',{exact:true}).fill('ClassDing2026!');
  await page.getByRole('button',{name:'进入工作台'}).click();
  await expect(page.getByRole('heading',{name:'选择接收班级'})).toBeVisible();
}

async function adminNavigate(page:Page,name:string){
  await page.getByRole('link',{name,exact:true}).filter({visible:true}).first().click();
  await expect(page.getByRole('heading',{name,exact:true})).toBeVisible();
}

test('管理员登录无需动态码且 HTTP 刷新保持会话',async({page,context})=>{
  await adminLogin(page);
  const cookie=(await context.cookies()).find(item=>item.name==='classding_refresh_admin');
  expect(cookie?.secure).toBe(false);
  await page.reload();
  await expect(page.getByRole('heading',{name:'工作概览',exact:true})).toBeVisible();
  const logout=page.waitForResponse(response=>response.url().endsWith('/api/v1/auth/logout'));
  await page.getByRole('button',{name:'退出登录'}).filter({visible:true}).first().click();
  await logout;
  await expect(page.getByLabel('管理员账号')).toBeVisible();
  await adminLogin(page);
  await page.reload();
  await expect(page.getByRole('heading',{name:'工作概览',exact:true})).toBeVisible();
});

test('教师登录在首次无会话后可恢复、退出并重新登录',async({page})=>{
  await teacherLogin(page);
  await page.reload();
  await expect(page.getByRole('heading',{name:'选择接收班级'})).toBeVisible();
  await page.goto(`${teacherURL}/me`);
  const logout=page.waitForResponse(response=>response.url().endsWith('/api/v1/auth/logout'));
  await page.getByRole('button',{name:'退出当前账号'}).click();
  await logout;
  await expect(page.getByRole('button',{name:'进入工作台'})).toBeVisible();
  await teacherLogin(page);
  await page.reload();
  await expect(page.getByRole('heading',{name:'选择接收班级'})).toBeVisible();
});

test('管理员可创建、编辑和删除教师及其班级',async({page},testInfo)=>{
  await adminLogin(page);
  const suffix=`${Date.now()}${testInfo.project.name==='mobile'?'m':'d'}`;
  const username=`e2e${suffix}`;
  const teacherName=`测试老师${suffix}`;
  const updatedTeacher=`已编辑老师${suffix}`;
  const className=`测试班级${suffix}`;
  const updatedClass=`已编辑班级${suffix}`;

  await adminNavigate(page,'教师管理');
  await page.getByRole('button',{name:'添加教师'}).click();
  let dialog=page.getByRole('dialog');
  await dialog.getByLabel('教师姓名').fill(teacherName);
  await dialog.getByLabel('用户名').fill(username);
  await dialog.getByLabel('登录密码').fill('Teacher2026!');
  await dialog.getByRole('button',{name:'创建账号'}).click();
  let row=page.locator('tbody tr').filter({hasText:username});
  await expect(row).toContainText(teacherName);
  await row.getByRole('button',{name:'编辑'}).click();
  dialog=page.getByRole('dialog');
  await dialog.getByLabel('教师姓名').fill(updatedTeacher);
  await dialog.getByRole('button',{name:'保存修改'}).click();
  await expect(row).toContainText(updatedTeacher);
  for(let attempt=0;attempt<8;attempt++){
    const failure=await page.request.post(`${teacherURL}/api/v1/auth/login`,{headers:{'X-ClassDing-Auth-Scope':'teacher'},data:{username,password:'wrong-password'}});
    expect(failure.status()).toBe(401);
  }
  const blocked=await page.request.post(`${teacherURL}/api/v1/auth/login`,{headers:{'X-ClassDing-Auth-Scope':'teacher'},data:{username,password:'Teacher2026!'}});
  expect(blocked.status()).toBe(429);
  await row.getByRole('button',{name:'解除登录限制'}).click();
  await expect(page.getByText('已解除登录限制，教师可以重新登录')).toBeVisible();
  const unlocked=await page.request.post(`${teacherURL}/api/v1/auth/login`,{headers:{'X-ClassDing-Auth-Scope':'teacher'},data:{username,password:'Teacher2026!'}});
  expect(unlocked.status()).toBe(200);

  await adminNavigate(page,'班级管理');
  await page.getByRole('button',{name:'添加班级'}).click();
  dialog=page.getByRole('dialog');
  await dialog.getByLabel('班级名称').fill(className);
  await dialog.getByLabel('年级').fill('测试年级');
  const owner=await dialog.getByLabel('负责人').locator('option').filter({hasText:username}).getAttribute('value');
  expect(owner).toBeTruthy();
  await dialog.getByLabel('负责人').selectOption(owner!);
  await dialog.getByLabel('教师上限').fill('4');
  await dialog.getByLabel('设备上限').fill('2');
  await dialog.getByRole('button',{name:'创建班级'}).click();
  row=page.locator('tbody tr').filter({hasText:className});
  await expect(row).toContainText(updatedTeacher);
  await row.getByRole('button',{name:'编辑'}).click();
  dialog=page.getByRole('dialog');
  await dialog.getByLabel('班级名称').fill(updatedClass);
  await dialog.getByRole('button',{name:'保存修改'}).click();
  row=page.locator('tbody tr').filter({hasText:updatedClass});
  await expect(row).toBeVisible();
  await row.getByRole('button',{name:'删除'}).click();
  await page.getByRole('dialog').getByRole('button',{name:'确认删除'}).click();
  await expect(row).toHaveCount(0);

  await adminNavigate(page,'教师管理');
  row=page.locator('tbody tr').filter({hasText:username});
  await row.getByRole('button',{name:'删除'}).click();
  await page.getByRole('dialog').getByRole('button',{name:'确认删除'}).click();
  await expect(row).toHaveCount(0);
});

test('HTTP 无剪贴板时复制激活码，失败时可手动选中',async({page})=>{
  await page.addInitScript(()=>{
    Object.defineProperty(navigator,'clipboard',{value:undefined,configurable:true});
    Object.defineProperty(window,'isSecureContext',{value:false,configurable:true});
    const state={allow:true,text:'',result:false};
    Object.defineProperty(window,'copyTest',{value:state});
    const original=document.execCommand.bind(document);
    document.execCommand=(command,showUI,value)=>{
      if(command!=='copy')return original(command,showUI,value);
      state.text=(document.activeElement as HTMLTextAreaElement).value;
      state.result=state.allow&&original(command,showUI,value);
      return state.result;
    };
  });
  await adminLogin(page);
  await adminNavigate(page,'激活码管理');
  await page.getByRole('button',{name:'生成激活码'}).click();
  const create=page.getByRole('dialog');
  await create.getByLabel('生成数量').fill('1');
  await create.getByRole('button',{name:'生成激活码',exact:true}).click();
  await expect(page.getByText('成功生成 1 个激活码')).toBeVisible();
  await page.getByRole('button',{name:'复制全部'}).click();
  const copied=await page.evaluate(()=>(window as unknown as {copyTest:{text:string;result:boolean}}).copyTest);
  expect(copied.result).toBe(true);
  expect(copied.text).toMatch(/^[A-Z2-9]{4}-[A-Z2-9]{4}$/);
  await page.evaluate(()=>{(window as unknown as {copyTest:{allow:boolean}}).copyTest.allow=false;});
  await page.getByRole('button',{name:'复制全部'}).click();
  const manual=page.getByRole('dialog',{name:'手动复制'});
  await expect(manual).toBeVisible();
  const field=manual.getByLabel('待复制内容');
  await expect(field).toHaveValue(copied.text);
  await field.click();
  expect(await field.evaluate((element:HTMLTextAreaElement)=>[element.selectionStart,element.selectionEnd])).toEqual([0,copied.text.length]);
  await manual.getByRole('button',{name:'完成'}).click();
  await expect(manual).toBeHidden();
});

test('教师协同码复制失败时可在班级弹窗内手动复制',async({page})=>{
  await page.addInitScript(()=>{
    Object.defineProperty(navigator,'clipboard',{value:undefined,configurable:true});
    Object.defineProperty(window,'isSecureContext',{value:false,configurable:true});
    document.execCommand=()=>false;
  });
  await teacherLogin(page);
  await page.locator('a[href="/classes"]').filter({visible:true}).first().click();
  const card=page.locator('.class-management-card').filter({has:page.getByRole('heading',{name:'三年级（1）班',exact:true})});
  await card.getByRole('button',{name:'连接大屏'}).click();
  await page.getByRole('button',{name:'协同老师'}).click();
  await page.getByRole('button',{name:'生成协同码'}).click();
  const invite=(await page.locator('.invite-result code').innerText()).trim();
  await page.locator('.invite-result').getByRole('button',{name:'复制',exact:true}).click();
  const manual=page.getByRole('dialog',{name:'手动复制'});
  await expect(manual).toBeVisible();
  const field=manual.getByLabel('待复制内容');
  await expect(field).toHaveValue(invite);
  await field.click();
  expect(await field.evaluate((element:HTMLTextAreaElement)=>[element.selectionStart,element.selectionEnd])).toEqual([0,invite.length]);
});

test('并发授权失败只发起一次刷新并重试请求',async({page})=>{
  await teacherLogin(page);
  await page.locator('a[href="/classes"]').filter({visible:true}).first().click();
  await expect(page.getByRole('heading',{name:'我的班级',exact:true})).toBeVisible();
  const failed=new Set<string>();
  let refreshes=0;
  page.on('request',request=>{if(request.url().endsWith('/api/v1/auth/refresh'))refreshes++;});
  await page.route(/\/api\/v1\/classes\/[^/]+\/(devices|members)$/,async route=>{
    const url=route.request().url();
    if(!failed.has(url)){
      failed.add(url);
      await route.fulfill({status:401,contentType:'application/json',body:JSON.stringify({code:'UNAUTHORIZED',message:'过期'})});
    }else await route.continue();
  });
  await page.route('**/api/v1/auth/refresh',async route=>{
    await new Promise(resolve=>setTimeout(resolve,100));
    await route.continue();
  });
  const card=page.locator('.class-management-card').filter({has:page.getByRole('heading',{name:'三年级（1）班',exact:true})});
  await card.getByRole('button',{name:'连接大屏'}).click();
  await expect.poll(()=>failed.size).toBe(2);
  await page.getByRole('button',{name:'协同老师'}).click();
  await expect(page.getByText('林老师（你）')).toBeVisible();
  expect(refreshes).toBe(1);
});

test('同一浏览器的管理端和教师端会话独立保持',async({page,context})=>{
  await teacherLogin(page);
  const admin=await context.newPage();
  await adminLogin(admin);
  await page.reload();
  await expect(page.getByRole('heading',{name:'选择接收班级'})).toBeVisible();
  await admin.reload();
  await expect(admin.getByRole('heading',{name:'工作概览',exact:true})).toBeVisible();
  const cookies=await context.cookies();
  expect(cookies.some(cookie=>cookie.name==='classding_refresh_teacher')).toBe(true);
  expect(cookies.some(cookie=>cookie.name==='classding_refresh_admin')).toBe(true);
  const logout=admin.waitForResponse(response=>response.url().endsWith('/api/v1/auth/logout'));
  await admin.getByRole('button',{name:'退出登录'}).filter({visible:true}).first().click();
  await logout;
  await page.reload();
  await expect(page.getByRole('heading',{name:'选择接收班级'})).toBeVisible();
  await admin.close();
});
