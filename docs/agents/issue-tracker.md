# Issue 追踪器：GitHub

本仓库的 issue 与 PRD 都以 GitHub issue 的形式存放。所有操作使用 `gh` CLI。

> **语言约定（务必遵守）：所有 issue 的标题与正文一律用中文撰写。**
> 这与本仓库的中文提交主题、中文 UI 文案约定一致。仅 `gh` 命令、标签名、代码标识符、文件路径保持英文字面量，其余内容（标题、正文、评论、验收标准等）全部中文。

## 约定

- **创建 issue**：`gh issue create --title "..." --body "..."`；多行正文用 heredoc。标题与正文用中文。
- **读取 issue**：`gh issue view <number> --comments`，用 `jq` 过滤评论并一并取回标签。
- **列出 issue**：`gh issue list --state open --json number,title,body,labels,comments --jq '[.[] | {number, title, body, labels: [.labels[].name], comments: [.comments[].body]}]'`，按需加 `--label`、`--state` 过滤。
- **评论 issue**：`gh issue comment <number> --body "..."`（评论用中文）。
- **增删标签**：`gh issue edit <number> --add-label "..."` / `--remove-label "..."`。
- **关闭**：`gh issue close <number> --comment "..."`（关闭说明用中文）。

仓库由 `git remote -v` 推断——在克隆目录内运行时 `gh` 会自动识别（当前为 `imbytecat/moonbase`）。

## PR 是否作为 triage 入口

**PR 作为请求入口：否。** _(若本仓库改为把外部 PR 当作功能请求，把此处改成 `yes`；`/triage` 会读取此开关。)_

置为 `yes` 时，PR 与 issue 走同一套标签与状态，使用 `gh pr` 对应命令：

- **读取 PR**：`gh pr view <number> --comments`，diff 用 `gh pr diff <number>`。
- **列出待 triage 的外部 PR**：`gh pr list --state open --json number,title,body,labels,author,authorAssociation,comments`，仅保留 `authorAssociation` 为 `CONTRIBUTOR`、`FIRST_TIME_CONTRIBUTOR`、`NONE` 的（丢弃 `OWNER`/`MEMBER`/`COLLABORATOR`）。
- **评论 / 打标签 / 关闭**：`gh pr comment`、`gh pr edit --add-label`/`--remove-label`、`gh pr close`。

GitHub 的 issue 与 PR 共享同一编号空间，裸 `#42` 可能是任意一种——先 `gh pr view 42`，回退 `gh issue view 42`。

## 当某个 skill 说"发布到 issue 追踪器"

创建一个 GitHub issue（标题与正文用中文）。

## 当某个 skill 说"取回相关工单"

运行 `gh issue view <number> --comments`。

## Wayfinding 操作

供 `/wayfinder` 使用。**map** 是一个 issue，**child** issue 作为工单（所有正文用中文）。

- **Map**：一个打了 `wayfinder:map` 标签的 issue，正文承载 Notes / Decisions-so-far / Fog。`gh issue create --label wayfinder:map`。
- **Child 工单**：作为 GitHub sub-issue 链接到 map（对 sub-issues 端点用 `gh api`）。未启用 sub-issues 时，把 child 加入 map 正文的任务列表，并在 child 正文顶部写 `Part of #<map>`。标签：`wayfinder:<type>`（`research`/`prototype`/`grilling`/`task`）。认领后指派给推进的开发者。
- **阻塞**：用 GitHub **原生 issue dependencies**（UI 可见的规范表示）。加边：`gh api --method POST repos/<owner>/<repo>/issues/<child>/dependencies/blocked_by -F issue_id=<blocker-db-id>`，其中 `<blocker-db-id>` 是阻塞方的数字**数据库 id**（`gh api repos/<owner>/<repo>/issues/<n> --jq .id`，*不是* `#number` 或 `node_id`）。GitHub 通过 `issue_dependencies_summary.blocked_by` 报告（仅未关闭的阻塞项——实时闸门）。不可用时回退到 child 正文顶部的 `Blocked by: #<n>, #<n>` 行。所有阻塞项关闭即解除阻塞。
- **Frontier 查询**：列出 map 的未关闭 children（`gh issue list --state open`，限定在 map 的 sub-issues / 任务列表），剔除仍有未关闭阻塞（`issue_dependencies_summary.blocked_by > 0`，或 `Blocked by` 行里有未关闭 issue）或已有 assignee 的；map 顺序中第一个胜出。
- **认领**：`gh issue edit <n> --add-assignee @me`——本 session 的第一次写。
- **解决**：`gh issue comment <n> --body "<中文答复>"`，然后 `gh issue close <n>`，再往 map 的 Decisions-so-far 追加上下文指针（gist + 链接）。
