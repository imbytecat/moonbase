# 基础设施不变量为 AI agent 优化：确定性生成 / 类型优先于可被糊弄的测试与反射

## 背景

本仓库的代码主要由 AI coding agent 读写。到 2026 年中，一手材料已把「可读性」从风格偏好升级为**可测的性能特征**，并给出稳定的工程结论：

- **大 context 未消解可读性需求**——失败模式从「检索容量」转成「导航显著性」（arxiv《Navigation Paradox》）；同一 prompt，空仓 vs 已布线仓 47min/$5.20/不可合 vs 11min/$0.85/可合，「模型没变强，是仓库更可读」（applighter, 2026-07）。
- **反射 / 运行时元编程把行为藏起来**，逼 agent「模拟 runtime 才能预测行为」，是明确的 AI 反模式（Agentic Developer Cookbook）。
- **别要求模型遵守，把规则做成结构**——生产 agent = 随机核心 + 确定性边界；71% 的失败定位在这条边界（arxiv《Stochastic-Deterministic Boundary》, 2026-05）。而测试是**可被糊弄**的 verifier：「AI 倾向于 hack 到测试通过，而非建立正确抽象」（Lattner）。
- **spec 是跨模型代际稳定的件**，实现可再生（Monperrus《The Specification Is the Program》, 2026-03）。
- **code > prose**：一项 138 仓 / 5,694 PR 研究里，手写 `AGENTS.md` 只帮 ~4%、AI 写的反而害 2–3%；「可执行脚手架传递能力，文字建议不传递」（EsoLang-Bench, 2026-06）。

这套结论约 **2 年**尺度有效，非永恒；但近 30–60 天材料未见反转。

## 决定

基础设施（channel / profile / binding / 权限 / authz / 配置）的一致性与正确性，按以下优先级保证——**越靠前越优先**：

1. **不可表达 > 可被糊弄。** 能做成**确定性、agent 无法蒙混**的约束（生成的表 / 类型 / 位置构造器 / drift-gate）时，优先于**可被 agent hack 到绿**的护栏测试或散文清单。测试**保留**，但降级为**行为兜底**，不作为结构真源。
2. **proto = 单一 spec。** 跨端契约与 channel 形状的唯一结构化真源；派生物生成，不手抄。
3. **无决策的机械映射 → build-time 生成。** 密钥掩码 / 编解码这类高频、易错、无判断的映射由 `protoc-gen-settings` 生成（见 `cmd/protoc-gen-settings`）。这类生成物 git-ignore 可接受——agent 几乎不需读它。
4. **反射不用于策略 / 目录表。** authz、权限目录、provider 派发保持**字面可 grep 的提交源**（如 `internal/server/authz.go` 的决策表），绝不改成启动时反射描述符建表。
5. **跨语言镜像用 drift-gate。** 前端对后端目录的镜像（`apps/web/src/lib/payments.ts`、`permissions.ts`）必须有一个把漂移变成**构建失败**的确定性闸（锚 + verify，或直接从 proto 生成），而非无兜底、也非可糊弄的测试。
6. **每个横切关注点保留一个「完整布线的范例」** 作 agent 的主教材（本仓库每个 channel 即是）；`AGENTS.md` 散文是**薄导航**，非主信号。

## 现状符合度（审计锚点）

**已符合**：统一形状 `Channel[P]`（`settings/settings.go`）+ `channelOps[P]`（`rpc/system_channel.go`）+ `Registry[P,Ops]` + `Catalog`（`channel/channel.go`）;机械映射已生成（`protoc-gen-settings`）;策略表零反射、字面可读（`authz.go`）;7 个 channel 在 `proto/system/v1/system.proto` 里形状高度一致。**本仓库本就大体贴合上述规范。**

**可被糊弄的缝（决策 1 的整改对象）**：`config.go` 的平行 `SetDefault` ↔ `TestLoadEnvOverrides`;`auth.Catalog` ↔ `Permission` 枚举;provider/method 的 `in:` ↔ Go 注册表 / `pay.Methods()`;`authz` 表 ↔ 覆盖测试——皆靠**运行时测试**对齐，可被 agent 糊弄。

**真正的洞（决策 5）**：`payments.ts` / `permissions.ts` 镜像后端目录，**无编译也无测试兜底**——后端加一个 method，前端静默烂掉，无处报错。

## 考虑过的替代

- **All-in codegen（把 authz / 目录 / 完备性全部生成，包括会「故意打断调用点」的位置构造器）。** 收回：那是为**编译器纯洁度**优化，不是为 **agent**。生成物 git-ignore 后冷启动 agent 看不见;且位置构造器的 `not enough arguments` 是**晦涩**错误，agent 可能塞 `nil` 糊弄过编译。仅对「无决策机械映射」采用生成（决策 3）。
- **反射建策略表。** 否决：逼 agent 模拟 runtime，违反决策 4。
- **提交生成物以求可见。** 否决：打破「模板 cherry-pick、diff 可移植」的既定约束。可见性改由「策略表本就手写提交 + 机械映射才生成」达成。

## 边界

i18n 非基准 locale 的键完备性，天然到不了编译期，停在构建 lint;驱动**行为**正确性靠集成测试，非本规范范围（那不是「漂移」，是代码）。
