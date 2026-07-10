# Provider contract 以 Go struct 为真源

> **状态**：accepted。修订 ADR-0006 的 provider config producer/运行时处理形状，并细化 ADR-0012 的 driver-owned descriptor 与原子 registration；保留两者关于 driver 拥有配置契约、base 统一守密钥、provider 为 drop-in 单元的内核。

允许不兼容重构后，每个 provider 以私有 Go config struct 为配置契约的作者真源。字段类型、JSON 名称、required、长度、范围、枚举等约束随该 struct 声明；`invopop/jsonschema` 从它生成标准 Draft 2020-12 JSON Schema。Moonbase 不手写 provider JSON Schema，不做 JSON Schema → Go codegen，也不维护自有 schema vocabulary 或 structural generator。

字段的结构约束以及 `title`、`description` 等标准契约信息写在 struct 的 `json` / `jsonschema` tags 中。带人读名称或说明的枚举使用标准 `oneOf` 分支表达，每项由 `const`、`title` 与可选 `description` 组成；provider 可以通过私有枚举类型的 `JSONSchema()` 实现发布它，不维护前端选项目录。`invopop/jsonschema.AddGoComments` 不采用，因为 schema 在生产运行时生成，而 distroless 单二进制旁没有 Go 源码可供解析。

当前版本不提供 provider-local UI sidecar 或 UI 扩展系统。rjsf 直接消费标准 JSON Schema：字段顺序沿 Go struct 声明，普通类型使用默认 widget；secret widget 与 create-only 禁用由 core 从 lifecycle policy 生成最小 `uiSchema`。不支持 provider 自定义 placeholder、widget、`ui:order` 或 ShowWhen；条件规则用标准 schema 表达。只有出现标准 rjsf 无法处理的真实需求后，才针对该需求增加共享能力。

跨字段静态约束仍属于同一份标准 JSON Schema。少数需要条件规则的 provider 通过 `invopop/jsonschema` 原生 `JSONSchemaExtend` 补充 `if/then`、`dependentRequired`、`oneOf` 等标准关键字，使 Santhosh 与 Ajv2020 执行同一规则；不得另建只在 Go 端运行的 provider `Validate()` 双轨。远端凭据有效性、网络连通性等无法由配置值本身判断的行为只属于 Test/运行时协议调用，不混入静态 schema 校验。

持久配置不采用隐式默认值。JSON Schema 的 `default` 只作为标准 annotation，为新建表单提供初始建议；validator、core 与 driver 都不得把它解释为缺失字段的服务端 fallback。任何影响运行行为的字段都必须 required 并将最终值显式写入 JSONB；真正允许缺失的字段才 optional，若 provider 需要区分缺失与 `""` / `0` / `false`，其私有 Go config 使用指针表达。这样无需 defaults 库或自建递归 default applicator，stored config 本身就是完整事实。

provider config 采用 closed-world 对象：由 Go struct 生成的每层对象保持 `additionalProperties: false`，严格解码同时启用 `DisallowUnknownFields`。动态键只有在领域确实需要时才以 `map[string]T` 显式开放，并由 `additionalProperties` 约束值类型；`map[string]any` 默认禁止进入 provider config。由于本次不承担向后兼容，字段拼写错误、已删除字段和未建模输入应立即失败，而不是进入 JSONB 静默残留。

生成的同一份标准 schema 由服务端 `santhosh-tekuri/jsonschema/v6` 与 Web `Ajv2020` 执行；服务端通过 `encoding/json` 严格解码为 provider 私有 config。provider driver 只接触强类型 config，不读取 `map[string]any`。可复用 integration 包只定义 provider 执行模型、registry 与 registration；应用侧 `apps/server/internal/<integration>` facade 独占 purpose catalog、binding 解析、settings loader 与按 purpose 寻址的业务 seam。presentation 与配置契约并列组成完整 provider descriptor。

schema 不产生持久化生成物。每个 provider registration 在 registry 构造时反射 config 类型、应用标准 schema 扩展与 lifecycle 投影、编译 Santhosh validator，并缓存不可变的 JSON Schema、core 生成的最小 UI Schema 与 typed decode/invoke closure；Describe、Validate 与运行时派发只读取该缓存。构造失败由显式组合根的 `MustRegistry` 立即 panic，并在构建 registry 的契约测试中暴露。仓库不新增 schema generate 命令、生成 `.go`/`.json` 文件或 freshness 检查。

