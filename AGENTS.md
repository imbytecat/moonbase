# AGENTS.md

moonrepo monorepo。`proto/`（Protobuf + Buf + ConnectRPC）是单一真源：`moon run proto:generate` 同时重新生成 Go 服务端（`apps/server`）与 TS 客户端（`packages/api-client`），后者由 React 19 SPA（`apps/web`）消费。字段/RPC 错配是**编译错误**，不是运行时惊喜。工具链由 **proto** 锁定（`.prototools`：go/node/pnpm/moon），任务由 **moon** v2 编排，包管理器 **pnpm**。

它作为一个管理系统**模板交付，而非框架**：下游项目复制本仓库、各自演化，并通过 `git remote add template` 把修复挑拣（cherry-pick）回来。由此带来的、会改变你工作方式的后果：保持 channel 包不引入业务代码（保证 diff 可移植）；**不要**抽取共享 Go 库、拆分微服务，或添加 semver/向后兼容垫片——没有任何外部依赖此代码，所以 settings 结构体的变更可以合理地把旧行零读（zero-read）。驱动注册表**就是**插件系统（编译期，`database/sql` 风格）。

**第三方库原则：应上尽上，差异过大不硬上。** 成熟前沿的库/最佳实践优先于手搓——省维护精力、保持逻辑清晰、把精力留给业务；但与需求实在有差异时不硬套。两个方向都要留下理据：采用了写清为什么（如 `coreos/go-oidc` 之于 OIDC），否决了也写清评估过什么、为何不值（如微信扫码手写 3 次调用——否决 silenceper/PowerWeChat；local 存储 handler ~120 行标准库——否决 gocloud.dev，SignedURL 之外 serve 的活它一行不省）。判据是**净收益**：库替你扛掉的怪癖/协议面，要大于它带来的依赖面。

## 命令（在仓库根目录执行）

- `proto install` —— 从 `.prototools` 安装 go/node/pnpm/moon；proto 激活会把它们放上 PATH（即使非交互式 shell 也是），无需 `export`。
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
- `apps/server/internal/systemcodec/` ← `moon run proto:generate`（`protoc-gen-settings`，一个仓库本地的 buf 插件）：channel 档案存储结构体 + 只写密钥的 `Mask`/`FromProto`/`Merge` 编解码器，每个 `option (moonbase.v1.profile)` 消息一套。`settings.*Profile` 是指向它的别名；驱动直接 import 它。
- `apps/server/internal/repository/` ← `moon run server:generate`（sqlc）。
- `apps/web/src/routeTree.gen.ts` ← `moon run web:gen`（TanStack Router）。
- `apps/web/src/paraglide/` ← `moon run web:gen-i18n`（Paraglide，源自 `messages/*.json`；Vite 插件也会在 dev/build 时重新生成）。
- **air 不监视 `proto/`** —— 改完 `.proto` 后，运行 `moon run proto:generate`（moon 会缓存，未变更时近乎免费）。
- **sqlc 不做清理**：删掉某个 `db/query/*.sql` 会留下过期的 `internal/repository/*.sql.go`，构建会因被删类型而中断——`rm` 掉那个生成的孪生文件，再重新生成。

## 护栏测试——测试变红意味着修你漏掉的那一侧，绝不弱化测试

- `internal/server/authz_test.go` —— 每个注册的 RPC 都需要一条授权规则。新增 proto service ⇒ 在此加生成的空导入（blank import）+ 路径前缀，规则写在 `authz.go`。
- `internal/rpc/providers_test.go` —— proto `provider` `in:` 列表里的每个 provider 都需要一个 Go 驱动，反之亦然；`TestPaymentMethodsMatchContract` + `TestPaymentProfileMethodsMatchContract` 让每笔订单的 method 与档案的已签约产品列表同 `pay.Methods()`（驱动目录的并集）对齐。
- `internal/rpc/secrets_test.go` —— 每个只写密钥必须在空值更新下存活；缺失 `keepSecrets` 分支 = 凭据被抹掉。
- `apps/web/src/lib/messages.test.ts` —— `zh-CN.json` / `en.json` 的键必须保持一致（parity）。
- `internal/config/config_test.go`（`TestLoadEnvOverrides`）—— 每个配置键都需要一个 viper 默认值，否则其 `MOONBASE_*` 环境变量会被静默忽略。

## 新增一个 API 域

