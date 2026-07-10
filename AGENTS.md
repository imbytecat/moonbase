# AGENTS.md

moonrepo monorepo。`proto/`（Protobuf + Buf + ConnectRPC）是单一真源：`moon run proto:generate` 同时重新生成 Go 服务端（`apps/server`）与 TS 客户端（`packages/api-client`），后者由 React 19 SPA（`apps/web`）消费。字段/RPC 错配是**编译错误**，不是运行时惊喜。完整工具链由 **mise** 锁定（`.mise.toml` + `mise.lock`），任务由 **moon** v2 编排，包管理器 **pnpm**。

它作为一个管理系统**模板交付，而非框架**：下游项目复制本仓库、各自演化，并通过 `git remote add template` 把修复挑拣（cherry-pick）回来。由此带来的、会改变你工作方式的后果：保持 integration 子包不引入业务代码（保证 diff 可移植）；**不要**抽取共享 Go 库、拆分微服务，或添加 semver/向后兼容垫片——没有任何外部依赖此代码，所以 settings 结构体的变更可以合理地把旧行零读（zero-read）。驱动注册表是编译期扩展系统（`database/sql` 风格），不是运行时插件系统。

**第三方库原则：应上尽上，差异过大不硬上。** 成熟前沿的库/最佳实践优先于手搓——省维护精力、保持逻辑清晰、把精力留给业务；但与需求实在有差异时不硬套。两个方向都要留下理据：采用了写清为什么（如 `coreos/go-oidc` 之于 OIDC），否决了也写清评估过什么、为何不值（如微信扫码手写 3 次调用——否决 silenceper/PowerWeChat；local 存储 handler ~120 行标准库——否决 gocloud.dev，SignedURL 之外 serve 的活它一行不省）。判据是**净收益**：库替你扛掉的怪癖/协议面，要大于它带来的依赖面。

## 命令（在仓库根目录执行）

- 首次工具链引导见 README 快速开始。交互式 shell 应按 mise 官方方式激活；非交互环境用 `mise exec -- <command>`。`.mise.toml` 把 `PROTO_HOME` 隔离到 `.moon/cache/proto`，避免 moon 2 把用户机器上遗留的全局 proto shims 注入任务 PATH；勿删除。
- `pnpm install` —— JS 依赖（工作区 `@moonbase/web`、`@moonbase/api-client`）。
- `docker compose up -d` —— Postgres 18（与默认 DSN 匹配）+ 可选 SeaweedFS（S3 演示）+ mailpit（SMTP，收件箱 :8025）。Seed 仅在 users 表为空时运行——若集成测试报 "invalid credentials"，重置：`docker compose down && docker volume rm moonbase_pgdata && docker compose up -d`。
- `moon run :dev` —— web :5173 + server :8080；迁移 + seed（`admin`/`admin123`）在启动时自动应用。
- 调用形式为 `moon run <project>:<task>`；项目：`proto`、`server`、`web`、`api-client`。
- **改完代码的本地验证序列固定为 `moon run :fix && moon run :test`**——不要自行发明 `go build`/`go vet`/`go test` 组合（moon 有缓存，未变更项目近乎免费）。`moon run :check`（只读闸门）供 CI 与 pre-commit 钩子使用，日常无需手动执行；`moon ci` = 对受影响项目 build/test/check。
- 单个 Go 测试：`cd apps/server && go test ./... -run TestName`。单个 web 测试：`cd apps/web && pnpm run test <file>`。
- Go 集成测试需要 `MOONBASE_DATABASE_URL='postgres://postgres:postgres@localhost:5432/app?sslmode=disable'`（没有则静默跳过）；邮件流测试还需要 mailpit（未启动时跳过）。

## 生成代码被 git 忽略——先生成，否则构建失败

全新克隆：这些文件在生成前并不存在；永远不要手改它们。
- `apps/server/internal/gen/` + `packages/api-client/src/gen/` ← `moon run proto:generate`（buf）。经由 `proto:generate` 任务依赖，被 server:build/dev/test/check/fix/release + web:build/typecheck/dev 依赖。
- `apps/server/internal/repository/` ← `moon run server:generate`（sqlc）。
- `apps/web/src/routeTree.gen.ts` ← `moon run web:gen`（TanStack Router）。
- **air 不监视 `proto/`** —— 改完 `.proto` 后，运行 `moon run proto:generate`（moon 会缓存，未变更时近乎免费）。
- **sqlc 不做清理**：删掉某个 `db/query/*.sql` 会留下过期的 `internal/repository/*.sql.go`，构建会因被删类型而中断——`rm` 掉那个生成的孪生文件，再重新生成。

