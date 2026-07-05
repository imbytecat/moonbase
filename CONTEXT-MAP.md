# 上下文地图

本仓库为多上下文布局。`proto/` 是跨端领域词汇的单一真源；各上下文的 `CONTEXT.md` 引用而非重新发明这些术语。

## 上下文

- [server](./apps/server/CONTEXT.md) — Go 后端领域：认证 / RBAC、系统与业务设置、各基础设施通道（存储 / 验证码 / 邮件 / 短信 / 消息推送 / AI / 第三方登录 / 支付）、审计、工作流、报表。

## 关系

- **proto/ → server / web**：Protobuf + ConnectRPC 契约一步生成两端类型安全代码；契约错配是编译错误。跨端领域概念（`Permission`、channel / profile / binding / purpose、payment order 等）在此定义。
- **系统级决策**见根 [`docs/adr/`](./docs/adr/)（跨端，如触及 wire 契约的支付币种模型）；后端专属决策见 `apps/server/docs/adr/`。