1. `proto/<domain>/v1/<domain>.proto`（protovalidate 规则内联）→ `moon run proto:generate`。
2. 从 `packages/api-client/src/index.ts` 重新导出生成的 `_pb` + `_connectquery`——忘了这步会破坏 web 的 import。
3. 迁移（`moon run server:migrate-new -- <name> sql`）+ `db/query/*.sql` → `moon run server:generate`。
4. 在 `internal/rpc/` 实现 service，带一句 `var _ <x>connect.Handler = (*Svc)(nil)` 断言；在 `internal/server/router.go` 注册。
5. 在 `internal/server/authz.go` 为**每一个** procedure 写授权规则 + 在 `authz_test.go` 加生成的空导入**和**路径前缀。
6. 新权限：给 `Permission` 枚举（`proto/auth/v1/permission.proto`）加一个值**并**加一条匹配的 `auth.Catalog` 条目（`internal/auth/permissions.go`）——两者漂移时 `TestPermissionEnumMatchesCatalog` 会失败。
7. Web：在 `src/routes/_authed/` 下加路由，beforeLoad 里用 `requirePermission`；在 `NAV_TREE`（`src/lib/navigation.tsx`）加叶子节点；在 `messages/{zh-CN,en}.json` **两个**文件里加文案；在 `src/lib/permissions.ts` 加一对 `permission_*` 文案。

移除一个域 = 反向操作 + 一个把已存 `role_permissions` 映射到后继键的迁移（键存在角色行里）+ `rm` 掉孤立的 sqlc 产物。

## 给现有通道新增一个 provider

1. 在 `proto/system/v1/system.proto` 新增一个配置消息（它**自己**的字段形状，绝不复用别的驱动的）+ 把该值加入对应 `*Profile.provider` 的 `in:` 列表。给每个只写凭据字段标注 `[(moonbase.v1.secret) = true]`，并给它一个兄弟字段 `bool <field>_set`（读侧的掩码标志；`<field>_set` 这个名字是 `protoc-gen-settings` 匹配的**硬性**约定）。→ 重新生成。
2. **不要**手写 mapper。`moon run proto:generate` 会运行 `protoc-gen-settings`（`apps/server/cmd/protoc-gen-settings`，在 `buf.gen.yaml` 里接线），它为每个标了 `option (moonbase.v1.profile) = true` 的消息，向 `internal/systemcodec`（git 忽略）产出：存储结构体（`<field>_set` 标志被丢弃——它们仅存在于 wire 上）、它的 `ProfileID`/`ProviderName`/`WithID`，以及一个带 `FromProto`/`Mask`/`Merge` 的 `<Channel>Codec`。`Mask` 清空密钥 + 置 `<field>_set`；`Merge` 在空值更新时保留已存密钥，并保留 `[(moonbase.v1.immutable) = true]` 字段（如 oauth 的 `key`）。`channelOps.keepSecrets` 就是 `systemcodec.<Channel>Codec.Merge`；处理器调用 `Codec.FromProto`/`.Mask`。
3. 在该 channel 包的 `drivers` 注册表（`channel.Registry`）加一条驱动条目；驱动代码寻址 `systemcodec.<Channel>Profile`（**不是** `settings.*Profile`——那些现在是生成的别名，且 Go 字段名遵循 proto 的 CamelCase：`AccessKeyId`、`ApiKey`、`OpAppId`，而非 `AccessKeyID`/`APIKey`/`OpAppID`）。
4. Web：在 `src/components/system/<channel>-profile-drawer.tsx`（captcha/llm 则是 `<channel>-panel.tsx`）加一个 `ProviderOption` 卡片 + 一个按 provider 分支的配置字段块 + 文案。抽屉**只**提交所选 provider 的值；生成的 `Merge` 会回填其他 provider 的已存配置，让凭据存活。掩码标志读作 `<field>Set`（如 `smtp.passwordSet`），与 proto 的 `<field>_set` 对应。

若改为新增一个用途（PURPOSE）= 一个常量 + channel 包里的 `Purposes` 目录条目 + 一个 `PURPOSE_LABELS` 条目 + web 文案。

新增一整个 channel = 以上全部，外加一个标了 `option (moonbase.v1.profile) = true` 的消息（生成器会自动发现它）、一个 `settings.<Channel>` 类型别名 + `Store` 的 getter/setter，以及一个带标准 `channelOps` 接线的 `system_<channel>.go`。

