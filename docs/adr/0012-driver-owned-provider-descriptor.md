# Driver 拥有完整 provider descriptor

> **状态**：accepted。

新增 provider 必须做到零前端、零 proto、零 base 特例：driver 除 config schema 与 Ops 外，还拥有 provider key、中文名称、说明、品牌色和可选 namespaced icon ref（如 `antd:WechatOutlined`）。图标由 Web 的通用 resolver 懒加载并提供缺省回退；前端不得维护 provider→名称、说明、图标或品牌色映射，也不再按 provider 拼 profile 摘要和 badge，profile 列表只显示 profile name 与通用 provider presentation。

Wire 上使用有序 `repeated ProviderDescriptor`，每项组合 key、presentation 与独立的 config form；不再使用 `map<string, ProviderForm>`，也不把 provider 品牌信息塞进只描述配置值的 JSON Schema。Payment 通过 `PaymentProviderDescriptor { provider, methods, capabilities }` 组合扩展通用 descriptor，其他 integration 不感知支付概念。

Go 侧 registry 是唯一组合根：每个有序 entry 同时拥有 provider key、presentation、config schema 与 Ops；descriptor、provider key 列表、Mask/Merge/Validate/Usable 与 Ops 派发都从 registry 派生。删除独立 `Schemas()`、每-driver `Usable` 函数和其他平行 provider 目录；重复 key、空 presentation、非法 icon ref 或缺失 schema/Ops 在 registry 构造或契约测试时失败。

Purpose 不属于 driver：具体 purpose key、展示文案和 `single/multiple` 绑定基数由 base/application catalog 拥有，并通过 `PurposeDescriptor` 下发。各 integration describe RPC 同时返回 purposes 与 providers；catalog 同时作为绑定写侧校验和管理端展示真源。新增整类 integration 仍是显式核心动作；本决策只保证高频 provider/driver 是 drop-in 扩展单元。

完整 descriptor 属管理控制面，仅经 `system.read` 保护的 describe RPC 返回。公开或付款运行时读面只从同一 descriptor 通用投影当前已绑定、当前上下文需要的最小安全字段：OAuth 登录页不获得 config form 或未绑定 provider，hosted checkout 不获得其他 purpose/profile 或完整 capability 目录。投影是安全裁剪，不是第二份 provider 元数据。

## 考虑过的替代

- **前端 provider 映射表**：实现直接，但每个 provider 都要求 Web 改动，违背 drop-in 边界。
- **把 presentation 塞进 JSON Schema**：混淆 provider 身份与 config value 形状，选择器、列表和非表单消费者无法自然复用。
- **driver 携带 SVG 或 React 组件**：前者增加资源交付与安全处理，后者反向绑定前端 runtime；namespaced icon ref 用受控体积换取更小的扩展 interface。

## 后果

- 所有 `Describe*Providers` RPC 改为有序 descriptor 列表；registry 声明顺序即管理端展示顺序。
- Provider presentation 的一致性、icon ref 格式和 descriptor/config form 完整性由契约测试守卫。
- 新 provider 只修改其 driver 与 registry；Web 仅在新增通用图标来源、字段类型或交互能力时演进。
