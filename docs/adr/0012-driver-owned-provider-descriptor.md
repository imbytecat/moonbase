# Driver 拥有完整 provider descriptor

> **状态**：accepted，provider contract 的 Go struct-first schema producer、typed registration 与 lifecycle wire 由 ADR-0014 进一步细化。

新增 provider 必须做到零前端、零 proto、零 base 特例：driver 除 config schema 与 Ops 外，还拥有 provider key、中文名称、说明、品牌色和可选 namespaced icon ref（如 `antd:WechatOutlined`）。图标由 Web 的通用 resolver 懒加载并提供缺省回退；前端不得维护 provider→名称、说明、图标或品牌色映射，也不再按 provider 拼 profile 摘要和 badge，profile 列表只显示 profile name 与通用 provider presentation。

Wire 上使用有序 `repeated ProviderDescriptor`，每项组合 key、presentation 与独立的 config form；不再使用 `map<string, ProviderForm>`，也不把 provider 品牌信息塞进只描述配置值的 JSON Schema。Payment 通过 `PaymentProviderDescriptor { provider, methods, capabilities }` 组合扩展通用 descriptor，其他 integration 不感知支付概念。

Go 侧 registry 是唯一组合根：每个有序 registration 同时拥有 provider key、presentation、私有 config 类型、lifecycle policy 与 typed Ops；descriptor、provider key 列表、schema 生成/验证、严格解码、lifecycle 与 Ops 派发都从 registry 派生。删除独立 `Schemas()`、每-driver `Usable` 函数和其他平行 provider 目录；重复 key、空 presentation、非法 icon ref 或缺失 config/Ops 在 registry 构造或契约测试时失败。

这里的「driver 拥有」必须落实为 Go 包边界，而不只是同一 integration 包里的代码约定：每个 provider 是独立、可单测的子包，独占自己的 key、presentation、config schema 与操作实现；可复用 integration 根包只拥有 provider 执行模型、registration 与 registry 类型，不得承载具体 provider 的协议实现或应用 purpose 路由。新增 provider 只新增其子包，并在显式组合根增加一个有序 registration。

每类 integration 的应用 facade 定义最小业务 seam；email 例如在 `apps/server/internal/mail` 暴露按 purpose 发送消息的 `Sender`。可复用 email registry 只提供“使用已选 Profile 发送”的执行入口。provider registration 的泛型构造器接收形如 `func(context.Context, PrivateConfig, Message) error` 的 typed Ops，并在 registry 边界完成类型擦除；provider 自行持有所需依赖。应用 facade 的 `registry.go` 按顺序显式选择本项目编译的 provider，并由服务器启动组合根构造后，把同一个不可变 registry 注入业务运行面与 system 管理面；可复用 integration 不提供 `builtin` provider set，也不保留包级全局 registry。不使用 `init()`、blank import 或全局可变自注册，以免注册集合、顺序和依赖藏进导入副作用。

跨 integration 只共享 `core/config.Contract[T]`：它负责 config schema、验证、严格解码、canonical 编码、lifecycle 与 UI 投影。每个 integration 以自己的请求/结果定义 typed registration 和 registry executor；不得为了复用 registry 机械代码而发明通用 `Driver.Execute(any, any)`。Go interface/seam 由消费该能力的一侧按最小需求定义，类型擦除只发生在 registration 内部闭包中。

registration 与 registry 都是不可变、并发安全的，并且只捕获 provider 的稳定依赖（如 `http.Client`、clock、local runtime）；不得捕获某个 Profile 的 config 或凭据。应用 facade 每次调用解析当前绑定 Profile，registry 将其 canonical config 验证/解码为私有类型后传给 typed Ops，因此多个 Profile 可共享同一 registration，配置修改在下一次调用立即生效。core 不缓存 provider SDK client 或凭据；若实测 client 构造昂贵，只允许对应 provider 自行实现有界、可失效的缓存，并以 canonical config 摘要寻址。

`Profile.config` 只在 wire、持久化与通用 config engine 中保持结构化动态值。registry 根据 registration 生成的标准 schema 验证后，严格解码成 provider 私有 config，再调用 typed Ops；动态值不得跨入 provider 协议实现。不得把 `SMTPConfig` 一类 provider 配置导出给 base，也不得建立汇总所有 provider 字段的 integration 级配置 union。JSON Schema 是跨控制面的公开契约，具体 Go 配置结构是它的作者真源与 driver 私有实现。

descriptor 与执行实现通过 integration 根包定义的 opaque `Registration` 原子绑定。每个 provider 只导出一个构造 registration 的入口，把 key、presentation、私有 config 类型、lifecycle policy 与 typed Ops 一次性交给泛型构造器；外部不能分别替换这些组成部分，也不能把一个 provider 的契约配给另一个 provider 的实现。registry 只接收擦除后的 registration，并由它派生 descriptor、通用 config 行为与运行时派发。

Purpose 不属于 driver，也不属于可复用 integration 包：具体 purpose key、展示文案和 `single/multiple` 绑定基数由 `apps/server/internal/<integration>` 应用 facade 拥有，并通过 `PurposeDescriptor` 下发。该 facade 同时拥有 settings loader、binding 解析与按 purpose 寻址的业务 seam；它先解析出已选 Profile，再调用 reusable registry。`packages/integrations/*` 只执行已经选定的 Profile，不得 import 应用 settings、声明 Purpose/Catalog、持有 Loader，或暴露 `Send/Verify/Complete(ctx, purpose, ...)` 一类业务路由方法。各 integration describe RPC 同时返回应用 purposes 与 provider descriptors；catalog 同时作为绑定写侧校验和管理端展示真源。新增整类 integration 仍是显式核心动作；本决策只保证高频 provider/driver 是 drop-in 扩展单元。

完整 descriptor 属管理控制面，仅经 `system.read` 保护的 describe RPC 返回。公开或付款运行时读面只从同一 descriptor 通用投影当前已绑定、当前上下文需要的最小安全字段：OAuth 登录页不获得 config form 或未绑定 provider，hosted checkout 不获得其他 purpose/profile 或完整 capability 目录。投影是安全裁剪，不是第二份 provider 元数据。

存量 Profile 的 provider 不存在或 config 不符合当前 contract 时，管理面只返回 `config_valid=false` 与安全投影：未知 provider 的 config 完全隐藏，已知 provider 的未知字段全部丢弃；前端以 descriptor 是否存在判断 provider 是否已移除。运行面不得派发此类 Profile；单条坏数据不阻止 registry 或服务启动。

## 考虑过的替代

- **前端 provider 映射表**：实现直接，但每个 provider 都要求 Web 改动，违背 drop-in 边界。
- **把 presentation 塞进 JSON Schema**：混淆 provider 身份与 config value 形状，选择器、列表和非表单消费者无法自然复用。
- **driver 携带 SVG 或 React 组件**：前者增加资源交付与安全处理，后者反向绑定前端 runtime；namespaced icon ref 用受控体积换取更小的扩展 interface。

## 后果

- 所有 `Describe*Providers` RPC 改为有序 descriptor 列表；registry 声明顺序即管理端展示顺序。
- Provider presentation 的一致性、icon ref 格式和 descriptor/config form 完整性由契约测试守卫。
- 新 provider 只新增自封闭 driver 包，并在应用 facade 的 `registry.go` 加一行构造；Web 仅在新增通用图标来源、字段类型或交互能力时演进。
- 下游项目可在应用组合根增删或替换 provider，不修改 reusable integration；测试可注入只含 fake registration 的 registry。