## Settings = 两个后端面（web 呈现为一个）

- `settings.v1` = 业务开关（`settings.*` 权限）：注册策略、注册标识符、手机区域、站点（SITE）标识。`GetSiteInfo` 是**公开**的（登录页在未登录时渲染它）。新的产品开关 → 放这里。
- `system.v1` = 带密钥的基础设施通道（`system.*` 权限）：storage/captcha/email/sms/llm/oauth/payment。新通道 → 放这里。**没有**通用的 UpdateSystemSettings——只有 GetSystemSettings + 每通道的档案 CRUD/Bind/Test。
- web 把两者都呈现在 `/settings/*` 下（按权限过滤的分组）；这个拆分存在于 proto/权限里，而非导航里。
- **密钥在 wire 上只写**：读取时掩码（`secret_set`）；更新时的空值保留已存密钥（每通道的 `keepSecrets`）。
- **Settings 存储是 JSONB，无迁移**（`internal/settings`）：结构体形状变更会静默地把旧行零读——重置 dev 卷或重新录入配置（真实部署也一样）。缺失的行读作零配置，所以无需 seeding。
- **统一的通道模型**：`settings.Channel[P]`（Profiles + Bindings `map[string][]string`）；`channelOps[P]`（`internal/rpc/system_channel.go`）是每个通道 Create/Update/Delete/Bind 背后唯一的生命周期。每通道的文件只做 proto⇆settings 映射。
- **绑定即激活** —— 任何地方都**没有**每档案的 `enabled` 标志（它会造出一个"已绑定但被禁用"、语义未定义的状态，如被静默禁用的 CAPTCHA）。要暂停就解绑。
- **用途目录是代码**：每个通道导出一个 `channel.Catalog`（`storage.Purposes`、`mail.Purposes`……）。业务代码按用途寻址通道，绝不用档案 id：`mail.Sender.Send(ctx, purpose, …)`。未绑定的用途 → `ErrNotConfigured`（CAPTCHA 例外 = 直通，让全新安装仍可登录）；删除已绑定的档案 = FailedPrecondition。多值用途（oauth 的 `login`、所有 payment）携带 `profile_ids` 并扇出；其余为单值。
- **驱动 = 一个接缝（seam）背后的每 provider 配置形状**：一个带标签联合的档案（每 provider 一个子消息）；**所有** provider 配置并排持久化，所以切换永不丢凭据。绝不要把 provider 参数摊平到共享字段——只有接缝（Send/Verify/Complete/……）是共享的。
- OAuth 档案的 `key` = `user_identities.provider` 里的 slug 以及流程 URL `/api/oauth/{key}/...`；创建后**不可变**；删除一个仍有身份行的 = FailedPrecondition。
- Web：每个通道复用 `ProfileManager` + `ProfileFormDrawer`；每个表单都走 `FormDrawer`（脏数据守卫关闭）——绝不要在表单外直接挂一个裸的 antd Drawer。新通道 = 在 `src/components/system/` 加新面板 + 一条 `src/lib/settings-nav.tsx` 条目。
- 用户可见文本（proto 注释、UI 文案、错误字符串）里不要出现工具/库名——描述协议（"SMTP"、"S3-compatible"），而非实现。错误展示一条通用的已翻译消息，绝不用裸的 `err.message`。

## 能力 = 控制平面 + 可选数据平面

把"什么连接"（控制平面：上面的档案/绑定/注册表模型——JSONB、无迁移、无状态）与"发生了什么"（数据平面——挑最弱的层级，别发明通用抽象）分开：第 1 层 短寿命密钥 → `verification_tokens`；第 2 层 只追加账本 → 审计模式；第 3 层 领域状态机 → 真正迁移的表（`payment_orders`、DBOS 检查点）。Settings JSONB 永不存状态；状态表永不存凭据。接缝签名刻意按域定制——没有通用的 channel 接口。

