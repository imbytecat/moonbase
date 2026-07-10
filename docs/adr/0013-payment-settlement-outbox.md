# 支付结算通过同事务 outbox 交付业务 domain

> **状态**：accepted。

Payment order 进入 `paid` 或 `refunded` 等需通知业务的结算状态时，payment domain 在同一数据库事务中写入 `payment_settlement_events` outbox；后台 dispatcher 按 order 的 purpose 调用对应 base/application `SettlementHandler`。事件携带稳定的 order id、purpose、business reference、state、amount、currency 与发生时间，不包含 provider 密钥或原始响应。

投递语义是至少一次：handler 必须按 event id 幂等，只有成功后事件才标记已投递，失败持续重试。Payment 状态提交不等待 handler；driver 不知道业务 reference、handler 或业务表。管理端演示 purpose 可以没有 handler；若未来 DBOS 能提供同事务的可靠 workflow enqueue，可替换 dispatcher implementation，但 settlement interface 与事件语义不变。

## 考虑过的替代

- **依赖 `return_url` 或收银台轮询**：用户可能不返回或关闭页面，无法作为资金结果交付保证。
- **provider notify 后直接调用业务函数**：状态事务提交与业务调用之间存在崩溃窗口，重放和错误恢复也无 durable 记录。
- **driver 直接更新业务状态**：把业务 purpose 和表泄漏进可复用 provider 实现，破坏 driver 无状态、无 DB 的边界。
