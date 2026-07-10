# 选项显示含义归 driver：`FieldDescriptor.options` 升级为结构化 `OptionDescriptor`

> **状态**：accepted，wire 与渲染机制已被 ADR-0010 超越；其中 payment 固定目录例外又被 ADR-0011/0012 超越。选项显示含义仍归 driver，且继续延续 ADR-0008（driver 的 `label`/`help` 直接写中文，选项文案同理）；provider config 由 ADR-0014 的私有 Go struct + `config.Contract[T]` 生成标准 JSON Schema，payment product input 仍使用 `form.Schema`，两者都由 rjsf 渲染。

## 背景

schema 驱动表单里，`FieldDescriptor.options` 是 `repeated string`——它只能承载「合法值」，塞不下任何人读信息。于是前端把原始值直接怼进下拉：`encryption` 显示 `starttls`/`ssl`/`none`，`authMethod` 显示 `public_key`/`cert`/`platform_cert`，`methods` 显示 `face_to_face`/`jsapi`/`app`。管理员看不懂选哪个，也没有一句说明的位置。这是「设置表单像残次品」体感的一半根因（另一半是必填/占位符/关闭动画，属渲染策略，见实施计划，不入本 ADR）。

补文案有两条路：在**前端**维护一张 `provider→field→选项文案` 查找表，或让 **driver** 随 schema 自带。前者无需动契约，但把 per-provider 显示知识重新散回前端，正是 ADR-0006 要消灭的东西，且 driver 新增一个选项就会与前端脱同步——破坏「加 provider 零前端」不变量。

## 决定

- **`FieldDescriptor.options` 从 `repeated string` 升级为 `repeated OptionDescriptor`**，`OptionDescriptor = { value, label, description }`。`value` 是存入 opaque config 的原始值，`label` 是人读名，`description` 是一句说明（可空）。
- **选项的显示含义归 driver**：driver 在 `Schema()` 里直接写中文 `label`/`description`。前端无脑渲染（下拉行显示 label + 灰色 description 次行），**不维护任何 value→文案 映射表**。加一个 provider 的新选项 = 零前端。
- **enum 的「未设置」不再用空 `value` 选项表达**：移除 `""` 哨兵选项，改为字段可选 + `placeholder`（`请选择…`）+ `allowClear`。「未选择」即未设置。`encryption` 的 `""`/`none` 语义重叠由 driver 顺带厘清。

**已被超越的 payment 例外**：本 ADR 当时允许 payment 的固定 proto 枚举继续由前端目录提供。ADR-0011/0012 已删除该中央枚举、生成器与前端镜像；payer-facing method、provider product、输入 schema 和展示信息现在全部由 payment driver descriptor 发布，托管收银台只消费通用投影与 typed action。其他普通 config 选项仍遵守“显示含义归 driver”。

这是**破坏性 wire 契约变更**，按 ADR-0006/0007 惯例**不留向后兼容**：旧 `repeated string` 直接换掉，无并行字段、无 alias。

## 考虑过的替代

- **前端查找表**：无需改契约，但违反 ADR-0006「加 provider 零前端」，且与 driver 新增选项易脱同步。否决。
- **只加 `label`、不加 `description`**：省一个字段，但用户明确要「标签＋说明」两者；enum 选项常需一句话点明前置条件（如「公钥模式：仅填支付宝公钥即可」）。否决。

## 后果

同一次改动需同步（proto 单源，错配即编译错误）：`proto/system/v1/system.proto`（新增 `OptionDescriptor`、改 `options`）→ 重新生成两端 → `core/schema.Field.Options` 类型与 `Validate` 的成员校验 → 唯一映射器 `fieldDescriptors()` → `email`/`payment` 两个 driver 的选项定义（含 `methods` 从 method 目录带出 label）→ 前端渲染器 → 相关 schema 单测。带选项的 driver 仅此两处，改动有界。
