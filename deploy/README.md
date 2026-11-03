# Docker 部署

复制 `.env.example` 为 `.env`，设置域名、至少32字符的随机 JWT_SECRET、管理员 TOTP 密钥与首次引导的管理员密码。域名需解析到部署机器。

```sh
cd deploy
cp .env.example .env
# 编辑 .env 后：
docker compose --env-file .env -f compose.yaml up -d
```

教师端在根路径，管理后台在 `/admin/`。生产管理员强制动态验证码。首次创建管理员后清空 BOOTSTRAP_ADMIN_USERNAME / PASSWORD 并重建容器；这些引导变量不会重置已存在账号密码。TOTP 密钥当前为部署级设置，只适合首版单运营管理员。

`classding_data` 保存数据库和录音，必须持久化；不要运行 `docker compose down -v`。主服务只允许一个实例。当前对象仓储在 SQLite WAL 中保存每个实体的 JSON 文档并维护索引，启动后加载内存，写入采用串行事务；适合小规模首版，尚未改成方案中的 sqlc 规范化仓储。请勿宣称数千设备性能已验证。

备份请使用 SQLite 在线备份工具或在停服后备份整个数据卷，不要运行中仅复制主 `.db` 文件（WAL可能未合并）。恢复时停止服务并还原完整备份。自动备份任务、异地备份和恢复演练尚未提供。

镜像由 GitHub Actions 在 Linux amd64 环境构建并推送到 GHCR，包含两个 Web 静态站点和 Go 服务，部署目标为 amd64 服务器。更新时调整 CLASSDING_IMAGE 的标签，再运行 `docker compose pull` 和 `docker compose up -d`。国内镜像仓库/CDN/源站白名单需要实际账号信息后接入。

服务器默认关闭云端语音；文本在教室端使用系统中文语音，原声录音走本地存储和限时签名链接。消息默认保留90天，录音7天，后台每30秒清理。安装包通过 GitHub Release 分发，`/client/latest` 暂不提供自动更新地址。

Compose 为反向代理使用独立私网 `172.30.86.0/24`，服务只信任此网段提供的客户端地址。若与现有网络冲突，请同时修改 `networks.default.ipam` 和 `TRUSTED_PROXY_CIDRS`；不要改成信任所有地址。服务未直接开放宿主机端口，对外访问统一经过 Caddy。
