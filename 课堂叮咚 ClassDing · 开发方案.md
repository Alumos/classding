# 课堂叮咚 ClassDing · 开发方案

Nov 3, 2026 · @Alumos

## 1. 项目概述与命名

项目定名 **课堂叮咚（英文 ClassDing）**：老师在手机上发一句话，教室大屏先"叮咚"一声再全屏显示并朗读。名字直接取自产品最有辨识度的动作，家长、老师一听就懂，英文名可直接做仓库名、域名和镜像名。

| 用途 | 名称 |
| --- | --- |
| GitHub 仓库 | `classding` |
| 服务端镜像 | `classding-server`、`classding-speech` |
| 教室客户端 | 课堂叮咚大屏端（`ClassDingScreen.exe`） |
| 教师端 / 后台 | 课堂叮咚（Web，PWA） / 课堂叮咚管理后台 |

- **目标客户**：中国大陆中小学。面向个人班主任和学校整体采购，统一用运营方生成的激活码开通，不内置支付。
- **对标产品**：小红书"Lee的班级工具箱"的「班级小广播」（19.8 元 / 2 个班，买断）。
- **差异点**：从一开始就支持"多老师 ↔ 多班级"的多对多关系和学校租户；面向 Win10+ 与 4K 大屏优化；服务端 Docker 化，CI 自动出包。
- 备选名：班级小喇叭、叮咚班级、ClassBell。

## 2. 竞品拆解（来自 6 张宣传图与笔记正文）

竞品是典型的"手机 H5 发送端 + 云端中转 + 教室 Windows 常驻客户端"三段式架构，客户端用 6 位绑定码与班级一次性配对。下表是从图中能确认的功能，以及我们的对应做法。

| 模块 | 竞品表现（图中可见） | 课堂叮咚做法 |
| --- | --- | --- |
| 手机端入口 | 浏览器打开网址，微信内也可用，不装 App；用户名+密码注册 | 同：Web/PWA，兼容微信内置浏览器 |
| 底部导航 | 发通知 / 班级 / 记录 / 我的 | 手机保留同样 4 栏；平板、电脑换侧边栏 |
| 发通知 | 多选班级（含全选）；打字 / 按住说话两种模式；正文限 200 字；快捷短语（请到办公室、请带上作业到办公室等）；学生姓名 + 落款（王老师）；发送设置"显示 1 分钟 · 朗读 2 遍"；大屏效果预览 | 全部保留；短语可自定义；增加样式（通知 / 表扬 / 紧急）和撤回 |
| 大屏展示 | 全屏卡片：左上"通知"标签、右上班级名、学生姓名绿色高亮、左下落款+时间、底部进度条 | 同样版式，按 4K 重新排字号 |
| 播报 | 先"叮咚"再自动朗读，或播老师原声；按班设置播报音量，播前自动调音量/解除静音，播完恢复 | 同（见第 8、9 节） |
| 回执 | 手机显示"已播放" | 分级回执：已送达 / 已播放 / 离线未送达 |
| 语音转文字 | 页脚注明使用 SenseVoice 模型（阿里 FunASR 开源） | 可选：开启语音服务时自建 SenseVoice，数据不出服务器 |
| 绑定 | 手机"班级→绑定码"显示 6 位数字，电脑端弹窗输入；绑一次，开机自动接收 | 同；一个班可绑多台屏 |
| 教室客户端 | 小于 1～2 MB；双击安装、无需管理员权限；装 D 盘规避还原卡；开机自启；桌面绿色喇叭图标；支持希沃/鸿合一体机、Win7～Win11 | 只支持 Win10+，其余照做（体积约 10 MB，见第 8 节） |
| 我的 | 激活码 / 协同码合一输入框；"已用 3/3 个班级名额"；安装说明、客服微信、改密码、隐私说明、退出 | 同 |
| 商业模式 | 19.8 元开 2 个班、买断；每班免费 2 位协同老师；加班、加协同老师各 4.9 元；赠送课堂点名 + 倒计时 | 不内置支付；后台生成激活码，一码开通一块大屏并限定教师数（见第 10 节） |

结论：竞品的核心价值在"发得快、听得到、装得上"，技术难点集中在教室端（免权限安装、静音处理、置顶覆盖 PPT）和长连接可靠性。

## 3. 总体架构

系统由 4 个可独立开发的客户端/服务组成：Go 主服务（REST + WebSocket）、可选的语音服务、Web 前端（教师端 + 管理后台）、Windows 大屏客户端。前后端之间只通过 `api/` 目录里的契约文件对接，方便分组并行。

&#91;embedded content: 系统架构 · 3 类客户端、CDN、单台 VPS（语音可选）\]

三类客户端都只和 CDN 打交道，源站不暴露；主服务是唯一的业务入口，SQLite、本地文件和可选的语音服务都由它调用。

### 3.1 技术栈

