import { expect, test, type APIRequestContext, type Page } from '@playwright/test';

const apiURL = 'http://127.0.0.1:18080';
const teacherURL = 'http://127.0.0.1:15173';
const password = 'StudentCallFixture2026';
const auth = (token: string) => ({ Authorization: 'Bearer ' + token });

async function signIn(api: APIRequestContext, username: string, secret = password, scope = 'teacher') {
  const response = await api.post('/api/v1/auth/login', {
    headers: { 'X-ClassDing-Auth-Scope': scope },
    data: { username, password: secret },
  });
  expect(response.status()).toBe(200);
  return (await response.json()).access_token as string;
}
async function login(page: Page, username: string) {
  await page.goto(teacherURL + '/login');
  await page.getByLabel('用户名', { exact: true }).fill(username);
  await page.getByLabel('密码', { exact: true }).fill(password);
  await page.getByRole('button', { name: '进入工作台' }).click();
  await expect(page.getByText('实时连接已开启')).toBeVisible();
}

test('真实设备呼叫向两位老师实时送达，打开不自动已读且共享处理持久保存', async ({
  page,
  browser,
  playwright,
}, info) => {
  const api = await playwright.request.newContext({ baseURL: apiURL });
  const otherContext = await browser.newContext({ viewport: info.project.use.viewport });
  const other = await otherContext.newPage();
  const screen = await otherContext.newPage();
  const suffix = Date.now().toString(36) + (info.project.name === 'mobile' ? 'm' : 'd');
  const ownerName = 'call.owner.' + suffix;
  const colleagueName = 'call.peer.' + suffix;
  let classId = '';
  const userIds: string[] = [];
  let adminToken = '';
  try {
    adminToken = await signIn(api, 'admin', 'AdminDing2026!', 'admin');
    for (const [username, display_name] of [
      [ownerName, '呼叫班主任'],
      [colleagueName, '呼叫任课老师'],
    ]) {
      const response = await api.post('/api/v1/admin/users', {
        headers: auth(adminToken),
        data: { username, password, display_name },
      });
      expect(response.status()).toBe(200);
      userIds.push((await response.json()).id);
    }
    const createdClass = await api.post('/api/v1/admin/classes', {
      headers: auth(adminToken),
      data: { name: '呼叫验收班' + suffix, owner_id: userIds[0], max_teachers: 2, max_devices: 1 },
    });
    expect(createdClass.status()).toBe(200);
    classId = (await createdClass.json()).id;
    const ownerToken = await signIn(api, ownerName);
    const colleagueToken = await signIn(api, colleagueName);
    const invite = await api.post('/api/v1/classes/' + classId + '/invites', { headers: auth(ownerToken) });
    expect(invite.status()).toBe(200);
    const joined = await api.post('/api/v1/classes/join', {
      headers: auth(colleagueToken),
      data: { code: (await invite.json()).code },
    });
    expect(joined.status()).toBe(200);
    const bindCode = await api.post('/api/v1/classes/' + classId + '/bind-code', {
      headers: auth(ownerToken),
    });
    expect(bindCode.status()).toBe(200);
    await screen.goto('http://127.0.0.1:15175');
    await screen.getByLabel('6 位班级绑定码').fill((await bindCode.json()).code);
    await screen.getByRole('button', { name: '设备与服务器设置', exact: true }).click();
    await screen.getByLabel('设备名称', { exact: true }).fill('呼叫验收大屏');
    await screen.getByRole('button', { name: '连接班级', exact: true }).click();
    await expect(screen.getByText('连接正常', { exact: true })).toBeVisible();
    await login(page, ownerName);
    await login(other, colleagueName);
    let callRequests = 0;
    screen.on('request', (request) => {
      if (request.url().endsWith('/api/v1/device/student-calls') && request.method() === 'POST')
        callRequests++;
    });
    await screen.getByRole('button', { name: '呼叫老师', exact: true }).click();
    const confirmation = screen.getByRole('dialog');
    await expect(confirmation.getByText('确认呼叫老师？', { exact: true })).toBeVisible();
    await screen.screenshot({ path: info.outputPath('student-call-confirmation.png'), fullPage: true });
    await confirmation.getByRole('button', { name: '取消', exact: true }).click();
    await expect(confirmation).toBeHidden();
    expect(callRequests).toBe(0);
    const afterCancel = await api.get('/api/v1/student-calls', { headers: auth(ownerToken) });
    expect(await afterCancel.json()).toMatchObject({ items: [], unread_count: 0 });
    await screen.getByRole('button', { name: '呼叫老师', exact: true }).click();
    const sent = screen.waitForResponse(
      (response) =>
        response.url().endsWith('/api/v1/device/student-calls') && response.request().method() === 'POST',
    );
    await confirmation.getByRole('button', { name: '确认呼叫', exact: true }).click();
    const response = await sent;
    expect(response.status()).toBe(200);
    expect(callRequests).toBe(1);
    await expect(confirmation.getByRole('status')).toContainText('呼叫已发送');
    const requestId = response.request().postDataJSON().request_id as string;
    const deviceAuthorization = response.request().headers()['authorization'];
    const send = () =>
      api.post('/api/v1/device/student-calls', {
        headers: { Authorization: deviceAuthorization },
        data: { request_id: requestId },
      });
    const call = await response.json();
    expect((await (await send()).json()).id).toBe(call.id);
    const tooSoon = await api.post('/api/v1/device/student-calls', {
      headers: { Authorization: deviceAuthorization },
      data: { request_id: requestId + '-new' },
    });
    expect(tooSoon.status()).toBe(429);
    expect(Number(tooSoon.headers()['retry-after'])).toBeGreaterThan(0);
    for (const teacher of [page, other]) {
      const entry = teacher
        .getByRole('link', { name: '学生呼叫，1 条未读', exact: true })
        .filter({ visible: true });
      await expect(entry).toBeVisible({ timeout: 8000 });
      await entry.click();
      await expect(teacher.getByRole('heading', { name: '学生呼叫', exact: true })).toBeVisible();
      await expect(teacher.locator('[data-call-id="' + call.id + '"]')).toContainText('呼叫验收大屏');
      await expect(teacher.getByRole('button', { name: '标记已读' })).toBeVisible();
    }
    // Viewing and reloading must never mutate shared unread state.
    await page.reload();
    await expect(page.getByRole('button', { name: '标记已读' })).toBeVisible();
    await page.screenshot({ path: info.outputPath('student-call-unread.png'), fullPage: true });
    const before = await api.get('/api/v1/student-calls', { headers: auth(ownerToken) });
    expect((await before.json()).unread_count).toBe(1);
    await other.getByRole('button', { name: '标记已读' }).click();
    for (const teacher of [page, other]) {
      const card = teacher.locator('[data-call-id="' + call.id + '"]');
      await expect(card).toContainText('呼叫任课老师 已处理', { timeout: 8000 });
      await expect(card.getByRole('button', { name: '标记已读' })).toHaveCount(0);
      await expect(teacher.getByText('0 条未读', { exact: true })).toBeVisible();
    }
    await page.reload();
    await expect(page.locator('[data-call-id="' + call.id + '"]')).toContainText('呼叫任课老师 已处理');
    const saved = await api.get('/api/v1/student-calls', { headers: auth(colleagueToken) });
    expect(await saved.json()).toMatchObject({
      unread_count: 0,
      items: [{ id: call.id, read_by: userIds[1] }],
    });
  } finally {
    await otherContext.close();
    if (adminToken && classId) {
      expect(
        (await api.delete('/api/v1/admin/classes/' + classId, { headers: auth(adminToken) })).status(),
      ).toBe(200);
    }
    for (const id of userIds) {
      expect((await api.delete('/api/v1/admin/users/' + id, { headers: auth(adminToken) })).status()).toBe(
        200,
      );
    }
    await api.dispose();
  }
});
