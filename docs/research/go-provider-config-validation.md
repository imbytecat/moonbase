# Go provider config：强类型解码、JSON Schema 与领域元数据

> 第二轮调研日期：2026-07-10
> 目的：复核 `google/jsonschema-go` 与 `kaptinlin/jsonschema`，并为 provider 私有强类型配置给出唯一选型。Greenfield 结论已由 ADR-0014 接受。

## 结论先行

> 2026-07-11 更新：在“完全不考虑兼容、最终模型优先”的新前提下，本文最终推荐已改为第七节的 schema-first 方案。下面“保留现有 producer”的结论只适用于最低迁移路径，不再是 greenfield 推荐。

**没有一个 Go 库可以完美替代 moonbase 的 `integrations/core/form` 与 `integrations/core/config`。唯一推荐是：保留它们作为规则、JSON Schema、UI Schema 和领域元数据的单一 producer，只引入 `github.com/santhosh-tekuri/jsonschema/v6` 作为服务端 Draft 2020-12 编译与验证引擎；验证通过后用标准库 `encoding/json` 解码为 provider 私有 `T`。**

推荐的数据流：

```text
provider 私有 config struct T
             ↑ 注册时检查 key/type 1:1
config.Schema（规则 + secret/immutable + 中文 UI）
             ├─ 生成 JSON Schema → rjsf + Ajv2020
             ├─ 编译并缓存 → santhosh-tekuri validator
             └─ 保留领域索引 → Mask / Merge / Usable / UI Schema

protobuf Struct / map[string]any
  → Merge 已存 secret/immutable
  → compiled JSON Schema Validate
  → encoding/json Marshal + Unmarshal 到 T
  → provider Driver 只接触 T
```

这推翻了第一轮“以 `google/jsonschema-go` 为底座并从 struct 推导 schema”的建议。moonbase 已经有一个贴合产品的 schema producer；真正薄弱的是服务端又手写了一套标准 JSON Schema 校验。替换 validator，而不是替换 producer，净收益更高、迁移面更小。

选型结论要区分四个问题：

- **最流行的普通 Go validator**：`go-playground/validator`，但它不产生 JSON Schema，因此不匹配。
- **功能面最激进**：`kaptinlin/jsonschema`，集 producer、validator、解码、i18n 于一体，但当前版本演进和实现风险也最高。
- **JSON Schema 标准验证最成熟**：`santhosh-tekuri/jsonschema/v6`，有多个 draft、官方测试套件/Bowtie、结构化错误和自定义 regex/format 接缝。
- **最匹配且最低风险**：moonbase producer + `santhosh-tekuri/jsonschema/v6`；这是本次唯一推荐。

**i18n 明确不是选型需求。** 全站直接使用中文常量（ADR-0008）；validator 的结构化错误只供日志、测试与字段路径定位，RPC 继续返回稳定的中文通用消息。库内置多语言错误不会获得加分，也不会引入前端或服务端翻译层。

## 一、采用前必须先统一 dialect

两个 Go spike 输出显式：

```json
{"$schema":"https://json-schema.org/draft/2020-12/schema"}
```

