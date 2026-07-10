# schema 驱动的 provider 插件化：config 形状归 driver、base 一套通用引擎守密钥

> **状态**：accepted，前端渲染决策已被 ADR-0010 超越。**超越 ADR-0002 决策 2/3 中关于 per-provider config 的部分**（config 形状不再进中央 proto，改由 driver 的运行时 schema 拥有）；**保留 ADR-0002 决策 1/4/6 内核**（掩码仍是不可糊弄的单点不变量、provider 派发仍编译期可 grep、integration 仍是完整布线范例）。**在 provider 粒度拉动 ADR-0005 的「schema 驱动运行时表单」触发器**。go-plugin / 运行时加载仍缓做。**已部分修订**：布局见 ADR-0007（integrations 收敛为单一模块，替代本文「布局与模块路径」的每-integration 独立 module）；schema 表单与前端 i18n 的张力由 ADR-0008 以「放弃 i18n、全站中文」了结（driver 的 `label`/`help` 直接写中文）。ADR-0010 以 rjsf + `ProviderForm`（JSON Schema + UI Schema）替代本文的前端薄渲染器；仍保留 driver 拥有 config schema、`Mask` / `Merge` / `Validate` / `Usable`、掩码单点不变量与编译期 registry。

## 背景

这次的起点是「integration 分包没解耦、和之前没太多区别」的体感。根因诊断落到一个点：`integrationkit/systemcodec` 是个**上帝类型包**（storage/captcha/email/sms/llm/oauth/payment 七家 profile 类型同居一个 816 行生成文件，每个 integration 模块都 import 它），且它**反向 import `apps/server/internal/gen`**——最底的 kit 依赖最顶的 server internal，模块级成环，kit 不自包含。ADR-0005 承诺的「可版本化核心、下游 depend 而非 copy」因此结构上做不到。

顺着「driver 自带配置」往下推，厘清两件事：

1. 真正值得 drop-in 的扩展单元是 **provider**（新短信厂商、新 LLM 端点**经常**加），不是整类 integration（email/sms/llm/oauth/captcha/storage/payment 已近乎齐全，**很少**新增）。
2. 关键张力在 **config**：它要被管理员在浏览器里编辑，天生是**前后端共享契约**。把 config 形状塞进中央 proto → 加一个 provider 就得动中央 proto + 前端；让 driver 完全自带且 opaque → 密钥掩码被迫**下沉到每个 driver**（安全敏感、N 处易漏）。

破局点（Terraform 的 `Sensitive` 模式，且 go-plugin 与 Terraform 同源）：让 config **值** opaque，但 config **schema** 有结构、带 per-field `secret` 标志——base 就能按标志**通用派发掩码**，driver 只声明、不实现。

## 决定

- **config 形状归 driver，以运行时 schema 描述；config 值在 wire 上走 `google.protobuf.Struct`（opaque）。** 每个 driver 发布一份 schema（字段 key/type + 结构标志 `secret`/`immutable`/`required` + 校验），语义只有 driver 懂。
- **OAuth 的流程 slug 是 `config.key` 这个 schema 字段，且标 `immutable`。** 它被 `user_identities.provider` 与 `/api/oauth/{key}/...` 流程 URL 引用；一旦创建，改名会让既有身份记录和外部回调入口失稳，所以稳定性属于 driver config 的字段约束，而不是顶层 `Profile` 字段。
- **base 拥有唯一一套 schema 驱动的通用引擎**：`Mask` / `Merge` / `Validate` / `Usable`，全按 schema 标志工作。**掩码是单点、可审计的不变量，driver 绝不实现掩码**——任一 driver 写错都无法泄凭证。这是 ADR-0002 决策 1（不可表达 > 可糊弄）+ 决策 3（机械映射集中）的延续，只是实现从「生成」换成「schema 驱动引擎」。
- **drop-in 单元 = provider/driver**。加一个 provider = 实现 `Schema()` + `Ops` 并注册，**零 proto、零前端、零核心**。**integration 是核心已知的有界集合**，保留带类型的 per-integration RPC + 权限 + seam；加一整类 integration 仍是有意的核心动作。
- **descriptor 与 base↔driver seam 照 Terraform provider schema/协议塑形**（`Sensitive`≈`Secret`、`ForceNew`≈`Immutable`、`Required`）。因 go-plugin 与 Terraform 同源，将来 P1 = 把进程内 registry 调用换成 gRPC，**base 引擎一行不改**。
- **前端（历史决策，已由 ADR-0010 超越）**：当时不引 Formily / rjsf / SurveyJS，改用薄的 `descriptor→antd` 渲染器。当前实现以 rjsf 消费 `ProviderForm`；前端校验仍只当 UX，**真正的闸是 base 的 `Validate`**（服务端不信任客户端）。
- **`systemcodec` / `protoc-gen-settings` 退役**，`integrationkit/schema`（纯通用 Go，零 proto、零 server）取而代之。systemcodec → `server/internal/gen` 的反向边随之消失，kit 重新自包含。