## 护栏测试——测试变红意味着修你漏掉的那一侧，绝不弱化测试

- `internal/server/authz_test.go` —— 每个注册的 RPC 都需要一条授权规则。新增 proto service ⇒ 在此加生成的空导入（blank import）+ 路径前缀，规则写在 `authz.go`。
- `internal/config/config_test.go`（`TestLoadEnvOverrides`）—— 每个配置键都需要一个 viper 默认值，否则其 `MOONBASE_*` 环境变量会被静默忽略。

## 新增一个 API 域

1. `proto/<domain>/v1/<domain>.proto`（protovalidate 规则内联）→ `moon run proto:generate`。
2. 从 `packages/api-client/src/index.ts` 重新导出生成的 `_pb` + `_connectquery`——忘了这步会破坏 web 的 import。
3. 迁移（`moon run server:migrate-new -- <name> sql`）+ `db/query/*.sql` → `moon run server:generate`。
4. 在 `internal/rpc/` 实现 service，带一句 `var _ <x>connect.Handler = (*Svc)(nil)` 断言；在 `internal/server/router.go` 注册。
5. 在 `internal/server/authz.go` 为**每一个** procedure 写授权规则 + 在 `authz_test.go` 加生成的空导入**和**路径前缀。
6. 新权限：给 `Permission` 枚举（`proto/auth/v1/permission.proto`）加一个值**并**加一条匹配的 `auth.Catalog` 条目（`internal/auth/permissions.go`）——两者漂移时 `TestPermissionEnumMatchesCatalog` 会失败。
7. Web：在 `src/routes/_authed/` 下加路由，beforeLoad 里用 `requirePermission`；在 `NAV_TREE`（`src/lib/navigation.tsx`）加叶子节点；用户可见文案直接写中文；在 `src/lib/permissions.ts` 加一对 `permission_*` 文案。

移除一个域 = 反向操作 + 一个把已存 `role_permissions` 映射到后继键的迁移（键存在角色行里）+ `rm` 掉孤立的 sqlc 产物。

## 给现有 integration 新增一个 provider

**Email 是 ADR-0014 struct-first 方案的已实现样板；其他 integration 仍在旧 `config.Schema` 引擎上，等待逐个迁移。不要把旧模式复制回 Email，也不要假装其他 integration 已完成迁移。**

给 Email 新增 provider：

1. 在 `packages/integrations/email/<provider>/` 定义私有 Go config struct（`json` + `jsonschema` tags）、`config.Contract[T]` policy、presentation 与 typed send 实现，导出一个返回 opaque `email.Registration` 的 `New(...)`。Provider 包不得 import settings、声明 purpose 或读取 `map[string]any`。
2. 在应用侧 `apps/server/internal/mail/registry.go` 的显式有序组合中加一行构造；provider key/presentation/schema/policy/Ops 不得在组合根重复。Registry 在 server 启动时构造并注入，不使用全局 registry、`init()` 或 blank import。
3. 普通字段完全由标准 Draft 2020-12 JSON Schema + rjsf 渲染；core 只根据 lifecycle policy 生成 secret/create-only 的最小 UI Schema。不要为 provider 新增前端分支或 UI sidecar。

其他 integration 在迁移前仍按现有 `config.Schema` + registry entry 模式新增 provider；迁移它们时以 Email 的包边界和 `Contract[T]` 为样板，而不是继续扩展旧引擎。所有 integration 的 wire 已统一为 `ProfileInput.config = ConfigWrite`、读侧 `Profile.config = ConfigView`；**不要改 `system.proto` 加 provider 专属消息**。

若改为新增一个用途（PURPOSE）= base 的 `internal/<integration>` facade 中一个常量 + `integration.Catalog` 目录条目；通用 Web 直接消费 descriptor，不再维护 `PURPOSE_LABELS`。

新增一整个 channel = 以上全部，外加 `settings.Integration[kitsettings.GenericProfile]` 的 Store getter/setter、通道 catalog、`system_<integration>.go` 的标准 `integrationOps` 接线、`Describe<Integration>Providers` RPC，以及一个 web 面板入口。

