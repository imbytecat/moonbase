# 支付 UI 以托管收银台为唯一业务 interface

> **状态**：accepted。

业务页面只创建 checkout session 并打开同源 `checkout_url`；profile/product 选择、driver-owned product input schema 表单、支付动作渲染与状态观察统一封闭在 hosted checkout module。payment driver 保持纯 Go 后端 adapter，拥有 provider/product descriptor、输入 schema、协议调用、验签与状态映射；base 继续独占支付订单、幂等与状态迁移。

Checkout session 与 payment order 是两个生命周期：session 短寿命且尚未选择实际支付路径，可过期而不产生订单；确认 profile/provider/product 后最多转换成一个 durable payment order。payment order 创建时必须完成这些身份快照，且 base 先建立本地订单再调用 provider，避免上游下单成功而本地没有可协调记录。

Checkout session 使用数据库支撑的签名 capability，而非把金额、标题或返回地址编码进自包含 URL：`checkout_url` 只携带高熵随机 session ID 与服务端 HMAC，业务数据全部留在数据库。服务端可从 session row 重建同一 URL，支持 `CheckoutIssuer` 幂等重试；过期、撤销或已转换的 session 即使签名有效也不能继续使用。确认时通过事务完成一次性转换，并发只能得到同一个 payment order；该表属于支付数据平面，不复用 settings 或认证 `verification_tokens`。

Checkout session 由业务 domain 在服务端通过内部 `CheckoutIssuer` interface 创建；金额、purpose、业务引用、可信返回地址与幂等键均由服务端决定，浏览器只能消费生成的 `checkout_url`，不能直接创建任意金额的 session。管理系统可保留受 `payment.write` 保护的演示入口，但它不是业务接入 interface。

`CheckoutIssuer.Create` 以 `(purpose, idempotency_key)` 唯一：相同 key 与相同规范化 command 返回同一 session/URL，相同 key 但金额、业务引用等内容不同则拒绝；已转换、过期或撤销的 session 也不隐式新建，新的支付尝试必须使用新 key。`business_reference` 只标识业务对象、允许多次尝试；返回位置首版仅接受同源相对路径。

首版 checkout session 只逻辑过期、不物理删除：完整保留 idempotency key、状态和 converted order 关联，不增加 retention 配置、janitor 或 tombstone。未来真实容量或隐私要求出现时，可清除非必要载荷但必须保留幂等墓碑。

付款人只选择 payer-facing payment method（如支付宝、微信支付、银行卡），不接触 profile/channel 或 provider API product。托管收银台根据 purpose 绑定、method、客户端环境与路由策略选择实际 profile；payment driver 再选择具体 product。最终 payment order 快照实际 method/profile/provider/product，保持查询、审计与历史解释能力。

Payment method catalog 不是中央 proto 枚举：各 driver descriptor 以全局稳定字符串 key 声明其 method 与 presentation，托管收银台动态合并多个 driver/profile 的同 key 候选路径，契约测试要求同 key 的名称、说明和图标一致。新增 method 只要复用现有 input 字段类型与 payment action，就无需修改 base、proto 或 Web。

同 method 的候选路径按 purpose binding 顺序确定性路由；托管收银台只跳过本地可确认未配置或不兼容的候选，并在展示 product input 表单前把 profile/provider/product 锁入 checkout session。payment order 创建后绝不自动跨 provider failover：上游超时可能已受理请求，切换路径会产生双重支付风险；此时只能沿原路径查询或以同一商户订单号幂等重试。权重、轮询与智能路由等真实需求出现后再扩展。

具体 product 由 payment driver 的 `Plan` 操作按 method 与服务端提供的原始 `ClientContext` 选择；driver 自己解释 User-Agent、客户端环境与 provider 规则，不兼容时返回标准结果。base/router 只遍历候选并接受第一个有效计划，不包含移动端、内嵌浏览器或 provider 产品分支。

