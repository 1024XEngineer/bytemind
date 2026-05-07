import DefaultTheme from 'vitepress/theme'
import type { Theme } from 'vitepress'
import Tab from './components/Tab.vue'
import Tabs from './components/Tabs.vue'
import './style.css'

export default {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    app.component('Tabs', Tabs)
    app.component('Tab', Tab)
  },
} satisfies Theme