## Settings = 两个后端面（web 呈现为一个）

- `settings.v1` = 业务开关（`settings.*` 权限）：注册策略、注册标识符、手机区域、站点（SITE）标识。`GetSiteInfo` 是**公开**的（登录页在未登录时渲染它）。新的产品开关 → 放这里。
- `system.v1` = 带密钥的基础设施通道（`system.*` 权限）：storage/captcha/email/sms/llm/oauth/payment。新通道 → 放这里。**没有**通用的 UpdateSystemSettings——只有 GetSystemSettings + 每通道的档案 CRUD/Bind/Test。
- web 把两者都呈现在 `/settings/*` 下（按权限过滤的分组）；这个拆分存在于 proto/权限里，而非导航里。
- **密钥在 wire 上只写**：写侧 `ConfigWrite.values` 是普通字段，`secrets` 是 JSON Pointer → 非空替换值；path 缺席即保留，不支持 clear。读侧 `ConfigView.values` 完全不含 secret，`set_secret_paths` 只报告已设置状态。驱动只看到合并后的真实配置。
- **Settings 存储是 JSONB，无迁移**（`internal/settings`）：结构体形状变更会静默地把旧行零读——重置 dev 卷或重新录入配置（真实部署也一样）。缺失的行读作零配置，所以无需 seeding。
- **统一的通道模型**：`settings.Integration[kitsettings.GenericProfile]`（Profiles + Bindings `map[string][]string`）。未迁移 integration 的 CRUD 仍复用 `integrationOps`；Email 样板由 `config.Contract[T]` 执行 config lifecycle，并在 `system_email.go` 显式接线通用 profile CRUD/Bind/Test。
- **绑定即激活** —— 任何地方都**没有**每档案的 `enabled` 标志（它会造出一个"已绑定但被禁用"、语义未定义的状态，如被静默禁用的 CAPTCHA）。要暂停就解绑。
- **用途目录是代码**：每个 base facade 导出一个 `integration.Catalog`（`storage.Purposes`、`mail.Purposes`……）。业务代码按用途寻址 integration，绝不用档案 id：`mail.Sender.Send(ctx, purpose, …)`。未绑定的用途 → `ErrNotConfigured`（CAPTCHA 例外 = 直通，让全新安装仍可登录）；删除已绑定的档案 = FailedPrecondition。多值用途（oauth 的 `login`、所有 payment）携带 `profile_ids` 并扇出；其余为单值。
- **驱动 = 一个接缝（seam）背后的每 provider 配置形状**：Email provider 的私有 struct 生成标准 JSON Schema，Registry 在边界严格解码后才调用 typed driver；其他 integration 尚待迁移。只有各 integration 自己的接缝（Send/Verify/Complete/……）共享，绝不要建立跨 integration 的万能 Driver 或把 provider 参数摊平成通道级共享字段。
- OAuth 档案的 `config.key` = `user_identities.provider` 里的 slug 以及流程 URL `/api/oauth/{key}/...`；创建后**不可变**；删除一个仍有身份行的 = FailedPrecondition。
- Web：每个通道复用 `ProfileManager` + `ProfileFormDrawer`；每个表单都走 `FormDrawer`（脏数据守卫关闭）——绝不要在表单外直接挂一个裸的 antd Drawer。新通道 = 在 `src/components/system/` 加新面板 + 一条 `src/lib/settings-nav.tsx` 条目。
- 用户可见文本（proto 注释、UI 文案、错误字符串）里不要出现工具/库名——描述协议（"SMTP"、"S3-compatible"），而非实现。错误展示一条通用的已翻译消息，绝不用裸的 `err.message`。

## 能力 = 控制平面 + 可选数据平面

把"什么连接"（控制平面：上面的档案/绑定/注册表模型——JSONB、无迁移、无状态）与"发生了什么"（数据平面——挑最弱的层级，别发明通用抽象）分开：第 1 层 短寿命密钥 → `verification_tokens`；第 2 层 只追加账本 → 审计模式；第 3 层 领域状态机 → 真正迁移的表（`payment_orders`、DBOS 检查点）。Settings JSONB 永不存状态；状态表永不存凭据。接缝签名刻意按域定制——没有通用的 channel 接口。

