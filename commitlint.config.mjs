// 提交信息风格约束（Conventional Commits），由 .moon/workspace.yml 的
// commit-msg 钩子调用：pnpm exec commitlint --edit <msg文件>。
// 格式：type(scope): 中文描述 —— 如 feat(web): 登录页加验证码。
// YAML 由 `yaml` 包同步解析（Node 无内置 YAML）；同步读取
// （不能用顶层 await，cosmiconfig 走 require 加载本文件）。
import { readFileSync } from 'node:fs'
import { parse as parseYaml } from 'yaml'

const workspace = parseYaml(readFileSync('.moon/workspace.yml', 'utf8'))

export default {
  extends: ['@commitlint/config-conventional'],
  plugins: [
    {
      rules: {
        // 描述必须含中文（CJK 统一表意文字区段）
        'subject-zh': ({ subject }) => [
          /[\u4e00-\u9fff]/.test(subject ?? ''),
          '描述必须使用中文（如 feat(web): 登录页加验证码）',
        ],
      },
    },
  ],
  rules: {
    'subject-zh': [2, 'always'],
    // 中文描述不适用大小写规则，关闭
    'subject-case': [0],
    // 中文按字符数计，放宽标题长度
    'header-max-length': [2, 'always', 120],
    // 作用域 = .moon/workspace.yml 的 projects（动态读取）+ deps/ci/agents 惯例值
    'scope-enum': [2, 'always', [...Object.keys(workspace.projects), 'deps', 'ci', 'agents']],
  },
};
