import { defineConfig } from 'vitepress'

const enNav = [
  { text: 'What is ByteMind?', link: '/what' },
  { text: 'Installation', link: '/installation' },
  { text: 'Features', link: '/features' },
  { text: 'Open Source', link: '/open-source' },
]

const enSidebar = [
  {
    text: 'Getting Started',
    items: [
      { text: 'What is ByteMind?', link: '/what' },
      { text: 'Installation', link: '/installation' },
    ],
  },
  {
    text: 'Reference',
    items: [
      { text: 'Features', link: '/features' },
      { text: 'Open Source', link: '/open-source' },
    ],
  },
]

const zhNav = [
  { text: 'ByteMind 是什么？', link: '/zh/what' },
  { text: '安装', link: '/zh/installation' },
  { text: '功能特性', link: '/zh/features' },
  { text: '开源参与', link: '/zh/open-source' },
]

const zhSidebar = [
  {
    text: '快速开始',
    items: [
      { text: 'ByteMind 是什么？', link: '/zh/what' },
      { text: '安装', link: '/zh/installation' },
    ],
  },
  {
    text: '参考',
    items: [
      { text: '功能特性', link: '/zh/features' },
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
      description:
        'A terminal-first AI Coding Agent — collaborate with LLMs without leaving your terminal.',
      themeConfig: {
        nav: enNav,
        sidebar: enSidebar,
      },
    },
    zh: {
      label: '中文',
      lang: 'zh-CN',
      title: 'ByteMind',
      description: '终端优先的 AI 编程助手 — 无需离开终端，即可与大语言模型协作开发。',
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