- **审计**（`internal/audit` + audit.v1）：一个拦截器接缝记录每个会改动的一元 RPC——处理器从不写审计行。请求载荷**从不**存储（密钥即便对审计轨迹也保持只写）；只读的 RPC 面；经 `MOONBASE_AUDIT_RETENTION_DAYS` 的每小时保留清道夫（默认 180，0 = 永久）。
- **工作流**（`internal/workflow` + workflow.v1）：DBOS 是一个**库**，把检查点写入**同一** Postgres 的 `dbos` schema，并在启动时续跑被中断的运行。工作流是注册的**代码**；nil 引擎（单元测试）会让工作流 RPC 回 FailedPrecondition。
- **支付**是唯一带数据平面的 integration：`payment_checkout_sessions` 是短寿命托管收银会话，`payment_orders` 拥有 `creating → pending → paid/refunding/refunded`（或 `failed`/`closed`）状态机，结算迁移与 `payment_settlement_events` outbox 同事务提交。付款人只选择 driver descriptor 发布的 payer-facing payment method；driver 的 `Plan(ClientContext)` 再从 profile 签约的 `Profile.config.products`（空 = 全部）中选择 provider-scoped product（支付宝 `precreate`/`page_pay`/`wap_pay`/`create`/`app_pay`；微信 `native`/`h5`/`jsapi`/`app`）。核心 driver interface = `Describe/Plan/Create/Query`，`Notify/Refund/RefundQuery/HostedFlow/ActionRecovery` 为按接口自动推导的可选能力。收银台只渲染 proto `PaymentAction` oneof（QR/redirect/form/wait/hosted flow），前端无 provider/product 目录镜像。回调 `POST /api/payment/notify/{provider}/{profile}` 由驱动验签；并发确认、回调重放与同步都由 SQL 状态守卫保持幂等。
- **通知**（`internal/notification` + notification.v1）：每用户的站内信收件箱。业务代码经 `notification.Publisher` 接缝通知——`Publish(userID,…)` / `PublishToPermission(perm,…)`（向持有某权限者扇出）——**绝不**直接写 `notifications` 行；读侧是自限定范围的 RPC（authz `{}` + `IdentityFromContext`，所以用户只看到自己的）。出站文本（收件箱标题/正文、验证/重置/验证码邮件）**直接写中文常量**——全站中文，无 i18n 层（见 ADR-0008）；RPC 错误消息在服务端保持为代码，由 SPA 人性化展示。

## 认证与 RBAC

