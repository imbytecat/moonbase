# mise 管工具链、moon 管任务的可行性研究

## 结论

建议迁移到 **mise 管理仓库级工具链，moon 继续管理任务图**。

> 实施状态（2026-07-10）：本研究提出的迁移已落地；下文“迁移前改动审阅”保留为
> 决策背景。完整清单已写入 `.mise.toml`，并生成三平台 `mise.lock`。

这两个工具的职责可以清晰分开：mise 负责安装、锁定版本并把可执行文件放入
`PATH`；moon 继续负责项目图、任务依赖、缓存、affected 计算和 CI 编排。moon 的
`system` toolchain 本来就是运行环境中已有的命令，因此不要求工具必须由 proto 或
moon 自己安装。[moon toolchain 配置][moon-toolchain]

当前仓库大量使用 `go run <module>@<version>`。它能以 Go 作为唯一引导工具，并且
Go 缓存会减轻重复编译成本，但版本散落在多个 `moon.yml`、共享任务和
`buf.gen.yaml` 中；工具升级、冷启动和跨平台问题都不够集中。mise 的 Go backend
仍可对没有合适发布二进制的工具执行 `go install`，但只在安装阶段做一次，并把版本
统一放在 `.mise.toml`。[mise Go backend][mise-go]

对提供正式发布产物的工具，优先使用 mise registry 默认映射的 Aqua backend。mise
官方说明 Aqua backend 使用 Aqua registry 的包定义，并支持其校验和与供应链验证
能力；这比每次用当前 Go 编译器从源码构建更适合 `golangci-lint`、`sqlc`、`buf`、
`air` 等仓库级 CLI。[mise Aqua backend][mise-aqua] golangci-lint 官方也明确推荐其
二进制安装方式，且不保证用 `go install` 安装所得结果。[golangci-lint 安装文档][golangci-install]

## 迁移前改动审阅

实施前工作区的方向正确，但只是迁移起点，当时不能作为完整迁移提交：

- `.mise.toml` 已能在本机解析并激活 `moon 2.3.5`、`go 1.26.4`、`node 26.4.0`、
  `pnpm 11.9.0`。
- `.prototools` 已删除，但 `Dockerfile` 仍复制它并执行 `proto install`；新构建会失败。
- GitHub Actions 仍使用 `moonrepo/setup-toolchain` 并按 `.prototools` 安装、缓存工具链。
- README、AGENTS、`.moon/toolchains.yml` 注释和依赖更新说明仍把 proto 当作工具链真源。
- `golangci-lint`、`sqlc`、`buf`、`air`、`goose`、`govulncheck` 和两个 Go protoc
  插件仍把版本写死在任务或 `buf.gen.yaml` 中，所以尚未解决版本分散问题。
- 尚无 `mise.lock`。mise 的 lockfile 可记录各平台的解析结果、下载 URL 和校验信息；
  CI 可用 `mise install --locked` 阻止安装时重新解析发布信息。[mise lock 命令][mise-lock]
- `.vscode/extensions.json` 增加 mise 扩展是合理的辅助改动，但不能替代 CLI、CI 和
  Docker 的激活接线。

## 建议纳入 mise 的工具

本机使用 mise 2026.7.0 对仓库现有精确版本做了只读解析和 `mise install --dry-run`，
以下条目均可解析：

| 工具 | 当前版本 | 推荐 mise 条目 | 说明 |
| --- | --- | --- | --- |
| moon | 2.3.5 | `aqua:moonrepo/moon` | registry 没有短名时显式使用 Aqua 包 |
| Go | 1.26.4 | `go` | mise core backend |
| Node.js | 26.4.0 | `node` | mise core backend |
| pnpm | 11.9.0 | `pnpm` | registry 默认映射到 Aqua |
| buf | 1.71.0 | `buf` | registry 默认映射到 Aqua |
| golangci-lint | 2.12.2 | `golangci-lint` | 使用官方发布二进制更合适 |
| sqlc | 1.31.1 | `sqlc` | registry 默认映射到 Aqua |
| air | 1.65.3 | `air` | registry 默认映射到 Aqua |
| goose | 3.27.2 | `aqua:pressly/goose` | 无短名，显式指定 Aqua 包 |
| protoc-gen-go | 1.36.11 | `protoc-gen-go` | registry 已有映射 |
| protoc-gen-connect-go | 1.20.0 | `protoc-gen-connect-go` | registry 默认使用 Go backend |
| govulncheck | 1.5.0 | `go:golang.org/x/vuln/cmd/govulncheck` | 官方 Go 工具，使用 Go backend |

