import {expect,test,type Page} from '@playwright/test';

const teacherURL=process.env.CLASSDING_E2E_TEACHER_URL??'http://127.0.0.1:15173';
const scope={'X-ClassDing-Auth-Scope':'teacher'};

async function login(page:Page,username='teacher',password='ClassDing2026!'){
  await page.goto(`${teacherURL}/login`);
  await page.getByLabel('用户名',{exact:true}).fill(username);
  await page.getByLabel('密码',{exact:true}).fill(password);
  await page.getByRole('button',{name:'进入工作台'}).click();
  await expect(page.getByRole('heading',{name:'选择接收班级'})).toBeVisible();
  await page.goto(`${teacherURL}/homework`);
  await expect(page.getByRole('heading',{name:'布置作业',exact:true})).toBeVisible();
}
async function selectClass(page:Page,name:string){
  await page.getByRole('combobox',{name:'选择班级'}).click();
  await page.getByRole('option',{name,exact:true}).click();
}
async function noOverflow(page:Page){
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
}

test('教师发布、修改、切换显示设置并删除作业，布局适应不同屏幕',async({page},info)=>{
  await login(page);
  await selectClass(page,'三年级（1）班');
  await page.getByRole('button',{name:'新建作业'}).first().click();
  let dialog=page.getByRole('dialog',{name:'新建作业'});
  await expect(dialog.getByText('发布老师：林老师')).toBeVisible();
  await expect(dialog.getByRole('switch',{name:'默认显示'})).toBeChecked();
  await expect(dialog.getByRole('switch',{name:'默认置顶'})).toBeChecked();
  await dialog.getByLabel('科目',{exact:true}).fill('字'.repeat(25));
  await dialog.getByLabel('作业内容').fill('练习册第 12 页');
  await expect(dialog.getByRole('button',{name:'发布作业'})).toBeDisabled();
  const subject=`数学${info.project.name}`;
  await dialog.getByLabel('科目',{exact:true}).fill(subject);
  const published=page.waitForResponse(response=>response.url().endsWith('/api/v1/homeworks')&&response.request().method()==='POST');
  await dialog.getByRole('button',{name:'发布作业'}).click();
  const publishedResponse=await published;
  expect(publishedResponse.status()).toBe(200);
  expect(publishedResponse.request().postDataJSON()).toMatchObject({subject,text:'练习册第 12 页',visible:true,always_on_top:true});
  const record=await publishedResponse.json();
  expect(record.teacher_name).toBe('林老师');
  let card=page.locator('.homework-card').filter({has:page.getByRole('heading',{name:subject,exact:true})});
  await expect(card).toContainText('练习册第 12 页');
  await expect(card).toContainText('林老师');
  await expect(card.getByRole('switch',{name:`显示${subject}作业`})).toBeChecked();
  await expect(card.getByRole('switch',{name:`置顶${subject}作业`})).toBeChecked();

  const hide=page.waitForResponse(response=>response.url().endsWith(`/api/v1/homeworks/${record.id}`)&&response.request().method()==='PATCH');
  await card.getByRole('switch',{name:`显示${subject}作业`}).click();
  expect((await hide).request().postDataJSON()).toEqual({visible:false});
  await expect(card.getByRole('switch',{name:`显示${subject}作业`})).not.toBeChecked();
  const unpin=page.waitForResponse(response=>response.url().endsWith(`/api/v1/homeworks/${record.id}`)&&response.request().method()==='PATCH');
  await card.getByRole('switch',{name:`置顶${subject}作业`}).click();
  expect((await unpin).request().postDataJSON()).toEqual({always_on_top:false});
  await expect(card.getByRole('switch',{name:`置顶${subject}作业`})).not.toBeChecked();

  await card.getByRole('button',{name:'编辑'}).click();
  dialog=page.getByRole('dialog',{name:'编辑作业'});
  await dialog.getByLabel('作业内容').fill('完成练习册第 13 页\n明天带到教室');
  await dialog.getByRole('button',{name:'保存修改'}).click();
  await expect(card).toContainText('完成练习册第 13 页');
  await expect(card.getByRole('switch',{name:`显示${subject}作业`})).not.toBeChecked();

  await expect(page.locator('[data-sonner-toast]')).toHaveCount(0);
  for(const scheme of ['light','dark'] as const){
    await page.emulateMedia({colorScheme:scheme});
    for(const width of [320,390,820,1440]){
      await page.setViewportSize({width,height:900});
      await noOverflow(page);
      if(width<768){
        const positions=await page.locator('.bottom-nav a').evaluateAll(links=>links.map(link=>{const rect=link.getBoundingClientRect();return {top:rect.top,left:rect.left,right:rect.right};}));
        expect(positions).toHaveLength(5);
        expect(new Set(positions.map(position=>position.top)).size).toBe(1);
        for(let index=1;index<positions.length;index++)expect(positions[index].left).toBeGreaterThanOrEqual(positions[index-1].right-1);
      }
      await expect(card).toBeVisible();
      await page.screenshot({path:info.outputPath(`homework-${width}-${scheme}.png`),fullPage:true});
    }
  }
  card=page.locator('.homework-card').filter({has:page.getByRole('heading',{name:subject,exact:true})});
  await card.getByRole('button',{name:'删除'}).click();
  await page.getByRole('dialog',{name:'删除这条作业？'}).getByRole('button',{name:'确认删除'}).click();
  await expect(card).toHaveCount(0);
  // Reload verifies the deletion using the authenticated application session.
  await page.reload();
  await selectClass(page,'三年级（1）班');
  await expect(page.getByRole('heading',{name:subject,exact:true})).toHaveCount(0);
});

