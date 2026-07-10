# 聚合支付产品的抽象边界：彩虹易支付、Jeepay、Pay Java 与国际对照

> 调研日期：2026-07-10  
> 目的：回答成熟聚合支付如何划分 base、channel/driver 与客户端，以及“新增 provider 零前端”在什么条件下才真实成立。本文是研究笔记，不是已接受的 ADR。

## 结论先行

这些产品没有证明“只返回一个 provider 字符串，任意前端就能自动完成任何支付”。它们实际采用了三种不同策略：

1. **托管支付页**：彩虹易支付把客户端特殊逻辑放进服务端 PHP plugin，plugin 可以直接输出 HTML、JavaScript、二维码页或重定向。接入方前端只跳到聚合支付页，因此看起来是零前端。
2. **有限动作词汇**：Jeepay 把创建结果压成 `payDataType + payData`，客户端仍必须理解 `payurl`、`form`、`wxapp`、`aliapp`、二维码等固定类型。
3. **SDK 操作接口**：Pay Java 统一配置、订单和 query/refund 等操作，但 `toPay`、`app`、`jsApi`、`getQrPay` 直接返回不同 Java 类型。它没有定义一个通用前端动作协议，应用负责消费结果。

因此，moonbase 可以实现的诚实边界是：

- base 不出现 `wechat`、`alipay`、`stripe` 或 provider 产品分支；
- driver 完整拥有配置、产品目录、协议调用、签名验签、状态映射和产品专属输入；
- web 只实现有限的、provider 无关的 `NextAction` ABI；
- 新 driver 复用已有 action 时零前端；若需要全新的浏览器/原生 SDK 能力，特殊代码必须随 provider 的 client adapter 一起交付，不能靠 JSON 元数据凭空产生；
- 如果坚持所有 provider 都绝对零前端，能力范围就只能限定为二维码、普通跳转、POST form、等待等声明式动作，或者改用托管收银台。

## 一、彩虹易支付：零前端来自 hosted flow，而非纯元数据驱动

### 1. 证据限制