推荐的配置形状如下，实际迁移时应由 `mise use --pin` 生成或校验，而不是只手写后假设
所有平台都可安装：

```toml
[env]
PROTO_HOME = "{{config_root}}/.moon/cache/proto"

[tools]
"aqua:moonrepo/moon" = "2.3.5"
go = "1.26.4"
node = "26.4.0"
pnpm = "11.9.0"
buf = "1.71.0"
golangci-lint = "2.12.2"
sqlc = "1.31.1"
air = "1.65.3"
"aqua:pressly/goose" = "3.27.2"
protoc-gen-go = "1.36.11"
protoc-gen-connect-go = "1.20.0"
"go:golang.org/x/vuln/cmd/govulncheck" = "1.5.0"
```

不建议把所有 CLI 都强行搬进 mise：

- `@bufbuild/protoc-gen-es`、`@connectrpc/protoc-gen-connect-query`、Biome 和 commitlint
  应继续由 pnpm workspace 管理。它们属于 JS 工具/包生态，其中两个生成器还必须与
  对应运行时依赖锁步。
- 仓库内的 `apps/server/cmd/protoc-gen-permissions` 应继续从当前源码运行；它不是外部
  工具版本问题。
- Go 运行时依赖继续由各 `go.mod` 管理。mise 只统一 CLI，不会自动统一例如 goose
  CLI 与服务端导入的 goose 库版本。

## 必须补上的 moon 缓存接线

把任务从：

```yaml
command: 'go'
args: ['run', 'github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1', 'generate']
```

改成：

```yaml
command: 'sqlc'
args: ['generate']
```

以后，工具版本不再出现在 moon 的任务定义中。如果 `.mise.toml` 和 `mise.lock` 不是
任务输入，升级工具可能仍命中旧缓存，尤其会留下旧版本生成器产生的代码。

moon 的任务配置支持 `implicitInputs`，可把输入自动注入所有继承该配置的任务。
[moon implicitInputs][moon-tasks] 推荐增加一个所有项目继承的共享任务配置：

```yaml
implicitInputs:
  - '/.mise.toml'
  - '/mise.lock'
```

这样工具链变化会让相关任务重新计算。若担心任一工具升级导致全仓重跑，也可以按
Go、TypeScript、proto 三类分别声明，但这个仓库工具升级频率低，全局失效更简单可靠。

## 激活、CI 与 Docker

`mise install` 只安装工具，并不会自动永久修改调用者的 `PATH`；交互式 shell 通常用
`mise activate`，自动化环境应使用 mise GitHub Action 或 `mise exec -- <command>`。
[mise 激活说明][mise-activate] [mise CI 文档][mise-ci]

因此推荐：

- 本地：开发者安装 mise，在 shell rc 中激活一次，然后保持现有 `moon run ...` 体验。
- 非交互脚本和诊断：用 `mise exec -- moon run ...`，避免依赖调用者的 PATH 顺序。
- Git hooks：若要兼容从 GUI 发起的提交，优先把钩子入口写成
  `mise exec -- moon run ...`；前提仍是系统能找到 `mise` 本身。
- GitHub Actions：用官方 `jdx/mise-action` 读取 `.mise.toml` 并安装/缓存工具，然后运行
  `pnpm install` 与 moon。Renovate 官方已有 `mise` manager，可识别 mise 配置；迁移后
  可删除当前专门匹配 `go run ...@version` 的正则 manager。[mise Action][mise-action]
  [Renovate mise manager][renovate-mise]