保留 vs 超越，划清：

- **保留**：决策 4（driver registry 字面可 grep、零反射派发——**注册仍是编译期显式**）；决策 6（完整布线范例）；决策 1（掩码仍是不可糊弄的确定性不变量，只是换了实现载体）。
- **超越**：决策 2 对 **per-provider config 形状**的「proto 单源」——config 形状改由 driver 的运行时 schema 拥有；proto 仍是 **integration 信封 + RPC + 权限**的单源。决策 3 的 config 编解码从「生成」改为「schema 驱动通用引擎」。

## 布局与模块路径（随 #8 一并拍平）

integration 模块的物理位置与 Go 模块路径随 `systemcodec` 删除**在同一次改动里**改——去掉模块路径的 `server/` 前缀技术上必须与 `systemcodec` 消失同步（前缀在，`systemcodec` 才能 import `server/internal/gen`；前缀走，反向边也走）。

统一规则：`apps/` = 可部署物（多语言）；`packages/` = 可复用库（多语言，**不限 npm 包**）；`proto/` = 契约单源。**文件夹定角色，包内 manifest（`go.mod` / `package.json` / 将来 `Cargo.toml`）定语言**；moon 编排跨语言，各语言 workspace（go.work / pnpm）管语言内链接。加新语言 = 往 `packages/` 丢，零特殊处理。

- **物理**：`packages/integrations/{core,sms,email,llm,oauth,captcha,storage,payment}`（`core` = 原 `integrationkit`，弃「kit」名）。
- **模块路径**：`github.com/imbytecat/moonbase/integrations/<name>`——丢 `packages/`，与既有约定同规（`apps/server` 的模块路径就是 `.../moonbase/server`，磁盘在 `apps/` 但路径里没有 `apps`）。故「路径≠磁盘」是本仓库既有规范，非意外。
- **moon project**：`packages/integrations/*` 用 glob 注册为 project（各加 `moon.yml`），补上「integration 模块测试没进 `:test` / CI」的洞。
- **`proto/` 留顶层**：它是契约生成源，不是被 import 的库，角色不同。
- **无向后兼容**：旧 `.../server/integrationkit`、`.../server/integrations/*` 路径直接删，不留 re-export / alias。

## 非目标（明确的「不」）

- **go-plugin / 运行时加载 / 独立进程 driver**——仍缓做（决策 4 精神）。本 ADR 只把 seam 塑成「将来换传输即可」的形状，不现在换。触发：出现「肯付费、死活不肯重编译宿主」的真实客户。
- **integration 也做全通用 RPC drop-in**——否决。会丢掉带类型、带权限的 RPC 面的可读性；integration 是有界集合，值得显式布线。drop-in 只到 provider 粒度。
- **config 走纯 `bytes` opaque blob**——否决。base 就无法逐字段脱敏；**必须是 `Struct`**（结构化值）base 才能通用掩码。
- **前端上重型 schema 引擎（Formily / rjsf / SurveyJS）**——当时否决：antd v6 不匹配、过重且反 grep。条件联动成为真实需求且 rjsf 已支持 antd v6 后，ADR-0010 已重新评估并采用 rjsf；Formily / SurveyJS 仍未采用。

## 考虑过的替代

- **只把 proto 类型搬出 `internal/`、systemcodec 留 kit**：只修「自包含」，不修上帝包、也不给 drop-in 扩展点；收回，没解决「没解耦」的本病。
- **每个 driver 自己实现掩码**：安全敏感、N 处易漏；被「schema 标 `secret` + base 派发」取代（Terraform `Sensitive` 模式）。
- **每个 integration 自带 proto 切片 + 独立生成（垂直切片 B）**：反转 proto 单源、碎裂前端 api-client、逼近「schema 驱动前端」的全量非目标；n=0 无「独立分发 / 售卖某 integration」信号，是拿确定成本对赌假设需求。收回，留触发器。
- **integration 模块留根目录 `integrations/`（或塞进 `packages/` 但模块路径带 `packages`）**：前者多一个顶层目录、与「`packages/` = 多语言可复用库」模型不一；后者把仓库布局泄进 import 路径。均收回——`packages/integrations/*` + 模块路径丢 `packages/`（对齐 `apps/server → .../moonbase/server` 既有约定）最干净、可 grep、且多语言可扩展。

## 触发重评

- 出现「肯付费、要独立分发某 integration/driver」的客户 → 重评 go-plugin（P1）与 per-integration proto 切片。
- config 表单出现深嵌套 / 条件联动 → 重评是否引入 Formily。
