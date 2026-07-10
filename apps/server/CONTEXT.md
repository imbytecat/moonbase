# server

Go 后端领域。领域词汇的权威真源是 `proto/`；本文件只收录本上下文里**需要被消歧或对齐**的术语，引用而非重造 proto 定义。

## Language — 集成（Integration / Provider / Driver / Seam / Schema / Config / Plugin / Domain）

**integration（集成）**：
一类基础设施关注点（storage / captcha / email / sms / llm / oauth / payment），统一走 profile CRUD + purpose 绑定。integration 是**关注点**，不是某个 provider 的实现，也不是它背后的持久表。（原称 channel/通道；因非通用总称且在 Go 里撞一等概念 `chan`，交叉验证后更名 integration——见 ADR-0005 术语；旧 ADR/代码里的 channel/通道 **等同** integration。）见 ADR-0003（工件归域不归 integration）、ADR-0005（integration 抽成独立模块）。
_Avoid_: channel（撞 Go `chan`，且暗示只限通讯/支付）、service 泛指；把单个 provider 或某张表叫 integration。

**purpose（用途）**：
application 对某类 integration 能力提出的固定需求槽位（如头像存储、认证邮件、验证码、结算）；具体 purpose key、展示文案及单值/多值绑定基数归 `apps/server/internal/<integration>` facade，driver 和可复用 integration 包只接受已选 Profile，不定义业务为何调用。Catalog 同时驱动写侧绑定校验与管理端 descriptor。
_Avoid_: 把具体 purpose、settings loader 或 binding 解析放进可复用 integration/driver 包；让 provider 决定应用有哪些用途；在前端维护 purpose→文案映射。

**provider（提供商 / 厂商身份）**：
一个外部厂商的**身份 = 选择键**（`alipay` / `s3` / `smtp`），即 registry entry 的稳定 key（`ProviderName()`）。provider 与 driver 是**同一事物的两面**——provider 是身份，driver 是它在本系统里的实现——**不是两个概念**。
_Avoid_: 把 provider 当成独立于 driver 的东西；照 Terraform 用法拿 provider 指整个 integration / 实现体。

**provider presentation（提供商展示身份）**：
provider 面向操作者的名称、说明、品牌色与可选图标代号，归对应 driver 所有，并以声明式数据发布。图标代号显式携带来源命名空间（如 `antd:WechatOutlined`）；前端只负责按命名空间通用解析、懒加载与缺省回退，不维护 provider→文案、图标或品牌色映射。
_Avoid_: 在前端按 provider key 分支；无命名空间的图标名；要求每个客户端都理解同一种图标来源。

**driver descriptor（驱动描述）**：
driver 对外发布的自描述，由 provider key、provider presentation 与 config schema 组成；payment driver 还发布 payment method/product 与 product input form schema。展示身份、配置表单和能力目录是并列信息，不能把 provider 品牌信息塞进只描述配置值的 schema。
_Avoid_: 把 JSON Schema 当作整个 driver 的元数据容器；让前端补齐 descriptor。

**driver（驱动）**：
一个 provider 的**无状态**实现，藏在 integration 的 seam 之后，独占该 provider 的怪癖。一个有序 registry entry 把 driver 的 presentation、config schema 与 Ops 组合成完整 descriptor；driver **不实现掩码**（base 按 config schema 通用派发，见 ADR-0006），也**不碰 DB**，持久工件归消费它的 domain（见 ADR-0003）。它是 **drop-in 扩展单元**：加一个 provider = 加一个 entry，零 proto / 零前端 / 零核心。「支付 driver」是它在 payment integration 的特例。
_Avoid_: plugin（见下）、adapter、handler。

**seam（缝）**：
integration 暴露给 base、driver 藏于其后的接口：storage 的 `ObjectStore`、captcha 的 `Verifier`、email/sms 的 `Sender`、llm 的 `Chatter`、oauth 的 `Flow`、payment 的 `Gateway`。base 只认 seam，不认具体 provider。
_Avoid_: interface 泛指、port。

**form schema（表单描述）**：
运行时字段的中立描述（key/type/required/options/条件/校验与 UI 文案），可统一生成 JSON Schema + UI Schema；provider 配置和 payment product input 共用它，但它不含密钥或不可变语义。
_Avoid_: 把 form schema 当 proto；为设置表单和支付输入各维护一套字段模型；用它描述任意客户端执行行为。

