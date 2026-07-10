# 聚合支付抽象研究：稳定支付域、driver 产品目录与客户端动作

> 调研日期：2026-07-10  
> 目的：回答 moonbase 应如何把统一支付能力留在 base，同时让 provider 的产品、配置、文案与实现封闭在 driver 内。本文是研究笔记，不是已接受的 ADR。

## 结论先行

成熟实现并不把所有支付差异压成一个万能 `method`，而是稳定三层：

1. **支付域对象与状态机**：订单、金额、商户订单号、成功/失败/处理中、退款等由平台拥有。
2. **provider/product 目录与协议适配**：产品 ID、配置、支持能力、状态映射、签名、回调解析由 driver 拥有。
3. **客户端下一步动作**：创建支付后返回一个有判别字段的 action；客户端只实现少量稳定 renderer，而不理解 provider 的服务端协议。

对 moonbase 的直接建议是：

- 保留 base 的 `payment_orders`、幂等状态迁移、绑定、权限、RPC 和审计；这些是应用支付域，不属于 integration driver。
- 删除中央 `proto/payment/v1/method.proto` 及前端 `PROVIDER_METHODS`、`METHOD_LABEL`、`METHOD_DESC` 镜像；产品目录改由每个 payment driver 自描述。
- 把当前 `Credential string + CredentialKind` 提升为结构化 `NextAction{type, payload}`。renderer vocabulary 应小而稳定，例如 `display_qr`、`redirect`、`submit_form`、`invoke_sdk`、`wait`。
- `invoke_sdk` 的 payload 可以 opaque，但 SDK adapter 不可能靠字符串自动出现。新增 provider 只有复用已有 renderer 时才能做到零前端；需要新浏览器/原生 SDK 的产品，本质上是在新增客户端能力，必须显式改客户端。
- provider/product 的名称、说明、排序、图标引用、所需输入 schema 都归 driver；图标可使用带命名空间的引用，例如 `antd:WechatOutlined`，前端提供通用 resolver 和回退。
- 不把 Stripe Connect 的平台分账拓扑提前塞进 base。它属于另一层“商户/资金路由能力”；当前 direct-only、CNY-only 范围继续成立。

## 一、Stripe：稳定 Intent，开放 payment method，并显式返回 next action

### 1. PaymentIntent 是支付状态机，不是 provider 产品枚举