彩虹当前官方导航仍链接其支付站点，但公开 GitHub 账号没有易支付核心仓库，只公开了支付宝、微信支付等第三方 SDK（[彩虹官方导航](https://www.cccyun.cn/)、[netcccyun 的公开 GitHub 仓库](https://github.com/netcccyun?tab=repositories)、[支付宝 SDK](https://github.com/netcccyun/alipay-sdk-php)、[微信支付 SDK](https://github.com/netcccyun/wechatpay-sdk-php)）。本次未找到可验证的当前官方核心源码或当前官方 API 文档。

所以下文只把标称“2020.02 原版开源”的社区镜像作为**低置信结构样本**。它能解释这类系统的工作方式，但不能单独证明当前商业版本的实现：

- 社区镜像：[Blokura/Epay](https://github.com/Blokura/Epay/tree/a2f29a310b3b0fbc75dc724d521e2f35f8063d8a)
- 仓库由第三方账号发布，描述为“2020.02 彩虹易支付原版开源”，不属于 `netcccyun`，因此不是一手官方源码。

### 2. 对外 API 抽象的是支付方式 `type`，不是具体 provider

商户提交 `pid`、`type`、`out_trade_no`、`notify_url`、`return_url`、商品名和金额。系统先创建内部订单；若没有传 `type`，就进入统一收银台；若传了 `type`，则由 channel 层为该支付方式挑选一个可用通道，再加载对应 plugin 的 `submit.php`（[社区镜像 `submit.php`](https://github.com/Blokura/Epay/blob/a2f29a310b3b0fbc75dc724d521e2f35f8063d8a/submit.php#L1-L121)）。

这里有四个不同概念：

- `type`：商户和付款人看到的支付方式，如支付宝、微信、QQ 钱包；
- `channel`：某支付方式下的一组实际商户配置，包含 plugin、费率、凭据和允许的 app type；
- `plugin`：具体上游协议实现；
- `order`：平台拥有的商户订单、金额、回调地址、内部通道与支付状态。

`Channel::submit()` 根据设备、商户组配置、显式通道、随机可用通道或轮询组选择 `channel + plugin`（[社区镜像 `Channel.php`](https://github.com/Blokura/Epay/blob/a2f29a310b3b0fbc75dc724d521e2f35f8063d8a/includes/lib/Channel.php#L13-L101)）。这是典型的“稳定支付方式 + 内部资金/通道路由”，provider 对外部商户透明。

### 3. plugin 自带管理元数据，但词汇并不完全开放

每个 plugin 的 `config.ini` 提供 `name`、`showname`、作者、链接、支持的 `types`、配置 `inputs` 和产品选择 `select`；`Plugin::updateAll()` 扫描目录并把这些字段同步到 plugin 表（[社区镜像 `Plugin.php`](https://github.com/Blokura/Epay/blob/a2f29a310b3b0fbc75dc724d521e2f35f8063d8a/includes/lib/Plugin.php#L5-L91)、[支付宝 plugin 元数据](https://github.com/Blokura/Epay/blob/a2f29a310b3b0fbc75dc724d521e2f35f8063d8a/plugins/alipay/config.ini#L1-L21)）。

这确实实现了“新增同类 plugin 时，管理端无需手写 provider 名称和配置字段”。但它没有做到所有概念都由 plugin 封闭：

- 支付方式 `pre_type` 是中央表，预置 `alipay`、`wxpay`、`qqpay`、`bank`、`jdpay`；
- channel 表把凭据摊平成固定的 `appid`、`appkey`、`appsecret`、`appurl`、`appmchid`；
- plugin 的 `types` 必须引用中央已有支付方式；
- 所以新增“支持已有 type 的 provider”接近 drop-in，新增全新支付方式仍要扩展 base 数据和收银台表现。

这些表结构可见于社区镜像的安装 SQL（[`pre_type`、`pre_plugin`、`pre_channel` 与 `pre_order`](https://github.com/Blokura/Epay/blob/a2f29a310b3b0fbc75dc724d521e2f35f8063d8a/install/install.sql#L84-L180)）。moonbase 不应照搬固定凭据列；当前每个 driver 自有 schema 的方向更深。

### 4. 为什么它看起来能够“新增 plugin 零前端”

plugin 不是只返回元数据。它拥有完整服务端页面接缝：`submit.php`、`qrcode.php`、`return.php`、`notify.php`，可选 `refund.php` 等。支付宝 plugin 会根据 user agent、channel 的 `apptype` 和设备直接：

- 输出 JavaScript 跳转；
- include 一个微信内页面；
- 跳到 plugin 的二维码页；
- 输出支付宝生成的 WAP/PC form HTML。

证据见[社区镜像支付宝 `submit.php`](https://github.com/Blokura/Epay/blob/a2f29a310b3b0fbc75dc724d521e2f35f8063d8a/plugins/alipay/submit.php#L1-L49)。

因此它的客户端模型其实是：

```text
商户页面
  -> 跳转到易支付 hosted checkout
    -> base 选 channel/plugin
      -> plugin 服务端直接渲染或跳转
        -> provider
```

“特殊前端代码”没有消失，而是被封闭在 PHP plugin 输出的页面和静态资源中。它不是 React SPA 接收一个未知 action 后自动学会调用任意 SDK。对 moonbase 的真正启示是：若要**绝对零宿主前端改动**，可提供 hosted checkout；若支付发生在现有 SPA 内，就需要稳定 action ABI 或 provider client adapter。

## 二、Jeepay：channel/provider 分层清楚，但 way 与客户端动作仍由 base 枚举

本节依据 Jeepay 官方公开仓库，固定到提交 [`ba37111`](https://github.com/jeequan/jeepay/tree/ba37111934c9c04183cc6cbdbdafc7f38941fa4b)。

### 1. 统一下单的公共域与开放逃生口

`UnifiedOrderRQ` 包含商户订单号、`wayCode`、金额、币种、IP、标题、描述、通知/返回地址和过期时间；渠道专属参数放入字符串 `channelExtra`（[官方 `UnifiedOrderRQ`](https://github.com/jeequan/jeepay/blob/ba37111934c9c04183cc6cbdbdafc7f38941fa4b/jeepay-payment/src/main/java/com/jeequan/jeepay/pay/rqrs/payorder/UnifiedOrderRQ.java#L41-L89)）。

这个形状与建议给 moonbase 的“公共下单字段 + driver-owned inputs”相近，但 Jeepay 的 `channelExtra` 只是 JSON 字符串，没有随 driver 下发的 schema。更重要的是，`buildBizRQ()` 在 base 中用一长串 `if wayCode == ...` 把 `channelExtra` 反序列化为 `AliJsapiOrderRQ`、`WxJsapiOrderRQ`、`WxNativeOrderRQ` 等类型（[官方 `buildBizRQ()`](https://github.com/jeequan/jeepay/blob/ba37111934c9c04183cc6cbdbdafc7f38941fa4b/jeepay-payment/src/main/java/com/jeequan/jeepay/pay/rqrs/payorder/UnifiedOrderRQ.java#L91-L168)）。

所以 Jeepay 不是“base 对具体产品完全透明”：新增 `wayCode` 若需要专属请求类型，仍会修改中央请求转换代码。

### 2. provider 与产品实现是两级派发

`IPaymentService` 是 provider/channel 统一接缝：声明接口 code、是否支持某 `wayCode`、前置检查、自定义订单号和实际支付调用（[官方 `IPaymentService`](https://github.com/jeequan/jeepay/blob/ba37111934c9c04183cc6cbdbdafc7f38941fa4b/jeepay-payment/src/main/java/com/jeequan/jeepay/pay/channel/IPaymentService.java#L30-L50)）。

provider service 再按 `wayCode` 找自己的产品实现。例如支付宝 service 调用 `PaywayUtil.getRealPaywayService(this, wayCode)`；这个工具按 provider package + `payway` 子包 + `wayCode` 转驼峰类名查找 Spring bean（[官方支付宝 service](https://github.com/jeequan/jeepay/blob/ba37111934c9c04183cc6cbdbdafc7f38941fa4b/jeepay-payment/src/main/java/com/jeequan/jeepay/pay/channel/alipay/AlipayPaymentService.java#L34-L58)、[官方 `PaywayUtil`](https://github.com/jeequan/jeepay/blob/ba37111934c9c04183cc6cbdbdafc7f38941fa4b/jeepay-payment/src/main/java/com/jeequan/jeepay/pay/util/PaywayUtil.java#L27-L65)）。

这可抽象为：

```text
wayCode（全局产品词汇）
  -> 商户 passage / ifCode（选实际 provider 配置）
    -> provider payment service
      -> provider/payway/<WayCode> 产品实现
```

它比把所有产品写进一个 provider 大类更清晰，但 `wayCode` 仍是全局目录，不是 provider/profile 作用域内的 `product_id`。

### 3. `payDataType + payData` 是早期 NextAction

统一响应公开订单号、状态、`payDataType`、`payData`、渠道错误；provider 专属的 `ChannelRetMsg` 明确不序列化（[官方 `UnifiedOrderRS`](https://github.com/jeequan/jeepay/blob/ba37111934c9c04183cc6cbdbdafc7f38941fa4b/jeepay-payment/src/main/java/com/jeequan/jeepay/pay/rqrs/payorder/UnifiedOrderRS.java#L32-L69)）。

公共 `payDataType` 包括：

- `payurl`：跳转链接；
- `form`：表单 HTML；
- `wxapp` / `aliapp` / `ysfapp`：对应客户端 SDK 参数；
- `codeUrl` / `codeImgUrl`：二维码内容或图片；
- `none`：无下一步数据。

这些常量在[官方 `CS.PAY_DATA_TYPE`](https://github.com/jeequan/jeepay/blob/ba37111934c9c04183cc6cbdbdafc7f38941fa4b/jeepay-core/src/main/java/com/jeequan/jeepay/core/constants/CS.java#L191-L201)。`CommonPayDataRS` 根据 `payUrl`、二维码或 form 字段构造类型和值（[官方 `CommonPayDataRS`](https://github.com/jeequan/jeepay/blob/ba37111934c9c04183cc6cbdbdafc7f38941fa4b/jeepay-payment/src/main/java/com/jeequan/jeepay/pay/rqrs/payorder/CommonPayDataRS.java#L29-L82)）；微信 JSAPI 响应则直接把类型设为 `WX_APP`，把 `payInfo` 放进 `payData`（[官方 `WxJsapiOrderRS`](https://github.com/jeequan/jeepay/blob/ba37111934c9c04183cc6cbdbdafc7f38941fa4b/jeepay-payment/src/main/java/com/jeequan/jeepay/pay/rqrs/payorder/payway/WxJsapiOrderRS.java#L28-L45)）。

这说明 Jeepay 承认“服务端返回的不是统一凭据，而是客户端下一步动作”。但它也说明后端返回字符串并不会自动实现客户端：消费端仍须知道 `wxapp` 如何调用微信、`aliapp` 如何调用支付宝。Jeepay 选择把 provider 名写进 action type，而 moonbase 若追求 host web 纯净，应改为通用 action 或由 provider 包交付 adapter。

### 4. Jeepay 是否做到新增 provider 零前端

结论是**有条件地做到**：

- 如果新 provider 支持既有 `wayCode`，并把结果归一成已有 `payurl/form/codeUrl/...`，商户前端可以不改；
- 如果返回 `wxapp/aliapp` 一类既有专用动作，前端本来就必须带相应 SDK 代码；
- 如果新增全新的 SDK 动作或新的专属请求类型，就要扩展客户端和/或中央 `UnifiedOrderRQ`；
- Jeepay 自身是完整支付系统，也可以让商户跳转到其支付站点完成部分交互，这会进一步减少商户前端参与。

## 三、Pay Java：统一服务端 SDK 操作，不尝试统一前端

本节依据 egzosn/pay-java-parent 官方公开仓库，固定到提交 [`5c3c1f1`](https://github.com/egzosn/pay-java-parent/tree/5c3c1f153b8aad526ac7d881703bed1eb9ce9014)。官方 README 对项目定位是可嵌入应用的支付 Java SDK，而不是完整聚合收银台（[官方仓库](https://github.com/egzosn/pay-java-parent)）。

### 1. `PayService` 抽公共操作，但保留不同返回形状

`PayService` 统一了配置、签名与验签、回调解析、订单查询、关闭、撤销、退款、账单等服务端操作；支付发起却按客户端场景分为：

- `toPay(order) -> String`：页面跳转信息；
- `app(order) -> Map<String,Object>`：App 参数；
- `getQrPay(order) -> String` / `genQrPay(order) -> BufferedImage`：二维码；
- `jsApi(order) -> Map<String,Object>`：小程序/JSAPI 参数；
- `microPay(order) -> Map<String,Object>`：条码/刷脸等主动扫码结果。

见[官方 `PayService`](https://github.com/egzosn/pay-java-parent/blob/5c3c1f153b8aad526ac7d881703bed1eb9ce9014/pay-java-common/src/main/java/com/egzosn/pay/common/api/PayService.java#L25-L197)。这不是 tagged `NextAction`，而是一组由调用方主动选择的 capability 方法；Java 应用在编译时知道自己正在做网页、App、二维码还是 JSAPI。

### 2. 配置与订单取最大公约数，并提供逃生口

`PayConfigStorage` 统一 app ID、合作商 ID、token、通知/返回 URL、签名、公私钥、测试环境等字段，同时有 `attach` 和通用属性容器承载附加配置（[官方 `PayConfigStorage`](https://github.com/egzosn/pay-java-parent/blob/5c3c1f153b8aad526ac7d881703bed1eb9ce9014/pay-java-common/src/main/java/com/egzosn/pay/common/api/PayConfigStorage.java#L12-L145)）。各 provider 再有自己的 config storage 子类。

`PayOrder` 抽商品、金额、商户订单号、币种、过期时间等，但也直接放入银行卡类型、付款码、`openid` 等条件字段，并通过 `addition` 承载附加信息（[官方 `PayOrder`](https://github.com/egzosn/pay-java-parent/blob/5c3c1f153b8aad526ac7d881703bed1eb9ce9014/pay-java-common/src/main/java/com/egzosn/pay/common/bean/PayOrder.java#L15-L86)）。

对 moonbase 的启示是：SDK 为了调用方便可以容忍较宽的公共 bean，但 wire API 不应不断增加 provider 条件字段。`inputs: Struct` 配合 driver product schema 能保留开放性，又不污染 base 顶层契约。

### 3. Pay Java 是否做到新增 provider 零前端

它没有承诺这一点，也没有必要承诺。它解决的是 Java 服务端接不同支付平台的代码复用；调用者取得 HTML、URL、二维码内容或 SDK 参数后，自行决定怎样呈现和调用客户端。新增 provider 可以不改 `pay-java-common`，但业务应用仍要选择对应 `PayService`，并消费所用 capability 的返回结果。

## 四、国际对照：Omnipay 与 Adyen

### Omnipay：公共 operation/capability，redirect 单独结构化

Omnipay 的公共 `GatewayInterface` 只强制名称、短名和参数；authorize、capture、purchase、completePurchase、refund、fetch、void、notification 等是可选操作（[官方 `GatewayInterface`](https://github.com/thephpleague/omnipay-common/blob/cd58a4e7b359b0c5b9748542d422b5cc0d4a692f/src/Common/GatewayInterface.php#L15-L81)）。`AbstractGateway` 通过 `supportsPurchase()`、`supportsRefund()` 等暴露 capability（[官方 `AbstractGateway`](https://github.com/thephpleague/omnipay-common/blob/cd58a4e7b359b0c5b9748542d422b5cc0d4a692f/src/Common/AbstractGateway.php#L169-L287)）。

公共响应只保证成功、取消、消息、code、transaction reference 和是否 redirect；redirect 响应再提供 URL、GET/POST method 与 form data（[官方 `ResponseInterface`](https://github.com/thephpleague/omnipay-common/blob/cd58a4e7b359b0c5b9748542d422b5cc0d4a692f/src/Common/Message/ResponseInterface.php#L15-L64)、[官方 `RedirectResponseInterface`](https://github.com/thephpleague/omnipay-common/blob/cd58a4e7b359b0c5b9748542d422b5cc0d4a692f/src/Common/Message/RedirectResponseInterface.php#L17-L45)）。

它证明“公共操作 + capability + 结构化 redirect”很稳定，但它仍是服务端库，不会自动给 SPA 安装 provider SDK renderer。

### Adyen：后端 action 是 tagged union，官方 Web SDK 内置组件注册表

Adyen Checkout 官方 OpenAPI 的 `PaymentResponse.action` 是 union，包含 redirect、QR code、SDK、await、bank transfer、voucher、3DS2 等动作；`PaymentMethodsResponse` 返回用于生成表单的 payment methods，包含 `type`、可显示 `name`、brand、issuer、configuration 等（[官方 Checkout OpenAPI v72](https://github.com/Adyen/adyen-openapi/blob/334900842b135eb9fc1a17afebd46a4f07793aec/json/CheckoutService-v72.json)）。redirect action 明确携带 URL、HTTP method 和 POST data；QR action 携带二维码内容和过期时间；SDK action 携带 `paymentMethodType` 与 `sdkData`。

但 Adyen Web 不是一个只靠服务器 JSON 的空壳。它在客户端维护：

- `action.type -> handler`，例如 redirect 使用通用 Redirect，QR/await/SDK 再按 `paymentMethodType` 查组件（[官方 `actionTypes.ts`](https://github.com/Adyen/adyen-web/blob/5672d01ef4f0cb43730dfb14ea3697bb0bf840c5/packages/lib/src/core/ProcessResponse/PaymentAction/actionTypes.ts#L13-L89)）；
- `payment method type -> UI component` 的大型注册表，例如 `wechatpay -> WeChat`、`pix -> Pix`、`klarna -> Klarna`（[官方 `components-map.ts`](https://github.com/Adyen/adyen-web/blob/5672d01ef4f0cb43730dfb14ea3697bb0bf840c5/packages/lib/src/components/components-map.ts#L1-L228)）。

Adyen 的“新增支付方式后商户页面常常无需改”成立，是因为商户升级/使用了一个由 Adyen 持续加入 provider 组件的官方 Web SDK。特殊客户端代码仍然存在，只是封闭在 Adyen SDK，而不是商户应用。

## 五、对照表

| 产品 | Base 抽什么 | Plugin/driver 抽什么 | 客户端如何完成支付 | 新增 provider 是否真的零前端 |
|---|---|---|---|---|
| 彩虹易支付（低置信社区样本） | 商户、订单、支付方式 `type`、channel 路由、费率、回调、状态 | provider 配置元数据；submit/notify/return/qrcode/refund 页面与协议 | 跳到 hosted checkout；plugin 直接输出 HTML/JS、二维码页或 redirect | 对“支持已有 type 的 plugin”接近是；特殊 UI 已在 plugin 服务端页面里。新增 type 仍动中央目录 |
| Jeepay | 订单状态、`wayCode`、passage/provider 选择、统一通知/查询/退款、有限 `payDataType` | provider service、provider 下各 payway 实现、配置、签名和状态映射 | 商户根据 `payurl/form/codeUrl/wxapp/aliapp/...` 消费 `payData`，或跳转到支付站点 | 复用已有 way/action 时可以；新请求类型或新 SDK action 仍需 base/客户端扩展 |
| Pay Java | 公共配置接口、订单 bean、query/close/refund 等操作接口 | 每 provider 的 config storage 与 `PayService` 实现 | Java 应用主动调用 `toPay/app/jsApi/getQrPay` 并自行呈现结果 | 仅服务端 SDK 扩展可独立；不解决宿主前端自动适配 |
| Omnipay | operation/capability、公共响应与 redirect 接口 | 独立 gateway package 的请求、签名、状态与 provider 参数 | 应用检查 redirect/response，并执行 URL 或 GET/POST form | 新 gateway 可不改 common；前端/宿主仍须消费已知响应能力 |
| Adyen | Payment、result code、payment methods 目录、action union | Adyen 服务端 provider 实现；官方 Web SDK 中的 payment method 组件 | Drop-in/Components 根据 action 与 payment method registry 渲染、调用 SDK | 商户应用通常零改，但 Adyen Web SDK 本身必须加入并发布新组件 |

## 六、对 moonbase 的具体启示

### 1. Base 应拥有的稳定抽象

- `PaymentOrder`、金额/币种、商户订单号、用途、状态机、幂等状态迁移；
- create/query/sync/refund/webhook 等生命周期操作与 capability；
- profile/binding、权限、审计、回调 ingress 与敏感数据边界；
- 一个有限、provider 无关的 `NextAction` ABI；
- action renderer/client adapter 的加载与安全策略，但不拥有任何具体 provider 分支。

### 2. Payment driver 应封闭的内容

- provider descriptor：key、中文 label/description、`icon_ref`、配置 schema；
- provider-scoped `ProductDescriptor`：官方 `product_id`、文案、输入 schema、可能的 action capability；
- provider API、签名验签、通知解析、query/refund、provider 状态映射；
- 从 `inputs` 到 provider 请求的全部特殊字段；
- 若确实需要专用 SDK，随 provider 交付可选 client adapter，而不是要求 web base 添加 `if provider == ...`。

### 3. 客户端协议建议

首批通用动作可保持小而稳定：

```text
redirect      { url }
submit_form   { url, method, fields }
display_qr    { data | image_url, expires_at? }
wait          { poll_after_ms? }
client_adapter { adapter_ref, payload }
```

前四种可以由 web base 完全通用实现。`client_adapter` 只负责统一加载 ABI；具体 adapter 必须是随宿主构建、审核、同源发布的可信模块，不能让后端下发任意远程脚本。

`payDataType + string` 不够深：至少应让每个 action 的 payload 有结构和校验，未知 action 明确报客户端不支持。provider 返回已有 action 时可以零 web 改动；返回新的 action capability 时，就是 base/client ABI 演进，不应伪装成普通 driver 元数据变化。

### 4. 对“base 与前端对具体实现透明”的精确定义

建议把目标写成：

> 新增复用既有支付域 capability、输入 widget 与 NextAction/client-adapter ABI 的 provider，只修改该 provider package；server base、web base、proto 公共产品枚举和前端 provider 映射均不修改。

这比“任意新 provider 永远零前端”更可验证，也更接近成熟系统的真实边界：

- 彩虹通过 hosted plugin 页面封闭特殊 UI；
- Jeepay 通过固定 `payDataType` 封闭一部分动作，但专用 SDK 类型仍泄漏；
- Adyen 通过官方 Web SDK 的组件注册表封闭特殊 UI；
- 没有任何系统能让浏览器仅凭未知 JSON 自动获得一个从未安装的安全 SDK 实现。

