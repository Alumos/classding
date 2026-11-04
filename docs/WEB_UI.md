# Web 界面重构说明（shadcn/ui）

v0.2.0 起，教师工作台（`web/apps/teacher`）和管理后台（`web/apps/admin`）改用 [shadcn/ui](https://ui.shadcn.com/) 的 Base UI 版本重写。旧版的自定义 `.cd-*` 控件已移入 Windows 大屏客户端（`screen/frontend/src/ui.tsx`、`ui.css`）。本文记录重写时必须保持的行为，供后续维护参考。

## 组件库结构

| 位置 | 内容 |
| --- | --- |
| `web/packages/ui/components.json` | shadcn 配置：`base-nova` 风格、中性色、lucide 图标；别名指向 `@classding/ui/*` |
| `web/packages/ui/src/components/ui/*` | 由 shadcn CLI 生成的组件，尽量不手改，便于以后用 CLI 升级 |
| `web/packages/ui/src/components/classding.tsx` | 项目自有组件：品牌、页脚、复制按钮、页面标题、加载与空状态 |
| `web/packages/ui/src/styles/globals.css` | shadcn 主题变量；站点主题（绿 / 蓝 / 灰）只替换 `--primary` |
| `web/packages/ui/src/styles/shadcn.css` | 从 `shadcn@4.23.0` 内联的 Base UI 状态变体，避免把 CLI 装进运行依赖 |
| `web/packages/ui/src/notice.tsx`、`notice.css` | 教室通知画面，大屏客户端和教师端预览共用 |
| `web/packages/ui/src/lib/delivery.ts` | 投递状态的文案与徽标配色，两端统一 |
| `web/packages/ui/src/version.ts` | 版本号，唯一来源是根目录 `package.json` |

新增组件：在仓库根目录运行 `npx shadcn@4.23.0 add <组件> -c web/packages/ui`。CLI 会把工具函数引用写成 `from "cn"`，添加后需改为 `from "@classding/ui/lib/utils"`。

深色模式：`SiteSettingsProvider` 根据系统设置给 `<html>` 加 `dark` 类；站点主题写在 `data-cd-theme` 上。

## 会话与数据（两端共用）

- `@classding/api` 的 `setAuthScope('teacher' | 'admin')` 在入口调用，两端使用独立刷新 Cookie。
- 启动时 `refreshSession()` 恢复登录；`onSessionExpired` 回调中清空缓存并回到登录页。
- 管理后台每 13 分钟刷新一次会话，用户或角色变化时强制退出。
- 站点配置来自 `GET /api/v1/config`，修改后派发 `classding:site-settings` 事件，两端即时刷新名称、备案号和主题。

## 教师工作台

管理员账号也可以登录教师工作台，但只能看到和操作自己加入的班级；管理所辖学校的全部班级请使用管理后台。

路由：`/login`、`/register`、`/send`（默认）、`/classes`、`/homework`、`/history`、`/me`。

- **实时通道**：`/ws/teacher`，先发送 `hello` 并携带 access token。收到 `welcome` 后每 25 秒 ping，60 秒无帧就重连。`receipt` 刷新消息，`homework` 刷新作业，`presence` 刷新班级与设备。断线后按指数退避重连。
- **发通知**：`POST /messages`，字段包括 `class_ids, kind, text, student_name, signature, style, display_sec, repeat, chime, play_mode, audio_id`。
  - 正文最多 200 字。落款固定为当前教师姓名，只读。
  - 文字通知默认不朗读（`repeat: 0`），可选 0–3 遍；原声通知播放 1–3 遍。
  - 只有紧急样式可以开启提示音，切换样式时提示音复位。
  - 录音上传 `POST /audio`（multipart），最长 30 秒。
- **班级**：
  - 列表：`GET /classes`。
  - 新建：`POST /classes`（名称 + 激活码）。加入：`POST /classes/join`（协同码）。
  - 详情：设备 `GET /classes/{id}/devices`；绑定码 `POST /classes/{id}/bind-code`（6 位，10 分钟有效）。
  - 成员：`GET /classes/{id}/members`；协同码 `POST /classes/{id}/invites`。
  - 移除成员、解绑设备、转让、退出、删除班级。
  - 课堂工具：`POST /classes/{id}/tools/{countdown|rollcall|stop}`；倒计时由分、秒两个输入框自由设定（1 秒至 120 分钟），另有常用时长快捷按钮。
  - 设置：`PATCH /classes/{id}`（名称、音量）。
- **作业**：
  - 读取：`GET /homeworks?class_id=`，每 30 秒刷新。
  - 增改删：`POST /homeworks`、`PATCH /homeworks/{id}`、`DELETE /homeworks/{id}`。
  - 限制：科目最多 24 字，内容最多 300 字。
  - 权限：作者和班级创建者可以管理，其他人只能查看。
- **发送记录**：`GET /messages?class_id=&cursor=` 按游标加载，前端每页 10 条。重发 `POST /messages/{id}/resend`，撤回 `POST /messages/{id}/recall`，删除 `DELETE /messages/{id}`（发送者或班级创建者，未完成的投递会同时撤回）。
- **个人中心**：
  - 修改落款：`PATCH /me`。修改密码：`PUT /me/password`，成功后退出登录。
  - 客户端下载：`GET /client/latest`。

## 管理后台

基路径 `/admin/`。路由：`/`、`/schools`、`/activation-codes`、`/users`、`/classes`、`/devices`、`/messages`、`/audit-logs`、`/settings`。学校、激活码、操作日志、系统配置四页仅运营管理员（`super_admin`）可见。

- **概览**：`GET /admin/overview`，包括统计卡片、近期趋势图（recharts）和最近通知。
- **学校**：`GET/POST /admin/schools`、`PATCH/DELETE /admin/schools/{id}`。教师、班级、激活码以 `org_id` 归属学校，页面一律显示学校名称（`schools.ts` 的 `useSchools`）。仍有教师、班级或激活码的学校不能删除。
- **激活码**：
  - 读取：`GET /admin/activation-codes?search=&status=`。
  - 生成：`POST /admin/activation-codes`，单次 1–1000 个。
  - 调整额度和有效期、吊销：`PATCH /admin/activation-codes/{code}`。
  - 批量删除：`POST /admin/activation-codes/delete {codes}`，只能删除没有班级使用的激活码，表格中使用中的激活码不可勾选。
  - CSV 导出会防止公式注入，以 `'` 前缀转义。
- **教师**：`/admin/users` 的增删改查，以及解除登录限制 `POST /admin/users/{id}/unlock-login`。用户名 3–32 位；密码 10–128 字节，须含字母和数字。
- **班级**：`/admin/classes` 的增删改查。负责人只能选状态正常、且属于同一学校的教师。
- **设备**：`GET /admin/devices` 返回大屏上报的 `lan_ips`（内网 IPv4）、`server_url`（实际连接的服务器）、`public_ip`（服务端看到的出口地址）和 `applied_revision`。批量切换服务器调用 `POST /admin/devices/{id}/config`，见 `deviceConfig.ts`；界面要求在确认页再次核对地址。大屏收到后直接切换，新站不认可设备或 10 分钟内无法连接时回到原服务器；若原服务器看到设备在线，且 `applied_revision` 等于配置版本，就显示「切换失败，已回退」。
- **消息审计**：`GET /admin/messages?cursor=` 按游标加载，前端按关键词和日期筛选后分页显示；`DELETE /admin/messages/{id}` 删除记录，未完成的投递会同时撤回。
- **系统配置**：
  - 站点信息：`PUT /admin/settings`。
  - 备份：`GET /admin/backup`，返回 ZIP，前端会校验文件头。
  - 恢复：`POST /admin/restore`，须输入“覆盖恢复”确认；文件最大 256 MiB；成功后退出登录。

## 布局约定

- 两端都使用 shadcn `Sidebar`（`collapsible="icon"`）：桌面固定在视口内，不随内容拉长；手机端改为抽屉。
- 内容区最大宽度一致（`max-w-6xl`），页面标题统一使用 `PageHeader`。
- `html` 预留滚动条位置，页面切换不会左右跳动。
- 备案号居中于内容区底部，链接到 `https://beian.miit.gov.cn/`。
- 所有列表和表格都分页显示，不会无限拉长页面：表格每页 10 条，作业卡片每页 6 张；分页用 `usePagination` 与 `PaginationBar`，按游标加载的列表到最后一页时会自动加载下一批。
- 顶栏的字号菜单（小 90% / 标准 100% / 大 115% / 特大 130%）通过 `PATCH /me {ui_scale}` 保存在每个用户的账号上，登录后在任何设备上自动应用；实现方式是修改 `<html>` 的字号，所有基于 rem 的尺寸会一起缩放。

## 测试依赖

浏览器测试主要通过可访问名称定位元素，例如表单标签（“管理员账号”“通知内容”“科目”）、按钮文字（“发送到 1 个班级”“生成激活码”）和标题，重写时保持这些文字不变。仍保留的少量结构标记有 `sidebar-brand`、`class-option`、`class-management-card`、`homework-card`、`history-row`、`invite-result`、`bind-digits`、`person-cell`，以及管理后台表格的 `数据表格` 区域。