Stripe 建议每个订单或客户会话创建一个 `PaymentIntent`；它贯穿多次支付尝试，并通过状态迁移最终产生至多一笔成功 charge。官方对象包含金额、币种、`payment_method`、`payment_method_types`、`next_action`、`status` 等稳定字段，状态包括 `requires_payment_method`、`requires_confirmation`、`requires_action`、`processing`、`requires_capture`、`canceled`、`succeeded`（[Stripe PaymentIntent 生命周期](https://docs.stripe.com/payments/paymentintents/lifecycle)，[stripe-go 的 `PaymentIntent`](https://github.com/stripe/stripe-go/blob/50a83f367deb7a92cc0b144411565c1c5ab647c9/paymentintent.go#L11373-L11481)）。

这说明统一层应该抽象“支付意图/订单如何前进”，而不是试图统一每家厂商的官方产品名。moonbase 当前由 base 持有 `payment_orders` 和 SQL 状态守卫，这个方向与 Stripe 一致；不足在于状态只覆盖国内最简收款，且把“下一步怎么完成支付”压成了字符串 credential。

### 2. PaymentMethod 是开放的 tagged union，特殊字段不会被摊平

Stripe 的 `PaymentMethod` 有公共 `type`，同时附带一个与该 type 同名的专属对象，例如 `card`、`paypal`、`pix`、`wechat_pay`；官方说明也明确“附加 hash 的名字与 type 相同，包含该类型专属信息”（[PaymentMethod API](https://docs.stripe.com/api/payment_methods/object)，[stripe-go 的 `PaymentMethod`](https://github.com/stripe/stripe-go/blob/50a83f367deb7a92cc0b144411565c1c5ab647c9/paymentmethod.go#L2138-L2223)）。`PaymentIntent.payment_method_options` 同样按 `alipay`、`card`、`paypal`、`wechat_pay` 等分支保存各自配置，而不是寻找所有支付方式的最大公约数字段（[stripe-go 的 payment method options](https://github.com/stripe/stripe-go/blob/50a83f367deb7a92cc0b144411565c1c5ab647c9/paymentintent.go#L11260-L11326)）。

因此，moonbase 不应继续扩大 `CreatePaymentOrderRequest` 的顶层 `payer_id`、`return_url` 等 provider/product 条件字段。更可扩展的形状是公共订单字段加 `product_id`，再加由 driver product descriptor 描述、服务端校验的结构化 `inputs`。

### 3. `next_action` 是客户端动作协议，不是一个 `credential_kind`

Stripe 的 `next_action` 是显式 tagged union：除了 `redirect_to_url`、`use_stripe_sdk`，还有二维码、银行转账指令、便利店信息、微额验证、WeChat Android/iOS 调起参数等多种动作。`use_stripe_sdk` 的内容甚至被明确标注为只供 Stripe.js 使用、形状可变（[Stripe PaymentIntent 对象](https://docs.stripe.com/api/payment_intents/object)，[stripe-go 的 `PaymentIntentNextAction`](https://github.com/stripe/stripe-go/blob/50a83f367deb7a92cc0b144411565c1c5ab647c9/paymentintent.go#L10459-L10486)）。

关键经验不是照抄 Stripe 的所有 action，而是把两个层次分开：

- base 只承诺客户端真正能消费的 action vocabulary；
- driver 选择一种 action 并生成 payload；
- provider SDK 私有数据保持 opaque，由对应 adapter 消费。

当前 moonbase 的 `qr | redirect | params` 已经抓住了这个方向，但 `params` 过宽：它无法告诉客户端该调用哪个 SDK，也无法区分 GET redirect、POST form、等待异步动作。应改成有判别字段的结构，而不是继续往 `credential` 字符串里塞 JSON。

### 4. Connect 证明“钱怎么路由”是独立能力轴

Stripe Connect 把收款拓扑分成 direct charges、destination charges、separate charges and transfers，并通过 `on_behalf_of`、`transfer_data`、application fee 等字段表达平台、connected account 与费用承担关系（[Stripe Connect charges](https://docs.stripe.com/connect/charges)，[Stripe destination charges](https://docs.stripe.com/connect/destination-charges)，[Stripe separate charges and transfers](https://docs.stripe.com/connect/separate-charges-and-transfers)）。

这不是 payment product 的展示元数据，而是资金归属、责任与分账模型。moonbase 当前 ADR-0001 明确 direct-only，因此不应为了“以后可能接 Stripe”提前把 Connect 能力塞进通用 `Gateway`。若未来真做 marketplace，应新增明确的 merchant/settlement topology 能力，而不是在 driver config 里放一团无语义参数。

## 二、Omnipay：统一操作与响应，gateway 只声明自己支持什么

Omnipay 的公共 `GatewayInterface` 只强制显示名、短名和配置参数；`authorize`、`capture`、`purchase`、`completePurchase`、`refund`、`fetchTransaction`、`void`、通知等均是可选操作（[官方 `GatewayInterface`](https://github.com/thephpleague/omnipay-common/blob/cd58a4e7b359b0c5b9748542d422b5cc0d4a692f/src/Common/GatewayInterface.php#L15-L81)）。`AbstractGateway` 通过 `supportsAuthorize()`、`supportsPurchase()`、`supportsRefund()` 等能力查询暴露差异（[官方 `AbstractGateway`](https://github.com/thephpleague/omnipay-common/blob/cd58a4e7b359b0c5b9748542d422b5cc0d4a692f/src/Common/AbstractGateway.php#L169-L287)）。

响应只稳定抽象成功、需要 redirect、取消、消息、错误码和 gateway reference（[官方 `ResponseInterface`](https://github.com/thephpleague/omnipay-common/blob/cd58a4e7b359b0c5b9748542d422b5cc0d4a692f/src/Common/Message/ResponseInterface.php#L15-L64)）；redirect 再单独提供 URL、GET/POST method 和 form data（[官方 `RedirectResponseInterface`](https://github.com/thephpleague/omnipay-common/blob/cd58a4e7b359b0c5b9748542d422b5cc0d4a692f/src/Common/Message/RedirectResponseInterface.php#L17-L45)）。各 gateway 作为独立包依赖 `omnipay/common`，官方 README 也把这种包布局作为新增 gateway 的扩展机制（[Omnipay 官方仓库](https://github.com/thephpleague/omnipay#package-layout)）。

对 moonbase 的启示有两点：

1. 能力应该显式声明，不能假定每个 driver 都支持 query、异步通知、全额/部分退款、同步/异步退款、authorize/capture。
2. “redirect”本身也需要结构，而不是只有一个 URL；POST form 是成熟聚合层常见的稳定客户端动作。

Omnipay 的局限也值得避免：它大量依赖 PHP 动态方法和 `array` 参数，类型约束较弱。moonbase 可以保留 Go 的编译期 registry 和明确接口，同时借鉴其“公共操作 + capability + 专属 request/response”的边界。

## 三、彩虹易支付：证据局限与可借鉴部分

### 证据等级

彩虹易支付当前官方站点仍把该系统作为授权商品展示（[彩虹官方站点](https://www.cccyun.cn/)，其中链接到 `pay.cccyun.cc`），官方 GitHub 组织公开了支付宝、微信支付等 SDK，但没有公开易支付核心仓库（[netcccyun 官方 GitHub](https://github.com/netcccyun)）。因此，本次没有找到可验证的当前官方核心源码或官方 API 文档，不能把网络上的“源码分享”仓库当成一手事实。

下面只把标称“2020 原版开源”的社区镜像作为低可信结构样本，不用它单独支撑最终设计结论：

- 商户入口接收稳定的 `type`、`out_trade_no`、`notify_url`、`return_url`、商品名和金额，再由 channel 选择 plugin 并加载 plugin 的 `submit` 文件（[社区镜像 `submit.php`](https://github.com/Blokura/Epay/blob/a2f29a310b3b0fbc75dc724d521e2f35f8063d8a/submit.php#L46-L121)）。
- plugin 的 `config.ini` 自带英文名、显示名、作者、支持的支付类型和配置输入字段（[社区镜像 `plugins/epay/config.ini`](https://github.com/Blokura/Epay/blob/a2f29a310b3b0fbc75dc724d521e2f35f8063d8a/plugins/epay/config.ini#L1-L21)）。
- 查询接口把多种插件结果压成订单号、类型、金额、状态等少量字段（[社区镜像 `api.php`](https://github.com/Blokura/Epay/blob/a2f29a310b3b0fbc75dc724d521e2f35f8063d8a/api.php#L108-L135)）。

这个样本体现了“外部稳定商户 API + 内部 channel/plugin + plugin 自带元数据”的经典聚合支付结构。但其 drop-in 体验依赖 PHP 运行时加载与服务端输出/跳转支付页面；它并没有解决 React SPA 如何自动获得新的 SDK renderer。moonbase 可借鉴 descriptor 与路由分层，不应照搬文件式插件或让 driver 直接渲染 HTML。

## 四、moonbase 当前契约的边界问题

当前设计已有正确基础：

- `payment_orders` 和状态守卫在 base，driver 无数据库状态。
- `pay.Gateway` 把创建、查询、退款、通知验签隔离到 provider seam 后。
- profile 配置由 driver schema 拥有，base 统一掩码、合并和校验。
- `CredentialKind` 已经意识到客户端动作不等于 provider product。

但仍有四处 provider 泄漏：

1. [`proto/payment/v1/method.proto`](../../proto/payment/v1/method.proto) 中央枚举列出支付宝、微信的全部官方产品，新增 driver 必须改 proto 和生成器。
2. [`packages/integrations/payment/catalog.go`](../../packages/integrations/payment/catalog.go) 又维护同一份产品定义，driver 注册表没有真正拥有 catalog。
3. [`apps/web/src/lib/payments.ts`](../../apps/web/src/lib/payments.ts) 维护 method 中文名称/说明；[`payment-profile-drawer.tsx`](../../apps/web/src/components/system/payment-profile-drawer.tsx) 还给表单 schema 回填文案。
4. [`payments.tsx`](../../apps/web/src/routes/_authed/payments.tsx) 根据 provider 写扫码提示，并把未知 `params` 只展示成 JSON；这说明行为差异尚未被 action protocol 封闭。

此外，`method` 这个词在成熟平台里通常指付款工具/支付方式，而当前 moonbase 用它表示“某 provider 的官方产品/API 方言”。`app` 等 ID 也可能跨 provider 冲突。更准确的领域名是 `product_id`；其作用域必须是选定的 `profile/provider`，不应再构造全局 union。

## 五、推荐抽象

### 1. Base 拥有的稳定支付能力

```text
PaymentOrder
  id / out_trade_no / purpose / profile snapshot
  product_id
  amount / currency / subject
  status
  provider_trade_no
  next_action（仅暴露给有权完成支付的客户端）
  provider_state（可选、JSONB、服务端 opaque、不得含长期凭据）

PaymentService
  ListPaymentOptions
  CreatePaymentOrder
  GetPaymentOrder
  SyncPaymentOrder
  RefundPaymentOrder
  webhook ingress
```

base 负责生成商户订单号、持久化、幂等键、状态转移、权限、审计、回调路由和敏感数据暴露边界。driver 只能返回“观察到的标准状态 + provider reference + opaque checkpoint”，不能直接改表。

状态不应机械复制 Stripe，而应按真实业务逐步增加。当前国内收款可保留 `created/paid/closed/refunding/refunded`；若要支持延迟支付和更广 provider，优先增加 `requires_action`、`processing`、`failed`，而不是把 provider 原始状态当本地状态。原始状态/错误码可作为诊断字段保存，但所有迁移仍由 base 的有限状态机校验。

### 2. Driver 拥有 provider 与 product descriptor

```text
DriverDescriptor
  key
  label
  description
  icon_ref            // 例如 antd:WechatOutlined
  config_schema
  capabilities        // query, notify, refund, async_refund...
  products[]

ProductDescriptor
  id                  // provider 官方 product/trade_type，作用域仅在本 driver
  label
  description
  input_schema        // 本次下单所需的额外输入
  action_types[]      // 该产品可能返回的客户端动作
  sort_order
```

`config_schema` 只服务 profile 配置表单；provider 身份和 product catalog 与它分开。这样选择器、设置页和收银台都消费同一份 descriptor，同时不把 JSON Schema 扭成品牌元数据容器。

profile 的“已签约产品”仍是 config 的一部分，但选项应由同一个 driver product catalog 生成，包含 label/description，不再由前端补齐。

### 3. 下单输入：公共字段 + schema 驱动的 product inputs

```text
CreatePaymentOrderRequest
  purpose
  profile_id
  product_id
  subject
  amount
  inputs: Struct
```

公共 base 不再枚举 `payer_id`、`return_url`、`client_ip`。其中：

- `client_ip` 属于服务端请求上下文，由 base 注入 driver request，不应让客户端填写；
- `return_url` 可作为通用 input descriptor；
- `openid`、buyer ID 等由 product input schema 声明并由 driver 校验；
- base 可通用执行 JSON Schema/typed schema 校验，driver 仍须做语义校验。

如果某输入需要 OAuth、定位、设备指纹等特殊采集器，descriptor 只写一个字符串并不能实现交互。此时需要新增一个明确的 input widget/collector vocabulary，属于 base 与客户端的能力升级。

### 4. 创建结果：结构化 `NextAction`

推荐的最小协议：

```text
NextAction
  type: string
  payload: Struct

内建 type：
  display_qr   { data, expires_at? }
  redirect     { url }
  submit_form  { url, method, fields }
  invoke_sdk   { adapter, payload }
  wait         { poll_after_ms? }
```

前端维护的是 `action type → renderer`，不是 `provider → renderer`。driver 只要返回已有 action，新 provider 就能零前端接入。`type` 应允许带命名空间的扩展，但未知 action 必须明确报“不支持此客户端动作”，不能静默展示原始 JSON。

`invoke_sdk` 中的 `adapter` 例如 `wechat-pay:web`、`alipay:miniapp`。这不会消除 adapter 本身的前端代码；它只是把选择与 payload 从 provider 页面分支中抽离。若目标是“任意新 driver 都零前端”，则可承诺的 product 只能限于 `display_qr`、`redirect`、`submit_form` 这类通用动作。

### 5. Provider opaque payload 的使用边界

建议区分两个 opaque 容器：

- `next_action.payload`：可发给付款客户端的短寿命数据，只供 renderer/SDK adapter 使用；不得包含 profile 长期密钥。
- `provider_state`：仅服务端持久化，保存后续 query/refund 所需的 provider session/reference/checkpoint；不能原样通过订单 RPC 返回。

不要把完整 provider response 当审计载荷或 API 响应。base 应只提升业务真正需要查询/索引的字段，例如 `provider_trade_no`、标准状态、失败原因分类；其余保持 opaque。

### 6. Capability 应分两级

- driver 级：`query`、`notify`、`refund`、`partial_refund`、`authorize_capture` 等服务端操作。
- product 级：可返回的 action types、所需 inputs、是否需要 payer-bound identity。

base 只调用 descriptor 声明支持且当前领域已实现的操作。新增 capability 是支付域演进，需要 base 明确接纳；新增复用既有 capability/action 的 product 才是纯 driver 改动。

## 六、建议的迁移顺序

1. 先给通用 provider descriptor 增加 `label`、`description`、`icon_ref`，让设置页删除 provider 名称、图标和摘要分支。
2. 将 payment `Method` 改为 driver-owned `ProductDescriptor`，把中文 label/description 和 input schema 搬入各 driver。
3. 让 `DescribePaymentProviders` 同时返回 provider descriptor 与 products，收银台直接消费，不再生成/导出中央 payment catalog。
4. 将 `CreatePaymentOrderRequest.method` 改为 provider-scoped `product_id`，把条件输入迁到 `Struct inputs`。
5. 将 `credential_kind + credential` 改为 `NextAction`；先实现 `display_qr`、`redirect`、`invoke_sdk`，如需兼容 POST gateway 再加 `submit_form`。
6. 删除 `method.proto`、`protoc-gen-paymentcatalog`、前端 method/provider 映射及 provider 专属列表摘要。
7. 最后再评估是否需要更丰富的状态、部分退款、authorize/capture 或 Connect 类资金路由；没有真实需求时不提前泛化。

## 最终边界判断

“新增 provider 零 proto、零前端、零 base”可以实现，但必须加一句精确定义：

> 新 provider 只能声明已有 base capability、已有 product input widget 和已有 client action renderer；它的配置、产品目录、文案、图标引用、协议调用、签名与状态映射全部封闭在 driver。任何真正新增的客户端交互或支付域能力，都不是 provider 元数据，必须显式扩展 base/客户端协议。

这条边界既能保持 base 纯净，也不会用 opaque JSON 假装已经消除了真实的客户端耦合。