| 层 | 选型 | 说明 |
| --- | --- | --- |
| 主服务 | Go 1.23+、chi 路由、coder/websocket、sqlc + modernc.org/sqlite（纯 Go 驱动）、goose 迁移 | 无 CGO，静态编译，镜像用 distroless |
| 数据库 | SQLite（WAL 模式，单文件） | 业务数据、消息、激活码；嵌在主服务进程里，不单独部署 |
| 缓存/总线 | 进程内存 | 绑定码、限流、在线连接表（带过期的内存 map），不再部署 Redis |
| 语音服务（可选） | Go + sherpa-onnx（CGO，单独镜像） | SenseVoice 语音识别、中文 TTS；默认不部署，大屏用 Windows 自带语音 |
| 文件存储 | 服务器本地磁盘 /data 目录 | 原声录音、TTS 音频、客户端安装包、数据库快照；异地备份可选 S3 兼容存储 |
| Web 前端 | Node 22、pnpm、Vite + React 19 + TypeScript、Tailwind CSS v4、shadcn/ui、TanStack Query、React Router | 纯静态产物，走 CDN |
| 大屏客户端 | Go + Wails v2（WebView2），前端复用 React/Tailwind | NSIS 单用户安装包，免管理员 |
| 契约 | OpenAPI 3.1（oapi-codegen / openapi-typescript 生成代码）+ WebSocket JSON Schema | 先定契约再开发 |
| CI/CD | GitHub Actions、GHCR + 阿里云 ACR 镜像同步 | 见第 12 节 |

为什么用 SQLite：数据基本是文本，即使给多所学校使用，数据量和写入量也很小，SQLite 免运维，备份就是一个文件。代价是主服务只能单实例运行。只有将来需要多实例、高可用部署时，再迁到 PostgreSQL + Redis；为此所有 SQL 只放在 repo 层（sqlc 生成），业务代码不直接写 SQL。

### 3.2 仓库结构（monorepo）

```
classding/
├─ api/                 # 契约：openapi.yaml、ws/*.schema.json、错误码表
├─ server/              # Go 主服务
│  ├─ cmd/classding/    # main
│  ├─ internal/{auth,org,class,device,message,license,realtime,admin,storage}
│  └─ migrations/
├─ speech/              # Go + sherpa-onnx 语音服务
├─ web/                 # pnpm workspace
│  ├─ apps/teacher/     # 教师端
│  ├─ apps/admin/       # 管理后台
│  ├─ packages/ui/      # shadcn 组件 + 主题，三端共用
│  └─ packages/api/     # 生成的 TS 客户端 + WS 类型
├─ screen/              # Wails 大屏客户端（frontend/ 引用 packages/ui）
├─ deploy/              # docker-compose.yml、Caddyfile、.env.example
└─ .github/workflows/
```

### 3.3 一条通知的链路

1. 老师在教师端选中班级、输入文字或录音，`POST /api/v1/messages`。
2. 主服务校验老师对每个班的权限，写入 `messages` 与 `message_targets`，开启了语音服务则生成 TTS 音频（有缓存则复用），否则交给大屏本地朗读。
3. 在进程内找到该设备的连接，经 WebSocket 推到教室客户端。
4. 客户端回 `delivered` → 调音量 → 播叮咚 → 全屏展示 + 朗读 N 遍 → 回 `played` → 恢复音量。
5. 主服务更新状态，经教师端的 WebSocket 实时推送回执；设备离线则排队，重连后补发，超时未送达的标记为过期。

## 4. 领域模型与数据库

老师与班级是多对多关系，由中间表 `class_members` 承载：一个老师可以是多个班的成员，一个班可以有一位创建者（owner）和多位协同老师（collaborator）。设备挂在班级上而不是老师上，换班主任时只需转让 owner，教室电脑不用重绑。

### 4.1 实体关系

- `orgs` 1—N `classes`；`orgs` 1—N `users`（个人用户归入自己的 personal org）
- `users` N—N `classes`（经 `class_members`，带角色）
- `classes` 1—N `devices`（一个班可绑多块屏）
- `messages` 1—N `message_targets`（一条消息可发多个班，每个班/设备一行状态）

### 4.2 表结构（核心字段）

| 表 | 关键字段 | 说明 |
| --- | --- | --- |
| `orgs` | id, name, type(personal/school) | 学校租户或个人空间 |
| `users` | id, org\_id, username(唯一), password\_hash(argon2id), display\_name(落款"王老师"), role(user/school\_admin/super\_admin), status | 教师账号 |
| `classes` | id, org\_id, owner\_id, name(三年二班), grade, volume(0–100), activation\_code, settings(JSON 文本), deleted\_at | 班级 |
| `class_members` | class\_id, user\_id, role(owner/collaborator), signature, joined\_at；PK(class\_id, user\_id) | **多对多核心** |
| `collab_invites` | code, class\_id, created\_by, expires\_at, max\_uses, used | 协同码 |
| `devices` | id, class\_id, token\_hash, name, os, client\_version, last\_seen\_at, last\_ip, bound\_at, revoked\_at | 教室电脑 |
| `messages` | id, sender\_id, kind(text/voice/tool), text, student\_name, signature, style, voice\_audio\_key, tts\_audio\_key, display\_sec, repeat, chime, expires\_at, recalled\_at | 通知本体 |
| `message_targets` | message\_id, class\_id, device\_id, status(queued/delivered/played/dismissed/expired/failed), seq, delivered\_at, played\_at | 每台设备一行状态 |
| `phrases` | id, user\_id, text, sort | 快捷短语 |
| `activation_codes` | code, max\_teachers, max\_devices, expires\_at, org\_id, batch\_id, note, status, class\_id, redeemed\_by, redeemed\_at | 激活码 |
| `license_events` | id, code, action(create/redeem/adjust/revoke), before, after, actor, at | 激活码变更记录 |
| `client_releases` | version, channel, url, sha256, min\_supported, notes | 客户端版本 |
| `audit_logs` | actor, action, target, ip, at | 审计 |