**config schema（配置描述）**：
driver 的 profile config 值契约，描述字段形状、单值合法性以及字段的人读名称与说明，并以标准 JSON Schema 提供给控制面。普通表单直接由它渲染；需要旧值或读写方向才能判断的 secret 保留/替换和 create-only 语义另属 lifecycle policy。见 ADR-0006、ADR-0014。
_Avoid_: 把 config 形状写进中央 proto；把 JSON Schema 当更新协议；在 schema 中发明 Moonbase 私有关键字；为前端复制一份字段、枚举或 UI 目录。

**config lifecycle policy（配置生命周期策略）**：
与 config schema 并列的 provider 声明，以 JSON Pointer 标识 secret 与 create-only 字段；base 据此统一执行安全读投影，以及 secret“缺席保留、非空替换”，不由 driver 重复实现。它描述跨新旧状态的写入语义，不描述字段值本身是否合法。
_Avoid_: 把 lifecycle policy 混称为 JSON Schema 校验；把它藏进自定义 schema keyword 或 Go field tag；让每个 driver 自己掩码或合并密钥；为未出现的需求预建 secret clear 状态机。

**config（配置值）**：
一个 profile 的连接参数**取值**，wire 上以结构化信封传递；形状由 driver 的 config schema 定义，协议含义只有 driver 懂。base 只执行 schema 校验与 lifecycle policy，不解释 SMTP、S3、OAuth 等 provider 语义。
_Avoid_: 把 config 形状写进中央 proto；纯 `bytes` blob（base 就无法逐字段执行安全策略）。

**option（选项）**：
一个 enum / string_array 字段的可选值，由 driver 在 schema 里声明。结构上 = `value`（存入 config 的原始值）+ `label`（人读名）+ `description`（一句说明，可空）。**显示含义归 driver**（随 provider 走），前端不维护 value→文案 的查找表——加一个 provider 的新选项 = 零前端。enum 的「未设置」**不**建模为空 `value` 选项，而是字段可选 + 占位符。见 ADR-0009。
_Avoid_: 把选项当纯字符串；在前端维护选项文案映射表；用空 value 选项表达「未设置」。

**plugin（插件）— 保留词，当前系统没有**：
特指**进程外、独立编译、运行时加载**的扩展（hashicorp/go-plugin 那种）。moonbase **刻意不做**（与 ADR-0002 决策 4 冲突）；编译期 driver registry **不是** plugin，go.work 多模块**不是** plugin，schema 驱动的 provider drop-in（编译期注册 + 运行时 config schema，ADR-0006）也**不是** plugin。除非在 go-plugin 语境，别用「插件 / 插件体系」称呼 driver / registry / 模块。见 ADR-0005 非目标、ADR-0006。
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
一个 provider 的实现（`alipay` / `wechat`），藏在 payment seam 之后，独占该 provider 的所有怪癖（金额格式、method/product 目录、product 规划、API 方言、交易态映射和回调验签）。
_Avoid_: gateway（指整个 seam / `pay.Client`，非单个 provider）、adapter。

**支付 profile**：
一条支付网关连接 = 一个 driver + 该 driver 的**直连商户**凭证 + 已签约 product 清单。运行时可增删改；它是内部路由候选，不是付款人可见的 payment method。
_Avoid_: account、merchant、渠道配置。

**支付 purpose**：
base 定义的固定结算槽位（如 `checkout`）。多值绑定提供一组内部候选 payment profile；托管收银台按付款人选择的 payment method 和路由策略从中选择实际路径，不把 profile 暴露为付款选项。
_Avoid_: scene、场景、slot 单独使用。

**payment method（支付方式）**：
付款人在托管收银台选择的品牌或支付工具，如支付宝、微信支付、银行卡；它不等于商户 profile/channel，也不等于 provider 的 API product。method 由 driver descriptor 以全局稳定字符串 key 开放声明；托管收银台合并多个 driver 的同 key 候选路径，契约测试要求其名称、说明和图标一致。
_Avoid_: 把 profile name、provider 配置或 `native`/`jsapi`/`page_pay` 暴露给付款人；沿用旧代码把 method 当 provider product。