- **两个边缘层，业务代码里零认证**（`internal/auth`）：authn（`NewMiddleware`，基于 `connectrpc.com/authn`）把 `session` cookie 解析为 `*auth.Identity`；它**从不**拒绝（匿名以 nil 继续）。authz（`NewAuthzInterceptor` + `internal/server/authz.go` 里的规则表）：每个 RPC → `{Public} | {}（任意会话）| {Permission}`。**未知的 procedure 默认拒绝。**
- 处理器读 `auth.IdentityFromContext(ctx)`；单元测试注入 `auth.WithIdentity(ctx, &Identity{…})`——无 HTTP、无登录、无 mock。
- **权限目录：proto 的 `Permission` 枚举是真源**（`proto/auth/v1/permission.proto`），由 Go、web 和（未来的）移动端类型安全地共享——一个权限拼写错误在每一端都是编译错误。它由 `TestPermissionEnumMatchesCatalog` 保持与 Go `auth.Catalog`（键 + 描述）1:1。DB（`role_permissions.permission`）、`Identity` 和 `authz.go` 规则表都用点分字符串键（`user.read`）；枚举只在 wire 边界映射到它们（`internal/rpc/permissions.go`，`permissionKey`/`permissionEnum`，在前端 `src/lib/permissions.ts` 镜像）。**新增**枚举值 + 目录条目，绝不**重命名**（会破坏已存的 `role_permissions`）。`PERMISSION_ALL` 是 `admin` 通配符 `*`，**不可变**（`isAdminRole` 守卫）；系统角色 `admin`/`user` 不能被重命名或删除。
- **会话由 DB 支撑**（不透明的 32 字节令牌，只存其 SHA-256），这是有意为之：单二进制 + Postgres 带来即时吊销——别换成 JWT。浏览器用 httpOnly cookie（`session`，或 `secure_cookie` 下的 `__Host-session`）；原生应用发 `Authorization: Bearer`。密码哈希是 argon2id。TLS 后设 `MOONBASE_AUTH_SECURE_COOKIE=true`。
- 登录按标识符形状路由：`@`→邮箱，`+`/数字→手机（E.164），否则用户名（`^[a-zA-Z][a-zA-Z0-9._-]{2,31}$`）；这些形状互斥——保持路由精确。密码登录没有启用/禁用开关（可能把所有人锁死），并对 `auth.DummyHash` 做时序均衡（保留它）。暴力破解防御 = CAPTCHA，而非限流。
- **第三方登录**（`integrations/oauth`；浏览器 HTTP 而非 RPC：`/api/oauth/{key}/authorize|callback`）：`oidc` 驱动跑在 `coreos/go-oidc/v3` + `golang.org/x/oauth2` 上（发现、JWKS ID-token 校验、nonce、PKCE）——别再手搓 token 交换；`wechat` 驱动是有意手写的 3 次调用扫码登录（没有 ID token / 没有签名可校验，所以 SDK 毫无收益——评估并否决了 silenceper/PowerWeChat）。`Flow` 接缝在 authorize 时铸造 `FlowSecrets{Nonce,Verifier}`，`auth_oauth_http.go` 通过 httpOnly `oauth_state` cookie（`state`+nonce+verifier 的 base64 JSON）来回传递它们：`state` 守卫 CSRF（回调时比对），nonce/verifier 留在客户端用于 OIDC 校验 + PKCE。
- 短寿命密钥（邮箱验证、密码重置、手机/邮箱绑定、短信登录、注册验证码）**全部**存在**一个** `verification_tokens` 表里，以一个 `Purpose` 常量为键——新增一个 Purpose，而非一张新表。通道支撑的标识符（邮箱/手机）在建账号前总是经验证码校验；公开的请求 RPC 按枚举策略回 ok/already_exists；ResetPassword 吊销**所有**会话。
- **Seed**（`auth.Seed`，迁移之后）：幂等的角色 `admin`（`*`）/ `user`（report.read）；初始 admin **仅**在 users 表为空时创建。该 admin 按设计**没有**邮箱（登录后在资料页绑定一个，经验证码校验）。
- **上传绝不经 RPC 代理字节**：`storage.v1` 预签名返回一个签名的 PUT URL + 一个服务端选定的 key；浏览器直接 PUT（S3 预签名，或本地驱动的 `/api/files/...` HMAC 端点），然后经拥有该域的 RPC 保存 key。
- 集成测试（`internal/rpc/integration_test.go`）构建真实路由，无 `MOONBASE_DATABASE_URL` 时跳过。

## API 契约：proto + Buf + ConnectRPC

- **校验用 protovalidate**（内联 `(buf.validate.field)…`，消息级 CEL），由服务端拦截器强制——别加 go-playground/validator，也别在 Go 里重复规则。
- **托管模式覆盖（勿删）**：`buf.gen.yaml` 把模块 `buf.build/bufbuild/protovalidate` → `buf.build/gen/go/…/protocolbuffers/go`；没有它，生成的 Go 会 import 一个不存在的本地 `…/internal/gen/buf/validate`，构建中断。`go mod tidy` 解析出真实路径。
- 外部 Go 插件由 mise 固定版本并安装，Buf 直接调用 `protoc-gen-go` / `protoc-gen-connect-go`；TS 插件经 `local: ['pnpm','exec',…]` 运行。**TS 插件的 devDeps（`@bufbuild/protoc-gen-es`、`@connectrpc/protoc-gen-connect-query`）放在根 `package.json`**，这样 `pnpm exec`（由 buf 从仓库根调用）能从根 `node_modules/.bin` 解析到它们。生成器↔运行时版本在 `pnpm-workspace.yaml` 里按 catalog 锁步固定（Buf 要求插件与运行时匹配）——两个 catalog 条目一起升，绝不只升一个。仓库内的 `protoc-gen-permissions` 仍从当前源码运行。
- 处理器挂在标准 `http.ServeMux` 上，规范路径 `/pkg.v1.Service/…`，经 `StripPrefix` 挂载在 `/api` 下。Connect 协议走 HTTP/1.1 + JSON（一元可 curl；无需 h2c）。