短期数据放在主服务进程内存（带过期时间的 map）：绑定码 → class\_id（10 分钟过期，用后即删）、设备在线表、限流计数器。服务重启后未用的绑定码失效，老师重新点一次即可。

### 4.3 权限矩阵

| 操作 | owner | collaborator | 学校管理员 |
| --- | --- | --- | --- |
| 向本班发通知、看本班记录 | 是 | 是 | 是 |
| 撤回通知 | 任意 | 仅自己发的 | 任意 |
| 生成绑定码、解绑设备、调播报音量 | 是 | 否 | 是 |
| 生成协同码、移除协同老师、转让 owner | 是 | 否 | 是 |
| 退出班级 | 须先转让 | 是 | — |
| 删除班级（激活码释放） | 是 | 否 | 是 |

规则：新建班级必须核销一个未使用的激活码；该班 owner 与协同老师合计不超过码上的 `max_teachers`，绑定的教室电脑不超过 `max_devices`。发送时对每个目标班逐一校验成员身份，一次可发多个班。

## 5. 后端主服务（Go）

主服务是一个 Go 二进制，同时提供 `/api/v1/*` REST、`/ws/*` 长连接和健康检查；持久状态都落在一个 SQLite 文件里，按单实例运行。

### 5.1 内部模块

| 包 | 职责 |
| --- | --- |
| `auth` | 注册/登录、argon2id 密码、访问令牌（JWT，15 分钟）+ 刷新令牌（httpOnly Cookie，30 天，轮换）、登录限流 |
| `org` | 学校租户、学校管理员批量建班/导入教师 |
| `class` | 班级 CRUD、成员、协同码、转让 owner、权限校验中间件 |
| `device` | 绑定码生成/核销、设备令牌、解绑、在线状态 |
| `message` | 发送、重发、撤回、历史、过期扫描（每 30 秒一次的后台任务） |
| `realtime` | WebSocket 网关、连接表（见第 6 节） |
| `license` | 激活码核销、教师席位与大屏数校验、到期判断 |
| `speech` | 语音服务客户端与 TTS 缓存（未配置 SPEECH\_URL 时整体关闭） |
| `storage` | 本地文件读写（/data）、限时签名下载链接；接口预留 S3 实现，用于异地备份 |
| `admin` | 管理后台接口（仅 super\_admin / school\_admin） |

约定：分层 handler → service → repo（sqlc）；统一错误体 `{"code":"CLASS_QUOTA_EXCEEDED","message":"..."}`；结构化日志用 `log/slog`；指标暴露 `/metrics`（Prometheus）；配置全部走环境变量。

### 5.2 REST API 清单（前缀 `/api/v1`）

| 分组 | 接口 |
| --- | --- |
| 认证 | `POST /auth/register`、`POST /auth/login`、`POST /auth/refresh`、`POST /auth/logout`、`PUT /me/password` |
| 我的 | `GET /me`（含所在班级）、`PATCH /me`（落款名）、`POST /classes/join`（输入协同码加入班级） |
| 班级 | `GET/POST /classes`、`PATCH/DELETE /classes/{id}`、`POST /classes/{id}/transfer` |
| 成员 | `GET /classes/{id}/members`、`POST /classes/{id}/invites`（生成协同码）、`DELETE /classes/{id}/members/{uid}`、`POST /classes/{id}/leave` |
| 设备（教师侧） | `POST /classes/{id}/bind-code`、`GET /classes/{id}/devices`、`PATCH /devices/{id}`（改名）、`DELETE /devices/{id}`（解绑） |
| 设备（客户端侧） | `POST /device/bind`（提交 6 位码 → 返回 device\_token）、`GET /device/me`、`GET /client/latest` |
| 通知 | `POST /messages`、`GET /messages?class_id=&cursor=`、`GET /messages/{id}`（含各班回执）、`POST /messages/{id}/resend`、`POST /messages/{id}/recall` |
| 语音 | `POST /audio`（上传录音；开启语音服务时同时转写）、`POST /tts/preview` |
| 短语 | `GET/POST /phrases`、`PATCH/DELETE /phrases/{id}` |
| 课堂工具 | `POST /classes/{id}/tools/countdown`、`POST /classes/{id}/tools/rollcall`、`POST /classes/{id}/tools/stop` |
| 后台 | `/admin/*`（见第 11 节） |

`POST /messages` 请求体示例：

```json
{
  "class_ids": ["c_01", "c_02"],
  "kind": "text",
  "text": "请带上作业到办公室",
  "student_name": "李强",
  "signature": "王老师",
  "style": "notice",
  "display_sec": 60,
  "repeat": 2,
  "chime": true,
  "play_mode": "tts"
}
```

