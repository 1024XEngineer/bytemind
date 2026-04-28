import { defineConfig } from 'vitepress'

const enNav = [
  { text: 'Start', link: '/quick-start' },
  { text: 'Guide', link: '/usage/chat-mode' },
  { text: 'Examples', link: '/examples/fix-bug' },
  { text: 'Reference', link: '/reference/cli-commands' }
]

const enSidebar = [
  {
    text: 'Getting Started',
    items: [
      { text: 'Quick Start', link: '/quick-start' },
      { text: 'Installation', link: '/installation' },
      { text: 'Configuration', link: '/configuration' },
      { text: 'Core Concepts', link: '/core-concepts' },
    ],
  },
  {
    text: 'Guide',
    items: [
      { text: 'Chat Mode', link: '/usage/chat-mode' },
      { text: 'Run Mode', link: '/usage/run-mode' },
      { text: 'Session Management', link: '/usage/session-management' },
      { text: 'Tools and Approval', link: '/usage/tools-and-approval' },
      { text: 'Provider Setup', link: '/usage/provider-setup' },
      { text: 'Plan Mode', link: '/usage/plan-mode' },
      { text: 'Skills', link: '/usage/skills' },
    ],
  },
  {
    text: 'Examples',
    items: [
      { text: 'Fix a Bug', link: '/examples/fix-bug' },
      { text: 'Refactor Code', link: '/examples/refactor' },
      { text: 'Generate Documentation', link: '/examples/doc-generation' },
    ],
  },
  {
    text: 'Reference',
    items: [
      { text: 'CLI Commands', link: '/reference/cli-commands' },
      { text: 'Config Reference', link: '/reference/config-reference' },
      { text: 'Environment Variables', link: '/reference/env-vars' },
      { text: 'FAQ', link: '/faq' },
      { text: 'Troubleshooting', link: '/troubleshooting' },
      { text: 'Open Source', link: '/open-source' },
    ],
  },
]

const zhNav = [
  { text: '开始', link: '/zh/quick-start' },
  { text: '指南', link: '/zh/usage/chat-mode' },
  { text: '示例', link: '/zh/examples/fix-bug' },
  { text: '参考', link: '/zh/reference/cli-commands' }
]

const zhSidebar = [
  {
    text: '入门',
    items: [
      { text: '快速开始', link: '/zh/quick-start' },
      { text: '安装', link: '/zh/installation' },
      { text: '配置', link: '/zh/configuration' },
      { text: '核心概念', link: '/zh/core-concepts' },
    ],
  },
  {
    text: '使用指南',
    items: [
      { text: '聊天模式', link: '/zh/usage/chat-mode' },
      { text: '单次执行模式', link: '/zh/usage/run-mode' },
      { text: '会话管理', link: '/zh/usage/session-management' },
      { text: '工具与审批', link: '/zh/usage/tools-and-approval' },
      { text: 'Provider 配置', link: '/zh/usage/provider-setup' },
      { text: 'Plan 模式', link: '/zh/usage/plan-mode' },
      { text: '技能', link: '/zh/usage/skills' },
    ],
  },
  {
    text: '示例',
    items: [
      { text: '修复 Bug', link: '/zh/examples/fix-bug' },
      { text: '代码重构', link: '/zh/examples/refactor' },
      { text: '文档生成', link: '/zh/examples/doc-generation' },
    ],
  },
  {
    text: '参考',
    items: [
      { text: 'CLI 命令', link: '/zh/reference/cli-commands' },
      { text: '配置参考', link: '/zh/reference/config-reference' },
      { text: '环境变量', link: '/zh/reference/env-vars' },
      { text: '常见问题', link: '/zh/faq' },
      { text: '故障排查', link: '/zh/troubleshooting' },
      { text: '开源参与', link: '/zh/open-source' },
    ],
  },
]

export default defineConfig({
  base: '/bytemind/',

  locales: {
    root: {
      label: 'English',
      lang: 'en-US',
      title: 'ByteMind',
      description: 'Terminal-first AI coding agent documentation.',
      themeConfig: {
        nav: enNav,
        sidebar: enSidebar,
      },
    },
    zh: {
      label: '中文',
      lang: 'zh-CN',
      title: 'ByteMind',
      description: 'ByteMind 终端优先 AI 编程助手文档。',
      themeConfig: {
        nav: zhNav,
        sidebar: zhSidebar,
      },
    },
  },

  themeConfig: {
    socialLinks: [
      { icon: 'github', link: 'https://github.com/1024XEngineer/bytemind' },
    ],
    footer: {
      message: 'Released under the MIT License.',
      copyright: 'Copyright © 2024-present ByteMind Contributors',
    },
  },
})
