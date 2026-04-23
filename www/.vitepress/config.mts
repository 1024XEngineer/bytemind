import { defineConfig } from 'vitepress'

const enNav = [
  { text: 'Quick Start', link: '/en/quick-start' },
  { text: 'Usage', link: '/en/usage/chat-mode' },
  { text: 'Reference', link: '/en/reference/cli-commands' },
  { text: 'Open Source', link: '/en/open-source' },
]

const enSidebar = [
  {
    text: 'Getting Started',
    items: [
      { text: 'Overview', link: '/en/index' },
      { text: 'Quick Start', link: '/en/quick-start' },
      { text: 'Installation', link: '/en/installation' },
      { text: 'Configuration', link: '/en/configuration' },
      { text: 'Core Concepts', link: '/en/core-concepts' },
    ],
  },
  {
    text: 'Usage',
    items: [
      { text: 'Chat Mode', link: '/en/usage/chat-mode' },
      { text: 'Run Mode', link: '/en/usage/run-mode' },
      { text: 'Session Management', link: '/en/usage/session-management' },
      { text: 'Tools and Approval', link: '/en/usage/tools-and-approval' },
      { text: 'Provider Setup', link: '/en/usage/provider-setup' },
      { text: 'Plan Mode', link: '/en/usage/plan-mode' },
      { text: 'Skills', link: '/en/usage/skills' },
    ],
  },
  {
    text: 'Examples',
    items: [
      { text: 'Fix a Bug', link: '/en/examples/fix-bug' },
      { text: 'Refactor Code', link: '/en/examples/refactor' },
      { text: 'Generate Documentation', link: '/en/examples/doc-generation' },
    ],
  },
  {
    text: 'Reference',
    items: [
      { text: 'CLI Commands', link: '/en/reference/cli-commands' },
      { text: 'Config Reference', link: '/en/reference/config-reference' },
      { text: 'Environment Variables', link: '/en/reference/env-vars' },
      { text: 'FAQ', link: '/en/faq' },
      { text: 'Troubleshooting', link: '/en/troubleshooting' },
      { text: 'Open Source', link: '/en/open-source' },
    ],
  },
]

const zhNav = [
  { text: '快速开始', link: '/zh/quick-start' },
  { text: '使用指南', link: '/zh/usage/chat-mode' },
  { text: '参考', link: '/zh/reference/cli-commands' },
  { text: '开源参与', link: '/zh/open-source' },
]

const zhSidebar = [
  {
    text: '入门',
    items: [
      { text: '产品概览', link: '/zh/index' },
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
        nav: [
          { text: 'English Docs', link: '/en/index' },
          { text: '中文文档', link: '/zh/index' },
        ],
      },
    },
    en: {
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
