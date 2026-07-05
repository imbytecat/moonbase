# 文件访问统一走永久链接 /f/{file_id}，存储 URL 降为实现细节

## 背景

文件访问 URL 目前是「解析时快照」：local driver 发 HMAC 签名的 `/api/files/{purpose}/{key}?exp=&sig=`，S3 driver 发 `public_base_url` 拼接或短期 presigned GET。URL 形状随 driver 泄漏给所有调用方；换 storage profile / 换 bucket 后历史 URL 全部失效；未来的 private 文件（消息附件类）没有鉴权入口。作为模板项目，URL 形状是下游复制后最难改的东西之一——每个 RPC 的返回值、每处前端拼接都依赖它。

visibility 是 purpose 的静态属性（代码写死 public / private，见 CONTEXT.md），不是 bucket 或 file 行的属性。

## 决定

**所有 RPC 返回的文件 URL 一律是 `/f/{file_id}`。** `GET /f/{file_id}` 查 `files` 表，按 purpose 的 visibility 分派；`ObjectStore.ResolveURL` 降级为该 handler 的内部实现，不再出现在任何 RPC 响应里。

这是 GitHub release assets（302 → S3 presigned）、GitLab uploads、Docker Registry（307 → 对象存储，OCI 分发规范标准姿势）、Slack files 的共同模式：**稳定入口负责身份与权限，存储 URL 是易变实现细节**。

分派矩阵（302 不是无脑用）：

| visibility × driver | 行为 | 缓存头 |
| --- | --- | --- |
| public × local | 直接 200 `ServeContent`（302 回自己是绕圈） | `public, max-age=31536000, immutable`（file 精神上不可变，成立） |
| public × S3 | 302 → public_base_url 稳定 URL | `public, max-age=3600`（换 bucket 后旧跳转最多存活 1h，可接受；给一年不可接受） |
| private × 任意 | 鉴权 → 302 → 短期签名 URL | `private, no-store`（缓存跳转 = 绕过鉴权窗口） |

302 vs 307：GET-only 场景无差别，按惯例用 302。

## 考虑过的替代

- **维持现状（RPC 现场解析 URL）**：当前 avatar/logo 都是每次 RPC 现算所以没有过期事故，但每个未来文件消费点都要重复解析逻辑，且 URL 一旦进邮件/外部页面就会随 profile 变更失效。模板下游迁移是 breaking change，早定便宜。
- **等第一个 private 场景再做**：省一个 handler，但下游用户此期间已按旧 URL 形状各自拼接，回收成本随时间单调上升。
- **全部 302（含 public × local）**：多一次同服务器 RTT 零收益，否决。
- **全部直接代理（服务器中转字节流）**：丧失 S3 直连带宽优势，回到「本地存储传服务器」被质疑的老路，否决。
