# moonbase

开箱即用的全栈**管理系统模板**：Go + React，单一 Protobuf 契约生成两端类型安全代码。

- 会话认证（用户名/邮箱/手机号 + 第三方登录（OIDC/微信扫码）+ TOTP 两步验证）与 RBAC 权限管理
- 业务设置与系统设置解耦：注册策略、站点品牌 / 文件存储（本地磁盘或 S3）、验证码（含内置离线 ALTCHA）、邮件、短信、消息推送（Bark/Telegram/企业微信/Webhook）、AI 通道、支付渠道（支付宝/微信支付）——全部运行时可改，密钥只写不读
- 支付订单：支付宝（当面付扫码 / 电脑网站 / 手机网站 / 小程序 JSAPI / APP 支付，按签约产品勾选）+ 微信支付（Native / H5 / JSAPI，APIv3，公钥/证书模式可选），扫码收银台演示、异步回调对账、全额退款
- 审计日志：所有变更操作由拦截器自动记录，只读查询页面，保留期可配
- 持久化工作流（DBOS）：崩溃自动续跑，执行轨迹 DAG 可视化
- 全站中文、亮暗主题、签名直传上传（S3 预签名 / 本地签名 URL），附一个数据报表仪表盘（真实系统数据聚合 + 图表）作为业务示例

