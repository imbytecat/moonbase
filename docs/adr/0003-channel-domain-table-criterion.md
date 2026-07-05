# 通道领域表判据：产出物是持久工件才建表，否则日志足矣

## 背景

系统有一排基础设施通道（storage / captcha / email / sms / 消息推送 / AI / 第三方登录 / 支付），全部走统一的 profile CRUD + purpose 绑定管理。但「通道配置可管理」不等于「通道产出可管理」：payment 有 `payment_orders` 台账，storage 的上传却无账可查。每接一个新通道都会重演一次「要不要给它建表」的争论，需要一条判据终结它。

## 决定

**通道的产出物是否是有独立生命周期和持续义务的持久工件（durable artifact）？是 → 领域表；否 → 产出即效果，slog / audit_logs 覆盖。**

逐通道裁定：

| 通道 | 产出物 | 持续义务 | 裁定 |
| --- | --- | --- | --- |
| payment | 一笔钱的状态机 | 退款、对账、争议 | ✅ `payment_orders`（已有） |
| storage | 占着字节的对象 | 存储成本、被引用、回收 | ✅ `files` + `file_attachments`（本 ADR 首个应用） |
| 消息推送 | 用户收件箱条目 | 已读/未读、留存 | ✅ `notifications`（已有） |
| email / sms | 一次发送尝试 | 无——发出即结束 | ❌ 不建 |
| captcha | 一次校验 | 无 | ❌ 不建 |
| oauth | 身份绑定 | 绑定/解绑 | ✅ 但工件属 auth 域（`identities`），不属通道 |
| llm | 一次补全 | 无（无配额/计费） | ❌ 不建 |

**工件归属域，不归通道**：OTP 挑战本身是持久工件，但它属 auth 域——`verification_tokens` 已按行业 `otp_requests` 模式建表（绑 user / purpose / 过期 / 尝试计数）。email/sms 通道只负责「把它发出去」这个瞬时效果。

### storage 的应用：files + attachments 双表

行业收敛做法（Rails ActiveStorage blobs/attachments、WordPress attachment post、Strapi upload_file/morph）：

- **`files`**：系统认账的对象元数据（key、content_type、size、上传者），一文件一行，精神上不可变。**presign 时落库**——每个可能存在的 object 从第一秒起就有账，「桶里多了」的孤儿在模型上不可能发生。
- **`file_attachments`**：多态引用（引用方类型 + ID → file_id），外键约束保证被引用的 file 删不掉——「引用悬空」在模型上不可能发生。
- **孤儿回收 = 一条通用查询**：创建超过宽限期（24h）且无 attachment 的 file → 删 object → 删行，由 DBOS 定时工作流执行（崩溃续跑收敛「删了 object 未删行」中间态）。不扫桶对账，不写 per-业务清理逻辑。
- 同步删对象永远只是 best-effort 优化，不是正确性机制；顺序必须「先提交 DB，后删对象」。
- **存量槽位迁入，不留双轨**：`users.avatar_key` / settings 的 `logo_key`/`favicon_key` 改为引用 file——换头像/换 logo = 新 file + 引用改指向，旧 file 归零后被 unattached 清理自然回收。替换泄漏与删用户清理不写专门代码，全部由同一条 unattached 查询收编；系统内只有一套文件语义。

## 非目标（明确的「不」）

- **Media Library 管理 UI**（列表 / 删除 / 文件夹 / 复用选择器）——模型难以事后补（桶里几万个无账文件回填是考古工程），UI 随时可加。地基现在打，门面等真实 CMS 需求。
- **email / sms 发送记录表**——driver 未接 provider 回执（DLR / webhook），建表只会记一堆永远停在 `sent` 的行。触发条件：接入回执 webhook、要做送达率对账/SLA 的那天再建。
- **llm usage 表**——触发条件：引入 token 配额或成本核算。

## 考虑过的替代

- **每通道配一张 log 表**（email_logs / sms_logs / captcha_logs…）：否决——把可观测性问题当领域问题，行数无义务、无生命周期，slog（文件日志 + 轮转已具备）与 audit_logs 已覆盖排障与变更追溯。
- **storage 不建表、靠生命周期规则 + 覆写式 key 清孤儿**：仅适用于单值槽位（头像/logo）。需求已明确为通用文件能力（CMS 附件、公开素材、多处引用），单账本推导法失效，收回。
- **上传完成后由客户端 Confirm 落库**：否决——confirm 没调就回到「桶里有、账上无」的老路；presign 即落库让漏账窗口为零。