`play_mode` 取 `tts`（文字朗读）或 `voice`（播老师原声，需先上传得到 `audio_id`）。安全要求：绑定接口按 IP 限 5 次/分钟，登录按账号+IP 限流，`text` 过敏感词过滤（见第 13 节）。

## 6. 实时通道（WebSocket）

设备与教师端各走一条 WebSocket，采用"先落库、再推送、靠回执确认、断线按序号补发"的至少一次投递模型；客户端按 `message_id` 去重，保证不重播。

### 6.1 端点与鉴权

- `wss://域名/ws/device`：教室客户端。连接后 5 秒内必须发 `hello` 帧携带 device\_token，否则断开。**令牌不放 URL**，避免被 CDN 访问日志记录。
- `wss://域名/ws/teacher`：教师端，同样用 `hello` 帧携带访问令牌，仅用于接收回执与设备在线状态。

### 6.2 帧格式

```json
{ "t": "notify", "id": "m_8f3a", "seq": 1042, "ts": 1793670000, "d": { } }
```

| 方向 | t | d 主要字段 |
| --- | --- | --- |
| 设备→服务 | `hello` | token, client\_version, os, last\_seq |
| 设备→服务 | `ack` | message\_id, status(delivered/playing/played/dismissed/failed), err |
| 设备→服务 | `ping` | — |
| 服务→设备 | `welcome` | class{id,name}, volume, server\_time, 最新版本号 |
| 服务→设备 | `notify` | message\_id, style, text, student\_name, signature, sender, class\_name, audio\_url, chime, repeat, display\_sec, volume, sent\_at |
| 服务→设备 | `recall` | message\_id（未播则丢弃，正在显示则关闭） |
| 服务→设备 | `tool` | name(countdown/rollcall/stop), params |
| 服务→设备 | `config` | volume、显示器选择等变更 |
| 服务→设备 | `unbind` | —（客户端清令牌，回到绑定页） |
| 服务→设备/教师 | `pong` | server\_time |
| 服务→教师 | `receipt` | message\_id, class\_id, device\_id, status, at |
| 服务→教师 | `presence` | class\_id, device\_id, online |

### 6.3 可靠性规则

1. 客户端每 25 秒发应用层 `ping`；60 秒收不到任何帧即判定断线重连。心跳间隔必须小于 CDN WebSocket 空闲超时的一半，具体以所选 CDN 控制台配置为准。
2. 重连退避：1s、2s、4s…封顶 30s，加 ±30% 随机抖动，避免全校断网恢复时同时涌入。
3. `seq` 按设备递增。`hello.last_seq` 之后仍为 queued 且未过期的消息在 `welcome` 后按序补发。
4. 消息默认 10 分钟过期（可配）：叫人类通知过时再播没意义，过期后教师端显示"教室电脑离线，未送达"。
5. 同一设备新连接建立时踢掉旧连接（防止重复接收）。

### 6.4 单实例与 CDN

- 单实例：设备连接表在进程内存，发送时直接找到连接推送。SQLite 只允许一个进程写，主服务只跑一个实例；单校几十到上百块屏完全够用。
- CDN 规则：`/assets/*` 长缓存（文件名带 hash）；`index.html` 不缓存；`/api/*` 不缓存、动态回源；`/ws/*` 开启 WebSocket 并调大超时。
- 回源全程 HTTPS；源站防火墙只放行 CDN 回源 IP 段；服务端只信任 CDN 传来的真实 IP 头（如 `X-Forwarded-For`），用于限流和审计。
- 预估容量：单台 2 核 4 GB 的 Go 实例维持数千条空闲 WebSocket 压力很小，单实例即可。

## 7. 教师端 Web

教师端是一个纯静态 SPA（Vite 构建），支持加到主屏的 PWA，必须在微信内置浏览器、iOS Safari、安卓 Chrome/国产浏览器、桌面 Chrome/Edge 上可用。UI 全部基于 `packages/ui` 里的 shadcn/ui 组件，主色沿用绿色系但重新定义设计令牌。

### 7.1 页面

| 路由 | 内容 | 主要 shadcn 组件 |
| --- | --- | --- |
| `/login`、`/register` | 用户名+密码；注册时填落款名 | Card, Form, Input, Button |
| `/send` 发通知 | 班级多选（含全选、显示在线点）、打字/说话切换、200 字文本框、快捷短语、学生姓名、落款、发送设置（显示时长、朗读次数、提示音、样式）、大屏预览、底部固定"发送到 ××班"按钮 | ToggleGroup, Tabs, Textarea, Badge, Collapsible, Select, Slider, Sonner |
| `/classes` 班级 | 班级列表与设备在线状态、新建班级（填激活码和班名）；详情页：绑定码（大号 6 位数字 + 倒计时）、设备列表与解绑、播报音量、协同老师（生成协同码、移除）、课堂工具（点名、倒计时远程控制） | Card, Dialog/Drawer, InputOTP（展示）, Slider, AlertDialog |
| `/history` 记录 | 按时间倒序，每条显示各班回执（已送达/已播放/未送达），可重发、撤回；按班筛选 | Table（桌面）/ 列表卡片（手机）, Badge |
| `/me` 我的 | 头像与落款名、协同码输入、客户端下载与安装说明、客服、改密码、隐私政策、退出 | Card, Input, Button |

