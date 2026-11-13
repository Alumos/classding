# 代码结构

本文说明各部分代码放在哪里、遵守什么约定。接口细节见 [`INTEGRATION.md`](INTEGRATION.md)，网页界面见 [`WEB_UI.md`](WEB_UI.md)。

## 仓库布局

| 目录 | 内容 |
| --- | --- |
| `server/` | Go 主服务：REST 接口、WebSocket、SQLite 持久化，可内嵌两个网页端 |
| `screen/` | Windows 大屏客户端：Wails 外壳（Go）与 `frontend/`（React） |
| `web/apps/teacher`、`web/apps/admin` | 教师工作台与管理后台 |
| `web/packages/ui` | 共享 shadcn/ui 组件、通知画面、站点设置、版本号 |
| `web/packages/api` | 接口客户端与生成的 TypeScript 类型 |
| `scripts/` | 接口契约生成、开发服务启动、静态文件准备 |
| `tests/e2e` | Playwright 浏览器测试 |

版本号只写在根目录 `package.json`，前端通过 `@classding/ui/version` 读取，接口契约生成脚本也从这里读取；Go 服务的版本是 `server/internal/app/models.go` 中的 `Version`，两处随发版一起修改。

## 主服务（`server/internal/app`）

整个数据库以 `State` 的形式常驻内存，由 `App.mu` 保护；每次成功的写操作后，`repo.Store` 只把变化的行写回 SQLite。

| 文件 | 职责 |
| --- | --- |
| `models.go` | 实体、状态常量（角色、账号状态、授权状态、投递状态）与视图类型 `ClassView`、`DeviceView` |
| `handler.go` | 请求管线：访问级别、事务、请求体缓冲、错误与 JSON 输出 |
| `routes.go` | 全部路由，一行一个接口，标明访问级别与读写 |
| `app.go` | 启动、旧数据升级、限流、审计、会话撤销 |
| `auth.go`、`security.go`、`validate.go` | 登录与刷新会话、令牌与密码哈希、输入校验 |
| `classes.go`、`devices.go`、`messages.go`、`homework.go`、`student_calls.go`、`audio.go`、`tools.go` | 各业务领域 |
| `admin.go`、`settings.go`、`backup.go` | 管理后台、站点设置、备份与恢复 |
| `realtime.go`、`hub.go` | WebSocket 协议与连接池 |
| `maintenance.go` | 定时清理过期会话、消息和录音 |
| `migration.go` | 0.3.0 之前的大屏迁移核对接口 |

几条约定：

- **路由声明权限。** `a.query(level, fn)` 只读，`a.command(level, fn)` 写入。`level` 取 `public`、`signedIn`、`adminOnly`、`superAdminOnly` 之一，处理函数不再自行判断角色。学校管理员的组织范围统一由 `User.Governs(orgID)` 判断。教师工作台的接口只认班级成员身份（`canClass`、`owns`），管理员也不例外；管理后台接口只认学校范围（`governedBy`）。
- **命令是事务。** 处理函数返回错误，或保存失败时，内存状态、绑定码、发送冷却和本次响应设置的 Cookie 都会回滚。推送消息、断开连接这类副作用用 `deferJob` 登记，提交成功后才执行；`deferRollback` 用于失败时清理已写出的文件。
- **存储与视图分开。** `Class`、`Device` 只含持久化字段；人数、在线状态等运行时信息只出现在 `ClassView`、`DeviceView` 中。
- **状态用具名常量。** 不在代码里直接写 `"super_admin"`、`"queued"` 这样的字符串；`DeliveryStatus.Pending()` 判断一条投递是否还可能出现在大屏上。

## 大屏客户端（`screen/`）

| 文件 | 职责 |
| --- | --- |
| `app.go` | 窗口与托盘生命周期、全屏播报 |
| `proxy.go` | 本机回环代理：网页视图经它访问服务器，换服务器无需刷新页面 |
| `storage.go` | 以 Windows DPAPI 加密保存前端状态 |
| `displays.go`、`volume.go`、`network.go` | 显示器选择、播报时的系统音量、内网 IPv4 |
| `homework*.go` | 桌面作业卡片（Win32 GDI 绘制） |
| `frontend/src/service.ts` | 连接、离线队列、播报、服务器切换与回退的状态机 |

`service.ts` 中所有状态修改都经 `mutate()` 串行执行并立即持久化，失败时整体回滚。每次重连使 `generation` 加一，旧连接上迟到的消息会被丢弃。

## 网页端

- 页面数据类型一律引用 `components['schemas'][...]` 生成类型，不手写接口结构。
- 投递状态的文案与配色只在 `@classding/ui/lib/delivery` 定义一次。
- 列表统一用 `usePagination` 与 `PaginationBar` 分页。

## 代码风格与检查

- TypeScript、CSS 与 JSON 使用 Prettier（`pnpm format`），Go 使用 `gofmt`；CI 会检查两者。
- 生成文件（`api/`、`web/packages/api/src/types.ts`、`ws-types.ts`）只能通过 `pnpm api:generate` 更新，CI 会核对。
- 提交前运行：`pnpm format:check`、`pnpm typecheck`、`pnpm test`、`go vet ./...` 与 `go test -race ./...`（`server/` 和 `screen/` 各一次）。
