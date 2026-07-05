# Triage 标签

技能层用五个规范化的 triage 角色说话。本文件把这些角色映射到本仓库 issue 追踪器里实际使用的标签字符串。

> **语言约定：本文件的说明性文字用中文。** 标签字符串本身是 GitHub 上的字面量，保持原样（英文），不要翻译。

| mattpocock/skills 中的角色 | 本仓库标签        | 含义                        |
| -------------------------- | ----------------- | --------------------------- |
| `needs-triage`             | `needs-triage`    | 维护者需要评估此 issue      |
| `needs-info`               | `needs-info`      | 等待报告者补充信息          |
| `ready-for-agent`          | `ready-for-agent` | 已完整规约，可交给 AFK 代理 |
| `ready-for-human`          | `ready-for-human` | 需要人来实现                |
| `wontfix`                  | `wontfix`         | 不予处理                    |

当某个 skill 提到某个角色（例如"打上 AFK-ready triage 标签"）时，使用本表右列对应的标签字符串。

右列可自行编辑，以匹配你实际使用的词汇。

## 首次使用前先创建标签

这些标签在 GitHub 上尚不存在。`gh issue edit --add-label` **不会**自动创建缺失的标签（会报错），所以首次使用前先建好：

```bash
gh label create needs-triage    --description "维护者需要评估"       --color BFDADC
gh label create needs-info      --description "等待报告者补充信息"   --color FBCA04
gh label create ready-for-agent --description "已完整规约，可交给代理" --color 0E8A16
gh label create ready-for-human --description "需要人来实现"         --color 1D76DB
gh label create wontfix         --description "不予处理"             --color FFFFFF
```

标签描述用中文；标签名保持上表中的英文字面量。