共享抽象止于配置控制面：`core/config.Contract[T]` 统一 schema 生成/编译、严格 decode、canonical encode、lifecycle policy 与 UI 投影。每类 integration 自己定义 consumer-owned 的 typed registration 与操作 seam，例如 email 注册 typed `Send`，captcha 注册 typed `Verify`，OAuth 注册流程操作，payment 保留其必需/可选能力接口。泛型 config 类型只在 provider 构造 registration 时可见，类型擦除封闭在对应 registry 内；不得建立跨 integration 的 `Execute(context.Context, any, any) (any, error)` 或通用 channel driver interface。

Contract/registration 不保存 Profile config。provider 构造器只注入稳定依赖，应用 facade 每次解析当前 Profile 后把 canonical config 交给 registry 验证、解码并调用 typed Ops；这样一个 registration 可服务多个 Profile，配置更新无需重启。core 不建立 SDK client/凭据缓存，provider 只有在测得构造成本后才可自行实现有界缓存。

registry/contract 定义错误与存量数据错误分开处理。无效 schema、policy path、registration 或重复 provider 属开发者错误，在 registry 构造时 panic；JSONB 不符合当前 schema、严格解码失败或 stored provider 已不在 registry 属数据错误，不阻断服务启动。管理 wire 只增加 `bool config_valid`：已知 provider 只投影当前 schema 允许的普通字段和当前已知 secret presence，所有未知字段丢弃；未知 provider 因没有可信 policy 判断 secret，完全不返回 config。前端通过 provider 是否仍存在于 descriptor 列表，自然区分“配置无效”和“provider 已移除”，不新增状态 enum 或错误详情协议。无效 profile 可编辑修复或删除，未知 provider profile 只允许删除；运行面在 `config_valid=false` 时不调用 driver。

写入边界按固定顺序处理：先应用 secret/create-only lifecycle 得到候选完整配置，再以 Santhosh 校验生成的 schema，然后用 `encoding/json` 严格解码为 provider 私有 config，最后重新编码该 typed config 并写入 JSONB。数据库保存 canonical config，而不保存客户端原始 JSON；因此 stored config、driver 实际读取值与 schema 可表达值保持一致，未知/多余表示不会长期沉积。optional 零值若有意义必须用指针表达，不得让 `omitempty` 吞掉显式的 `""`、`0` 或 `false`。

JSON Schema 只负责单个配置值的结构与合法性。需要旧值或读写方向才能判断的 secret、create-only 和安全读投影属于 Moonbase 的 config lifecycle policy，以 provider-local Go sidecar 声明，由 `core/config` 的一个通用 lifecycle module 执行。policy 使用显式 RFC 6901 JSON Pointer 列表定位字段，例如 `Secrets: Paths("/password")`、`CreateOnly: Paths("/key")`；不使用自定义 Go field tag，也不作为自定义 JSON Schema keyword 发布。core 可以从 policy 派生标准 `writeOnly` annotation 和 rjsf UI Schema，但安全行为以 policy 为准。

registration 构造时必须把每条 policy path 对照生成后的 schema 校验：path 必须规范、存在并落到受支持的叶子；secret 只能指向字符串叶子；重复 path、父子冲突、类型不符和指向数组动态元素等无法稳定寻址的形式均立即失败。这样 JSON Pointer 的字符串拼写风险在启动和契约测试时暴露，不进入运行期静默分支。

profile 写入中的普通非 secret 配置使用**完整替换**，不采用 RFC 7396 Merge Patch：`values` 表示全部普通可变字段的最终状态，optional 字段缺失即删除，`""` / `false` / `0` / 空数组均是显式值。secret 不得出现在 `values`；create-only 字段可以省略或回传原值，提交不同值则拒绝。这样 lifecycle interface 不引入 `null` 删除、对象递归合并和数组替换等额外 patch 方言。

secret 写入只支持 policy 声明的非空字符串叶子，并以 RFC 6901 JSON Pointer 定位。`ConfigWrite.secrets` 是 `map<string, string>`：update 中 path 缺席即保留旧值，path 存在即以非空值设置或替换；不支持 clear，也不支持空 secret。create 缺失 required secret、空值、未知或非 secret path、以及 secret 混入普通 `values` 均拒绝。optional secret 一旦设置不提供单字段清除，罕见的清除需求通过删除并重建 Profile 解决；core 和 Web 不为此维护 SecretMutation oneof 或 KEEP/SET/CLEAR 状态机。

