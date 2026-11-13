import { expect, test, type Page, type WebSocketRoute } from '@playwright/test';
import type { StudentCall } from '../../web/apps/teacher/src/models';

const teacherURL = 'http://127.0.0.1:15173';
const call = (id = 'call-1'): StudentCall => ({
  id,
  class_id: 'class-1',
  class_name: '一年级（1）班',
  device_id: 'device-1',
  device_name: '教室大屏',
  created_at: '2026-11-13T01:00:00Z',
  read_at: '',
  read_by: '',
  read_by_name: '',
});

async function mockCalls(page: Page, initial = [call()]) {
  const state = {
    items: initial,
    reads: 0,
    gets: 0,
    legacy: false,
    account: 'teacher-a',
    loggedIn: true,
    cursor: false,
  };
  const sockets = new Set<WebSocketRoute>();
  await page.routeWebSocket('**/ws/teacher', (socket) => {
    sockets.add(socket);
    socket.onMessage((message) => {
      const frame = JSON.parse(String(message));
      if (frame.t === 'hello') socket.send(JSON.stringify({ t: 'welcome', d: {} }));
      if (frame.t === 'ping') socket.send(JSON.stringify({ t: 'pong', d: {} }));
    });
    socket.onClose(() => sockets.delete(socket));
  });
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace('/api/v1', '');
    const user = { id: state.account, username: state.account, display_name: '测试老师', ui_scale: 1 };
    if (path === '/auth/logout') {
      state.loggedIn = false;
      return route.fulfill({ json: { ok: true } });
    }
    if (path === '/auth/login') {
      state.loggedIn = true;
      return route.fulfill({ json: { access_token: state.account, user } });
    }
    if (path === '/auth/refresh' || path === '/me')
      return route.fulfill(
        state.loggedIn
          ? { json: path === '/me' ? user : { access_token: state.account, user } }
          : { status: 401, json: { code: 'UNAUTHORIZED', message: '请登录' } },
      );
    if (path === '/student-calls') {
      state.gets++;
      if (state.legacy)
        return route.fulfill({ status: 404, json: { code: 'NOT_FOUND', message: 'unknown endpoint' } });
      const later = new URL(route.request().url()).searchParams.has('cursor');
      return route.fulfill({
        json: {
          items: state.cursor ? state.items.slice(later ? 1 : 0, later ? undefined : 1) : state.items,
          unread_count: state.items.filter((item) => !item.read_at).length,
          next_cursor: state.cursor && !later ? 'older-page' : null,
        },
      });
    }
    if (path.endsWith('/read')) {
      state.reads++;
      const id = path.split('/')[2];
      state.items = state.items.map((item) =>
        item.id !== id || item.read_at
          ? item
          : {
              ...item,
              read_at: '2026-11-13T01:02:00Z',
              read_by: 'teacher-b',
              read_by_name: '李老师',
            },
      );
      return route.fulfill({ json: state.items.find((item) => item.id === id) });
    }
    if (path === '/public/site') return route.fulfill({ json: {} });
    return route.fulfill({ json: [] });
  });
  const emit = (kind: 'created' | 'read', id = 'call-1') => {
    for (const socket of sockets)
      socket.send(JSON.stringify({ t: 'student_call', d: { id, class_id: 'class-1', kind } }));
  };
  return { state, sockets, emit };
}

function navigation(page: Page) {
  return page.locator((page.viewportSize()?.width ?? 1440) < 768 ? '.bottom-nav' : '.side-nav');
}

test('学生呼叫打开不自动已读，显式处理共享结果并刷新恢复，后页角标同步', async ({ page }) => {
  const { state } = await mockCalls(page, [call(), call('call-2')]);
  state.cursor = true;
  await page.goto(`${teacherURL}/student-calls`);
  const nav = navigation(page).locator('a[href="/student-calls"]');
  await expect(nav).toHaveAttribute('aria-label', '学生呼叫，2 条未读');
  await expect(page.locator('.student-call-card')).toHaveCount(1);
  await expect(page.getByText('打开页面不会自动标记已读。', { exact: false })).toBeVisible();
  expect(state.reads).toBe(0);
  await page.getByRole('button', { name: '加载更早的呼叫' }).click();
  const older = page.locator('[data-call-id="call-2"]');
  await expect(older).toBeVisible();
  expect(state.reads).toBe(0);
  await older.getByRole('button', { name: '标记已读' }).click();
  await expect(older).toContainText('李老师 已处理');
  await expect(nav).toHaveAttribute('aria-label', '学生呼叫，1 条未读');
  expect(state.reads).toBe(1);
  await page.reload();
  await page.getByRole('button', { name: '加载更早的呼叫' }).click();
  await expect(older).toContainText('已处理');
  await expect(older.getByRole('button', { name: '标记已读' })).toHaveCount(0);
  expect(state.reads).toBe(1);
});