## 数据库与迁移

- Postgres 18+（`uuidv7()` 默认值）。无 `.env`、无配置文件——只有默认值 + `MOONBASE_*` 环境变量（`internal/config`，viper）。**每个配置键都必须有 `v.SetDefault`**，否则其环境变量被静默忽略（AutomaticEnv+Unmarshal 只看得见已知键；由 `TestLoadEnvOverrides` 守卫——加键时扩展它）。
- **迁移在服务器启动时自动应用**（`db/migrations/` 里的 goose SQL，内嵌，加咨询锁以防副本竞争）。`migrate-*` 这些 moon 任务仅供手动运维。
- sqlc 只把迁移的 Up 段解析为它的 schema；查询在 `db/query/*.sql`；部分更新用 `sqlc.narg('x')` + `coalesce`，配合 proto3 的 `optional`。

## moon v2 配置陷阱

- 项目 `moon.yml` 用 `layer:` + `stack:`（**不是** `type:`/`platform:`/`toolchain:`——它们会报错）。工作区用 `vcs.client`（**不是** `vcs.manager`）。
- v2 把 toolchain 文件改名为复数 `.moon/toolchains.yml`，但其 `$schema` 仍是**单数** `…/toolchain.json`。它不管理任何语言工具链（版本归 mise 管）；任务经 moon 的 `system` 工具链运行。`.mise.toml` 与 `mise.lock` 是所有任务的隐式输入，工具升级会正确使 moon 缓存失效。
- 官网的 JSON schema 落后于 2.3.5 二进制——相信 moon 实际的解析错误，而非官网。
- 共享任务只定义一次，放在 `.moon/tasks/{go,typescript}.yml`（按 `language:` 匹配继承）；同名的项目任务会**合并**（追加 deps），如 server 的 `test`/`check` 加 `deps: ['proto:generate','~:generate']`。`proto` 不设 `language`，自定义它自己的任务。

## Lint 与 git 钩子

- 每个项目的只读闸门是 `check`，写侧孪生是 `fix`。后端 = mise 安装的 golangci-lint v2（`.golangci.yml`）；前端 = Biome（`biome.json`：2 空格、单引号、无分号、宽度 100；格式化 + linter + import 排序；自动跳过生成文件）。Proto = buf lint + format。
- **不要**单独运行 `go vet`、`go fmt`、`gofmt`、`goimports`——它们已是 golangci-lint v2 的子集（`default: standard` 含 govet；formatters 含 gofmt/goimports），裸跑既冗余又可能与 `.golangci.yml` 配置不一致。本地质量环路只有一条命令：`moon run :fix`。
- **Pre-commit 钩子**（自动同步；用 `moon sync hooks` 安装一次）：`moon run :fix` → `git update-index --again`（重新暂存修复）→ `moon run :check`，全部 `--affected --status=staged`。只有不可修复的错误才拦截。
- **Commit-msg 钩子**：`pnpm exec commitlint` 强制 Conventional Commits，要求**中文主题**（`subject-zh` 自定义规则）+ 一个 `scope-enum`（项目名 + `deps`/`ci`/`agents`），在 `commitlint.config.mjs` 里从 `.moon/workspace.yml` 的 `projects` 动态读取。
- 仓库移动后 golangci-lint 报错文件路径不对 = 缓存过期：`golangci-lint cache clean`。

## 前端（apps/web）