**payment product（支付产品）**：
payment driver 发布的一个官方 provider 产品，如 Alipay 的 API 产品（`precreate` / `page_pay` / `wap_pay` / `create` / `app_pay`）或 WeChat 的 trade type（`native` / `h5` / `jsapi` / `app`）。product id 只在所属 provider/profile 内有意义；profile 只提供它签约过的子集，driver 的 `Plan` 按 payment method 与客户端环境选择其一。
_Avoid_: 全局 method 枚举；把不同 provider 的同名 product 当成同一产品；channel、pay type 泛指。

**product input（产品输入）**：
某个 payment product 在公共订单字段之外要求的本次下单参数，由该 product 的 input schema 描述并校验；普通输入可由客户端通用渲染，特殊采集交互必须成为显式客户端能力。
_Avoid_: 把 `openid`、buyer ID、`return_url` 等条件字段不断摊进公共下单请求；让客户端提交 `client_ip`、`notify_url` 等服务端上下文。

**hosted checkout（托管收银台）**：
payment module 面向业务调用者的唯一支付 UI interface；业务页面只创建 checkout session 并打开同源 `checkout_url`，付款人只选择 payment method，托管收银台在内部完成 profile/provider/product 路由、product input 表单、支付动作和状态观察。
_Avoid_: 让每个业务页面直接消费 provider/product 目录或实现支付动作；把托管收银台叫 provider 页面。

**checkout session（收银会话）**：
进入托管收银台的短寿命交互，尚未选择实际 profile/provider/product，可过期而不产生支付订单；确认支付路径后最多转换成一个 payment order。
_Avoid_: 把未选择支付路径的 session 当作 payment order；允许一次 session 创建多个订单。

**payment action（支付动作）**：
payment driver 创建支付后返回、由托管收银台消费的有限声明式下一步动作，如展示二维码、跳转、提交表单或等待；它描述浏览器能力，不描述 provider 身份。
_Avoid_: 静态挂在 product 上的 `credential kind`；`provider → UI` 分支；用任意 JSON 指令发明客户端脚本 DSL。

**hosted flow（托管特例流程）**：
声明式 payment action 无法覆盖 SDK 生命周期时，由对应 payment driver 通过受限 HTTP seam 提供的同源、短寿命交互流程；特殊 HTML/JS 留在 driver 后端实现内，业务 Web 与托管收银台均不含 provider 分支。
_Avoid_: provider 前端模块；运行时加载任意远程 JavaScript；允许 hosted flow 直接修改支付订单状态。

**amount（金额）**：
`int64` 整数分（100 分 = 1 元）。Alipay driver 格式化成元字符串（`cents/100`），WeChat 直接用整数分。系统 CNY-only，故不做按币种的最小单位抽象。见 ADR-0001。
_Avoid_: 以「元」为单位的浮点；per-currency「最小单位 / minor units」抽象。

**currency（币种）**：
恒为 `CNY`。`payment_orders.currency` 列保留只是「这是个 CNY 系统」的诚实标注，不是运行时可变维度；wire 上没有 `currency` 字段。见 ADR-0001。
_Avoid_: 把它当作 per-order / per-provider 的可变维度。

**payment order（支付订单）**：
`payment_orders` 表的一行，即支付渠道背后那台持久状态机：`creating → pending → paid → refunding → refunded`，或 `creating → failed`、`pending → closed`。创建时已确定 profile/provider/product 并快照其身份；driver 调用只能发生在 durable order 建立之后，结果不明时停在 `creating` 沿原路径协调，绝不跨 provider 重试。
_Avoid_: transaction、交易单（指 provider 侧记录，非本地订单）。

**settlement event（结算事件）**：
payment order 进入需通知业务的结算状态时，与状态迁移同事务写入的 durable outbox 记录；dispatcher 按 payment purpose 至少一次投递给 base/application handler，driver 不感知业务引用或处理结果。
_Avoid_: 依赖 `return_url`、页面轮询或内存回调完成业务交付；让 driver 直接修改业务表；假设事件只会投递一次。

**out_trade_no（商户订单号）**：
本地生成、发给 provider 的商户侧订单号，每订单唯一。
_Avoid_: trade no（指 provider 侧 `provider_trade_no`）。