Payment action 使用 proto `oneof` 定义有限、provider 无关的 typed browser capabilities：展示二维码、跳转、提交表单、等待与 hosted flow。hosted checkout 只对这些抽象 action 分支；不采用开放的 `type + Struct`，避免动作错配退化成运行时错误或裸 JSON。driver 后续协调所需的私有 checkpoint 与客户端 action 分离，只能作为服务端 opaque state 存在。

创建成功的 typed payment action 以 JSONB 持久化在 payment order，并可带过期时间，以支持收银台刷新恢复；它只含允许付款客户端看到的短寿命数据，只能经签名 checkout session 保护的读面返回，管理端订单列表不暴露。当前支付宝/微信可按 `out_trade_no` 协调，故暂不增加通用 `provider_state`；真实 driver 需要服务端 checkpoint 时再单独评估。

Payment order 状态区分 `creating` 与 `pending`：base 先插入 `creating` 的 durable order，再调用 driver；provider 明确接受并返回 action 后进入 `pending`，明确拒绝进入 `failed`，网络超时等结果不明情形保持 `creating` 并沿原 profile/provider/product 查询。已受理订单可从 `pending` 进入 `paid` 或 `closed`，退款继续走 `paid → refunding → refunded`；所有迁移使用 SQL 状态守卫。

Payment driver 的核心 interface 要求 descriptor、`Plan`、`Create` 与 `Query`；`Create` 必须按 `out_trade_no` 安全幂等，`Query` 必须能判断 provider 是否存在该订单及其标准状态。`notify`、`refund`、`refund_query`、`hosted_flow` 由可选 capability interfaces 提供，registry 根据实际实现自动推导，禁止 driver 另写一份可能漂移的布尔目录。Notify 只降低结算延迟，不能替代 query；异步退款也必须有退款 query 或 notify 作为协调路径。

Base 必须先持久化 action 再向浏览器返回。若进程在 provider 建单后、action 落库前崩溃，恢复先 query 原 provider/out_trade_no：订单不存在则同 key 重试 Create，已终态则协调本地状态，pending 且 action 缺失时优先调用可选 `ActionRecoveryDriver`。无法恢复 action 的 attempt 进入带内部原因的 `failed`，由业务用新 idempotency key 创建新 session/order；当前 order 不切换路径，也不为该窗口提前引入通用 provider state。

现有 driver schema 拆成中立 `form.Schema` 与 settings 专属 `config.Schema`：前者拥有字段类型、选项、条件、校验及 JSON Schema/UI Schema 转换；后者组合 form 字段并增加 `Secret`、`Immutable`、Mask/Merge/Usable。provider profile 使用 config schema，payment product inputs 使用 form schema，前端继续复用同一个 rjsf renderer。

Hosted checkout 只消费有限、provider 无关的 typed payment action（展示二维码、跳转、提交表单、等待）。JSON Schema 只描述“收集什么输入”，不扩张为执行任意客户端 SDK 的脚本 DSL。极少数无法声明的 SDK 流程可由 driver 通过受限的 hosted flow HTTP seam 提供同源、短寿命页面；base 统一验证 token、设置安全响应头并分派，driver 不能直接写订单状态，也不能要求 Web 运行时加载任意远程模块。

## 考虑过的替代

- **业务 SPA 直接消费 `NextAction`**：会把 profile/product 选择、动作渲染、轮询和错误处理散到每个业务页面，interface 浅且重复。
- **provider 同时交付 Go driver 与 TS client adapter**：能力最广，但过早引入跨语言 manifest、ABI、构建、CSP、缓存与版本协调；仅在真实需求必须内嵌客户端 SDK 时重评。
- **用 JSON 元数据描述 SDK 调用**：会演化成难以校验和审计的客户端脚本 DSL，无法诚实封闭 SDK 生命周期。

## 后果

- 新 provider 复用现有 product input 字段类型与 payment action 时，只修改其 provider package，不修改业务 Web、web base 或 server base。
- 托管收银台成为支付 UI 的深 module，视觉、返回、过期、轮询、CSP 和 session 安全由它集中承担。
- 需要强内嵌体验的原生或浏览器 SDK 不被伪装成普通元数据变化；出现真实需求时再新增明确的 client adapter seam。