- **路径别名 `#*` → `./src/*`**，经 package.json 的 `imports`（Node 子路径 import，**不是** tsconfig 的 `paths`）。`["./src/*","./src/*.ts","./src/*.tsx"]` 这个回退数组是**必需**的（TS 按字面解析 `imports`，不猜扩展名）。用 `#lib/…`、`#components/…`；绝不用 `../`。
- **数据层是 ConnectRPC**，而非手写 fetch：传输在 `src/lib/transport.ts`（`baseUrl: '/api'`）；从 `@moonbase/api-client` import 生成的方法引用，用 `@connectrpc/connect-query` 的 `useSuspenseQuery`/`useMutation` 调用。proto 的 `Timestamp` 是一个消息——用 `@bufbuild/protobuf/wkt` 的 `timestampDate()` 渲染，而非 `Date`/字符串。
- `@connectrpc/connect` 必须是 `apps/web` 的**直接**依赖（仅传递依赖会破坏 `tsc`）。路由 search-param 接口必须被 `export`（未导出 → `routeTree.gen.ts` 里 TS4023）。
- `vite.config.ts`：`tanstackRouter()` **必须**在 `react()` 之前。dev 服务器把 `/api` 代理到 `:8080`。
- **tsconfig**：`allowJs: true` 是**必需**的（工具链仍可能消费 JSDoc 类型的 `.js`）。`noUncheckedIndexedAccess` + `verbatimModuleSyntax` 已开启。
- **全站中文，无前端 i18n 层**：用户可见字符串直接写中文 literal 或中文常量；不要新增 Paraglide、inlang、`messages/*.json` 或语言切换器。用 `humanizeError`（`src/lib/errors.ts`）映射后端错误，绝不用裸的 `err.message`。
- 会话状态 = GetMe 查询（`src/lib/session.ts`），无 store/context；登录/登出调用 `queryClient.clear()`。认证流 UI 由 `GetAuthConfig`（公开）能力驱动。导航是声明式的 `NAV_TREE`（`src/lib/navigation.tsx`）；settings 有自己的目录（`src/lib/settings-nav.tsx`）。
- **antd v6 + Tailwind v4** 经 `src/styles.css` 里的 `@layer` 顺序 + `<StyleProvider layer>` 共存——改动任一都会破坏 antd 样式。主题 token 只在 `AntThemeBridge` 定义一次；暗色模式经 `ThemeModeProvider` 同时驱动 Tailwind 的 `.dark` 和 antd 的 `darkAlgorithm`。用朴素的 antd Table/Drawer/Form（无 ProComponents）。要遵守的 antd v6 API 改名：`Drawer width`→`size`、`Alert message`→`title`、`Dropdown dropdownRender`→`popupRender`、`List`→语义化的 `<ul>/<li>`。
- 图表用 `@ant-design/plots`；主题跟随 `useThemeMode()`。后端时间序列是**稀疏**的（零值日被省略）——在客户端补齐空缺（`fillDaily`）。手机输入：`<PhoneInput allowedRegions>` + `phoneRule()`；值端到端都是 E.164。

## SPA 嵌入：dev 代理 vs 生产单二进制

- Dev：Vite 把 `/api` 代理到 :8080；SPA **不**嵌入（`internal/web/stub.go` 里的 `//go:build !embed` 桩）。
- Prod：`pnpm run release` = `moon run server:release` → `-tags embed`。`web:build` 是一个**任务**依赖（不是项目 `dependsOn`——moon 的 `enforceLayerRelationships` 拒绝 app→app 边），所以 SPA 先构建，产物进 `apps/server/internal/web/dist/`，经 `//go:build embed` 嵌入。一个二进制、同源、无 CORS。

## 后端布局（apps/server，单一 Go 模块）

- 模块 `github.com/imbytecat/moonbase/server`，入口 `cmd/server`；跨语言类型对齐是 proto 的活。ConnectRPC 拥有自己的路径（无路由框架）。
- 把**唯一**的 `internal/logging` slog logger 接入每个库（pgx/goose/DBOS）——绝不给某个库单独一个 logger。处理器返回有类型的 `connect.NewError`；内部失败经 slog 记录 + 返回通用 `CodeInternal`（不泄漏）；`pgx.ErrNoRows` → `CodeNotFound`。
- **可观测性 = 日志 + 指标 + 链路追踪，各一个接缝。** 指标（`internal/metrics`，Prometheus）：一个放在**最外层**的 Connect 拦截器（这样它也统计 authz 拒绝）记录 `moonbase_rpc_*`（procedure/code 标签——有界），外加 pgxpool/Go-runtime/`build_info` 采集器，在**外层** mux 的 `/metrics` 提供（在 `/api` authn 之外——抓取端没有会话；在网络层限制）。链路追踪（`internal/tracing`，OpenTelemetry）默认**休眠**：没有 `otel.trace_endpoint` → 无 provider，且 otelconnect 拦截器 + `NewSlogHandler`（在有活跃 span 时注入 `trace_id`）是廉价的 no-op。两者仅在各自配置开关打开时才接线。构建/版本（`internal/buildinfo`）来自 `runtime/debug`（发布时用 ldflags `-X …/buildinfo.version` 覆盖），在 `/health` 和启动日志里呈现。
- `CGO_ENABLED=0` 设在 `apps/server/moon.yml` 的 `env:`——保持服务器纯 Go（goose 用 modernc sqlite）。sqlc/goose/buf/air/golangci-lint/govulncheck 及 Go protoc 插件全部由 mise 安装并锁定版本；任务只调用裸命令，版本统一在 `.mise.toml` 升级。
- **测试**：service 依赖 sqlc 的 `repository.Querier` 接口 → 单元测试用结构体嵌入的伪实现（无 DB、无 mock 框架）。集成测试构建真实栈，无 `MOONBASE_DATABASE_URL` 时跳过。
- 热重载 `server:dev` = air（`.air.toml`），监视 `.go`+`.sql`，`pre_cmd` 重新生成 sqlc。经 `moon run server:dev` 运行，以继承 CGO/env。保留 `.air.toml` 的 `[build.windows]` `.exe` 块——air 只会给它的默认 cmd 自动加 `.exe`，所以删掉它会静默地破坏 Windows。

