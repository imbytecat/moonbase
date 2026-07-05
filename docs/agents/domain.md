# 领域文档

工程技能在探索本代码库时，应如何消费本仓库的领域文档。

> **语言约定：领域文档（`CONTEXT-MAP.md`、各 `CONTEXT.md`、`docs/adr/` 下的 ADR）一律用中文撰写。** 仅代码标识符、类型名、proto 字段名、文件路径保持英文字面量。
>
> **本仓库为多上下文（multi-context）布局。**

## 探索前先读

- 根目录 **`CONTEXT-MAP.md`**——它指向各上下文各自的 `CONTEXT.md`。读与当前主题相关的那些。
- 各上下文的 **`CONTEXT.md`**（见下方结构）。
- **`docs/adr/`**——读与你即将改动区域相关的 ADR。多上下文仓库里，另查各上下文自己的 `docs/adr/`（例如 `apps/server/docs/adr/`）。

若这些文件尚不存在，**静默继续**。不要报告缺失，也不要一上来就建议创建。`/domain-modeling` 技能（经由 `/grill-with-docs`、`/improve-codebase-architecture` 触达）会在术语或决策真正被敲定时惰性创建它们。

## 文件结构

本仓库（多上下文，根有 `CONTEXT-MAP.md`）：

```
/
├── CONTEXT-MAP.md                     ← 指向各上下文的 CONTEXT.md
├── docs/adr/                          ← 系统级决策（跨端）
├── proto/                             ← 单一契约真源（跨端领域词汇的源头）
├── apps/
│   ├── server/
│   │   ├── CONTEXT.md                 ← Go 后端领域
│   │   └── docs/adr/                  ← 后端专属决策
│   └── web/
│       ├── CONTEXT.md                 ← React 前端领域
│       └── docs/adr/                  ← 前端专属决策
└── packages/
    └── api-client/                    ← 生成的 TS 客户端（契约的产物）
```

领域词汇的权威真源是 `proto/`：权限（`Permission` 枚举）、通道 / 档案 / 绑定 / 用途（channel / profile / binding / purpose）等概念在此定义，两端类型安全共享。各上下文的 `CONTEXT.md` 应**引用**而非重新发明这些术语。

## 使用词汇表里的词

当你的输出命名某个领域概念（issue 标题、重构提案、假设、测试名）时，使用相关 `CONTEXT.md` 里定义的术语。别漂移到词汇表明确回避的同义词。

若你需要的概念还不在词汇表里，这是个信号——要么你在发明项目并不使用的语言（重新考虑），要么存在真实缺口（记给 `/domain-modeling`）。

## 标出 ADR 冲突

若你的输出与既有 ADR 抵触，明确指出而非悄悄覆盖：

> _与 ADR-0007（event-sourced orders）抵触——但值得重开，因为……_
