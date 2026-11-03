# Docker 部署

复制 `.env.example` 为 `.env`，设置域名、至少32字符的随机 JWT_SECRET 与首次引导的管理员密码。域名需解析到部署机器。

```sh
cd deploy
cp .env.example .env
# 编辑 .env 后：
docker compose --env-file .env -f compose.yaml up -d
```

教师端在根路径，管理后台在 `/admin/`。管理员使用账号密码登录，无需动态验证码。首次创建管理员后清空 BOOTSTRAP_ADMIN_USERNAME / PASSWORD 并重建容器；这些引导变量不会重置已存在账号密码。旧版的 `ADMIN_TOTP_SECRET` 已不再使用，可从编排中删除。

`classding_data` 保存数据库和录音，必须持久化；不要运行 `docker compose down -v`。主服务只允许一个实例。当前对象仓储在 SQLite WAL 中保存每个实体的 JSON 文档并维护索引，启动后加载内存，写入采用串行事务；适合小规模首版，尚未改成方案中的 sqlc 规范化仓储。请勿宣称数千设备性能已验证。

备份请使用 SQLite 在线备份工具或在停服后备份整个数据卷，不要运行中仅复制主 `.db` 文件（WAL可能未合并）。恢复时停止服务并还原完整备份。自动备份任务、异地备份和恢复演练尚未提供。

镜像由 GitHub Actions 在 Linux amd64 环境构建并推送到 GHCR，包含两个 Web 静态站点和 Go 服务，部署目标为 amd64 服务器。更新时调整 CLASSDING_IMAGE 的标签，再运行 `docker compose pull` 和 `docker compose up -d`。国内镜像仓库/CDN/源站白名单需要实际账号信息后接入。

服务器默认关闭云端语音；文本在教室端使用系统中文语音，原声录音走本地存储和限时签名链接。消息默认保留90天，录音7天，后台每30秒清理。安装包通过 GitHub Release 分发，`/client/latest` 暂不提供自动更新地址。

Compose 为反向代理使用独立私网 `172.30.86.0/24`，服务只信任此网段提供的客户端地址。若与现有网络冲突，请同时修改 `networks.default.ipam` 和 `TRUSTED_PROXY_CIDRS`；不要改成信任所有地址。服务未直接开放宿主机端口，对外访问统一经过 Caddy。

## 1Panel 高位端口直连

将 [`compose.1panel.yaml`](compose.1panel.yaml) 内容粘贴到 1Panel 的编排编辑器，填写 JWT_SECRET 和首次管理员密码即可，不需要创建 `.env`。示例映射 `18080:8080`，访问 `http://服务器IP:18080/admin/`。HTTP 直连使用 `COOKIE_SECURE: "false"`；默认 `auto` 也会按实际协议选择。若改为 HTTPS 入口，可设为 `true`，或配好可信代理后使用 `auto`。

升级到 0.1.2 时保留原编排名称、数据卷、JWT_SECRET 和密码，只修改镜像版本与 Cookie 设置。首次升级后重新登录一次。不要新建同名服务并挂载空卷替代原数据卷。HTTP 支持登录和激活码复制，浏览器录音仍需 HTTPS 或本机安全上下文。
