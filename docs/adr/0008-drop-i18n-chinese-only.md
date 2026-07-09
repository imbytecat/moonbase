# 放弃 i18n，全站中文

> **状态**：accepted。产品级决策，跨 web + server + driver 层。

## 背景

schema 驱动的 provider 表单（ADR-0006）把字段 `label`/`help` 的来源从前端挪到了后端 driver——运行时经 `DescribeProviders` 下发。而前端 i18n 用 **Paraglide**（inlang，封闭集、编译期、TS 单源）。

两者形状根本不同：

| | chrome（页面/按钮/hint/导航） | provider 字段（label / help / 校验报错） |
|---|---|---|
| 集合 | **封闭**，编译期全知道 | **开放**，将来加 provider 才冒出 |
| 消费者 | 前端自己（编译期） | 前端是运行时 wire 的下游，编译期不可能知道 |

前端**不可能**为将来才出现的 provider 预埋 Paraglide 词条。要同时保双语，只剩「两形状 i18n」（chrome 走 Paraglide、provider 字段双语随 driver 下发、校验走 `{code, field}` 前端拼）——技术自洽，但长期背「两套 i18n 形状」的心智负担。在 90% 面向国内、i18n 非强需求的场景下，这税不划算。

## 决定

**放弃 i18n，全站中文。**

- 删除 Paraglide：`vite.config.ts` 插件、`project.inlang`、`apps/web/messages/`、`apps/web/src/paraglide/`、`package.json` 的 `gen:i18n` 脚本与 `@inlang/paraglide-js` 依赖。
- 562 处 `m.xxx()` 就地替换为中文串（取自现有 `zh-CN.json`）。
- driver schema 的 `Label`/`Help` 直接写中文；`core/schema` 校验报错中文。
- `<html lang="zh-CN">`，去掉语言切换 UI；README 去掉「中英双语」卖点。

## 考虑过的替代

- **A′：两形状 i18n**（chrome 留 Paraglide + provider 字段双语随 driver 下发 + 校验 `{code,field}` 前端拼）。唯一能同时满足「一致双语 + 零前端扩展 + 不重复维护」的模型，且保住双语卖点。因当前非强需求、不愿背两套形状的心智负担而收回——记录在此，避免日后重复论证。
- **保留 Paraglide、基线改英文**。面向国内不合理，收回。

## 触发重评

- 出现真实多语言需求（海外客户 / 合规）→ 重新上 i18n。届时优先「顶层单源、编译到 Go + TS」的 protobuf 式方案，而非把开放的 provider 字段硬塞进前端封闭目录。