### 7.2 响应式布局

| 断点（Tailwind） | 设备 | 布局 |
| --- | --- | --- |
| < 768px | 手机 | 底部 4 栏导航；发送页单列，预览放在可折叠区；弹窗用 Drawer（底部抽屉） |
| 768–1279px | 平板 | 可收起的图标侧边栏；发送页左右两栏（表单 / 大屏预览） |
| ≥ 1280px | 电脑 | 完整侧边栏（shadcn Sidebar）；记录页用表格；弹窗用 Dialog |

细节要求：触控目标不小于 44px；适配 iPhone 安全区（`env(safe-area-inset-bottom)`）；暗色模式跟随系统；所有状态用 TanStack Query 管理，WebSocket 回执到达后直接更新缓存。

### 7.3 按住说话

- 用 `MediaRecorder` 录音（安卓多为 webm/opus，iOS 多为 mp4/aac），最长 30 秒，松手上传，开启语音服务时由服务端转码并转写；关闭时原声直接下发，不出转写文字。
- 开启语音服务时，上传后返回转写文字填入文本框，老师可修改，并选"朗读文字"或"播我的原声"。
- 微信内若无法获取麦克风，提示用输入法自带的语音输入，或在浏览器中打开。上线前需在 iOS / 安卓微信中实测。

## 8. 教室大屏客户端（Windows 10+）

客户端用 Go + Wails v2 构建：系统相关能力（音量、自启、置顶窗口、长连接）在 Go 侧，展示界面用 WebView2 + React，和教师端的"大屏预览"共用同一套组件，保证所见即所得。

### 8.1 选型理由与取舍

| 方案 | 安装包体积 | 优点 | 缺点 |
| --- | --- | --- | --- |
| **Wails v2（推荐）** | 约 8–12 MB | 纯 Go、界面可复用 React、4K 排版和动画容易做 | 依赖 WebView2（Win11 自带；Win10 绝大多数已装，缺失时安装包内置引导程序） |
| Go + Win32 原生窗口 | 约 3–5 MB | 体积小、无运行时依赖 | 自绘 UI 成本高，样式难与预览保持一致 |
| Electron | 60 MB+ | 生态最全 | 体积和内存对一体机不友好 |

竞品宣传的"不到 1MB"无法用 Wails 达到。若这是硬性卖点，改走 Go + Win32 原生方案，工作量约增加 1 人周。

### 8.2 安装与运行