moonbase 当前使用的 `@rjsf/validator-ajv8` 默认实例化普通 `Ajv`，不是 `Ajv2020`（[rjsf v6.6.2 `createAjvInstance.ts`](https://github.com/rjsf-team/react-jsonschema-form/blob/v6.6.2/packages/validator-ajv8/src/createAjvInstance.ts)）。实测 Google 与 Kaptinlin 两份 schema 都报：

```text
no schema with key or ref "https://json-schema.org/draft/2020-12/schema"
```

将 rjsf validator 通过 `customizeValidator({ AjvClass: Ajv2020 })` 接线后，两份 schema 均编译成功。Ajv 官方也明确把 Draft 2020-12 放在独立的 `Ajv2020` 导出中，并说明它与旧 draft 不可混用（[Ajv JSON Schema versions](https://ajv.js.org/json-schema.html#draft-2020-12-breaking)）。

因此采用条件是二选一，并且前后端必须显式一致：

1. **推荐**：producer 输出 Draft 2020-12；Web 改用 `Ajv2020`；Go compiler 固定 Draft 2020-12。
2. 若不愿改前端 dialect，则 producer 保持 Draft-07，Go compiler 也显式固定 Draft-07。

不能省略 `$schema` 后让 Ajv 与 Go 库各自套默认 draft；那会把同一份规则重新变成两个隐式语义。

## 二、moonbase 的 12 项硬需求

符号：✅ 满足；⚠️ 需要 moonbase 适配或存在限制；❌ 不满足。

| 硬需求 | 当前 producer + Santhosh | Google v0.4.3 | Kaptinlin v0.9.3 |
| --- | --- | --- | --- |
| 1. provider 私有强类型 struct | ✅ `T` 只用于注册检查与解码 | ✅ `For[T]` | ✅ `FromStruct[T]` |
| 2. 严格校验 map 的 unknown/type/required/range/pattern/enum/unique | ✅ 编译最终 schema；spike 全通过 | ✅ spike 全通过 | ⚠️ 全通过，但 `FromStruct` 默认不封闭对象，须补 `additionalProperties:false` |
| 3. 验证后可靠解码到 `T`，含 `float64 → int` | ✅ 先验证，再标准库 JSON round-trip | ✅ 同左 | ⚠️ 必须强制先验证；其 `Unmarshal` 单独调用会把 `443.5` 静默变成 `443` |
| 4. Draft 2020-12 给 rjsf/AJV | ⚠️ producer 可输出；必须接 `Ajv2020` | ⚠️ 可表示；必须接 `Ajv2020` | ⚠️ 默认输出；必须接 `Ajv2020` |
| 5. `if/then` / `showWhen` | ✅ 当前 producer 已生成，spike 通过 | ✅ 原生字段，spike 通过 | ✅ 程序化 API 通过；struct tag 方案并不支持 |
| 6. title/description/oneOf label/顺序 | ✅ 当前 producer + `ui:order` 已覆盖 | ✅ 标准注解 + `PropertyOrder` | ⚠️ 标准注解可用，但输出确定性排序不等于 provider 声明顺序 |
| 7. secret/immutable 元数据及 Mask/Merge/Usable/UI | ✅ 继续由领域层拥有 | ⚠️ `Extra` 只负责承载，领域行为仍需自研 | ⚠️ `Extra` 同样不能替代领域行为 |
| 8. 同一规则源，避免 tags + schema 双写 | ✅ `config.Field` 是规则真源；struct 只声明形状 | ⚠️ 类型来自 struct，约束仍要由领域层装饰并做漂移检查 | ⚠️ 若使用 tags 会与领域层双写；不用 tags 又失去其主要 producer 优势 |
| 9. rich error path，供日志/测试/字段定位 | ✅ instance/keyword path + 层级错误 | ❌ 只有拼接字符串；rich error 仍是 open proposal | ⚠️ 有路径，但 `DetailedErrors` 会混入 `if` 谓词失败等诊断噪声 |
| 10. regex/format 与 AJV 一致 | ⚠️ 可换 regex engine、启用 format；仍应限定 house subset 并做跨端契约测试 | ❌ `format` 不验证，regex 固定 Go RE2 | ⚠️ format 可断言，但 regex 固定 Go RE2 |
| 11. schema/validator 缓存和并发安全 | ✅ 注册时 compile 一次；`Validate` 每次创建局部状态 | ✅ `Resolve` 预编译 regex，`Validate` 使用局部状态 | ❌ validation 首次/重复懒写共享 regex cache，无同步 |
| 12. 稳定版本、维护、采用度、依赖、许可证 | ✅ v6.0.2；2017 至今；Apache-2.0；生产依赖仅 `x/text` | ⚠️ v0.4.3；项目始于 2025；MIT；运行时近零依赖 | ❌ v0.9.3；Go patch directive 与实验 JSON 依赖风险；MIT |

## 三、为什么选择 `santhosh-tekuri/jsonschema/v6`

### 1. 它补的是当前真正缺失的一层

当前 [`form.Schema`](../../packages/integrations/core/form/form.go) 已经能从一份字段定义生成：

- `type`、`required`、`minimum`、`maximum`、`maxLength`、`pattern`、`uniqueItems`；
- `oneOf` 中的 `const` + 中文 `title`；
- `if/then` 条件字段；
- `ui:order`、widget、placeholder、help 和 option descriptions。

[`config.Schema`](../../packages/integrations/core/config/config.go) 又集中拥有 `secret`、`immutable`、`Mask`、`Merge` 与 `Usable`。这些都不是通用 JSON Schema 库应该理解的 provider 生命周期语义。

Santhosh 不生成 schema，也不解码 struct；这在本项目反而是优势：它只取代 `form.Validate` 中手写的标准协议校验，不争夺领域模型的所有权。

### 2. 标准覆盖和错误模型明显强于另外两库

v6.0.2 声明通过 JSON-Schema-Test-Suite（optional 目录除外），并提供 Draft 4/6/7/2019-09/2020-12 的 Bowtie 结果（[v6.0.2 README](https://github.com/santhosh-tekuri/jsonschema/blob/29cbed948d24a04700eb94436416b18a07953b71/README.md)）。

`ValidationError` 暴露：

- `SchemaURL`；
- `InstanceLocation []string`；
- 可识别的 `ErrorKind`；
- 层级 `Causes`。

并能投影成 Basic/Detailed JSON Schema output，包含 `keywordLocation` 与 `instanceLocation`（[`validator.go`](https://github.com/santhosh-tekuri/jsonschema/blob/29cbed948d24a04700eb94436416b18a07953b71/validator.go)、[`output.go`](https://github.com/santhosh-tekuri/jsonschema/blob/29cbed948d24a04700eb94436416b18a07953b71/output.go)）。moonbase 只按 path/keyword 记录日志和支撑测试；用户侧继续返回直接写死的中文通用错误，不消费库的本地化消息。

### 3. 编译后对象适合 registry 缓存

`Compiler` 在注册阶段解析 draft、引用、pattern 与 format，产出 `*Schema`；`Schema.Validate` 每次创建新的 validator 状态，不修改 compiled schema（[`compiler.go`](https://github.com/santhosh-tekuri/jsonschema/blob/29cbed948d24a04700eb94436416b18a07953b71/compiler.go)、[`validator.go`](https://github.com/santhosh-tekuri/jsonschema/blob/29cbed948d24a04700eb94436416b18a07953b71/validator.go)）。这正适合 provider registration：构建失败立即 panic/error，运行时只读共享。

v6.0.2 的 `go.mod` 要求 Go 1.21；生产代码依赖 `golang.org/x/text`。仓库列出的 `regexp2` 只用于测试示例，消费方未引用时不会进入生产依赖图（[`go.mod`](https://github.com/santhosh-tekuri/jsonschema/blob/29cbed948d24a04700eb94436416b18a07953b71/go.mod)）。

### 4. regex 与 format 仍不能“自动完美一致”

Santhosh 默认也使用 Go `regexp`，但公开 `UseRegexpEngine`；官方示例展示了替换为 ECMAScript 模式引擎（[`example_regexp_test.go`](https://github.com/santhosh-tekuri/jsonschema/blob/29cbed948d24a04700eb94436416b18a07953b71/example_regexp_test.go)）。Draft 2020-12 下 `format` 默认是 annotation，需调用 `AssertFormat` 才成为断言（[`compiler.go`](https://github.com/santhosh-tekuri/jsonschema/blob/29cbed948d24a04700eb94436416b18a07953b71/compiler.go)）。

不存在一个 Go regexp 实现能无条件保证与浏览器 JavaScript 正则完全相同。推荐策略是：

- provider pattern 限定在 Go RE2 与 ECMAScript 的共同子集；
- registration 时两端契约 fixture 同时跑 Go validator 与 Ajv2020；
- 没有明确业务价值时不用 `format`，需要时前后端启用同名 format 并加入 fixture。

## 四、两个重点候选为何落选

### `google/jsonschema-go`：低依赖、设计干净，但不是最匹配

v0.4.3 的优点是真实的：

- `For[T]` 按 `json` tag 推导类型，struct 默认 `additionalProperties:false`，普通字段默认 required，并记录字段顺序（[`infer.go`](https://github.com/google/jsonschema-go/blob/8c4ab4f02ef64dcea5502e47a6113e8292944087/jsonschema/infer.go)）。
- `Schema` 原生表示 Draft-07/2020-12、`if/then/else`、`oneOf`、`uniqueItems`、`Extra` 和 `PropertyOrder`（[`schema.go`](https://github.com/google/jsonschema-go/blob/8c4ab4f02ef64dcea5502e47a6113e8292944087/jsonschema/schema.go)）。
- `Resolve` 预编译 regex，之后 `Resolved.Validate` 使用局部 state，适合并发共享（[`resolve.go`](https://github.com/google/jsonschema-go/blob/8c4ab4f02ef64dcea5502e47a6113e8292944087/jsonschema/resolve.go)、[`validate.go`](https://github.com/google/jsonschema-go/blob/8c4ab4f02ef64dcea5502e47a6113e8292944087/jsonschema/validate.go)）。

但决定性短板是：

- rich validation error 仍是 open proposal [#53](https://github.com/google/jsonschema-go/issues/53)；spike 只得到类似 `validating /properties/port` 的拼接字符串，没有 instance path 结构。
- 官方文档明确 `format` 只记录、不验证；pattern 固定 Go regexp（[`doc.go`](https://github.com/google/jsonschema-go/blob/8c4ab4f02ef64dcea5502e47a6113e8292944087/jsonschema/doc.go)）。
- 从 struct 推导基础类型并不能删除 moonbase 的 field 定义；title、选项、showWhen、secret、immutable 仍要再装饰。既然现有 producer 已经完整，换 producer 的净收益很小。

因此 Google 是“若项目没有现成 schema producer”时很有吸引力的低依赖方案，不是 moonbase 当前的最佳方案。

### `kaptinlin/jsonschema`：功能最全，但版本演进风险过高

v0.9.3 同时提供 struct tags、constructor、多个 draft validator、rich errors、i18n 与 `Unmarshal`；其中 i18n 对全站中文的 moonbase 没有选型价值。然而源码和 spike 还暴露了几个不能忽略的问题：

1. `Schema.Unmarshal` 文档明确“不执行验证”；实测把 map 中 `443.5` 解到 `int` 时返回 nil，并得到 `443`（[`unmarshal.go`](https://github.com/kaptinlin/jsonschema/blob/e39feffecd18896173804f627c8ec2b9486ad181/unmarshal.go)）。只有先调用 validator 才能挡住它，API 本身不是安全的 parse seam。
2. `evaluatePattern` 首次验证时写 `compiledStringPattern`；`compilePatterns` 会写 `compiledPatterns`，没有同步（[`pattern.go`](https://github.com/kaptinlin/jsonschema/blob/e39feffecd18896173804f627c8ec2b9486ad181/pattern.go)、[`pattern_properties.go`](https://github.com/kaptinlin/jsonschema/blob/e39feffecd18896173804f627c8ec2b9486ad181/pattern_properties.go)）。包含 pattern 的共享 schema 不能标记为并发安全。
3. `FromStruct` 的全局 cache 返回共享可变 `*Schema`，且 cache key 不包含 `FieldNameMapper`、`SchemaProperties`、`RequiredSort` 等选项（[`struct_tags.go`](https://github.com/kaptinlin/jsonschema/blob/e39feffecd18896173804f627c8ec2b9486ad181/struct_tags.go)）。
4. 当前 `go.mod` 写的是 `go 1.26.4`，对应的 patch-version 传播问题仍在 open issue [#110](https://github.com/kaptinlin/jsonschema/issues/110)；它还直接依赖 2026 pseudo-version 的 `go-json-experiment/json`（[`go.mod`](https://github.com/kaptinlin/jsonschema/blob/e39feffecd18896173804f627c8ec2b9486ad181/go.mod)）。
5. 近期问题说明 producer 仍快速演进：`go-json-experiment` 触发分析器 panic 的 [#103](https://github.com/kaptinlin/jsonschema/issues/103) 已关闭为上游工具问题但依赖仍保留；enum + default tag 丢失的 [#114](https://github.com/kaptinlin/jsonschema/issues/114) 已在 v0.7.13 修复；conditional tags 的 [#95](https://github.com/kaptinlin/jsonschema/issues/95) 最终确认是文档错误——该能力从未实现，只删除了错误文档。

这些并非说明项目质量差，而是说明它仍处于功能和 API 快速变化期。moonbase 没必要为了已拥有的 producer，引入它更宽的依赖与状态面。

## 五、其他候选

### Zog

Zog v0.22.2 是最接近 Zod 使用体验的 parser/validator，但稳定 release 不产生 JSON Schema；ZSS → JSON Schema 直到 2026-07-05 才合入未发布分支，ZSS 仍标 experimental。声明式 conditional validation 仍是 open issue [#216](https://github.com/Oudwins/zog/issues/216)。它不能作为当前跨 Go/rjsf 契约底座。

### `invopop/jsonschema` + Santhosh

`invopop/jsonschema` v0.14.0 是成熟的 Draft 2020-12 generator，支持 ordered properties、`if/then` 和 `jsonschema_extras`（[`schema.go`](https://github.com/invopop/jsonschema/blob/f678162af201cba391c9bf759b429dee5ad5c0bc/schema.go)、[`reflect.go`](https://github.com/invopop/jsonschema/blob/f678162af201cba391c9bf759b429dee5ad5c0bc/reflect.go)）。与 Santhosh 组合在通用项目中是可靠方案。

moonbase 已有更贴合 rjsf、中文 option description、showWhen 与 secret 生命周期的 producer。再加 Invopop 只会要求 tags/hook 与 `config.Field` 对齐，收益不如直接复用当前输出。

### `swaggest/jsonschema-go`

它提供强大的反射 interceptor、extra properties、if/then exposer 和字段映射（[`reflect.go`](https://github.com/swaggest/jsonschema-go/blob/a79ab04d29f161354edcafdf474ab2ed11f72adf/reflect.go)），但本身仍是 generator，不验证实例；其 schema model 以旧 `definitions`/`additionalItems` 形状为主。没有理由用更复杂的反射扩展系统替换当前小型 producer。

### Huma schema 层

Huma v2 的 schema/validator 是为 OpenAPI 3.1 HTTP 请求设计的高性能子集；源码明确称其只支持 JSON Schema 子集，并且 `Schema` 没有 `if/then/else`（[`schema.go`](https://github.com/danielgtaylor/huma/blob/v2.38.0/schema.go)）。为单个 provider config seam 引入整个 Huma 模块依赖面不符合净收益原则。

### CUE

CUE 适合把约束语言本身提升为真源，再导出 JSON Schema/OpenAPI。那会把当前问题从“给 Go provider config 找 validator”变成“全项目采用另一种 schema 语言”，并新增 CUE ↔ 私有 Go struct ↔ rjsf UI metadata 三个接缝。除非 moonbase 未来整体采用 CUE，否则不应为这一处配置引入不同范式。

## 六、Mask/Merge 库评估

> 本节来源均为项目官方文档、源码或标准文本，访问日期均为 2026-07-11。

### 先定义这里的“基础功能”到底是什么

moonbase 需要的不是普通的 map merge，而是一个由 provider schema 驱动的状态转换：

- read：`secret` 字段输出空值，并额外输出 `<key>_set`，不能返回部分掩码后的秘密；
- update：incoming secret 为空时保留 stored，非空时替换；
- update：`immutable` 字段始终保留 stored；
- 普通字段的 `""`、`false`、`0`、空 slice 都必须是有效的显式更新，不能被“忽略空值”吞掉；
- unknown key 必须报错，不能被静默保留或静默丢弃；
- 结果必须确定、只遍历 schema 已知字段，不靠通用反射扫描对象，以降低意外泄密面。

JSON Schema 的 `readOnly` / `writeOnly` 只能承载意图，不能执行这个转换。Draft 2020-12 明确把它们定义为 annotation：`readOnly` 的修改由 owning authority 自行忽略或拒绝，`writeOnly` 表示读取时不出现；具体动作仍由应用决定（[JSON Schema Validation 2020-12 §9.4](https://json-schema.org/draft/2020-12/json-schema-validation.html#section-9.4)）。因此 schema 可以继续作为元数据真源，但 validator 不会自动提供 Mask/Merge。

### 候选对比

| 候选 | 可复用的通用机制 | 为什么不能直接满足完整语义 |
| --- | --- | --- |
| `dario.cat/mergo` | struct/map 的零值填充、override、允许空值覆盖 | 它的策略是全局的：默认/`WithOverride` 会保留所有空值，导致普通 `""`、`false`、`0`、空 slice 无法清空；`WithOverwriteWithEmptyValue` 又会把空 secret 一并覆盖。transformer 只按 `reflect.Type` 选择，拿不到 provider schema 的 key/secret/immutable 策略（[`merge.go`](https://github.com/darccio/mergo/blob/bd2790490aef9dcdde7bf6226db31808dc067330/merge.go#L40-L52)、[`WithOverride` / `WithOverwriteWithEmptyValue`](https://github.com/darccio/mergo/blob/bd2790490aef9dcdde7bf6226db31808dc067330/merge.go#L310-L340)）。 |
| `github.com/jinzhu/copier` 等 copier | struct 间复制、`IgnoreEmpty`、deep copy、字段 tag | `IgnoreEmpty` 与 Mergo 有相同的全局零值歧义；关闭后 secret 被清空。它也没有 stored/incoming/schema 三方语义、`<key>_set` 投影或 unknown-key 拒绝（[`Option`](https://github.com/jinzhu/copier/blob/c6b47b092d9840406d0abc347e68a28a7b812643/copier.go#L40-L48)、[`CopyWithOption`](https://github.com/jinzhu/copier/blob/c6b47b092d9840406d0abc347e68a28a7b812643/copier.go#L124)）。把 secret/immutable 再写成 copier tag 还会制造第二份元数据。 |
| RFC 7396 JSON Merge Patch + `evanphx/json-patch` | 标准化局部更新；`false`、`0`、`""`、空数组都会被明确应用 | RFC 算法对每个非 null patch 值直接递归替换，因此空 secret 会覆盖旧 secret，immutable 会被改写；新 key 会直接加入结果，unknown 不会被 schema 拒绝。数组只能整体替换，`null` 表示删除（[RFC 7396 §2](https://www.rfc-editor.org/rfc/rfc7396#section-2)、[`MergePatch`](https://github.com/evanphx/json-patch/blob/84a4bb100ade42a86fce2647c95a7dbcbf569cb2/merge.go#L105-L110)）。若未来 API 改成真正 PATCH，可把“省略 secret = 保留”作为 wire 语义，但 immutable、unknown 校验和 read projection 仍需本地层。 |
| Kubernetes strategic merge patch | 在 RFC merge 基础上按 schema/tag 处理复杂 list merge、merge key、retainKeys | 它的 schema 元数据是 `patchStrategy` / `patchMergeKey`，服务于 Kubernetes 对象和列表，不包含 secret/immutable。源码会把 original 中不存在的普通 patch key直接加入结果，所以也不负责 unknown-field 拒绝（[`StrategicMergeMapPatch`](https://github.com/kubernetes/apimachinery/blob/0838f1f442d52387e92ed7668fc5bf911a8509a9/pkg/util/strategicpatch/patch.go#L852-L872)、[`mergeMap`](https://github.com/kubernetes/apimachinery/blob/0838f1f442d52387e92ed7668fc5bf911a8509a9/pkg/util/strategicpatch/patch.go#L1322-L1403)）。为几个平坦 provider 字段引入 `k8s.io/apimachinery` 与它的 OpenAPI/patch 模型，依赖和认知面远大于删除的代码。 |
| Terraform Plugin Framework | `Sensitive`、`WriteOnly`、prior state 与 plan modifier 展示了成熟的字段生命周期模型 | `Sensitive` 只遮蔽 CLI 输出，不影响 state 存储；`WriteOnly` 恰好是不写入 plan/state；`UseStateForUnknown` 只在 planned value 为 **unknown** 时复制 state，已知空 string 不会触发；`RequiresReplace` 是字段变化后重建资源，不是保留旧值（[`StringAttribute`](https://github.com/hashicorp/terraform-plugin-framework/blob/635c41e805031e862b1458d63ce0c7643fe9863c/resource/schema/string_attribute.go#L40-L180)、[`UseStateForUnknown`](https://github.com/hashicorp/terraform-plugin-framework/blob/635c41e805031e862b1458d63ce0c7643fe9863c/resource/schema/stringplanmodifier/use_state_for_unknown.go)、[`RequiresReplace`](https://github.com/hashicorp/terraform-plugin-framework/blob/635c41e805031e862b1458d63ce0c7643fe9863c/resource/schema/stringplanmodifier/requires_replace.go)）。这些能力嵌在 Terraform protocol/plan/state engine 中，不是可抽出的通用 Mask/Merge 包。 |
| Pulumi | secret 属性与 state 加密、`replaceOnChanges` | secret 解决的是 state 加密/显示传播；`replaceOnChanges` 把 diff 改为 replacement，均不定义“空输入保留 stored secret”或 `<key>_set` 响应（[Pulumi metaschema](https://github.com/pulumi/pulumi/blob/af36b7db341077ff4ee3bf7559a2f8216e7a08a3/docs/references/metaschema.md#replaceonchanges)、[state secrets](https://github.com/pulumi/pulumi/blob/af36b7db341077ff4ee3bf7559a2f8216e7a08a3/docs/architecture/deployment-execution/state.md#secrets--encryption)、[`applyReplaceOnChanges`](https://github.com/pulumi/pulumi/blob/af36b7db341077ff4ee3bf7559a2f8216e7a08a3/pkg/resource/deploy/step_generator.go#L3015-L3070)）。和 Terraform 一样，它是完整 IaC engine 的能力，不是 provider config helper。 |
| `go-viper/mapstructure` | map → struct 解码，可用 `ErrorUnused` 报 unknown key | 它只负责 decode；`ZeroFields` 和 `WeaklyTypedInput` 是解码策略，不比较 stored/incoming，也不做 read projection（[`DecoderConfig`](https://github.com/go-viper/mapstructure/blob/52aa5c6dc1d27226460807054ca2107b2d54fb2d/mapstructure.go#L253-L323)）。既然本设计已选择 JSON Schema validate 后用 `encoding/json` 严格解码，再引入它没有净收益。 |
| `go-masker` 一类日志 masker | 按 struct tag 把值替换为星号/部分掩码，或用 `Sensitive[T]` 防止普通格式化泄漏 | moonbase 的 wire 约束是 secret 必须返回空值 + set flag，而非返回可辨认的部分内容；它也不做 update merge、immutable 或 unknown 校验，并要求另一套反射/tag 元数据（[`go-masker` README](https://github.com/ggwhite/go-masker/blob/ff7b9587f9ae3b71a61c7cca6966552108893b88/README.md)）。它可用于日志值类型，但不能替代 profile response projector。 |

### Spike：最接近的三类通用库仍需要重写核心策略

spike 位于 `/tmp/moonbase-mask-merge/spike`，使用 Mergo v1.0.2、Copier v0.4.0 和 `evanphx/json-patch/v5` v5.9.11；未修改产品代码。输入是：stored secret/key/普通字段均有值，incoming 把 secret 置空、修改 immutable，并把普通 string/bool/int/slice 显式清零。

期望 effective config：

```json
{"password":"stored-secret","key":"immutable-key","name":"","enabled":false,"retries":0,"allowlist":[]}
```

实测：

| 调用 | 关键结果 |
| --- | --- |
| Mergo `WithOverride` | 保住 secret，但错误保留普通 name/true/3/旧 slice；immutable 被改写 |
| Mergo + `WithOverwriteWithEmptyValue` | 普通零值正确，但 secret 被清空；immutable 被改写 |
| Copier `IgnoreEmpty:true` | 与 Mergo 默认同类；普通零值不能表达，immutable 被改写 |
| Copier `IgnoreEmpty:false` | secret 被清空，immutable 被改写；空 slice 仍未覆盖旧 slice |
| JSON Merge Patch | 普通零值正确，但 secret 被清空、immutable 被改写，并保留 unknown key |

没有一个调用能生成 read 侧的 `{"password":"","password_set":true}`；这不是 merge 算法的输出，而是 schema-aware response projection。

### 唯一结论，以及彻底重构时的区别

**无论最低迁移还是从零重构，都不为 Mask/Merge 引入通用 merge/mask 库。** 真正不可替代的代码恰好就是逐字段读取 `Secret` / `Immutable` 并执行两条分支。任何通用 merge/copy/patch 库只能替代最简单的赋值循环，却仍需在调用前后自行实现 secret、immutable、unknown-key 和 `<key>_set`；同时会引入零值策略、反射/tag 或 patch dialect，审计面反而扩大。

但“不引入库”不等于必须保留当前 producer 形状。两条路线应明确区分：

| 路线 | 契约真源 | Mask/Merge 形态 |
| --- | --- | --- |
| 复用现有代码的最低迁移方案 | 当前 `config.Schema` / `form.Schema` | 修正为 compiled field index 上的几十行本地 policy executor |
| 不考虑兼容、追求最终模型 | provider-local `provider.schema.json` + `x-moonbase-*` vocabulary | build-time 生成只读 `FieldPolicy`、read projector、update resolver 与私有 typed config；运行时仍由集中 lifecycle executor 执行，不散落到 driver |

从零模型中，**标准 JSON Schema 本身不能充当完整领域真源**：标准已经明确把 `readOnly` / `writeOnly` 的执行留给应用，无法表达 secret 的 keep/set/clear、create-only 更新拒绝和安全读投影。但 provider-local schema 加上受控的 `x-moonbase-*` vocabulary 可以成为完整真源：标准关键词表达合法性，自定义 annotation 表达 lifecycle/UI，生成器把它们编译成标准展开 schema、私有 Go 类型和只读 policy。CUE 可以成为另一种约束语言，却同样不会替 moonbase 执行这些生命周期动作，并额外引入有损的 JSON Schema/Go 导出接缝。

因此，用户允许零兼容迁移改变的是：**producer 可以彻底重写为 schema-first manifest，旧的空值哨兵与 `<key>_set` wire 也可以删除**；不变的是 lifecycle policy 仍不能外包给通用 merge/mask 库。

后续实现应让这层更窄、更严格，而不是更通用：

1. registration 时把 schema 编译成只读 field index；Mask/Merge 只遍历该 index，不反射 provider struct；
2. incoming 先对照 index 拒绝 unknown key，不能先丢弃 unknown 再让 JSON Schema 验证；
3. secret 使用显式 keep/set/clear mutation；空 string 只是普通候选值，由 schema 决定是否合法；普通字段原样接受 `""`、`false`、`0`、空 slice；
4. create-only 字段在 update 中省略或回传相同值均可，提交不同值明确拒绝，不静默吞掉；
5. 合并后的 effective config 再交给同一份 compiled JSON Schema validate，然后严格 decode 到 provider 私有 `T`；
6. read projection 只复制 schema 已知普通字段；secret 值绝不进入 config/view，presence state 放在独立字段集合，不再派生 `<key>_set` 伪字段。

这层是生命周期 policy，不是值得外包的通用算法；保持本地、短小和表驱动，反而最符合“净收益”原则。

## 七、Greenfield 最优模型

> 本节回答一个更强的问题：若完全不保留当前 `form/config` API、settings JSON 形状或生成方式，什么模型能让 provider 配置真正只有一个真源？来源均为官方规范、源码、release 或 issue，复核日期为 2026-07-11。

### 唯一推荐：provider-local JSON Schema + moonbase vocabulary + 小型结构 codegen

**唯一推荐 B：每个 provider 以一份 Draft 2020-12 JSON Schema 作为唯一真源；标准关键词表达结构和合法性，`x-moonbase-*` vocabulary 表达 UI 与配置生命周期；仓库内小型生成器只把 schema 的结构投影为 provider 私有 Go 类型和原子 registration；Santhosh 与 Ajv2020 验证同一份 schema。**

这不是“JSON Schema 能自动完成一切”。标准明确把 `readOnly` / `writeOnly` 定义为 annotation，并把读取、更新时如何处理交给应用（[JSON Schema Validation 2020-12 §9.4](https://json-schema.org/draft/2020-12/json-schema-validation.html#section-9.4)）。因此必须把两类内容分开：

- 标准 JSON Schema 执行 `type`、`required`、数值/长度、`pattern`、`enum`、`uniqueItems`、`if/then` 和 unknown-field 拒绝；
- moonbase vocabulary 只声明标准无法执行的状态语义，通用 lifecycle engine 执行 read projection 与 create/update transition。

推荐目录：

```text
packages/integrations/email/
  email.go                       # Message、Driver seam、Registration 封装
  providers.go                   # 显式有序组合 smtp.New()/cloudflare.New()
  smtp/
    provider.schema.json         # 唯一真源：key/presentation/config/UI/lifecycle
    config_gen.go                # 生成：私有 config + compiled field index
    registration_gen.go          # 生成：schema embed + 原子 Registration 工厂
    driver.go                    # 只写发送实现；只接触私有 config
  cloudflare/
    provider.schema.json
    config_gen.go
    registration_gen.go
    driver.go
tools/provider-config-gen/       # 仓库内小型、受限 dialect 的生成器
packages/integrations/core/config/
  compile.go                     # meta-schema、Santhosh compile、只读缓存
  transition.go                  # 通用 Mask/ResolveUpdate；无 provider 分支
```

schema 示例：

```json
{
  "$schema": "https://moonbase.dev/schema/provider-config-1",
  "$id": "https://moonbase.dev/providers/email/smtp",
  "title": "SMTP",
  "description": "通过 SMTP 服务器发送邮件",
  "type": "object",
  "additionalProperties": false,
  "x-moonbase-provider": {
    "key": "smtp"
  },
  "properties": {
    "host": {
      "type": "string",
      "title": "服务器地址",
      "minLength": 1,
      "x-moonbase-ui": { "placeholder": "smtp.example.com" }
    },
    "port": {
      "type": "integer",
      "title": "端口",
      "minimum": 1,
      "maximum": 65535
    },
    "authMode": {
      "type": "string",
      "title": "认证方式",
      "enum": ["none", "password"],
      "x-moonbase-options": [
        { "value": "none", "label": "无认证" },
        { "value": "password", "label": "用户名和密码" }
      ]
    },
    "password": {
      "type": "string",
      "title": "密码",
      "writeOnly": true,
      "x-moonbase-secret": true,
      "x-moonbase-visible-when": {
        "property": "authMode",
        "equals": "password"
      }
    }
  },
  "required": ["host", "port", "authMode"],
  "allOf": [{
    "if": {
      "properties": { "authMode": { "const": "password" } },
      "required": ["authMode"]
    },
    "then": { "required": ["password"] }
  }]
}
```

`x-moonbase-visible-when` 与 `if/then` 不应由作者双写。上例为最终发给 validator/rjsf 的展开制品；源码 dialect 中只声明 visibility condition，生成器确定性地产生匹配的 `if/then`，并拒绝无法投影的条件。这样 show/hide 与 required validation 不会漂移。若某个条件只影响显示、不影响 required，则显式声明 `requiredWhenVisible:false`。

同理：

- `x-moonbase-secret:true` 必须同时生成标准 `writeOnly:true`；两者冲突时生成失败；
- `x-moonbase-create-only:true` 表示 create 可写、update 保留 stored。它不能冒充标准 `readOnly`，因为 `readOnly` 并不表达“创建时可输入、更新时保留旧值”；
- `title` / `description` 直接写中文，不引入 message key 或 i18n；
- select 的 wire 值使用标准 `enum`，中文 label/description 使用 `x-moonbase-options`；生成器校验 options 与 enum 1:1；
- `x-moonbase-ui` 只允许受控的 widget、placeholder、help、order，不允许 provider 注入任意前端组件名。

根 `$schema` 应指向 moonbase 自己的 meta-schema。它以 Draft 2020-12 meta-schema 为基础，并通过 `$vocabulary` 声明上述关键词。JSON Schema Core 正式定义了 meta-schema/vocabulary 机制（[JSON Schema Core 2020-12 §8.1](https://json-schema.org/draft/2020-12/json-schema-core.html#section-8.1)）；Santhosh v6 原生提供 `RegisterVocabulary`、keyword compiler 与 meta-schema 支持（[`vocab.go`](https://github.com/santhosh-tekuri/jsonschema/blob/29cbed948d24a04700eb94436416b18a07953b71/vocab.go)、[`example_vocab_uniquekeys_test.go`](https://github.com/santhosh-tekuri/jsonschema/blob/29cbed948d24a04700eb94436416b18a07953b71/example_vocab_uniquekeys_test.go)）。Web 的 Ajv2020 同样要注册这些 annotation keyword，严格模式下不能靠“碰巧忽略未知字段”。

### lifecycle engine：保留，但成为 schema 编译产物的执行器

没有现成库能执行 moonbase 的完整语义，本节前面的库评估已经说明原因。greenfield 下应改的是 wire 与编译边界，而不是继续寻找通用 map merge：

```text
Create(incoming)
  → reject response-only state
  → validate expanded JSON Schema
  → strict decode private config

Update(incoming, stored)
  → secret: omitted/empty = stored；non-empty = incoming
  → create-only: stored exists 时无条件取 stored
  → ordinary: incoming 的 "" / false / 0 / [] 原样生效
  → validate effective config
  → strict decode private config

Read(stored)
  → 只复制 schema 已知的非 secret 字段
  → secret 值绝不进入 response
  → secret-set 状态放在独立字段集合，不再伪装成 config 的 `<key>_set`
```

理想 wire 应把 `secret_set`（或等价的 field-state map）放在 `Profile` 的 config 之外。这样读响应无需违反 input schema 的 `additionalProperties:false`，也无需在 config 中塞入 validator 必须特殊忽略的 `<key>_set` 伪字段。允许不兼容重构时，这是比当前 Mask 输出更干净的模型。

lifecycle engine 在 registration 时把 vocabulary 编译成不可变的 `[]FieldPolicy`；运行时只遍历该索引，不扫描 struct、不修改 schema、无全局可变注册，因此可审计且并发安全。`Usable` 不再有独立规则：它等于“stored effective config 通过同一个 compiled schema”。

### 生成 API 与 provider 封装

生成代码应使 provider 只导出一个构造入口：

```go
// Code generated. DO NOT EDIT.
type config struct {
    Host     string `json:"host"`
    Port     int    `json:"port"`
    AuthMode string `json:"authMode"`
    Password string `json:"password"`
}

func newRegistration(send func(context.Context, config, email.Message) error) email.Registration
```

provider 手写代码：

```go
func New(httpClient *http.Client) email.Registration {
    d := &driver{httpClient: httpClient}
    return newRegistration(d.send)
}

func (d *driver) send(ctx context.Context, cfg config, msg email.Message) error {
    // 这里不存在 map key、schema 或 descriptor。
}
```

`Registration` 的字段保持不导出；生成函数把 provider key、presentation、schema、compiled validator、field policy 和 typed adapter 原子绑定。组合根只能显式调用 `smtp.New()` / `cloudflare.New(httpClient)`，不使用 `init()` 或全局可变 registry。

### 三条路线比较

| 路线 | 单一真源 | 条件/UI/lifecycle | 私有 typed Go | 漂移发现 | 依赖与长期成本 | 结论 |
| --- | --- | --- | --- | --- | --- | --- |
| A. Go struct code-first：Invopop/Google/Kaptinlin + Santhosh + tags/hooks | ⚠️ 结构来自 struct，但复杂 schema、中文 option、showWhen 和 lifecycle 会散在 tags、`JSONSchemaExtend`/decorator hooks | ⚠️ 能实现，但 tag 是字符串，复杂 `if/then` 需要手写 schema mutation；要另写 verifier | ✅ | ⚠️ 多数只能 registration-time；Go 编译器不检查 tag 内 key/条件引用 | 反射 producer 成熟，但 moonbase 仍需一层不小的 tag DSL 与 hook 约定 | 不选 |
| B. JSON Schema schema-first + vocabulary + structural codegen | ✅ schema 文件同时产生 Go type、validator、UI 和 lifecycle index | ✅ 标准关键词 + 受控扩展；Mask/ResolveUpdate 由通用 engine 执行 | ✅ 生成 unexported type | ✅ meta-schema + codegen + Go compile + Santhosh/Ajv contract tests | 新增一个很小的仓库工具；没有第二份手写字段目录 | **唯一推荐** |
| C. CUE schema-first + attributes + JSON Schema/Go 导出 | ⚠️ CUE 可作真源，但导出 JSON Schema 和 Go 都不能保证等价 | ⚠️ CUE 约束强，attributes 可装 metadata；但 UI/lifecycle 仍要自写 consumer | ⚠️ 当前 Go type generation 会把无法表示的 disjunction 放宽为 `any` | ⚠️ CUE runtime 可验证真约束，但 rjsf/Ajv 只看到可能被弱化的投影 | 引入新语言、CLI、Go runtime 与两个 experimental generator seam | 不选 |

### 为什么不选 A：`Definition[T]` 仍是两种表达系统

即使重写成漂亮的泛型 API：

```go
config.Define[providerConfig](
    config.String("host").Required(),
    config.Secret("password").VisibleWhen("authMode", "password"),
)
```

`providerConfig` 的 `json` 字段和 `Define` 的 key/类型仍然重复；registration verifier 只能在运行时发现漂移。若把全部规则改进 struct tag，复杂条件、中文 option description 与 UI object 会退化成长字符串；若用 `JSONSchemaExtend`，schema 结构又藏进命令式 mutation。Invopop 确实是成熟的 Draft 2020-12 generator，支持 ordered properties、`If/Then/Else` schema model、custom extras 和 `JSONSchemaExtend`（[`schema.go`](https://github.com/invopop/jsonschema/blob/f678162af201cba391c9bf759b429dee5ad5c0bc/schema.go)、[`reflect.go`](https://github.com/invopop/jsonschema/blob/f678162af201cba391c9bf759b429dee5ad5c0bc/reflect.go)）；它适合以 Go public API types 为真源的项目，但 provider 配置首先是跨 Go/rjsf 的数据契约，schema-first 更直接。

Google 与 Kaptinlin 也没有改变这个所有权问题：它们能从 struct 产生 schema，不会替 moonbase 定义 create-only、secret preserve、visibility 或 option presentation。最终仍需自定义 tags/hooks，且必须证明这些装饰与反射生成的 schema 没有漂移。

### 为什么不直接采用现成 JSON Schema → Go generator

最活跃候选是原 `atombender/go-jsonschema`、现由 `omissis/go-jsonschema` 维护；v0.23.1 发布于 2026-05-09。它能生成 structs、enum、`allOf`/`anyOf` 的部分模型以及 unmarshal-time validation，但官方状态页仍明确列出 `writeOnly`、`if/then/else`、`oneOf`、`uniqueItems` 等未完整实现，并称项目“not finished”（[v0.23.1 README](https://github.com/omissis/go-jsonschema/blob/v0.23.1/README.md#status)）。源码虽然已含部分 `anyOf`/`allOf` 合并逻辑，但没有通用 `oneOf` typed union；README 与实现共同说明不能把完整 Draft 2020-12 映射为等价 Go 类型。

这里也不应该尝试做一个“完整 JSON Schema → Go”生成器：Go 类型系统本来就无法等价表达任意 `oneOf`、条件约束或 dependent schema。推荐的仓库工具只处理 **structural house subset**：object/property、primitive、array、map、required/optional 与 `$ref`；所有合法性仍由 Santhosh 执行。条件字段的 properties 必须在稳定的 root/object shape 中声明，`if/then` 只改变 required/constraint/UI，不在不同分支制造互斥 Go 结构。超出 subset 时 generator 直接失败，而不是生成 `any` 或静默放宽。

这段 codegen 的职责比现有 `protoc-gen-permissions` 还窄：读取 meta-schema 已验证的 AST，确定性输出 Go struct、embed 和 field-policy table。采用 omissis 再在前后包一层，仍要处理其未支持关键词、命名和 lifecycle index；第三方没有替掉足够多的 moonbase 代码，净收益不足。仓库工具反而能把受支持 dialect 变成可审计的硬边界。

### 为什么不选 CUE

CUE v0.17.0 已提供 `encoding/jsonschema.Generate`，但官方注释明确标记为 **experimental**、输出形式可能随 release 改变，且目前只导出 Draft 2020-12（[`generate.go`](https://github.com/cue-lang/cue/blob/v0.17.0/encoding/jsonschema/generate.go#L35-L123)）。更关键的是，源码对无法表示到 JSON Schema 的二元约束和未知函数会放宽，部分路径直接输出接受任意值；因此“CUE 后端验证通过”不保证发给 Ajv2020 的投影拥有同等约束（[`generate.go`](https://github.com/cue-lang/cue/blob/v0.17.0/encoding/jsonschema/generate.go#L494-L553)）。

`cue exp gengotypes` 也明确标记 experimental，并说明生成 Go 类型只保证接受所有 CUE 有效值，但可能更宽；如 `string | int` 会生成 `any`（[`exp.go`](https://github.com/cue-lang/cue/blob/v0.17.0/cmd/cue/cmd/exp.go#L75-L122)）。CUE attributes 很适合承载自定义 metadata，但官方规范也明确 attributes 不影响 CUE 求值、语义由 consumer 自己解释（[CUE spec：Attributes](https://github.com/cue-lang/cue/blob/v0.17.0/doc/ref/spec.md#attributes)）；所以 secret/create-only/UI engine 一行也不会消失。

若 moonbase 全仓未来采用 CUE，它值得重新评估。只为 provider config 引入 CUE，会得到 CUE validator、实验 JSON Schema exporter、实验 Go generator和自定义 attribute consumer四个接缝；而本需求最终仍必须把 JSON Schema 交给 rjsf。净收益不成立。

TypeSpec 也不进入最终候选：官方 `@typespec/json-schema` 支持 Draft 2020-12 emitter 和 `@extension`，但 TypeSpec 主仓当前官方 emitter 目录有 JS/Java/C#/Python client/server，没有 Go emitter；仍需另一个 JSON Schema → Go 接缝（[`@typespec/json-schema` README](https://github.com/microsoft/typespec/blob/main/packages/json-schema/README.md)、[官方 packages](https://github.com/microsoft/typespec/tree/main/packages)）。它会在 JSON Schema 之前再加一种语言，却没有替掉本地结构 codegen。Jsonnet 是数据模板语言，不提供本需求所需的 type/constraint/codegen 闭环，也不是现实候选。

### 相对当前 `config.Field` 的决定性改进

1. 字段 key、类型、required、约束、条件、中文 presentation、secret/create-only 只出现一次；不再由 Go struct 与 `config.Field` 互相校验。
2. rjsf/Ajv2020 收到的就是构建制品中的同一 schema；服务端不是“按另一套 DSL 重新生成一个相似 schema”。
3. provider 包独占 schema、私有 config 与 driver；integration 根包只定义 seam 和显式组合。
4. 生成时检查 vocabulary、enum/options、条件引用、structural subset；注册时 Santhosh compile；Web 契约测试实际用 Ajv2020 compile。错误尽可能前移。
5. lifecycle 仍是几十行本地代码，但它只解释正式 vocabulary，并编译成只读策略表；不再同时充当 schema producer、validator 和 UI builder。
6. 没有 i18n 层；所有 title/description/option label 直接是中文 schema annotation，RPC 错误仍返回稳定的中文通用消息。

### Greenfield 决策摘要

```text
provider.schema.json（唯一真源）
  ├─ build-time meta-schema + vocabulary check
  ├─ provider-config-gen → private config + atomic Registration + FieldPolicy
  ├─ exact expanded schema → Santhosh v6（服务端）
  └─ exact expanded schema → rjsf + Ajv2020（Web）

stored + incoming + FieldPolicy
  → lifecycle ResolveCreate/ResolveUpdate
  → one compiled JSON Schema validation
  → strict decode private config
  → provider driver
```

这条路线不是依赖最多的路线，而是唯一把“跨端契约”和“provider 私有 Go 类型”都从同一制品推导出来、同时承认 state transition 必须由应用执行的路线。

## 八、JSON Schema → Go 生成实现

> 本节是 2026-07-11 grill 后的实现收敛，**取代**前文把 lifecycle/UI 写成
> `x-moonbase-*` JSON Schema vocabulary 的草案。源码是一份 `provider.json` manifest：
> `config` 是纯标准 Draft 2020-12 JSON Schema，`lifecycle` 与 `ui` 是并列 sidecar；
> 在 moonbase 的 house profile 中，字符串字段的标准 `writeOnly: true` 表示 secret。

### 有现成生成器，但没有一个可以直接生成 moonbase 所需的完整 registration

现成工具能证明“手写 JSON Schema 生成 Go struct”是成熟做法，但它们的目标是把尽可能多的
JSON Schema 猜成通用 Go 类型。moonbase 的目标更窄：只投影 provider config 的稳定结构，完整
合法性仍交给 Santhosh；另外还要原子绑定 provider descriptor、lifecycle policy 和私有 typed
decoder。这后一半不是任何通用生成器的职责。

| 候选 | `object/properties/required`、`enum`、array/map、`$ref` | `if/then`、`oneOf`、`writeOnly` | 对 moonbase 的判断 |
| --- | --- | --- | --- |
| [`omissis/go-jsonschema` v0.23.1](https://github.com/omissis/go-jsonschema/releases/tag/v0.23.1)（原 `atombender`） | 能生成 struct、primitive enum、array、`additionalProperties` map，并解析 `$defs`/旧 `definitions` 和引用；也公开可复用的 [`pkg/generator`](https://github.com/omissis/go-jsonschema/blob/v0.23.1/pkg/generator/config.go) API | 官方状态仍把 `writeOnly`、`if/then/else`、`oneOf` 标为未实现，并把嵌套引用列为未完成；源码虽有 `oneOf` AST 字段，也不能把它当作受支持的 typed union 契约（[状态表](https://github.com/omissis/go-jsonschema/blob/v0.23.1/README.md#status)、[schema model](https://github.com/omissis/go-jsonschema/blob/v0.23.1/pkg/schemas/model.go)） | 最接近，但仍需在外层重做 house-subset 拒绝规则、私有命名、sidecar policy、embed 和 registration；复用后保留的第三方内部面大于删掉的结构 walker，不选 |
| [`a-h/generate`](https://github.com/a-h/generate/tree/96c14dfdfb601f0f624e776e44ced4aa3dadf8d9) | 旧 Draft-04/07 风格实现支持 properties、required、array、`additionalProperties` map 和 `definitions` 引用（[schema model](https://github.com/a-h/generate/blob/96c14dfdfb601f0f624e776e44ced4aa3dadf8d9/jsonschema.go)、[generator](https://github.com/a-h/generate/blob/96c14dfdfb601f0f624e776e44ced4aa3dadf8d9/generator.go)） | `if/then`、`writeOnly` 不在 model 中；`oneOf` 虽被解析，但主生成路径不投影它，enum 也不生成专用 Go 类型 | 无 release/tag，当前源码头仍是 2022 年提交；能力和 dialect 均落后，不选 |
| [`quicktype`](https://github.com/glideapps/quicktype) | 官方明确支持 JSON Schema 输入和 Go 输出；parser 会处理 required、array、enum、map、`$ref`，Go renderer 会产生 struct、string enum 与 union serde（[README](https://github.com/glideapps/quicktype/blob/9013ecd7eb03f974d04faa684b15310c1c49223b/README.md#generating-code-from-json-schema)、[JSON Schema input](https://github.com/glideapps/quicktype/blob/9013ecd7eb03f974d04faa684b15310c1c49223b/packages/quicktype-core/src/input/JSONSchemaInput.ts)、[Go renderer](https://github.com/glideapps/quicktype/blob/9013ecd7eb03f974d04faa684b15310c1c49223b/packages/quicktype-core/src/language/Golang/GolangRenderer.ts)） | `oneOf` 会被投影为通用 union；`if/then` 与 `writeOnly` 不参与 Go 结构投影，也不会替代 JSON Schema validator | 很适合多语言 DTO 生成，但为几个 provider 私有结构引入 Node generator、通用类型图和 union serde，仍不能生成 lifecycle/registration，不选 |
| [`oapi-codegen` v2.7.2](https://github.com/oapi-codegen/oapi-codegen/releases/tag/v2.7.2) | 能从 OpenAPI components 生成 models，并支持 OpenAPI 的 `anyOf/allOf/oneOf` | 输入是 OpenAPI 3.0，不是独立 Draft 2020-12 JSON Schema；官方仍写明 OpenAPI 3.1 等待上游支持（[README](https://github.com/oapi-codegen/oapi-codegen/blob/v2.7.2/README.md#does-oapi-codegen-support-openapi-31)） | 为 config 人为包一层 HTTP/OpenAPI 文档会制造第二种契约，完全不适用 |
| [CUE v0.17.0 `cue exp gengotypes`](https://github.com/cue-lang/cue/blob/v0.17.0/cmd/cue/cmd/exp.go) | 可先用 `cue import jsonschema` 把 JSON Schema 转为 CUE，再从 exported CUE definitions 生成 Go 类型 | 两个映射都不是“JSON Schema 直接等价到 Go”：`gengotypes` 明确为 experimental，并保证生成类型可能更宽，例如 disjunction 生成 `any`；JSON Schema → CUE 也只保证接受全部原合法值，可能更宽（[`Extract`](https://github.com/cue-lang/cue/blob/v0.17.0/encoding/jsonschema/jsonschema.go)） | 只有把 CUE 提升为项目真源时才合理；当前会增加 JSON Schema → CUE → Go 两个有损接缝，不选 |

因此唯一推荐仍是：**仓库内实现 `provider-config-gen`，但明确它不是完整 JSON Schema
code generator。它只编译 moonbase structural house subset；Santhosh v6 执行未经删减的完整
`config` schema。** 条件、范围、pattern、`uniqueItems`、`if/then` 等约束不需要也不应该都
映射成 Go 类型：`int` 无法表达 `1..65535`，struct 也无法表达条件 required。Go 类型只负责让
driver 不再访问 `map[string]any`，validator 才负责证明数据满足完整契约。

### 输入、编译阶段与输出

provider 作者只手写：

```json
{
  "$schema": "https://moonbase.dev/schema/provider-manifest-v1",
  "key": "smtp",
  "presentation": {
    "name": "SMTP 邮件",
    "description": "通过 SMTP 服务器发送邮件"
  },
  "config": {
    "$schema": "https://json-schema.org/draft/2020-12/schema",
    "type": "object",
    "additionalProperties": false,
    "properties": {
      "host": { "type": "string", "minLength": 1 },
      "port": { "type": "integer", "minimum": 1, "maximum": 65535 },
      "authMode": { "type": "string", "enum": ["none", "password"] },
      "password": { "type": "string", "writeOnly": true }
    },
    "required": ["host", "port", "authMode"]
  },
  "lifecycle": {
    "createOnly": []
  },
  "ui": {
    "order": ["host", "port", "authMode", "password"]
  }
}
```

构建步骤固定为：

```text
发现 **/provider.json
  → 用 provider-manifest-v1 meta-schema 校验 manifest/sidecar
  → 用 Santhosh compile 原样 config（Draft 2020-12、全部约束）
  → 解析 structural AST，并拒绝 house subset 外或有损的结构
  → 校验 lifecycle/ui JSON Pointer 都指向存在的字段
  → 确定性生成 *_gen.go
  → go/format
  → 原子替换输出，并清除只有 generated marker、但已无 manifest 的孤儿产物
```

structural AST 不复刻完整 JSON Schema，只需要 `Object`、`Property`、`Primitive`、`Array`、
`Map`、`Enum`、`Ref` 和 required set；原始 `config` 保持为 JSON bytes，不能由 AST 再序列化，
避免丢掉 generator 不理解但 Santhosh/Ajv 理解的合法性关键词。Go emitter 使用标准库
[`go/ast`](https://pkg.go.dev/go/ast)、`go/token` 与
[`go/format`](https://pkg.go.dev/go/format)，不拼接未经转义的 Go 源码。

每个 provider 生成一个文件即可，内容包括：

```go
// Code generated by provider-config-gen. DO NOT EDIT.

//go:embed provider.json
var providerManifestJSON []byte

type authMode string

const (
    authModeNone     authMode = "none"
    authModePassword authMode = "password"
)

type config struct {
    Host     string   `json:"host"`
    Port     int      `json:"port"`
    AuthMode authMode `json:"authMode"`
    Password string   `json:"password"`
}

var fieldPolicies = [...]configcore.FieldPolicy{
    {Pointer: "/password", Secret: true},
}

func newRegistration(send func(context.Context, config, email.Message) error) email.Registration
```

`newRegistration` 把 embedded manifest、compiled schema、只读 `FieldPolicy`、严格 typed decoder
与 send adapter 一次性传给字段不导出的 `email.Registration`。driver 手写的 `New` 只注入
`http.Client` 等运行依赖并调用它；外部包拿不到 `config`、schema 或 driver 的可交换零件。

### structural house subset 的硬边界

- **字段命名**：JSON key 确定性转为 exported Go field（否则 `encoding/json` 无法赋值）；类型名、
  enum 类型和常量保持包内私有。转换后发生碰撞（如 `foo-bar` / `foo_bar`）直接失败，不自动加
  随机后缀。原 JSON key 永远保留在 struct tag 与 policy JSON Pointer 中。
- **required / optional**：required 且非 null 的 scalar/object 使用值类型；optional scalar/object
  使用指针，optional array/map 用其 nil 与非 nil 形态区分 absent 和显式空集合。严格 decode 只在
  Santhosh 验证后执行，所以 `null` 不会偷偷进入非 nullable 字段。
- **nullable**：provider config house profile 不接受 `type: [..., "null"]`。当前 wire 已分别表达
  optional absence、secret clear 与普通零值，`null` 没有额外领域语义；与其用 `*T` 把 absent/null
  静默合并，不如在生成时拒绝。未来若出现真实三态需求，先为该三态设计显式领域字段，再扩 dialect。
- **nested object**：按 JSON Pointer 路径确定性生成私有 named struct；所有 object 必须显式
  `additionalProperties: false`。
- **map**：只接受“没有固定 `properties`，且 `additionalProperties` 是受支持 schema”的纯 map，
  生成 `map[string]T`。`additionalProperties: true`、固定字段与动态字段混合的 object 均拒绝，避免
  退化成 `map[string]any` 或自定义 marshal 层。
- **enum**：支持单一 primitive 类型的非空 enum；string enum 生成私有 named type + constants。
  混合类型 enum 拒绝。enum 的值域仍由 Santhosh 验证，Go 常量只是 driver ergonomics。
- **`$ref`**：只接受同一 `provider.json` 内的 `#/$defs/<name>`；外部 ref、任意深层 pointer 与
  recursive cycle 均拒绝。provider 的配置结构因此自包含，删除或复制 provider 包不会暗带外部契约。
- **`if/then/else`**：允许作为 validation-only 关键词，但条件分支不能新增 property 或改变字段
  structural type；所有可能字段必须预先声明在所属 object 的 `properties` 中。
- **`oneOf` / `anyOf` / `allOf`**：仅当各分支投影出的 structural signature 完全相同时，才作为
  validation-only 约束保留；会形成不同 Go 结构的 union 直接失败。provider config 不引入通用 tagged
  union；若确有不同协议形状，拆成明确字段或独立 provider。
- **`writeOnly`**：只允许 string leaf；生成 `FieldPolicy.Secret`，但不改变 Go 基础类型。read
  projection 与 KEEP/SET/CLEAR 仍由 lifecycle engine 执行，Santhosh 只负责最终值是否合法。

这些限制不是 JSON Schema 支持不足，而是刻意保证“结构投影无损”。house subset 外的合法
JSON Schema 继续可以被 Santhosh 理解，但 provider config generator 会在构建期明确拒绝，绝不
静默生成 `any`。

### moon / mise 接线与 freshness

工具放在 `packages/integrations/cmd/provider-config-gen`，随 integrations Go module 编译；命令用
`go run ./cmd/provider-config-gen`，Go 版本已经由 mise lock 固定，不再把自家 generator 作为另一份
外部 CLI 版本写进 `.mise.toml`。`packages/integrations/moon.yml` 增加唯一的 `generate` task：输入为
所有 `provider.json`、generator/core contract 源码、`go.mod`/`go.sum`，输出为 provider-local
`*_gen.go`。`integrations:{test,check,fix}` 和会编译它的
`server:{dev,build,release,test,check,fix}` 都依赖 `integrations:generate`。

生成文件遵循本仓库现有策略放进 `packages/integrations/.gitignore`，因此 freshness 不靠开发者提交
diff：moon 的输入哈希变化会先重跑 generator，新克隆也会先生成；generator 的 orphan cleanup 防止
删除 manifest 后旧类型继续参与编译。若将来改为提交生成文件，再增加 `provider-config-gen --check`
进行临时目录生成与 byte diff；当前忽略制品模式下没有必要同时维护第二套 freshness 机制。

### 是否复用 omissis

**唯一推荐是不复用。** omissis 的 `pkg/generator` 确实可作为库调用，但 moonbase 仍必须在它前面
实现 structural house-subset verifier，在后面实现私有命名/AST 改写、manifest embed、FieldPolicy、
registration、orphan cleanup 和实际 descriptor 投影；而它额外带来的 validation code、通用 ref/
composition/format 行为又全部不能成为本项目契约。最终只替掉最简单的 object walker 与 struct
emitter，净收益不足。

不对自建工具承诺虚构的代码行数；用可验收职责界定规模：一个 manifest loader/meta validator、
一个 structural compiler、一个 `go/ast` emitter、一个 artifact reconciler，以及覆盖上面每条
accept/reject 边界的 table tests 和 golden tests。它不实现 validator、不解析网络 ref、不生成
union serde、不执行 lifecycle，范围因此是封闭且可审计的。

## 九、可复现 spike

spike 位于 `/tmp/moonbase-jsonschema-research/spike`，没有修改产品代码。测试版本：Google v0.4.3、Kaptinlin v0.9.3、Santhosh v6.0.2、Ajv 8.20.0、rjsf validator 6.6.2。

执行命令：

```bash
cd /tmp/moonbase-jsonschema-research/spike
go run ./google
go run ./kaptinlin
go run ./santhosh
node ./ajv.mjs
```

关键结果：

| 用例 | Google | Kaptinlin | Santhosh |
| --- | --- | --- | --- |
| `float64(443)` 验证为 integer | 通过 | 通过 | 通过 |
| 验证后解码到 `int` | 标准库得到 443 | `Unmarshal` 得到 443 | 标准库得到 443 |
| `443.5` integer 校验 | 拒绝 | 拒绝 | 拒绝，path `/port` |
| `443.5` 直接解 `int` | 标准库报错 | **nil，静默得到 443** | 不提供危险的混合 API |
| unknown field | 拒绝 | 补 `additionalProperties:false` 后拒绝 | 拒绝 |
| `if/then` required | 拒绝缺失字段 | 拒绝缺失字段 | 拒绝，keyword `/then/required` |
| `uniqueItems` | 拒绝重复 | 拒绝重复 | 拒绝，path `/tags` |
| custom extension round-trip | `Extra` 保留 | `PreserveExtra` 后保留 | producer 保留；validator 无需拥有它 |
| 当前 rjsf 默认 validator | Draft 2020-12 编译失败 | Draft 2020-12 编译失败 | 同一 schema 同样失败 |
| rjsf + `Ajv2020` | 编译通过 | 编译通过 | 编译通过 |

三组程序还以 32 goroutine × 100 次完成共享 validator 压力运行。环境没有 C compiler，无法执行 Go race detector；因此并发结论同时依赖源码审计。Google 在 Resolve 阶段预编译、Santhosh 每次创建局部 validator；Kaptinlin 的共享 schema 写操作可从上述源码直接确认。

## 十、最低迁移路径的实现边界（非 Greenfield 推荐）

以下是后续 ADR/实现应固化的约束，不是本次已实施代码：

1. `config.Definition[T]` 在 provider registration 时校验 `T`：只能使用支持的 JSON 形状，JSON key 与 `config.Field.Key` 必须 1:1，字段 Go 类型与 `form.Type` 必须匹配。
2. producer 输出显式 `$schema` 和 `additionalProperties:false`；JSON Schema 与发给 rjsf 的 schema 必须来自同一个对象/深拷贝。
3. registration 时 compile 一次并缓存；运行时绝不重新编译 schema。
4. 更新顺序固定为 `Merge → Validate → Decode[T]`。Create 没有旧值可合并，required secret 会正常失败。
5. `Decode[T]` 使用 `json.Marshal(map)` + `json.Decoder`/`json.Unmarshal`，不能用反射强转或允许 float 截断的 coercion。
6. `secret`、`immutable`、中文 label/help、option descriptions、`showWhen`、UI Schema、Mask/Merge/Usable 留在 moonbase 领域层；不塞进 provider Driver interface，也不指望 validator 执行。
7. Web 切换到 `Ajv2020` 后加契约测试：后端生成的每个 provider schema 都必须能在实际 rjsf validator 中 compile。

相对当前自研代码，预计可以删除/收缩：

- `form.Validate` 中手写的 unknown/type/range/pattern/enum/unique 通用校验；
- `number`、`duplicates`、`stringSlice` 等为标准校验服务的辅助逻辑；
- 服务端规则与 JSON Schema 输出行为的重复测试。

需要新增：

- `Definition[T]` 的 struct key/type 漂移检查；
- JSON Schema compile/cache 与 structured error 适配；
- 严格 `Decode[T]`；
- rjsf `Ajv2020` 接线和 Go/AJV 契约 fixtures。

不会消失、也不应该消失的是 moonbase 的领域层。它正是通用库无法“完美实现需求”的部分。

## 最终判断

> **Greenfield 唯一推荐：provider-local `provider.json` manifest 为唯一真源，其中 `config` 是纯标准 Draft 2020-12 JSON Schema，`lifecycle` / `ui` 是并列 sidecar；仓库内小型 structural codegen 生成私有 Go config、原子 Registration 与只读 FieldPolicy；Santhosh v6 和 rjsf/Ajv2020 验证同一份 config schema；通用 lifecycle engine 执行 secret/create-only 的状态转换。**

Google/Kaptinlin/Invopop 的 code-first 路线无法避免 struct 与复杂 metadata/hooks 的两种表达；CUE 的 JSON Schema 与 Go 导出当前仍是 experimental 且可能放宽约束；通用 Mask/Merge 库也不能执行旧值相关语义。允许彻底重构时，schema-first 是唯一同时满足跨端单一真源、provider 封装、私有强类型与构建期漂移发现的路线。