- **审计**（`internal/audit` + audit.v1）：一个拦截器接缝记录每个会改动的一元 RPC——处理器从不写审计行。请求载荷**从不**存储（密钥即便对审计轨迹也保持只写）；只读的 RPC 面；经 `MOONBASE_AUDIT_RETENTION_DAYS` 的每小时保留清道夫（默认 180，0 = 永久）。
- **工作流**（`internal/workflow` + workflow.v1）：DBOS 是一个**库**，把检查点写入**同一** Postgres 的 `dbos` schema，并在启动时续跑被中断的运行。工作流是注册的**代码**；nil 引擎（单元测试）会让工作流 RPC 回 FailedPrecondition。
- **支付**是唯一带数据平面的通道：迁移出来的 `payment_orders` 表拥有状态机；每次结算写入都用 SQL 状态守卫（`WHERE status IN (...)`），所以被重放的 provider 回调和并发同步是幂等的。回调是朴素的 `POST /api/payment/notify/{provider}/{profile}`，由驱动的签名校验鉴权（无会话）。**method 是 provider 范围内的官方产品 id**（支付宝 API method `precreate`/`page_pay`/`wap_pay`/`create`/`app_pay`；微信 trade_type `native`/`h5`/`jsapi`/`app`）——**不是**一个共享三元组：每个驱动声明一个 `pay.Method` 目录（id + `CredentialKind` qr/redirect/params + 必需的 `Inputs`），一个档案为其中一个子集签约（`PaymentProfile.Methods`，空 = 全部 → `pay.Offered`），收银台只提供那些，前端在 `src/lib/payments.ts` 镜像该目录、并按 `order.credentialKind` 渲染。支付宝 `create`（小程序 JSAPI）需要 `op_app_id`，否则下单失败。
- **通知 + 出站 i18n**（`internal/notification` + notification.v1；`internal/i18n`）：每用户的站内信收件箱。业务代码经 `notification.Publisher` 接缝通知——`Publish(userID,…)` / `PublishToPermission(perm,…)`（向持有某权限者扇出）——**绝不**直接写 `notifications` 行；读侧是自限定范围的 RPC（authz `{}` + `IdentityFromContext`，所以用户只看到自己的）。出站文本（收件箱标题/正文、验证/重置/验证码邮件）经 `internal/i18n` 本地化（`Resolve`：`user.locale` → 请求 `Accept-Language` → 默认 `zh-CN`），并**按收件人**已渲染地存储/发送——但 RPC 错误消息在服务端**不**本地化（它们保持为代码，由 SPA 人性化展示）。`users.locale`（`CurrentUser.locale`）是账号语言；SPA 在登录时经 `setLocale` 应用它（靠重载收敛），公开的认证页带一个匿名切换器。

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
- 插件以固定版本运行，无需安装：Go 经 `local: ['go','run','…@ver']`，TS 经 `local: ['pnpm','exec',…]`。**TS 插件的 devDeps（`@bufbuild/protoc-gen-es`、`@connectrpc/protoc-gen-connect-query`）放在根 `package.json`**，这样 `pnpm exec`（由 buf 从仓库根调用）能从根 `node_modules/.bin` 解析到它们。生成器↔运行时版本在 `pnpm-workspace.yaml` 里按 catalog 锁步固定（Buf 要求插件与运行时匹配）——两个 catalog 条目一起升，绝不只升一个。
- 处理器挂在标准 `http.ServeMux` 上，规范路径 `/pkg.v1.Service/…`，经 `StripPrefix` 挂载在 `/api` 下。Connect 协议走 HTTP/1.1 + JSON（一元可 curl；无需 h2c）。

## 数据库与迁移

- Postgres 18+（`uuidv7()` 默认值）。无 `.env`、无配置文件——只有默认值 + `MOONBASE_*` 环境变量（`internal/config`，viper）。**每个配置键都必须有 `v.SetDefault`**，否则其环境变量被静默忽略（AutomaticEnv+Unmarshal 只看得见已知键；由 `TestLoadEnvOverrides` 守卫——加键时扩展它）。
- **迁移在服务器启动时自动应用**（`db/migrations/` 里的 goose SQL，内嵌，加咨询锁以防副本竞争）。`migrate-*` 这些 moon 任务仅供手动运维。
- sqlc 只把迁移的 Up 段解析为它的 schema；查询在 `db/query/*.sql`；部分更新用 `sqlc.narg('x')` + `coalesce`，配合 proto3 的 `optional`。

## moon v2 配置陷阱