- **免管理员**：NSIS 安装包设 `RequestExecutionLevel user`，只写当前用户目录与 `HKCU`。
- **规避还原卡/影子系统**：安装时自动检测可写的 D/E 盘，默认装到 `D:\ClassDing\`；令牌和配置也存在安装目录下的 `data\`（用 Windows DPAPI 加密），这样 C 盘还原后仍保持绑定。注意：自启注册表在 C 盘，还原后会丢，因此同时写"启动"文件夹快捷方式，并在使用说明里写明"还原卡需将本软件加入保护例外"。
- **开机自启**：`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`，带 `--silent` 参数启动到托盘。
- **单实例**：命名互斥量；重复启动时唤起已有实例。
- **守护**：Go 侧自带崩溃重启（主进程监控 WebView 渲染进程），日志滚动保留 7 天。
- **自动更新**：启动和每 6 小时查 `GET /client/latest`，从自己服务器的 /downloads（经 CDN）下载、校验 sha256，在无播报时静默替换重启。

### 8.3 界面状态

| 状态 | 表现 |
| --- | --- |
| 未绑定 | 居中小窗"绑定到班级"，6 格数字输入，粘贴自动分格（同竞品） |
| 待命 | 只在托盘显示绿色喇叭图标；右键菜单：当前班级、连接状态、选择显示器、测试播报、解绑、退出；离线时图标变灰 |
| 通知 | 无边框、置顶、全屏窗口（可覆盖 PPT 放映和希沃白板），版式同竞品；点击任意处或到时自动关闭；多条排队依次播，右上角显示"还有 N 条" |
| 课堂工具 | 倒计时：大号数字，结束响铃；点名：名单滚动后定格（名单来自班级花名册，可选功能） |

### 8.4 4K 与多显示器

- 开启 Per-Monitor DPI v2；全部尺寸用 `vh`/`vw` + `clamp()`，以 3840×2160 为设计稿。正文约 9vh、标签与落款约 2.5vh，确保教室最后一排可读；超长文字自动缩字号。
- 字体内置思源黑体子集（常用 3500 字，约 1–2 MB），缺字回退微软雅黑，避免不同一体机字体不一致。
- 默认在主显示器弹出；接了多块屏时可在托盘菜单或教师端选择显示器，按显示器硬件 ID 记忆。

### 8.5 音量与播放

1. 收到 `notify`，用 Windows Core Audio（`IAudioEndpointVolume`，Go 侧用 go-ole/go-wca）读取当前默认输出设备的音量和静音状态并保存。
2. 取消静音，把音量调到班级设置值；若老师在放视频，可选将其他应用音量压低（会话级音量，第二期做）。
3. 播叮咚 → 朗读/原声 × N 遍，与屏幕展示同步开始。
4. 播完恢复原音量与静音状态，回 `played`。未开启服务端语音、或音频下载失败时，用 Windows 自带中文语音（SAPI）本地朗读。

## 9. 语音（可选模块）

语音不是核心功能，做成可配置项：默认不部署语音服务，大屏用 Windows 自带中文语音朗读；需要统一音色或"按住说话转文字"时，再在配置里打开。

| 配置 | 朗读（TTS） | 按住说话 | 服务器要求 |
| --- | --- | --- | --- |
| 关闭（默认） | 大屏客户端用 Windows SAPI 中文语音本地朗读 | 只发原声，不转文字 | 无额外要求 |
| 开启 | 服务端 sherpa-onnx 中文 TTS 生成音频，各教室音色一致；失败时回退本地朗读 | 原声 + SenseVoice 转写成文字 | 语音容器，建议 4 核 8 GB |

开关方式：主服务环境变量 `SPEECH_URL`，留空即关闭；`GET /api/v1/config` 把开关下发给教师端，关闭时隐藏"转文字"相关按钮。

实现要点：

- **本地朗读**：中文版 Win10/11 一般自带中文语音。客户端启动时检测有无 zh-CN 语音，没有则在后台设备列表标红，并提示安装 Windows 中文语音包。
- **朗读内容**：学生姓名 + 正文；朗读次数、语速在发送设置里指定。
- **服务端 TTS（开启时）**：按 `sha256(文本+音色+语速)` 缓存音频；超过 1.5 秒未生成就先推 `notify`，客户端改为本地朗读，保证从点发送到大屏出声 ≤ 2 秒。
- **原声**：WebView2 可直接播放 webm/opus 与 mp4/aac，关闭语音服务时原样下发；开启时顺便转码并做响度归一化。
- 录音文件存在服务器 `/data` 目录，7 天后自动删除。

## 10. 激活码与授权（不内置支付）

系统不做在线支付。授权全部靠运营方在管理后台生成的激活码：**一个激活码开通一块班级大屏（即一个班级），并规定这块大屏最多可绑定几位教师**。码与班级永久对应，换教室电脑不需要新码。

### 10.1 激活码字段

| 字段 | 含义 | 示例 |
| --- | --- | --- |
| code | `XXXX-XXXX`，去掉易混的 0/O/1/I/L | H7KQ-2M9X |
| max\_teachers | 可绑定这块大屏的教师上限（含创建者） | 3 |
| max\_devices | 该班可同时绑定的教室电脑数，默认 1 | 1 |
| expires\_at | 授权到期日，空 = 长期 | 空 |
| org\_id / batch\_id / note | 所属学校、批次、备注 | 某某小学 2026 秋 |
| status | unused / active / revoked | unused |
| class\_id | 核销后关联的班级 | — |
| redeemed\_by / redeemed\_at | 核销教师与时间 | — |

### 10.2 开通流程

1. 运营方在后台按批次生成码（数量、教师上限、大屏数、有效期、备注），导出 CSV 发给老师或学校。
2. 老师在教师端「班级 → 新建班级」输入激活码和班名，成为该班 owner，占 1 个教师席位。
3. owner 点「绑定码」得到 6 位数字，在教室电脑客户端输入，大屏上线。
4. owner 生成协同码发给任课老师；任课老师在「我的」输入协同码加入，直到达到 max\_teachers，超出时接口返回 `TEACHER_LIMIT_REACHED`。
5. 一位老师可以用多个激活码建多个班，也可以作为协同老师加入任意多个班，多对多关系不受影响。

### 10.3 规则

- 席位按"教师 × 大屏"计：owner 和协同老师都占席位；老师退出或被移除即释放。
- 换教室电脑：owner 解绑旧设备、重新生成绑定码即可，激活码与班级不变。
- 运营方可在后台随时调整某个码的教师上限、大屏数、有效期，或吊销；吊销后大屏停止接收，教师端提示联系管理员。
- 到期后大屏不再弹出新通知，教师端仍可看历史；续期只需在后台改到期日。
- 老师删除班级后，激活码回到"已释放"状态，由运营方决定是否允许重新使用。
- 协同码 7 天有效、单次使用；激活码核销接口按账号限 10 次/小时，防枚举；核销加行锁，防并发重复使用。
- 学校整体采购：在后台为学校批量生成码并挂到该校 org，学校管理员可看本校所有码的使用情况。

## 11. 管理后台

后台是 `web/apps/admin`，与教师端共用 `packages/ui`，桌面优先、平板可用、手机可查看。同一套页面按角色裁剪：运营方（super\_admin）看全局，学校管理员（school\_admin）只看本校。

| 模块 | 功能 | 可见角色 |
| --- | --- | --- |
| 概览 | 在线设备数、今日发送数、送达率、平均送达耗时、新增用户 | 两者 |
| 学校 | 创建学校、指定学校管理员、查看本校激活码使用情况 | super\_admin |
| 教师 | 搜索、禁用、重置密码、查看所属班级；学校管理员可批量导入（Excel） | 两者 |
| 班级 | 列表、成员、转让 owner、强制解绑设备；学校管理员可批量建班（按年级×班号生成） | 两者 |
| 设备 | 在线状态、客户端版本分布、最后心跳、IP、远程发测试播报 | 两者 |
| 激活码 | 批量生成（数量、教师上限、大屏数、有效期、所属学校、备注）、导出 CSV、调整上限与到期、吊销、核销记录 | super\_admin |
| 消息审计 | 按学校/班级/时间查看消息与回执（用于投诉处理），访问也记审计日志 | 两者 |
| 客户端版本 | 发布新版本、灰度比例、最低支持版本 | super\_admin |
| 系统设置 | 敏感词库、默认 TTS 音色、消息过期时间、公告 | super\_admin |
| 审计日志 | 后台所有写操作 | super\_admin |

后台登录强制二次验证（TOTP），建议后台域名不走 CDN 公开或加 IP 白名单。

## 12. 部署与 CI/CD

打 `v*` 标签即自动产出服务端镜像和 Windows 安装包；镜像同时推到 GHCR 和国内镜像仓库，安装包同时发到 GitHub Release 和自己服务器的 /downloads 目录。从大陆 VPS 和学校网络拉 GitHub 资源常常很慢，所以国内副本是必需的。

### 12.1 GitHub Actions 工作流

| 文件 | 触发 | 内容 |
| --- | --- | --- |
| `ci.yml` | PR、push 到 main | Go：`go vet`、golangci-lint、`go test -race`（数据库用临时 SQLite 文件，无需外部服务）；Web：pnpm install、typecheck、lint、vitest、build；契约：校验 OpenAPI 并检查生成代码是否已提交 |
| `server-image.yml` | `v*` 标签、手动 | docker buildx 构建 linux/amd64 + linux/arm64；推到 `ghcr.io/<org>/classding-server` 和阿里云 ACR；同样构建 `classding-speech`；生成 SBOM |
| `screen-client.yml` | `v*` 标签、手动 | `windows-latest` 上安装 Go、Node、Wails CLI、NSIS → `wails build -nsis -platform windows/amd64` → 代码签名 → 上传 Release + 推送到服务器 /downloads → 调用后台接口登记新版本（默认 0% 灰度） |
| `deploy.yml`（可选） | 镜像构建成功后手动确认 | SSH 到 VPS 执行 `docker compose pull && docker compose up -d`，先迁移数据库再切换 |

Secrets：`ACR_USERNAME/ACR_PASSWORD`、`BACKUP_S3_*（可选）`、`CODESIGN_CERT/CODESIGN_PASSWORD`、`DEPLOY_SSH_KEY`、`ADMIN_API_TOKEN`。

### 12.2 服务端镜像

多阶段 Dockerfile：`node:22-alpine` 构建教师端和后台静态文件 → `golang` 阶段用 `go:embed` 把静态文件打进二进制（`CGO_ENABLED=0`）→ `gcr.io/distroless/static` 运行，镜像约 20–30 MB。这样一个镜像就能跑全部 Web；也可以把静态文件单独上传到对象存储，由 CDN 直接回源。

### 12.3 VPS 上的 docker compose

| 容器 | 镜像 | 说明 |
| --- | --- | --- |
| caddy | caddy:2 | TLS 终结、反代 `/api` `/ws` 和静态文件，只对 CDN 回源 IP 开放 443 |
| server | classding-server | 主服务，单实例；SQLite 文件放在数据卷 `/data`，每日用 `VACUUM INTO` 生成快照，可选再传到异地 S3 兼容存储，保留 30 天 |
| speech | classding-speech | 可选，放在 compose 的 speech profile 里，默认不启动 |

建议起步规格：不开语音 2 核 2 GB 足够；开启语音建议 4 核 8 GB。监控用 Uptime Kuma 做外部拨测，另加 Prometheus + Grafana 看在线数与送达耗时。

### 12.4 CDN 接入清单

- 选支持 WebSocket 的国内 CDN / 全站加速产品（阿里云、腾讯云等均有），开通前确认其 WebSocket 开关与超时上限。
- 一个域名即可（如 `app.classding.cn`），按路径配缓存规则（见 6.4）。
- 开 HTTP/2、Brotli、强制 HTTPS；关闭 `/api` `/ws` 的参数忽略和缓存。
- 客户端安装包放在主服务 /downloads 路径，由 CDN 缓存。

## 13. 安全与合规

面向大陆学校，上线前至少要解决备案和未成年人个人信息保护两件事。以下为工程侧应做的准备，具体法律要求请在上线前咨询专业人士确认。

### 13.1 备案与资质

- 服务器在境内、或使用境内 CDN 加速的域名，需要完成 ICP 备案；网站上线后还需办理公安联网备案。
- 学校采购常见要求：软件著作权登记证书（软著），部分地区还会要等保测评或教育 App 备案，建议提前了解目标地区要求。
- Windows 客户端做代码签名，并提前向 360、腾讯电脑管家等提交白名单，否则"开机自启 + 全屏置顶 + 改音量"很容易被拦截。

### 13.2 个人信息

- 通知内容经常带学生姓名，属于未成年人个人信息。最小化收集：默认不建学生花名册（点名功能由老师自行开启）。
- 消息与回执默认保留 90 天，录音 7 天，到期自动物理删除；保留期在后台可配。
- 语音识别和 TTS 默认自建，数据不出服务器；如切换云厂商，需在隐私政策中写明第三方。
- 提供隐私政策、用户协议、账号注销（删除数据）入口。

### 13.3 安全基线

| 风险 | 措施 |
| --- | --- |
| 绑定码被枚举，陷生人向教室推内容 | 6 位码 10 分钟过期、一次性；接口按 IP 限 5 次/分钟 + 全局限流；绑定成功后通知 owner |
| 不当内容上大屏 | 敏感词过滤；每条消息留存发送人；发送频率限制（如每班 1 条/5 秒） |
| 设备令牌泄露 | 仅存哈希；客户端用 DPAPI 加密存储；可在教师端一键解绑吊销 |
| 账号被盗 | argon2id、登录限流、异地登录提醒；后台 TOTP |
| 源站暴露 | 源站只放行 CDN 回源 IP，SSH 用密钥 + 非标准端口 |
| 依赖漏洞 | Dependabot、`govulncheck`、`pnpm audit` 进 CI |

## 14. 模块拆分、分工与里程碑

按 7 个模块分工，第 1 周先由后端负责人产出契约（OpenAPI + WS Schema + 数据库迁移），之后各模块对着契约和 Mock 并行开发。下表的人周为粗估，用于排期参考。

| 模块 | 内容（对应章节） | 技能 | 依赖 | 交付物 | 粗估 |
| --- | --- | --- | --- | --- | --- |
| M0 契约 | OpenAPI、WS Schema、错误码、迁移脚本（§4、§5、§6） | Go/架构 | 无 | `api/` 目录 + Mock Server | 1 人周 |
| M1 后端业务 | 认证、班级/成员、设备绑定、通知、授权（§5、§10） | Go | M0 | `classding-server` + 单元测试 | 3 人周 |
| M2 实时网关 | WS、投递与补发、回执（§6） | Go | M0 | `internal/realtime` + 压测报告 | 2 人周 |
| M3 语音服务 | 可选模块，可延后到 v1.0 之后（§9） | Go + CGO / 音频 | M0 | `classding-speech` + 延迟实测 | 1.5 人周 |
| M4 教师端 | 4 个主页面、响应式、录音（§7） | React/shadcn | M0 | `web/apps/teacher` + `packages/ui` | 3 人周 |
| M5 管理后台 | §11 全部页面 | React/shadcn | M0、M4 的 ui 包 | `web/apps/admin` | 2 人周 |
| M6 大屏客户端 | 绑定、展示、音量、自启、更新（§8） | Go/Win32 + React | M0 | NSIS 安装包 + 希沃/鸿合实机测试报告 | 3 人周 |
| M7 DevOps | Actions、镜像、compose、CDN、备份、监控（§12） | DevOps | M1、M6 可构建 | `.github/workflows`、`deploy/` | 1.5 人周 |

协作规则：契约改动必须走 PR 并 @ 相关模块负责人；前端和客户端的 TS 类型一律从契约生成，禁止手写接口类型；每个模块自带 README 说明本地启动方式。

### 里程碑

&#91;embedded content: 里程碑 · 4 个阶段、4 个闸口\]

合计约 17 人周；第 1 周契约冻结是关键闸口，它一旦延误，后面所有模块都会顺延。

### 验收标准（MVP）

- [ ] 一位老师绑定 2 个班，另一位老师通过协同码加入其中 1 个班，两人都能发送且权限符合§4.3
- [ ] 一次选中 2 个班发送，两块屏均在 2 秒内叮咚并朗读，手机显示"已播放"
- [ ] 教室电脑拔网线 3 分钟再插回，期间发的通知补播；超过 10 分钟的显示未送达
- [ ] 电脑静音状态下播报正常出声，播完恢复静音
- [ ] 普通用户账号安装客户端不弹 UAC；重启后自动上线
- [ ] 4K 大屏教室最后一排可看清正文；PPT 放映中能弹出覆盖
- [ ] 手机、平板、电脑三种宽度下教师端无横向滚动、无遮挡
- [ ] 打 `v0.1.0` 标签后自动产出镜像和安装包，VPS 一条命令完成升级

## 15. 待定事项

- [ ] VPS 放境内（需 ICP 备案，延迟低）还是香港（免备案，但境内 CDN 仍要求域名备案）？本方案按境内 + 备案设计。
- [ ] 客户端体积：接受 Wails 的约 10 MB，还是必须像竞品一样 2 MB 以内（改用 Win32 原生）？
- [ ] 首发是否就做学校层（激活码挂到学校、学校管理员可见），还是先只做单码管理？影响 M1 的 org 层和 M5 后台工作量。
- [ ] 是否需要服务端语音（统一音色、按住说话转文字）？不需要则 M3 整体延后，大屏用 Windows 自带语音。
- [ ] 是否需要学生花名册（姓名自动补全、点名）？涉及未成年人信息，默认不做。
- [ ] 代码签名证书（OV 或 EV）的采购主体与预算。
- [ ] 正式域名与商标检索（"课堂叮咚"、"ClassDing"）。