test('获取作业失败时可重试并恢复真实数据',async({page})=>{
  await login(page);
  await page.route('**/api/v1/homeworks?*',route=>route.fulfill({status:503,contentType:'application/json',body:JSON.stringify({code:'UNAVAILABLE',message:'暂时无法获取作业'})}));
  await page.reload();
  await expect(page.getByRole('alert')).toContainText('暂时无法获取作业');
  await page.unroute('**/api/v1/homeworks?*');
  await page.getByRole('button',{name:'重试'}).click();
  await expect(page.getByRole('heading',{name:'班级作业'})).toBeVisible();
});

test('任课老师只能改自己的作业，班级创建者可管理全部并实时收到更新',async({page,browser,playwright},info)=>{
  const ownerAPI=await playwright.request.newContext({baseURL:teacherURL,extraHTTPHeaders:scope});
  const collaboratorAPI=await playwright.request.newContext({baseURL:teacherURL,extraHTTPHeaders:scope});
  const otherContext=await browser.newContext();
  const other=await otherContext.newPage();
  let classId='';let memberId='';const itemIds:string[]=[];
  try{
    const ownerLogin=await ownerAPI.post('/api/v1/auth/login',{data:{username:'teacher',password:'ClassDing2026!'}});
    expect(ownerLogin.ok()).toBe(true);
    const ownerToken=(await ownerLogin.json()).access_token as string;
    const ownerAuth={Authorization:`Bearer ${ownerToken}`};
    const classResponse=await ownerAPI.get('/api/v1/classes',{headers:ownerAuth});
    const classList=await classResponse.json() as {id:string;name:string}[];
    classId=classList.find(item=>item.name==='三年级（1）班')!.id;
    const invite=await ownerAPI.post(`/api/v1/classes/${classId}/invites`,{headers:ownerAuth});
    expect(invite.ok()).toBe(true);
    const code=(await invite.json()).code as string;
    const username=`hw${Date.now()}${info.project.name==='mobile'?'m':'d'}`;
    const registered=await collaboratorAPI.post('/api/v1/auth/register',{data:{username,password:'Teacher2026!',display_name:'协同老师'}});
    expect(registered.ok()).toBe(true);
    const collaborator=await registered.json();memberId=collaborator.user.id;
    const collaboratorAuth={Authorization:`Bearer ${collaborator.access_token}`};
    const joined=await collaboratorAPI.post('/api/v1/classes/join',{headers:collaboratorAuth,data:{code}});
    expect(joined.ok()).toBe(true);

    await login(page);await selectClass(page,'三年级（1）班');
    await login(other,username,'Teacher2026!');
    await expect(page.getByText('实时连接已开启')).toBeVisible();
    await expect(other.getByText('实时连接已开启')).toBeVisible();
    const create=async(subject:string,headers:Record<string,string>)=>{
      const response=await ownerAPI.post('/api/v1/homeworks',{headers,data:{class_id:classId,subject,text:'实时更新测试',visible:true,always_on_top:true}});
      expect(response.ok()).toBe(true);const item=await response.json();itemIds.push(item.id);return item;
    };
    const ownerItem=await create(`班主任${info.project.name}`,ownerAuth);
    const ownerCard=other.locator('.homework-card').filter({has:other.getByRole('heading',{name:ownerItem.subject,exact:true})});
    await expect(ownerCard).toBeVisible({timeout:8000});
    await expect(ownerCard.getByText('仅查看')).toBeVisible();
    await expect(ownerCard.getByRole('switch',{name:`显示${ownerItem.subject}作业`})).toBeDisabled();
    await expect(ownerCard.getByRole('button',{name:'编辑'})).toHaveCount(0);
    const forbidden=await collaboratorAPI.patch(`/api/v1/homeworks/${ownerItem.id}`,{headers:collaboratorAuth,data:{text:'无权修改'}});
    expect(forbidden.status()).toBe(403);
    const collaboratorItem=await create(`英语${info.project.name}`,collaboratorAuth);
    const collaboratorCard=page.locator('.homework-card').filter({has:page.getByRole('heading',{name:collaboratorItem.subject,exact:true})});
    await expect(collaboratorCard).toBeVisible({timeout:8000});
    await expect(collaboratorCard.getByRole('button',{name:'编辑'})).toBeVisible();
    await expect(other.locator('.homework-card').filter({has:other.getByRole('heading',{name:collaboratorItem.subject,exact:true})}).getByRole('button',{name:'编辑'})).toBeVisible();
  }finally{
    const ownerLogin=await ownerAPI.post('/api/v1/auth/login',{data:{username:'teacher',password:'ClassDing2026!'}});
    if(ownerLogin.ok()){
      const token=(await ownerLogin.json()).access_token as string;
      const headers={Authorization:`Bearer ${token}`};
      for(const id of itemIds)await ownerAPI.delete(`/api/v1/homeworks/${id}`,{headers});
      if(classId&&memberId)await ownerAPI.delete(`/api/v1/classes/${classId}/members/${memberId}`,{headers});
    }
    await otherContext.close();await ownerAPI.dispose();await collaboratorAPI.dispose();
  }
});