**架构**：`proto/`（Protobuf + Buf + ConnectRPC）是单一真源，一步生成 Go 服务端桩（`apps/server`）与 TS 客户端（`packages/api-client`），供 React SPA（`apps/web`）消费——契约错配是编译错误，不是运行时惊喜。工具链由 [mise](https://mise.jdx.dev) 锁定，任务由 [moon](https://moonrepo.dev) 编排。

## 快速开始

需要 Docker（本地 PostgreSQL）。先安装一次 mise，并按[官方说明](https://mise.jdx.dev/getting-started.html#activate-mise)激活 shell：

```bash
# macOS / Linux
curl https://mise.run | sh
# Windows
winget install jdx.mise
```

然后在仓库根目录：

```bash
mise trust && mise install --locked # 安装仓库锁定的完整工具链
pnpm install                        # 安装 JS 依赖
docker compose up -d                # PostgreSQL 18（默认连接开箱即用）
moon run :dev                       # 前端 :5173 + 后端 :8080
```

打开 http://localhost:5173，用 `admin` / `admin123` 登录。代码生成、数据库迁移、初始数据全部自动完成。

<details>
<summary>可选的本地演示服务（compose.yaml 已带）</summary>

- **SeaweedFS**（S3 演示，可选——本地存储 driver 无需任何外部服务）：设置页存储选 S3 兼容，填 endpoint `localhost:8333`、region `us-east-1`、bucket `app`、密钥 `seaweedadmin`/`seaweedadmin`、关闭 SSL
- **Mailpit**（SMTP 演示）：设置页邮件服务填 host `localhost`、port `1025`、加密 `None`；收件箱在 http://localhost:8025
- 管理员账号仅在用户表为空时创建，可用 `MOONBASE_AUTH_ADMIN_USERNAME` / `MOONBASE_AUTH_ADMIN_PASSWORD` 覆盖

</details>

## 功能地图

| 页面 | 权限 | 说明 |
| --- | --- | --- |
| `/login` `/register` 等 | 公开 | 多标识密码登录、短信验证码登录、第三方登录（OIDC/微信扫码）、TOTP 第二步、邮件找回密码、邮箱验证——均按通道可用性动态显隐 |
| `/` | `report.read` | 数据报表仪表盘：用户/会话统计卡片、注册与登录趋势图、角色/工作流状态/第三方登录占比图（@ant-design/plots） |
| `/workflows` | `workflow.*` | 工作流运行列表 + 执行轨迹 DAG，取消/恢复/触发 |
| `/payments` | `payment.*` | 支付订单：演示收银台（按渠道已签约产品选择支付方式、状态轮询）、订单列表筛选、全额退款——支付宝（当面付/电脑网站/手机网站/JSAPI/APP）+ 微信支付（Native/H5/JSAPI，APIv3） |
| `/users` · `/roles` | `user.*` `role.*` | 用户与角色管理，权限按资源分组勾选、支持搜索 |
| `/audit` | `audit.read` | 审计日志：变更操作自动记录（操作人/模块/资源/结果/IP），可筛选分页 |
| `/settings` | `settings.*` / `system.*` | 统一设置区（左侧分组导航，按权限显隐）：通用（站点信息、账号与注册）/通讯渠道（邮件/短信/消息推送（Bark / Telegram / 企业微信 / Webhook））/身份与安全（验证码（Turnstile / 极验 / 内置离线 ALTCHA）/第三方登录（通用 OIDC + 微信））/支付（支付宝 + 微信支付，公钥/证书模式可选）/基础设施（存储（本地磁盘 / S3 兼容）/AI），全部多配置化、统一按用途绑定生效 |
| `/profile` | 登录即可 | 头像、改密、标识绑定/解绑、两步验证、设备管理 |

## 常用命令

| 命令 | 说明 |
| --- | --- |
| `moon run :dev` | 同时启动前后端 |
| `moon run :check` / `:fix` | 代码检查 / 自动修复（格式化 + lint autofix） |
| `moon run :test` | 全部测试（设 `MOONBASE_DATABASE_URL` 时含集成测试） |
| `moon run proto:generate` | 改完 `.proto` 后重新生成两端代码 |
| `moon run server:migrate-new -- <名称> sql` | 新建数据库迁移 |
| `moon sync hooks` | 安装 Git 钩子（pre-commit 自动修复 + 检查，commit-msg 校验提交信息） |

完整任务清单见各项目 `moon.yml`。

## 部署

**单二进制**（前端嵌入，同源无 CORS，迁移启动时自动执行）：

```bash
pnpm run release          # 构建前端 + go build -tags embed
./apps/server/bin/server  # :8080 同时提供 API 与页面
```

**Docker**（distroless 静态镜像，非 root）：

```bash
docker build -t moonbase .
docker run -p 8080:8080 -e MOONBASE_DATABASE_URL='postgres://…' moonbase
```

配置一律 `MOONBASE_*` 环境变量覆盖，无 `.env`、无配置文件——全部旋钮与默认值见 `apps/server/internal/config/config.go`：

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `MOONBASE_DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/app?sslmode=disable` | pgx 池参数写进 DSN，如 `…&pool_max_conns=10` |
| `MOONBASE_SERVER_HOST` / `MOONBASE_SERVER_PORT` | `0.0.0.0` / `8080` | 监听地址 |
| `MOONBASE_SERVER_PUBLIC_URL` | `http://localhost:5173` | 邮件链接使用的外部可达地址，生产必设 |
| `MOONBASE_AUTH_SECURE_COOKIE` | `false` | 生产 TLS 后必开（`__Host-` cookie） |
| `MOONBASE_AUTH_ADMIN_USERNAME` / `MOONBASE_AUTH_ADMIN_PASSWORD` | `admin` / `admin123` | 仅首次启动（用户表为空）时生效。初始管理员只有用户名标识，登录后在资料页用验证码绑定真实邮箱/手机号 |
| `MOONBASE_AUTH_SESSION_TTL_HOURS` / `MOONBASE_AUTH_SESSION_MAX_LIFETIME_HOURS` | `168` / `720` | 会话空闲滑动过期 / 总寿命硬上限 |
| `MOONBASE_AUDIT_RETENTION_DAYS` | `180` | 审计日志保留天数，`0` 表示永久保留 |
| `MOONBASE_CORS_ALLOWED_ORIGINS` | `http://localhost:5173` | 逗号分隔多个；同源部署（单二进制/Docker）无需设置 |
| `MOONBASE_LOG_LEVEL` / `MOONBASE_LOG_FORMAT` | `info` / `auto` | 日志级别（`debug`/`info`/`warn`/`error`）；控制台格式 `auto`（TTY 彩色，否则 JSON）/`pretty`/`json` |
| `MOONBASE_LOG_FILE` | 空（关闭） | 设为路径（如 `/var/log/moonbase/server.log`）即开启文件日志：JSON 格式，自动轮转 + gzip 压缩 |
| `MOONBASE_LOG_FILE_MAX_SIZE_MB` / `_MAX_BACKUPS` / `_MAX_AGE_DAYS` | `100` / `10` / `30` | 轮转阈值（MB）/ 保留段数 / 保留天数 |
| `MOONBASE_LOG_FILE_ROTATE_AT` / `_COMPRESS` | `midnight` / `true` | 定时轮转（`midnight`/`hourly`/空关闭）/ 轮转段 gzip 压缩 |
| `MOONBASE_LOG_SQL` | `false` | 开启后以 debug 级别记录每条 SQL（语句/参数/耗时），排障用 |
| `MOONBASE_METRICS_ENABLED` | `true` | 开启 Prometheus `/metrics`（在 `/api` 鉴权链之外，抓取端需在网络层限制访问） |
| `MOONBASE_OTEL_TRACE_ENDPOINT` | 空（关闭） | 设为 OTLP/gRPC 采集器地址（如 `localhost:4317`）即开启 OpenTelemetry 链路追踪；留空则全程零开销 |
| `MOONBASE_OTEL_SERVICE_NAME` / `_INSECURE` / `_SAMPLE_RATIO` | `moonbase` / `false` / `1.0` | 追踪的服务名 / OTLP 明文传输（本地采集器）/ 头部采样比例 `[0,1]` |

## 深入了解

架构决策、约定与开发细节（认证方案取舍、供应商注册表模式、护栏测试、新增 API 域的完整清单等）见 [`AGENTS.md`](./AGENTS.md)——同时服务于人类维护者与 AI coding agent。
