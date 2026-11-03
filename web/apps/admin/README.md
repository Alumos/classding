# 课堂叮咚管理后台

在仓库根目录安装工作区依赖后运行 `pnpm --filter @classding/admin dev`，访问 `http://localhost:5174/admin/`。开发代理将 `/api` 和 `/ws` 转发至 `localhost:8080`；需要先启动真实服务端。生产构建为 `pnpm --filter @classding/admin build`。

管理员使用账号、密码和 TOTP 验证码登录；具体验证规则由服务端控制。后台按服务端返回的角色展示权限范围；所有数据来自实际 API，请求失败会显示错误。激活码和操作日志页面仅对运营管理员开放。部分开发方案中的高级管理功能依赖尚未纳入 MVP 契约的接口，当前不提供虚构操作。