读侧使用独立 `ConfigView { Struct values; repeated string set_secret_paths; }`。secret 字段完全不进入 `values`，也不以空值或掩码字符串占位；已配置状态只通过 JSON Pointer 集合表达。`ConfigView` 只用于响应，写入只接受 `ConfigWrite`，从 wire 类型上阻止把安全投影误作更新输入。

`Usable` 等于 stored config 通过同一标准 schema 验证并严格解码。每个 provider 只需手写私有 config struct、少量 lifecycle policy、presentation、typed driver 和必要时的标准 schema 扩展；schema 生成、服务端验证、严格解码、descriptor 投影、secret lifecycle 与前端通用表单均由 core 一次实现。所有用户文案直接写中文，不引入 i18n 层。

## 考虑过的替代

- **JSON Schema-first + 自建 structural codegen**：理论单一真源换来了自有 schema subset、生成器、产物新鲜度和双端兼容矩阵；这些维护成本超过了消除少量 Go tags/policy 的收益，因此放弃。
- **手写 JSON Schema + 通用 JSON Schema → Go generator**：现有生成器对复杂 schema、annotation 和 idiomatic Go 类型的支持边界不一致；生成后的类型仍需人工审阅，不能降低总体复杂度。
- **只用 Go struct validator**：无法给 Web/rjsf 提供同一份标准配置契约，会重新形成前后端双写。
- **JSON Schema 加 provider 专属 Go `Validate()`**：跨字段规则会形成前后端不一致的第二校验源；能用标准 schema 表达的规则统一通过 `JSONSchemaExtend` 发布。
- **服务端根据 schema `default` 自动补值**：JSON Schema 把 `default` 定义为 annotation 而非 mutation；自建递归 applicator 会制造额外方言。行为值改为 required 且显式持久化，`default` 只改善新建表单体验。
- **验证通过后原样保存客户端 JSON**：会让数据库保留 typed driver 永远看不到或会被转换的表示。改为严格解码后由 typed config 重编码，存储唯一 canonical 形状。
- **默认允许未知字段**：会掩盖拼写错误并让旧字段长期残留。config struct 默认封闭；只有有类型的动态 map 是显式例外。
- **把反射结果写成生成文件**：会重新引入 codegen 任务、派生产物和 freshness 闸门。schema 只在 registry 构造时生成、编译并缓存。
- **所有 integration 共用一个 Driver/Execute interface**：会把 Send、Verify、Exchange、Create/Query 等不同语义压回 `any`。只共享 `config.Contract[T]` 机制，各 integration 保持 consumer-owned typed seam。
- **无效存量 config 原样回传或让服务启动失败**：前者可能泄露已从当前 policy 删除的旧 secret，后者让单条坏数据拖垮整个系统。改为安全投影、显式状态和运行面 fail-closed。
- **`google/jsonschema-go` 或 `kaptinlin/jsonschema` 同时承担所有职责**：前者的运行时校验与错误能力不适合本场景，后者仍在 v0 快速演进且实测存在数值解码语义风险；生成与验证分别选用职责更专一的成熟库。
- **Mergo、JSON Merge Patch、Terraform/Pulumi state 能力**：前两者只能替代机械合并，后两者属于完整 IaC/configuration engine；为简单的“缺席保留、非空替换”secret 语义引入它们都没有净收益。

## 后果

- 新增 provider 的常见路径变为“一个私有 config struct + 一个 typed driver + presentation/policy + 应用组合根一行”；零 proto、零 Web provider 特例。
- SMTP、Cloudflare 等 provider 各自拥有配置类型和实现，只在 email seam 后组合；不再共享 integration 级配置 union 或散落的 `cfgStr`/`cfgInt`。
- 可复用 integration 包不再依赖 settings 或接受 purpose；应用 facade 先把 purpose 解析成已选 Profile，再调用 registry 执行。下游项目增删业务用途无需修改 `packages/integrations/*`。
- 本项目编译的 provider 集合与顺序由应用 facade 的 `registry.go` 选择，服务器启动时构造并注入同一个不可变 registry；不提供 reusable `builtin` 集合或包级全局 registry。
- proto 的 profile 写入/读取形状、Web secret widget、registry 与旧配置引擎已经完成一次性不兼容重构；当前 `Contract[T]` 是唯一 provider config lifecycle。
- core 仍需维护一个薄且表驱动的 lifecycle module；这是应用状态语义，不是重复实现 JSON Schema validator。
- 契约测试必须守卫 schema 可同时被 Santhosh/Ajv 编译、严格解码、secret 不泄露、create-only、普通零值与 provider registration 原子性。
