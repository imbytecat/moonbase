# integrations 收敛为单一 `packages/integrations` 模块（修订 ADR-0006 布局）

> **状态**：accepted。**修订 ADR-0006「布局与模块路径」一节**（每-integration 独立 module → 单一 integrations module）；**保留 ADR-0006 内核**（provider 为 drop-in 单元、schema 驱动的单点掩码引擎、Terraform 形状的 seam）。

## 背景

ADR-0006 把每个 integration 定为独立 Go module（`github.com/imbytecat/moonbase/integrations/<name>`）。落地时既误带了 `packages/` 前缀（违背 ADR-0006 第 37 行），又暴露出 8 模块布局的真实成本：`go.work` + 每模块一份 `go.mod` + `apps/server/go.mod` 漏 `require` 导致 `GOWORK=off` 构建直接失败（仅靠 workspace 才编得过，`go mod tidy` 从未跑通）。

而收益并未兑现：独立版本 / 独立分发 n=0，是未使用的期权。ADR-0006 自证——integration「近乎齐全、**很少**新增」，provider「**经常**加」。可 8 模块把边界画在了**罕见轴（integration）**上：每个罕见新增的 integration 独享一个重量级 module，而频繁新增的 provider 根本不需要 module。边界画反了频率轴，为极少发生的事天天付仪式成本。

## 决定

integrations 收敛为**单一模块** `github.com/imbytecat/moonbase/integrations`（磁盘 `packages/integrations/`，路径丢 `packages/`，对齐既有约定 `apps/server → .../moonbase/server`）。

- 内部按 integration 分**子包**：`core/schema`（引擎）、`sms/`、`email/`、`llm/`、`oauth/`、`captcha/`、`storage/`、`payment/`。
- 每家用**统一模板形状**：一个 seam 接口（`Sender`/`Flow`/`Gateway`/`Verifier`/`Chatter`/`ObjectStore`）+ `drivers` 注册表 + `Schemas()` + 每 provider 一个 driver 文件。**加 provider = 复制一个 driver 文件**；加 integration（罕见）= 复制一个子包骨架。
- 编译期依赖方向仍锁死：integrations 与 server 分属两模块，back-edge（integration import server）不可能——这是当初 `systemcodec` 成环病的根治，单模块照样拿得到。
- `apps/server` 通过 `require` + `go.work` 引用；跑 `go mod tidy` 让 go.mod 自洽（`GOWORK=off` 亦可构建）。

## 非目标

- **每-integration 独立版本 / 独立分发**——YAGNI，n=0。真出现「要独立分发 / 售卖某 integration」的客户再拆。
- **go-plugin / 运行时加载**——仍缓做（承 ADR-0006）。seam 已是 Terraform 形状，将来换传输 base 引擎不改。

## 考虑过的替代

- **M2：塞回 `apps/server` 内做分包**——方向只能靠 `internal/` 约束，能绕；且把 integration 重新耦合进可部署物。收回。
- **M3：维持 8 模块**——隔离最强、moon CI 缓存最细，但为罕见轴买单，且背 `go.work` + 漏 `require` 的地雷。收回。

## 触发重评

- 出现「肯付费、要独立分发 / 单独版本化某 integration」的客户 → 重评拆回多模块。
