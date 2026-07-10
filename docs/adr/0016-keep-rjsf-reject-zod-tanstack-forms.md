# provider 配置表单保留 rjsf，否决 zod `fromJSONSchema` + TanStack Form

> **状态**：accepted。本 ADR 复核 ADR-0010（采用 rjsf）在「zod 4 已提供 `z.fromJSONSchema`」这一新事实下是否仍成立，结论是成立；同时延续 ADR-0014「服务端 Go struct 生成的标准 JSON Schema 是唯一契约，两端同规则、不漂移」的内核。

## 背景

provider 配置表单的形状在**运行时**由各 driver 的 Go config struct 反射生成标准 JSON Schema（ADR-0014），前端**编译期完全不认识**这些字段。评审期间提出：zod 4 新增了 `z.fromJSONSchema`（把 JSON Schema 转成 Zod），配合 TanStack Form，是否比 rjsf 更好实现。

诊断的前提事实：本次触发这次复核的两个缺陷（下拉只有 value 没有 label、表单不随签约产品变化）**都是服务端 schema 的问题**——裸 `enum` 缺 `title`、缺 `if/then` 条件。任何忠实读 schema 的渲染器读到这份 schema 都只能显示原始值；换渲染框架**修不了**这两个 bug（已按 ADR-0009 的 option 不变量把 `.Enum` 改为 `oneOf`+`const`+`title`、并用标准 `if/then` 表达 `opAppId` 条件必填修复）。因此框架取舍与这两个 bug 正交，本 ADR 只记录框架决定。

## 决定

- **provider 配置表单保留 rjsf 直吃标准 JSON Schema**，前端无 per-provider 分支（ADR-0010 不变量）。
- **否决 `z.fromJSONSchema` + TanStack Form 承担 provider 配置表单**。

关键事实（决定性）：

- **TanStack Form 是 headless 表单 state 库，不按 schema 自动渲染字段。** rjsf 在此处的全部价值就是把运行时才知道的 JSON Schema 零 per-provider 代码地渲成整张表单；换 TanStack 得自己手写一个通用「JSON Schema → 字段」遍历器，等于重造 rjsf，代码只多不少。
- **`fromJSONSchema` 恰好在本仓依赖的关键字上有损。** provider config 用到 `if/then`、`allOf`、`oneOf`+`const`+`title`、`dependentRequired`、`contains`（authMethod 条件、products→opAppId 条件、枚举 label）。这些正是 ADR-0010 选 rjsf 的理由；`fromJSONSchema` 对基础类型可靠，但条件/oneOf 支持很弱，社区实测存在「缺约束时静默退化成 `z.any()`、连 `required` 都漏」的坑。转过去，条件必填语义会当场丢失。
- **客户端把 JSON Schema 转 Zod = 重新引入 ADR-0010 删掉的前端翻译层 + 第二套校验语义。** ADR-0014 要求同一份 schema 由服务端 `santhosh-tekuri/jsonschema/v6` 与 Web `Ajv2020` 执行同一规则、永不漂移；再生一套 Zod 校验必然与之分叉。
- **secret write-only、immutable 禁用、`ui:order`** 现由 rjsf uiSchema 从 lifecycle policy 派生（ADR-0014）；换栈这些都要重写。

## 适用边界

`z.fromJSONSchema` + TanStack Form 适合**编译期已知、手写、需精细 UX** 的表单（如登录、资料页），那里 Zod schema 由 TS 作者手写、字段固定。provider 配置表单是相反场景——运行时、服务端生成、前端不认识字段——rjsf 正对口。二者可并存，不是全站二选一。

## 触发重评

- 出现被广泛采用、**按 schema 自动渲染**且忠实处理 `if/then` / `oneOf` / `contains` 的 headless 表单方案（能替掉 rjsf 的渲染职责而不牺牲条件语义）。
- rjsf 维护停滞、或 `@rjsf/antd` 再次与 antd 大版本脱节（与 ADR-0010 同一触发器）→ 回退薄渲染器，标准 schema 契约可原样复用。
