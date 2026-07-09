# integration 抽成独立模块：超越模板 cherry-pick 约束、转向可依赖核心（保留 ADR-0002 的 legibility 内核）

> **状态**：被 ADR-0006 修订。integration 模块化方向保留；`systemcodec`、typed profile、以及“schema 驱动运行时表单缓做”的非目标已被 schema-driven provider 配置取代。

## 背景

moonbase 起于「模板」：复制一份、快速开干。但模板有结构性的病——每复制一份就分叉一份，base 的 bug 修复 / 安全补丁 / 新增共享能力**传不回副本**，副本越多、漂移越远（碎片化）。目标随之升级：把平时都要用的横向能力做成**一处、版本化、下游 depend 而非 copy** 的核心，业务方只写业务。

灵感来自彩虹易支付那类「支付插件市场」。但调研厘清两件事：(1)「丢文件即插件」是 **PHP 解释型**语言的性质，编译型 Go 拿不到；那类市场真正的支柱是**中心化授权 / 计费平台**（授权 UID + 余额 + 充值），不是某个加载技术。(2) 真正的运行时插件（hashicorp/go-plugin）是子进程 + gRPC，**与 ADR-0002 决策 4「provider 派发保持字面可 grep 的提交源、绝不反射 / 运行时描述符」正面冲突**，并牺牲端到端类型安全。

ADR-0002 把「模板 cherry-pick、diff 可移植」列为既定约束——而这**恰是碎片化的根源**。于是真正的抉择不是「加不加插件系统」，而是「要不要开始离开 ADR-0002 的模板模型、转向依赖模型」。团队选择：开始转向。

## 决定

**开始从「模板 cherry-pick」转向「可版本化的核心 + 独立 integration 模块」，go.work 多模块为第一步。只超越 ADR-0002 的可移植性约束，保留其 legibility 内核。**

保留 vs 超越，划清：

- **保留（ADR-0002 仍在 force）**：provider 派发保持**编译期、字面可 grep、零反射**（决策 4）。故 **go-plugin / 运行时插件 / proto 切片，继续缓做**，不在本 ADR 范围；schema 驱动表单已由 ADR-0006 接手。
- **超越**：「单模块 + cherry-pick、diff 可移植」这一条。integration 从 base 的 `internal/` 包，抽成各自的 Go module，由 go.work 组织；下游未来可作**版本化依赖**消费，而非复制。

边界规则（承接 ADR-0003：工件归域不归 integration）：

- **integration module = 无状态 driver + 配置形状**。driver 藏在 seam（`ObjectStore` / `Sender` / `Verifier` / `Chatter` / `Flow` / `Gateway`）之后，**不碰 DB**。
- **domain 表留在 base**：`files` / `file_attachments`（文件域）、`payment_orders`（支付域）按 ADR-0003 归域，不进 integration 模块。配置仍是共享 `settings` 表的一行 JSONB，**integration 模块零迁移、零建表**。
- **共享地基另立一个模块**（暂名 `integrationkit`）：`integration` 原语（Catalog / Provider / Driver / Registry）+ 配置载体形状（`Integration[GenericProfile]` / `GenericProfile`）+ schema 处理。DAG：`server → integrations/<name> → integrationkit`，无环。base 依赖各 integration 模块并**显式编译期注册**（派发仍可 grep，legibility 不丢）。

第一步只抽 **5 个无状态 integration**（email / sms / captcha / llm / oauth）。storage / payment 因 driver 与域代码现仍同居，列为 phase 2：先验证模式，再拆出无状态 driver、把域留 base。

## 术语：channel → integration

同批把总称 **channel/通道 更名为 integration/集成**。理由（交叉验证 Laravel / Terraform / K8s / Auth.js / RabbitMQ / AWS CLI / Go database/sql）：channel 只在**通知 / 支付**语境地道，做 storage/captcha/llm 的通用总称不标准，且在 Go 里撞一等概念 `chan`。其余词经验证为最佳实践、不动：**driver**（Go `Register(name, driver)` 铁律）、**provider**（厂商身份 / 选择键）、**profile**（AWS named profile）、**binding**（RabbitMQ / K8s RoleBinding）。

历史 ADR-0002 / 0003 保留原文，其中 `channel` / `通道` **等同** integration；代码里的 `channel` 随本次抽取重构**一并**改名（`internal/channel` → `integrationkit` 的 `integration` 包、`internal/<name>` → `integrations/<name>`、`Channel[P]` → `Integration[P]`），不单独折腾。glossary 权威在 `apps/server/CONTEXT.md`。

## 非目标（明确的「不」）

- **go-plugin / 运行时加载 / 独立进程插件**——被 ADR-0002 决策 4 精确排除。触发条件：出现「肯付费、且死活不肯重编译宿主」的真实客户。
- **schema 驱动运行时表单 / 前端通用渲染器**——已由 ADR-0006 重新评估并采纳，用于 provider config，不等同于运行时加载 integration。
- **per-integration proto 切片 / 独立 buf 生成**——proto 保持中央单一真源。触发条件：某 integration 真要独立发版 / 出售。
- **把 integration 抽成独立仓库**——现阶段留 monorepo，保住「完整布线范例」（决策 6）与本地 go.work 联调；独立仓库待真实分发需求。

## 考虑过的替代

- **维持 ADR-0002 模板模型不动、只去建应用**：n=0 时本是最稳选项；收回，因为团队已决意主动承担平台化的 API 契约负担以根治碎片化，且 integration 是最干净的起点。
- **现在就上 go-plugin**：否决——违反决策 4；n=0 无「不肯重编译的付费客户」信号，子进程 / IPC / 按平台编译分发 / 中心授权全是未挣来的成本。
- **单模块内做包整理即可**：integration 本已是 seam 后独立包（决策 6），纯包整理近乎 no-op；给不了「可版本化依赖」这一去碎片化的关键杠杆，故进一步到 module。

## 触发重评（什么信号让我们再往前一格）

- ≥2–3 个真实业务应用消费此核心并暴露真正复现的缝 → 扩核心边界至下一个最痛子系统。
- 出现不肯重编译的付费客户 → 重评 go-plugin。
- 真要运行时可装 integration → 重评 schema 驱动前端。