- 项目 `moon.yml` 用 `layer:` + `stack:`（**不是** `type:`/`platform:`/`toolchain:`——它们会报错）。工作区用 `vcs.client`（**不是** `vcs.manager`）。
- v2 把 toolchain 文件改名为复数 `.moon/toolchains.yml`，但其 `$schema` 仍是**单数** `…/toolchain.json`。它不管理任何语言工具链（版本归 proto 管）；任务经 moon 的 `system` 工具链运行。
- 官网的 JSON schema 落后于 2.3.5 二进制——相信 moon 实际的解析错误，而非官网。
- 共享任务只定义一次，放在 `.moon/tasks/{go,typescript}.yml`（按 `language:` 匹配继承）；同名的项目任务会**合并**（追加 deps），如 server 的 `test`/`check` 加 `deps: ['proto:generate','~:generate']`。`proto` 不设 `language`，自定义它自己的任务。

## Lint 与 git 钩子

- 每个项目的只读闸门是 `check`，写侧孪生是 `fix`。后端 = golangci-lint v2（`.golangci.yml`）经 `go run`；前端 = Biome（`biome.json`：2 空格、单引号、无分号、宽度 100；格式化 + linter + import 排序；自动跳过生成文件）。Proto = buf lint + format。
- **不要**单独运行 `go vet`、`go fmt`、`gofmt`、`goimports`——它们已是 golangci-lint v2 的子集（`default: standard` 含 govet；formatters 含 gofmt/goimports），裸跑既冗余又可能与 `.golangci.yml` 配置不一致。本地质量环路只有一条命令：`moon run :fix`。
- **Pre-commit 钩子**（自动同步；用 `moon sync hooks` 安装一次）：`moon run :fix` → `git update-index --again`（重新暂存修复）→ `moon run :check`，全部 `--affected --status=staged`。只有不可修复的错误才拦截。
- **Commit-msg 钩子**：`pnpm exec commitlint` 强制 Conventional Commits，要求**中文主题**（`subject-zh` 自定义规则）+ 一个 `scope-enum`（项目名 + `deps`/`ci`/`agents`），在 `commitlint.config.mjs` 里从 `.moon/workspace.yml` 的 `projects` 动态读取。
- 仓库移动后 golangci-lint 报错文件路径不对 = 缓存过期：`go run …/golangci-lint/v2/cmd/golangci-lint@<v> cache clean`。

## 前端（apps/web）

- **路径别名 `#*` → `./src/*`**，经 package.json 的 `imports`（Node 子路径 import，**不是** tsconfig 的 `paths`）。`["./src/*","./src/*.ts","./src/*.tsx"]` 这个回退数组是**必需**的（TS 按字面解析 `imports`，不猜扩展名）。`#messages/*` → `./messages/*`。用 `#lib/…`、`#components/…`；绝不用 `../`。
- **数据层是 ConnectRPC**，而非手写 fetch：传输在 `src/lib/transport.ts`（`baseUrl: '/api'`）；从 `@moonbase/api-client` import 生成的方法引用，用 `@connectrpc/connect-query` 的 `useSuspenseQuery`/`useMutation` 调用。proto 的 `Timestamp` 是一个消息——用 `@bufbuild/protobuf/wkt` 的 `timestampDate()` 渲染，而非 `Date`/字符串。
- `@connectrpc/connect` 必须是 `apps/web` 的**直接**依赖（仅传递依赖会破坏 `tsc`）。路由 search-param 接口必须被 `export`（未导出 → `routeTree.gen.ts` 里 TS4023）。
- `vite.config.ts`：`tanstackRouter()` **必须**在 `react()` 之前。dev 服务器把 `/api` 代理到 `:8080`。
- **tsconfig**：`allowJs: true` 是**必需**的（Paraglide 产出 JSDoc 类型的 `.js`；移除它会把所有 `m.*()` 退化成 `any`）。`noUncheckedIndexedAccess` + `verbatimModuleSyntax` 已开启。
- **i18n = Paraglide**（基于编译器）：目录 `messages/{zh-CN,en}.json`（扁平键，`{param}`）。所有字符串都是消息函数引用：`m.nav_dashboard()`；未知键或拼错的 param = 编译错误。导航里的 `label` 是消息**引用**（`m.nav_users`，不调用）。`--strategy` 列表同时存在于 `vite.config.ts` 和 `gen:i18n` 脚本里——保持二者一致。用 `humanizeError`（`src/lib/errors.ts`）映射后端错误，绝不用裸的 `err.message`。
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
- `CGO_ENABLED=0` 设在 `apps/server/moon.yml` 的 `env:`——保持服务器纯 Go（goose 用 modernc sqlite）。sqlc/goose/buf/air/golangci-lint 全部经固定版本的 `go run …@<ver>` 运行——在 `moon.yml` 里升版本，而非包管理器。
- **测试**：service 依赖 sqlc 的 `repository.Querier` 接口 → 单元测试用结构体嵌入的伪实现（无 DB、无 mock 框架）。集成测试构建真实栈，无 `MOONBASE_DATABASE_URL` 时跳过。
- 热重载 `server:dev` = air（`.air.toml`），监视 `.go`+`.sql`，`pre_cmd` 重新生成 sqlc。经 `moon run server:dev` 运行，以继承 CGO/env。保留 `.air.toml` 的 `[build.windows]` `.exe` 块——air 只会给它的默认 cmd 自动加 `.exe`，所以删掉它会静默地破坏 Windows。

