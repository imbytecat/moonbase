# 支付渠道范围：国内最简收款，故意锁 CNY-only、直连-only

## 背景

支付渠道只服务一件事：**国内最简收款**——接入微信 / 支付宝把钱收进来。不做服务商（ISV），不接 Stripe，不碰国际。

底层 `channel`（`Catalog` + `Registry[Provider,Ops]`）与 `pay.Gateway` seam 本就 provider 无关，容易让人以为要把币种、商户模式都抽象成可变维度。但对「最简收款」而言，这些不是 seam 需要抽象的维度：两个 CN driver 都是 CNY、都是直连商户。

## 决定

**币种锁 CNY。** 金额固定为 `int64`「整数分」（100 分 = 1 元）；Alipay driver 用 `cents/100` 格式化成元字符串，WeChat 直接用整数分。`payment_orders.currency` 列保留、恒为 `'CNY'`，作为「这是个 CNY 系统」的诚实标注，而非运行时可变维度。**不**给 wire 加 `currency` 字段，**不**建 ISO 4217 最小单位指数表。

**商户模式锁直连。** 支付 `Profile.config` 只承载直连商户凭证 + 已签约产品清单（`methods`）+ 验签模式。

**签约参数不提前泛化。** Alipay 小程序 JSAPI 的 `op_app_id` 保留为一次性特例字段，不泛化成通用的 `method → params` 槽——目前只有它一个这种参数，泛化即早产抽象。

## 非目标（明确的「不」）

多币种、汇率、按订单/按 profile 变币种、Stripe 等国际 driver；服务商 / 子商户模式（`sp_mchid`/`sub_mchid`/`app_auth_token`）；花呗分期 / 周期代扣 / 合单等高级签约产品。

这些**都是增量的**：真要国际化就那时加一个新 driver + 一个 `currency` 字段；真要服务商就那时给 per-driver 配置加字段 + driver 里透传。seam 已验证不需要为它们现在动手术，所以现在不建。

**境外买家怎么办：** 定价与收款都用 CNY，汇率交给买家侧的 provider / 卡组织——境外买家用支付宝 / 微信能受理的支付工具（绑定境外卡、Alipay+ 合作钱包等）付款时，账单按 provider 的汇率折成其本币，而商户号**仍结算 CNY**。这满足「偶有境外买家」，代价是买家在店内看到的是 CNY 价、而非本地化币种展示；本地化那套（按币种独立定价 + 国际 driver + 报表本位币折算，Hetzner 模型）就是上面那条增量路径，明确推迟。

## 保留的本质复杂度

以下不是过度设计，是支付宝 / 微信**强加**的、接入就必须处理的复杂度，全部保留：验签模式（支付宝 公钥/证书、微信 公钥/平台证书）、method 目录（当面付/网站/JSAPI/APP…，商户只签约并使用其子集）、主动查询 + 异步回调双路对账（回调会丢）。

## 已知限制（有意接受）

对「最简收款」，这些是有意接受的限制，不是待办：

- **退款仅全额、一次性、无台账。** `RefundPaymentOrderRequest` 不带金额，`refundNo = out_trade_no + "R"`（每单一个、幂等）。部分退款 / 多次退款 / `partially_refunded` 态留给更重的生意。
- **订单无本地超时。** 未付款的 `created` 单会一直留在列表，直到有人 `SyncPaymentOrder` 撞见 provider 报 closed；无 `expires_at`、无扫描器。
- **创建无客户端幂等键。** `newOutTradeNo()` 每次随机，重试 / 双击 `CreatePaymentOrder` 会生成两张订单；演示由 admin 驱动，风险低。

## 考虑过的替代

早前一版曾决定给 wire 加 per-order `currency` + 按币种最小单位指数，理由是「加 Stripe 只是加 driver」。澄清需求后（只做国内最简收款、不碰国际、不做服务商）判定为**为不存在的未来造抽象**，收回。