- Docker：先固定一个 mise 引导版本，安装 Node 官方 Linux 二进制所需的 `libatomic1`，
  复制并信任 `.mise.toml` 和 `mise.lock`，安装工具，再复制 package manifests 和源码。
  构建命令使用 `mise exec -- pnpm ...` 与
`mise exec -- moon ...`，不要假设 Docker 的非交互 shell 已执行激活脚本。

moon 2 自身仍初始化 proto 环境；若用户机器已有 proto，它会把全局 proto shims 注入
system toolchain 的任务 PATH。迁移实测因此在 `.mise.toml` 中把 `PROTO_HOME` 指向仓库内
已忽略的 `.moon/cache/proto`，隔离兼容存储，确保 mise 是本工作区唯一的可执行文件来源。

mise 自身无法由自己的项目配置引导，这是所有版本管理器共有的一层例外。CI action
版本和 Docker 中的 mise 安装版本仍需在外层固定并由 Renovate 或人工更新。

## lockfile 边界

建议提交 `mise.lock`，并至少覆盖项目声称支持的 `linux-x64`、`macos-arm64`、
`windows-x64`。当前四个基础工具的 `mise lock --dry-run` 已能为这三个平台解析。

完整工具清单生成 lockfile 后，已在 Linux x64 成功执行 `mise install --locked`，其中
两个 Go backend 工具也能安装。Go backend 的 lock 条目只记录版本，不像下载型 backend
那样带平台 URL/校验和；`--locked` 仍接受这种条目。macOS arm64 与 Windows x64 的下载型
工具已经写入 lockfile，但仍应由对应平台 CI/开发机持续验证。[mise install 命令][mise-install]

## 推荐迁移顺序

1. 补全 `.mise.toml`，生成覆盖支持平台的 `mise.lock`，在 Linux/macOS/Windows 至少做
   一轮安装验证。
2. 把 moon 和 Buf 中的外部 `go run module@version` 改成直接调用 mise 管理的命令；
   保留仓库内生成器和 pnpm 工具原样。
3. 把 `.mise.toml`、`mise.lock` 加入 moon 的隐式输入，防止版本升级命中旧缓存。
4. 同一提交中替换 GitHub Actions 与 Dockerfile 的 proto 引导流程，不能先删
   `.prototools` 再留旧消费者。
5. 更新 README、AGENTS、`.moon/toolchains.yml` 注释、Renovate 配置和 VS Code 建议。
6. 使用 `mise exec -- moon run :fix && mise exec -- moon run :test` 验证，再验证
   `docker build` 和 CI 的 fresh clone 路径。

## 最终判断

对本仓库而言，这是有净收益的迁移：工具版本从多份任务定义集中到一个清单；正式
二进制工具不再在冷环境中逐个源码编译；没有发布二进制的 Go 工具仍可由同一个管理器
安装；moon 的任务模型无需改变。

真正需要谨慎的不是 mise 与 moon 是否兼容，而是迁移的外围闭环：PATH 激活、moon
缓存输入、CI、Docker、lockfile 平台覆盖和 bootstrap 版本。把这些在一个迁移提交中
一起处理后，`mise + moon system toolchain` 是比当前 `proto + 分散 go run` 更适合本仓库
的组合。

[moon-toolchain]: https://moonrepo.dev/docs/config/toolchain
[moon-tasks]: https://moonrepo.dev/docs/config/tasks
[mise-go]: https://mise.jdx.dev/dev-tools/backends/go.html
[mise-aqua]: https://mise.jdx.dev/dev-tools/backends/aqua.html
[mise-lock]: https://mise.jdx.dev/cli/lock.html
[mise-install]: https://mise.jdx.dev/cli/install.html
[mise-activate]: https://mise.jdx.dev/getting-started.html#activate-mise
[mise-ci]: https://mise.jdx.dev/continuous-integration.html
[mise-action]: https://github.com/jdx/mise-action
[renovate-mise]: https://docs.renovatebot.com/modules/manager/mise/
[golangci-install]: https://golangci-lint.run/docs/welcome/install/