## CI、Docker、约定

- `.github/workflows/ci.yml`：push 到 `main` + PR 时跑 `moon ci`（`jdx/mise-action` 按 lockfile 安装/缓存工具链 + `pnpm install`）；checkout `fetch-depth: 0` 以做受影响 diff。PR 还会跑 `moon run proto:breaking` 对比 `origin/main`。一个专门的 `moon run server:vuln` 步骤跑 `govulncheck`（可达漏洞扫描；不在 `moon ci` 图里——依赖网络，`runInCI:false`）。一个 `postgres:18-alpine` 服务支撑集成测试。
- **依赖更新**——本地一次性，无需 CI：`mise upgrade --bump && mise lock --platform linux-x64,macos-arm64,windows-x64`（完整工具链）、`pnpm update -Lr`（所有工作区 JS），以及在 `apps/server` 里 `go get -u ./... && go mod tidy`（Go 模块）；然后 `moon run :fix && moon run :test`。Renovate 原生识别 `.mise.toml` 与 `mise.lock`，无需为 CLI 版本维护自定义正则 manager。可达 CVE（可选）：`moon run server:vuln`（govulncheck）。
- `Dockerfile`：固定版本的 mise → 信任 `.mise.toml` → 按 `mise.lock` 安装工具链 → `moon run server:release` → distroless static（CGO 关）。mise 在容器里不会自动信任复制进来的项目配置，`mise trust` 不可省；Node 官方 Linux 二进制在 Debian slim builder 上需要 `libatomic1`。`compose.yaml` 的 PG18 把卷挂在 `/var/lib/postgresql`（**不是** `…/data`——PG18 移动了数据目录）。
- 提交：Conventional Commits，**中文主题**，scope = 一个项目名或 `deps`/`ci`/`agents`（由钩子强制）。远端是 Gitea，默认分支 `main`。
- `.gitignore` 是混合的：根目录只放工作区全局规则；每个项目拥有自己的构建/生成忽略。新增忽略规则加到拥有它的项目。不要行内 `#` 注释（必须自成一行）。
- **README vs AGENTS**：README 是给访客的推介（~90 行）；设计理据、不变量、坑、清单都放**这里**。绝不在两个文件里重复同一事实。

## Agent skills

> **语言约定（务必遵守）**：本仓库的 issue、PRD、评论，以及 `docs/agents/` 下这些文档本身，一律用**中文**撰写。仅 CLI 命令、标签名、代码标识符、文件路径保持英文字面量，其余全部中文。

### Issue tracker

Issue 与 PRD 存于 GitHub Issues（用 `gh` CLI）；外部 PR **不**作为 triage 入口。**所有 issue 的标题与正文一律用中文。** 详见 `docs/agents/issue-tracker.md`。

### Triage labels

五个规范化 triage 角色使用默认标签字符串：`needs-triage` / `needs-info` / `ready-for-agent` / `ready-for-human` / `wontfix`。详见 `docs/agents/triage-labels.md`。

### Domain docs

多上下文（multi-context）布局：根 `CONTEXT-MAP.md` 指向各上下文的 `CONTEXT.md`（`apps/server`、`apps/web`），系统级 ADR 在 `docs/adr/`，`proto/` 为跨端领域词汇的单一真源。详见 `docs/agents/domain.md`。
