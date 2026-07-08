# server

Go 后端领域。领域词汇的权威真源是 `proto/`；本文件只收录本上下文里**需要被消歧或对齐**的术语，引用而非重造 proto 定义。

## Language — 集成（Integration / Provider / Driver / Seam / Plugin / Domain）

**integration（集成）**：
一类基础设施关注点（storage / captcha / email / sms / llm / oauth / payment），统一走 profile CRUD + purpose 绑定。integration 是**关注点**，不是某个 provider 的实现，也不是它背后的持久表。（原称 channel/通道；因非通用总称且在 Go 里撞一等概念 `chan`，交叉验证后更名 integration——见 ADR-0005 术语；旧 ADR/代码里的 channel/通道 **等同** integration。）见 ADR-0003（工件归域不归 integration）、ADR-0005（integration 抽成独立模块）。
_Avoid_: channel（撞 Go `chan`，且暗示只限通讯/支付）、service 泛指；把单个 provider 或某张表叫 integration。

**provider（提供商 / 厂商身份）**：
一个外部厂商的**身份 = 选择键**（`alipay` / `s3` / `smtp`），即 registry 的 map key（`ProviderName()`）。provider 与 driver 是**同一事物的两面**——provider 是身份，driver 是它在本系统里的实现——**不是两个概念**。
_Avoid_: 把 provider 当成独立于 driver 的东西；照 Terraform 用法拿 provider 指整个 integration / 实现体。

**driver（驱动）**：
一个 provider 的**无状态**实现，藏在 integration 的 seam 之后，独占该 provider 的怪癖。driver **不碰 DB**；持久工件归消费它的 domain（见 ADR-0003）。它是可被抽成独立 Go 模块的单元（ADR-0005）。「支付 driver」是它在 payment integration 的特例。
_Avoid_: plugin（见下）、adapter、handler。

**seam（缝）**：
integration 暴露给 base、driver 藏于其后的接口：storage 的 `ObjectStore`、captcha 的 `Verifier`、email/sms 的 `Sender`、llm 的 `Chatter`、oauth 的 `Flow`、payment 的 `Gateway`。base 只认 seam，不认具体 provider。
_Avoid_: interface 泛指、port。

**plugin（插件）— 保留词，当前系统没有**：
特指**进程外、独立编译、运行时加载**的扩展（hashicorp/go-plugin 那种）。moonbase **刻意不做**（与 ADR-0002 决策 4 冲突）；编译期 driver registry **不是** plugin，go.work 多模块也**不是** plugin。除非在 go-plugin 语境，别用「插件 / 插件体系」称呼 driver / registry / 模块。见 ADR-0005 非目标。
_Avoid_: 用「插件 / 插件体系」指代编译期 driver registry 或 integration 模块。

**domain（域）**：
一个持久工件及其生命周期义务的归属方。integration 的产出若是持久工件（`files`、`payment_orders`），工件与表归 domain，**不归 integration**（ADR-0003）。domain 表留在 base，integration 模块零建表（ADR-0005）。
_Avoid_: 把 integration 当 domain；把 domain 表塞进 integration 模块。

## Language — 文件（File）

**file（文件）**：
`files` 表的一行——系统**认账**的一个已上传对象的元数据（object key、content_type、size、上传者、创建时间）。presign 即落库，一文件一行，精神上不可变（替换 = 新 file，不改旧行）。file 属领域层；桶里的原始条目属 driver 层，叫 object。头像 / 站点 logo / favicon 等一切上传物统一走 file，不存裸 key。见 ADR-0003。
_Avoid_: blob（与 Postgres bytea/lo 歧义）、object（保留给 `ObjectStore` seam / S3 层）、attachment（指引用关系，非文件本体）。

**attachment（引用）**：
`file_attachments` 表的一行——某个领域实体对一个 file 的引用（引用方类型 + 引用方 ID → file_id）。同一 file 可被多处引用。删引用方即删 attachment；file 的 attachment 归零后由清理任务回收。
_Avoid_: relation、usage、link。

**visibility（可见性）**：
purpose 的静态属性（public / private），代码写死，管理员不可改，file 行上不存。public = 读免鉴权、URL 稳定可长缓存（如 avatar、site asset）；private = 每次访问先鉴权、发短期签名 URL。写（PUT）永远要凭证，与 visibility 无关。driver 只执行 visibility，不定义它。
_Avoid_: 把 public/private 当成 bucket 或单个 file 的属性；ACL（暗示 per-file 粒度）。

**unattached（无引用文件）**：
创建超过宽限期且没有任何 attachment 的 file。是孤儿清理的唯一判定依据——清理任务删 unattached file 及其 object，而非扫桶对账。直传（presign）天然先产生 unattached file，业务保存时建 attachment 才「转正」。
_Avoid_: orphan（保留给「桶里有 object 但无 file 行」的另一种病态）、临时文件。

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