test('学生呼叫 WS 实时更新只提醒新增，处理后共享消角标，断线重连补查', async ({ page }) => {
  const { state, sockets, emit } = await mockCalls(page, []);
  await page.goto(`${teacherURL}/student-calls`);
  await expect(page.getByText('实时连接已开启')).toBeVisible();
  state.items = [call()];
  emit('created');
  await expect(page.locator('.student-call-card')).toHaveCount(1);
  await expect(page.locator('[data-sonner-toast]')).toContainText('一年级（1）班的学生正在呼叫老师');
  emit('created');
  await expect(page.locator('[data-sonner-toast]')).toHaveCount(1);
  state.items = [
    { ...call(), read_at: '2026-11-13T01:02:00Z', read_by: 'teacher-b', read_by_name: '李老师' },
  ];
  emit('read');
  await expect(page.locator('.student-call-card')).toContainText('李老师 已处理');
  await expect(navigation(page).locator('a[href="/student-calls"]')).toHaveAttribute(
    'aria-label',
    '学生呼叫',
  );
  expect(state.reads).toBe(0);
  for (const socket of sockets) socket.close();
  state.items.unshift(call('missed-call'));
  await expect(page.locator('[data-call-id="missed-call"]')).toBeVisible();
});

test('学生呼叫旧服务404停止自动重试且不刷toast，手动刷新可恢复', async ({ page }) => {
  const { state } = await mockCalls(page);
  state.legacy = true;
  await page.clock.install();
  await page.goto(`${teacherURL}/student-calls`);
  await expect(page.getByText('当前服务暂未开放学生呼叫')).toBeVisible();
  const requests = state.gets;
  await page.clock.fastForward(120000);
  expect(state.gets).toBe(requests);
  await expect(page.locator('[data-sonner-toast]')).toHaveCount(0);
  state.legacy = false;
  await page.getByRole('button', { name: '刷新', exact: true }).click();
  await expect(page.locator('.student-call-card')).toHaveCount(1);
});

test('学生呼叫断开实时通道仍轮询补查', async ({ page }) => {
  const { state, sockets } = await mockCalls(page, []);
  await page.clock.install();
  await page.goto(`${teacherURL}/student-calls`);
  await expect(page.getByText('还没有学生呼叫')).toBeVisible();
  for (const socket of sockets) socket.close();
  state.items = [call('polled-call')];
  await page.clock.fastForward(31000);
  await expect(page.locator('[data-call-id="polled-call"]')).toBeVisible();
});

test('学生呼叫手机六项导航两行三列，标准与大字号不横向拥挤', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 760 });
  await mockCalls(page);
  await page.goto(`${teacherURL}/student-calls`);
  await expect(page.locator('.student-call-card')).toHaveCount(1);
  for (const size of [16, 24]) {
    await page.evaluate((size) => {
      document.documentElement.style.fontSize = `${size}px`;
    }, size);
    const links = page.locator('.bottom-nav a');
    await expect(links).toHaveCount(6);
    const boxes = await links.evaluateAll((elements) =>
      elements.map((item) => {
        const rect = item.getBoundingClientRect();
        return { x: rect.x, y: rect.y, width: rect.width, height: rect.height, right: rect.right };
      }),
    );
    expect(boxes[0].y).toBe(boxes[2].y);
    expect(boxes[3].y).toBe(boxes[5].y);
    expect(boxes[3].y).toBeGreaterThan(boxes[0].y);
    expect(boxes.every((box) => box.height >= 44 && box.width >= 80 && box.right <= 321)).toBe(true);
    await expect(page.locator('.bottom-nav a[href="/student-calls"]')).toContainText('学生呼叫');
    await page.getByRole('button', { name: '标记已读' }).scrollIntoViewIfNeeded();
    await expect(page.getByRole('button', { name: '标记已读' })).toBeInViewport();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
  }
});

test('学生呼叫退出换账号不泄漏列表角标和提醒', async ({ page }) => {
  const { state, emit } = await mockCalls(page);
  await page.goto(`${teacherURL}/student-calls`);
  await expect(page.locator('.student-call-card')).toHaveCount(1);
  emit('created');
  await expect(page.locator('[data-sonner-toast]')).toHaveCount(1);
  await navigation(page).locator('a[href="/me"]').click();
  await page.getByRole('button', { name: '退出当前账号' }).click();
  await expect(page.getByRole('button', { name: '进入工作台' })).toBeVisible();
  await expect(page.locator('[data-sonner-toast]')).toHaveCount(0);
  state.account = 'teacher-c';
  state.items = [];
  await page.getByLabel('用户名', { exact: true }).fill('teacher-c');
  await page.getByLabel('密码', { exact: true }).fill('test-password');
  await page.getByRole('button', { name: '进入工作台' }).click();
  await navigation(page).locator('a[href="/student-calls"]').click();
  await expect(page.getByText('还没有学生呼叫')).toBeVisible();
  await expect(page.locator('.student-call-unread')).toHaveCount(0);
  await expect(page.locator('.student-call-card')).toHaveCount(0);
});
