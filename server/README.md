# ClassDing 主服务

Go 主服务提供 `/api/v1` REST 接口、教师和设备 WebSocket、授权和消息投递。服务也可在构建时内嵌教师端与管理端静态文件；接口契约见仓库 `api/` 目录。

## 本地开发

安装 Go 1.27 或更高版本，并在仓库根目录安装 Node.js 与 pnpm 依赖，然后在仓库根目录运行：

```sh
pnpm dev:server
```

该脚本设置 `DEV_SEED=1`，监听 `127.0.0.1:8080`，将数据写入 `server/data/`，并优先使用仓库本地的 Go 工具链。需要配合前端开发服务使用：教师端 `pnpm dev`，后台 `pnpm dev:admin`，大屏浏览器预览 `pnpm dev:screen`。

空数据目录首次启动时，教师账号为 `teacher` / `ClassDing2026!`，运营管理员为 `admin` / `AdminDing2026!`，另有未使用的演示激活码 `TEST-2345`。已有同名用户不会被种子流程重置；演示密码、默认开发密钥及开发模式均不可用于生产。若设置了 `ADMIN_TOTP_SECRET`，管理员登录需提供对应验证码。

若要构建内嵌最新网页的服务，可在仓库根目录执行：

```sh
pnpm build
pnpm prepare:static
cd server
go build ./cmd/classding
```

`prepare:static` 会替换 `server/static/teacher` 和 `server/static/admin` 中的文件。开发期间可直接运行 Vite，无须每次重新内嵌前端。

## 配置

| 环境变量 | 用途 |
| --- | --- |
| `DATA_DIR` | 数据目录，默认服务启动目录下的 `data`，包含 `classding.db` 和 `audio/`。 |
| `LISTEN_ADDR` | HTTP 监听地址，默认 `:8080`；开发脚本固定为本机 `127.0.0.1:8080`。 |
| `JWT_SECRET` | 签名密钥，至少 32 字符；开发种子模式在未配置时使用公开的开发专用值。 |
| `DEV_SEED` | 设为 `1` 才创建演示数据，并允许未配置 TOTP 的开发管理员登录；生产请勿启用。 |
| `ADMIN_TOTP_SECRET` | 管理员登录使用的 Base32 TOTP 密钥；生产管理员需要配置。当前是整个部署共用的密钥。 |
| `BOOTSTRAP_ADMIN_USERNAME`、`BOOTSTRAP_ADMIN_PASSWORD`、`BOOTSTRAP_ADMIN_NAME` | 首次引导运营管理员；用户名不存在时创建，不会重置已有账号。首次创建成功后同时移除引导用户名和密码。 |
| `TRUSTED_PROXY_CIDRS` | 可信反向代理的 IP 或 CIDR，以逗号或空格分隔。默认不信任转发头；只填写实际代理地址及必要的代理网段。 |
| `SENSITIVE_WORDS` | 逗号分隔的通知敏感词。 |

服务在开发模式以外设置 Secure Cookie，实际使用应通过 HTTPS。使用容器部署时，需确保上述所需环境变量确实传入容器；尤其在反向代理后按客户端限流时，必须正确配置可信代理。切勿将公网或不受控网段当作可信代理，否则客户端可能伪造来源地址。

## 数据与当前限制

当前 SQLite 使用 WAL，持久化按实体保存 JSON 并建索引，启动时载入内存，写入由主服务串行处理；它不是开发方案中规划的规范化 sqlc 仓储。**同一数据目录只能由一个主服务进程使用**，不支持多实例共享数据库。消息与原声文件均需备份；运行时使用 SQLite 在线备份，或停服后备份完整数据目录。自动备份、异地备份、恢复演练及规模性能验证尚未完成。

服务端语音识别及 TTS 服务未实现，录音为原声文件；文本朗读依靠大屏系统语音。`/api/v1/client/latest` 当前没有安装包地址，不能当作已实现的自动更新。

## 检查

```sh
cd server
go vet ./...
go test -race ./...
```

测试需要可用的本地监听端口。上线、上传或发布前，还需按根目录说明完成端到端及目标设备验收。部署配置示例见 [`deploy/`](../deploy/README.md)；工作流的存在不代表 CI 或发布已经完成。
