# server

Go 后端领域。领域词汇的权威真源是 `proto/`；本文件只收录本上下文里**需要被消歧或对齐**的术语，引用而非重造 proto 定义。

## Language — 支付（Payment）

**支付 driver**：
一个 provider 的实现（`alipay` / `wechat`），藏在 `pay.Gateway` seam 之后，独占该 provider 的所有怪癖（金额格式、各 method 的 API 方言、交易态映射、回调验签）。
_Avoid_: gateway（指整个 seam / `pay.Client`，非单个 provider）、adapter。

**支付 profile**：
一条支付网关连接 = 一个 driver + 该 driver 的**直连商户**凭证 + 已签约产品清单（`methods`）。运行时可增删改。
_Avoid_: account、merchant、渠道配置。

**支付 purpose**：
代码定义的固定结算槽位（如 `checkout`）。多值：绑定到一个 purpose 的**每个** profile 都成为可选支付项，付款人在结算时择一。
_Avoid_: scene、场景、slot 单独使用。

**method（已签约产品）**：
一个官方 provider 产品——Alipay 的 API method（`precreate` / `page_pay` / `wap_pay` / `create` / `app_pay`）或 WeChat 的 trade_type（`native` / `h5` / `jsapi` / `app`）。profile 只提供它签约过的子集；是 per-order 的选择。
_Avoid_: channel、pay type 泛指。

**credential / credential kind**：
结算时客户端要渲染的凭据（credential），及其消费方式（credential kind）：`qr`（渲染二维码）、`redirect`（打开 URL）、`params`（把 JSON 交给 provider SDK）。
_Avoid_: token、payload。

**amount（金额）**：
`int64` 整数分（100 分 = 1 元）。Alipay driver 格式化成元字符串（`cents/100`），WeChat 直接用整数分。系统 CNY-only，故不做按币种的最小单位抽象。见 ADR-0001。
_Avoid_: 以「元」为单位的浮点；per-currency「最小单位 / minor units」抽象。

**currency（币种）**：
恒为 `CNY`。`payment_orders.currency` 列保留只是「这是个 CNY 系统」的诚实标注，不是运行时可变维度；wire 上没有 `currency` 字段。见 ADR-0001。
_Avoid_: 把它当作 per-order / per-provider 的可变维度。

**payment order（支付订单）**：
`payment_orders` 表的一行，即支付渠道背后那台持久状态机：`created → paid → refunding → refunded`，或 `created → closed`。`profile_id/name/provider` 是创建时快照，删 profile 不改写历史。
_Avoid_: transaction、交易单（指 provider 侧记录，非本地订单）。

**out_trade_no（商户订单号）**：
本地生成、发给 provider 的商户侧订单号，每订单唯一。
_Avoid_: trade no（指 provider 侧 `provider_trade_no`）。