## CI、Docker、约定

- `.gitea/workflows/ci.yml`：push 到 `main` + PR 时跑 `moon ci`（`moonrepo/setup-toolchain` + `pnpm install`）；checkout `fetch-depth: 0` 以做受影响 diff。PR 还会跑 `moon run proto:breaking` 对比 `origin/main`。一个专门的 `moon run server:vuln` 步骤跑 `govulncheck`（可达漏洞扫描；不在 `moon ci` 图里——依赖网络，`runInCI:false`）。一个 `postgres:18-alpine` 服务支撑集成测试（`MOONBASE_DATABASE_URL` 的 host = `postgres`，即服务 id）。
- **依赖更新**——本地一次性，无需 CI：`proto outdated --update --latest`（工具链 go/node/pnpm/moon → `.prototools`）、`pnpm update -Lr`（所有工作区 JS），以及在 `apps/server` 里 `go get -u ./... && go mod tidy`（Go 模块）；然后 `moon run :check && moon run :test`。`go run <module>@<ver>` 的工具固定（在每个 `moon.yml`、`.moon/tasks/*.yml`、`buf.gen.yaml` 里）没有标准的本地更新器——某个漂移时手改字符串（少见）；`renovate.json`（自定义 go manager，datasource=go，需要 `'module@vX.Y.Z'` 单引号形状）在有 runner 时可自动化那些 + 其余一切。`go run @version` 保留（buf 从仓库根而非 app 模块运行插件，所以 go.mod 的 `tool` 指令不适用）。可达 CVE（可选）：`moon run server:vuln`（govulncheck）。
- `Dockerfile`：proto → `.prototools` 工具链 → `moon run server:release` → distroless static（CGO 关）。`compose.yaml` 的 PG18 把卷挂在 `/var/lib/postgresql`（**不是** `…/data`——PG18 移动了数据目录）。
- 提交：Conventional Commits，**中文主题**，scope = 一个项目名或 `deps`/`ci`/`agents`（由钩子强制）。远端是 Gitea，默认分支 `main`。
- `.gitignore` 是混合的：根目录只放工作区全局规则；每个项目拥有自己的构建/生成忽略。`apps/web/src/paraglide/` 自我忽略（Paraglide 产出自己的）。新增忽略规则加到拥有它的项目。不要行内 `#` 注释（必须自成一行）。
- **README vs AGENTS**：README 是给访客的推介（~90 行）；设计理据、不变量、坑、清单都放**这里**。绝不在两个文件里重复同一事实。

## Agent skills

> **语言约定（务必遵守）**：本仓库的 issue、PRD、评论，以及 `docs/agents/` 下这些文档本身，一律用**中文**撰写。仅 CLI 命令、标签名、代码标识符、文件路径保持英文字面量，其余全部中文。

### Issue tracker

Issue 与 PRD 存于 GitHub Issues（用 `gh` CLI）；外部 PR **不**作为 triage 入口。**所有 issue 的标题与正文一律用中文。** 详见 `docs/agents/issue-tracker.md`。

### Triage labels

五个规范化 triage 角色使用默认标签字符串：`needs-triage` / `needs-info` / `ready-for-agent` / `ready-for-human` / `wontfix`。详见 `docs/agents/triage-labels.md`。

### Domain docs

多上下文（multi-context）布局：根 `CONTEXT-MAP.md` 指向各上下文的 `CONTEXT.md`（`apps/server`、`apps/web`），系统级 ADR 在 `docs/adr/`，`proto/` 为跨端领域词汇的单一真源。详见 `docs/agents/domain.md`。
