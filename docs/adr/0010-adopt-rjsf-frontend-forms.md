# 采用 rjsf 渲染前端配置表单

> **状态**：accepted；其中 provider config producer、服务端校验与 lifecycle 已被 ADR-0014 修订。保留的决定是 rjsf 直接消费标准 JSON Schema、前端无 provider 分支，以及 secret widget 使用 `ConfigWrite.secrets` / `ConfigView.set_secret_paths`。下文 `[]Field` 与旧生命周期引擎仅记录当时决策背景；当前 provider config 单一真源是私有 Go struct + `config.Contract[T]`，`form.Schema` 只用于 payment product input。

## 背景

ADR-0006 拒绝 rjsf 的理由之一「antd 主题停在 v5」已过时：`@rjsf/antd@6.6.x` 声明 `antd: ^5 || >6.3.5`（issue #4995 / PR #5035），与本仓库 antd 6.5 对得上。且 config 表单存在**真实条件联动**（payment 的 `authMethod` 决定填公钥还是三件套证书），扁平 descriptor 全字段同显、靠硬编码 `paymentAuthUsable` 兜底，正是 ADR-0006 写明的重评触发器。真实 spike（payment/alipay，见截图）验证 rjsf 的 JSON-Schema `if/then` 条件渲染可用、与 antd v6 + React 19 集成干净。

## 决定

- **driver 仍写带类型的 Go `schema.Field[]`**——它是**单一真源**，且**本身就是校验器**（这是 Go 版「Zod」：一份定义，既校验又生成 JSON Schema，两视图不漂移）。
- **base 把 `[]Field` 转成 JSON Schema + uiSchema**（Go 转换器）；describe 接口 wire 上**直接发 JSON Schema**（`google.protobuf.Struct`）。**删除 proto `FieldDescriptor` / `ProviderSchema` / `OptionDescriptor` / `ShowWhen` 消息**——语义收进 Go `[]Field` 与生成的 JSON Schema。前端 rjsf 直吃、**无前端翻译层**。
- **服务端 `schema.Validate` 仍跑在 `[]Field` 上，是唯一权威校验闸**（拦：未知字段、必填缺失、类型错、MaxLen、Pattern、枚举非法、Unique、整数越界，外加 `show_when` 条件必填）。rjsf/ajv 前端只做 UX。**JSON Schema 与校验同源（`[]Field`），永不漂移。**
- **`Field` 增 `ShowWhen{field, values}`** 表条件；`Validate`/`Usable` 把条件不满足的字段视为不存在，**收敛掉硬编码 `paymentAuthUsable`**；转换器生成 JSON Schema `if/then` + `ui:order` 让条件字段就地排序。
- **secret 写-only 生命周期保留**（后端对 config **值**的行为，与「wire 传 JSON Schema」正交）；前端以 rjsf custom widget 渲染，写侧通过独立 `secrets` map 非空替换，读侧通过 `set_secret_paths` 表达已设置状态。
- **定制行为以 rjsf custom widget/template 实现**：secret、enum（label + 描述两行 + 「请选择」+ allowClear）、通用占位符、immutable 禁用、help。
- **payment `methods` 沿用 ADR-0009 例外**（文案来自前端 `payments.ts`），转换时注入 label。

## 为什么是 `[]Field`，而非 proto 契约或反射库

- **proto FieldDescriptor 当 wire 契约**（曾考虑）→ 前端还得 `FieldDescriptor → JSON Schema` 翻译一层。改由 Go 直接发 JSON Schema，前端直吃、少一层。
- **Go 反射库**（`invopop/jsonschema`、`swaggest/jsonschema-go`、`google/jsonschema-go`）反射**静态 struct**；我们的 schema 是**运行时 driver 定义的 `[]Field`**，且 secret / 条件 / 选项描述本就需自定义扩展——反射库要求改成静态 struct **且仍要写自定义扩展**，得不偿失。`[]Field` 贴合领域、单源不漂移。

## 后果（明确接受的代价）

- 新增 rjsf 全家桶依赖（`@rjsf/core`+`utils`+`antd`+`validator-ajv8`+`ajv`，约 11MB）。
- 新增 Go `[]Field → JSON Schema` 转换器（~120 行）；describe wire 从结构化 `FieldDescriptor` 变 opaque `Struct`（前端无生成类型，但前端只喂 rjsf、不需要类型）。
- ajv 前端校验与服务端 `schema.Validate` 并存（前端 UX、后端为闸），约束大致对齐。

## 触发重评

- rjsf 维护停滞、或 `@rjsf/antd` 再次与 antd 大版本脱节 → 回退薄渲染器（`show_when` 条件模型可复用）。
