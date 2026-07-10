# 依赖装配用编译期构造注入，否决运行时 DI 容器（fx / samber/do）

> **状态**：accepted。本 ADR 是 ADR-0002（agent-legibility 内核）决策 1/4 在依赖注入选型上的具体化：DI 若要引入，必须保持「错配是编译错误」，绝不把依赖图完整性推迟到运行时。ADR-0006 拆 `systemcodec` 上帝类型包的诊断在此复用。

## 背景

项目变大，组合根（`cmd/server/main.go` + `internal/server/NewRouter`，约 55 行、41 个构造调用）与 `SystemService`（14 个依赖）催生「是不是该上 DI 框架」的体感。评估了 uber/fx、samber/do、google/wire，并以 TypeScript 的 EffectTS 作为「理想 DI」的对照。

诊断：体感到的「复杂」不在**接线机制**（组合根扁平、自上而下、编译器全程护航、diff 可移植），而在**领域职责**——`SystemService` 是 7 家 integration 管理面同居的上帝服务（方法已按 `system_<integration>.go` 分文件，但依赖全挤在一个 struct，每家其实只用 3–4 个字段），与 ADR-0006 拆掉的 `systemcodec` 上帝类型包同病。DI 框架不减少领域概念，只把接线换一种写法。

关键事实（决定性）：

- **uber/fx、samber/do 是运行时容器**：依赖图是否完整，要到运行时 `Invoke`（返回 error）/ `MustInvoke`（panic）才知道。这正是 ADR-0002 决策 4 点名否决的「逼 agent 模拟 runtime 才能预测行为」，也打断「使用点→定义点」的 grep / go-to-definition 导航链。
- **google/wire** 是编译期代码生成、与铁律相容，但自 2023 进入 maintenance mode，且当前 152 行组合根不值得代码生成工具化。
- **EffectTS 的 DI 是纯编译期**：依赖编码进 `Effect<A, E, R>` 的 `R`，漏 provide → TypeScript 编译错误（`ts(2379)`，`R` 不收敛为 `never`）。它令人向往之处**正是编译期依赖追踪**。Go 无 higher-kinded types，无法复刻 `R` channel；Go 里最忠于该精神的等价物是**构造函数签名 + 编译器**（签名 = 依赖需求，漏参 = `not enough arguments`），即手动构造注入本身。fx / do 恰好扔掉了这一点，只留运行时壳。

## 决定

1. **依赖装配 = 手动构造函数注入。** 组合根显式、有序、字面可 grep：`main.go`（进程生命周期，`defer` 收尾）+ `NewRouter`（HTTP 面对象图）。这是 Go 版的编译期依赖追踪——漏依赖是编译错误，等价 EffectTS 的 `R != never`。
2. **否决运行时 DI 容器**（uber/fx、samber/do、uber/dig 及同类）。它们把依赖图完整性从编译期挪到运行时 `Invoke` / `panic`，违反 ADR-0002 决策 1（不可表达 > 可糊弄）与决策 4（零反射、字面可 grep、不逼 agent 模拟 runtime）。
3. **不引入 google/wire。** 维护停滞 + 当前规模用不上代码生成。若组合根变长，改用**纯函数按域分组**（`newIntegrations()` / `newServices()` / `newInterceptors()`）——仍编译期、零依赖、diff 可移植。
4. **「项目大 / 难维护」归类为领域问题。** 解药是拆上帝服务 + 清限界上下文（延续 ADR-0006），不是加容器。首个目标：把 `SystemService` 从 14 依赖的 God struct 拆成按 integration 内聚的 sub-handler，用 Go embedding 重新组合成同一个 handler——零 proto / 零 authz / 零 wire 改动。

## 考虑过的替代

- **uber/fx**：运行时反射，最违反铁律；否决。
- **samber/do**：泛型让单点 `Provide` / `Invoke` 类型安全（优于 fx 的 `interface{}`），但依赖图完整性仍运行时才知（`Invoke` 返回 error / `MustInvoke` panic）——同类，否决。
- **google/wire**：编译期、兼容铁律，但 maintenance mode + 当前规模用不上；出局。若未来真需工具化，它仍是唯一与铁律相容的候选，优先于任何运行时容器。
- **EffectTS 式 effect system**：Go 语言级不支持（无 HKT）；其编译期 DI 精神已由手动构造注入承接。

## 触发重评

- 出现**多个可部署二进制**共享同一大对象图，手写接线复制成为真实负担。
- 组合根构造显著膨胀（纯函数分组后仍 >250 行 / 数十服务且接线易错）。
- Go 生态出现被广泛采用、**编译期**检查依赖满足性的 DI 方案（类 EffectTS `R` channel）。
- 出现「肯付费、要运行时装配 / 热插拔模块」的真实客户（与 ADR-0002 / 0006 的 go-plugin 触发器同源）。
