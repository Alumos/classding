# 课堂叮咚 ClassDing

课堂叮咚是面向班级通知的项目：教师通过网页向班级发送文字或原声，教室大屏接收、显示并播报，管理后台负责激活码及基础管理。仓库包括 Go 主服务、教师网页、管理后台和大屏客户端；需求目标见 [开发方案](课堂叮咚%20ClassDing%20·%20开发方案.md)。当前代码是首版实现，使用前仍需按实际环境验收。

## 本地启动

准备 Node.js 22.12 或更高版本、pnpm 11.25.0 和 Go 1.27 或更高版本。在仓库根目录安装依赖：

```sh
pnpm install --frozen-lockfile
```

分别打开终端，在仓库根目录运行：

```sh
pnpm dev:server   # Go 服务，127.0.0.1:8080；启用本地演示数据
pnpm dev          # 教师端，默认 http://localhost:5173
pnpm dev:admin    # 管理后台，默认 http://localhost:5174/admin/
pnpm dev:screen   # 大屏浏览器预览，默认 http://localhost:5175
```

开发服务会将 `/api` 和 `/ws` 转发到本机的 Go 服务。大屏浏览器预览便于检查界面和消息链路，不能代替 Windows 客户端验收。Go 开发脚本优先使用 `tools/go/bin/go`，不存在时使用系统 Go；数据保存在 `server/data/`。

开发模式初次使用空数据目录时提供以下演示数据：

| 用途 | 账号或激活码 | 密码或说明 |
| --- | --- | --- |
| 教师 | `teacher` | `ClassDing2026!`，预置两个班级 |
| 运营管理员 | `admin` | `AdminDing2026!`，未设置 TOTP 密钥时开发模式免验证码 |
| 新建班级 | `TEST-2345` | 一次性演示激活码 |

演示账号和默认密钥只供本地开发。已有同名账号不会被重置，数据目录也会跨启动保留；重新测试前请先备份已有数据。生产环境不要启用 `DEV_SEED`，须自行配置密钥和管理员，见 [服务端说明](server/README.md) 与 [部署说明](deploy/README.md)。

## 现有范围与限制

- 教师端提供账号、班级与成员、设备绑定、文字和原声通知、发送记录、回执及课堂工具界面；管理端提供激活码、用户、班级、设备、消息和操作日志等基础页面。权限以服务端校验结果为准。
- 服务端提供 REST 与 WebSocket、设备通知队列、回执和本地录音存储；大屏已有浏览器预览与 Windows 客户端代码，Windows 安装、系统音量、置顶和开机启动等仍需在目标设备上测试。
- 当前没有服务端语音识别或 TTS 服务：录音按原声发送，文字由大屏尝试使用系统语音播报。自动更新、自动备份及异地恢复流程尚未完成；大屏其余限制见 [大屏说明](screen/README.md)。
- 学校创建与教师 Excel 导入、后台客户端版本与系统设置、账号注销尚未实现；学校角色隔离已有基础代码与测试，完整学校采购流程尚未开放。
- SQLite 数据在服务启动时加载到内存并由单个进程写入，**同一数据目录只能运行一个主服务实例**。尚未完成规范化仓储和大规模性能验证。备份应使用 SQLite 在线备份，或停服后备份完整数据目录及录音。

## 本地检查和验收

```sh
pnpm typecheck
pnpm check:toolchain
pnpm test
pnpm build
pnpm exec playwright install chromium  # 首次运行浏览器测试前
pnpm test:e2e
cd server
go vet ./...
go test -race ./...
```

浏览器测试使用独立临时数据和本地端口，不会替代 Windows、iOS 或 Android 设备测试。正式使用前还应核验：班级和协同权限、通知与回执、断线重连、浏览器录音、Windows 安装及音量恢复、备份恢复，以及目标教室的显示与播放效果。关于发布或性能的目标不能仅凭现有代码或浏览器预览认定达成。

管理后台已通过首版本地试用。版本标签触发 GitHub Actions：原生 Windows amd64 环境构建安装包并上传 Release，Linux amd64 环境构建服务端镜像并推送至 `ghcr.io/alumos/classding-server`，无需 QEMU。构建状态和下载文件以 Actions 与 Release 为准；Windows 安装及系统集成功能仍需在目标设备验收。

本次后台试用步骤见 [本地验收](docs/LOCAL_TESTING.md)。

应用使用 TypeScript 7.0.2。接口生成器在独立工作区使用微软的 `@typescript/typescript6` 兼容包，以继续提供 `openapi-typescript` 所需的 AST API；该依赖不参与应用类型检查。

更多说明：[教师端](web/apps/teacher/README.md) · [管理后台](web/apps/admin/README.md) · [大屏端](screen/README.md) · [部署](deploy/README.md) · [更新记录](CHANGELOG.md)
