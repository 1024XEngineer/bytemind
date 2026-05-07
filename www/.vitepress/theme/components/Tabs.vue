<script lang="ts">
import {
  Comment,
  Fragment,
  Text,
  cloneVNode,
  computed,
  defineComponent,
  getCurrentInstance,
  h,
  ref,
  type VNode,
} from 'vue'

type TabItem = {
  id: string
  key: string
  title: string
  value: string
  vnode: VNode
}

function normalize(value: string) {
  return value.trim().toLowerCase()
}

function slug(value: string, fallback: string) {
  return (
    value
      .trim()
      .toLowerCase()
      .replace(/[^a-z0-9\u4e00-\u9fa5]+/g, '-')
      .replace(/^-+|-+$/g, '') || fallback
  )
}

function flatten(nodes: VNode[]): VNode[] {
  return nodes.flatMap((node) => {
    if (node.type === Fragment && Array.isArray(node.children)) {
      return flatten(node.children as VNode[])
    }

    if (node.type === Comment || node.type === Text) {
      return []
    }

    return [node]
  })
}

function isTabNode(node: VNode) {
  const type = node.type as { name?: string; __name?: string }
  return type?.name === 'Tab' || type?.__name === 'Tab'
}

export default defineComponent({
  name: 'Tabs',
  props: {
    defaultTab: {
      type: String,
      default: '',
    },
    defaultValue: {
      type: String,
      default: '',
    },
  },
  setup(props, { slots }) {
    const instance = getCurrentInstance()
    const baseId = `doc-tabs-${instance?.uid ?? '0'}`
    const activeValue = ref(props.defaultTab || props.defaultValue)

    const tabs = computed<TabItem[]>(() =>
      flatten(slots.default?.() ?? [])
        .filter(isTabNode)
        .map((vnode, index) => {
          const vnodeProps = (vnode.props ?? {}) as { title?: string; value?: string }
          const title = vnodeProps.title ?? `Tab ${index + 1}`
          const value = vnodeProps.value ?? title
          const id = `${baseId}-${slug(value, `tab-${index}`)}-${index}`

          return {
            id,
            key: `${value}-${index}`,
            title,
            value,
            vnode,
          }
        }),
    )

    const activeTab = computed(() => {
      const current = normalize(activeValue.value)
      return tabs.value.find((tab) => normalize(tab.value) === current) ?? tabs.value[0]
    })

    function selectTab(tab: TabItem) {
      activeValue.value = tab.value
    }

    function focusTab(tab: TabItem) {
      const element = document.getElementById(`${tab.id}-tab`)
      element?.focus()
    }

    function handleKeydown(event: KeyboardEvent, index: number) {
      if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) {
        return
      }

      event.preventDefault()

      const items = tabs.value
      const lastIndex = items.length - 1
      const nextIndex =
        event.key === 'Home'
          ? 0
          : event.key === 'End'
            ? lastIndex
            : event.key === 'ArrowRight'
              ? index === lastIndex
                ? 0
                : index + 1
              : index === 0
                ? lastIndex
                : index - 1
      const nextTab = items[nextIndex]

      if (nextTab) {
        selectTab(nextTab)
        requestAnimationFrame(() => focusTab(nextTab))
      }
    }

    return () => {
      const items = tabs.value
      const current = activeTab.value

      if (items.length === 0) {
        return null
      }

      return h('section', { class: 'doc-tabs' }, [
        h(
          'div',
          {
            class: 'doc-tabs__list',
            role: 'tablist',
          },
          items.map((tab, index) => {
            const active = current?.key === tab.key

            return h(
              'button',
              {
                id: `${tab.id}-tab`,
                key: `${tab.key}-tab`,
                class: ['doc-tabs__tab', { 'doc-tabs__tab--active': active }],
                type: 'button',
                role: 'tab',
                tabindex: active ? 0 : -1,
                'aria-selected': active,
                'aria-controls': `${tab.id}-panel`,
                onClick: () => selectTab(tab),
                onKeydown: (event: KeyboardEvent) => handleKeydown(event, index),
              },
              tab.title,
            )
          }),
        ),
        h(
          'div',
          { class: 'doc-tabs__panels' },
          items.map((tab) => {
            const active = current?.key === tab.key

            return h(
              'div',
              {
                id: `${tab.id}-panel`,
                key: `${tab.key}-panel`,
                class: ['doc-tabs__panel', { 'doc-tabs__panel--hidden': !active }],
                role: 'tabpanel',
                tabindex: 0,
                'aria-labelledby': `${tab.id}-tab`,
                'aria-hidden': !active,
              },
              [cloneVNode(tab.vnode)],
            )
          }),
        ),
      ])
    }
  },
})
</script>

<style scoped>
.doc-tabs {
  margin: 24px 0;
}

.doc-tabs__list {
  display: flex;
  min-width: 0;
  gap: 26px;
  overflow-x: auto;
  border-bottom: 1px solid rgba(120, 130, 145, 0.24);
}

.doc-tabs__tab {
  flex: 0 0 auto;
  margin: 0 0 -1px;
  padding: 10px 0 11px;
  border: 0;
  border-bottom: 2px solid transparent;
  background: transparent;
  color: var(--vp-c-text-1);
  cursor: pointer;
  font-size: 14px;
  font-weight: 700;
  letter-spacing: 0;
  line-height: 1.5;
  white-space: nowrap;
}

.doc-tabs__tab:hover,
.doc-tabs__tab:focus-visible {
  border-bottom-color: rgba(120, 130, 145, 0.42);
  outline: none;
}

.doc-tabs__tab--active {
  border-bottom-color: var(--vp-c-brand-1);
  color: var(--vp-c-brand-1);
}

.doc-tabs__panels {
  margin-top: 22px;
}

.doc-tabs__panel--hidden {
  display: none;
}

.doc-tabs__panel :deep(> :first-child) {
  margin-top: 0;
}

.doc-tabs__panel :deep(> :last-child) {
  margin-bottom: 0;
}

.doc-tabs__panel :deep(div[class*='language-']) {
  margin-top: 14px;
}

.doc-tabs__panel :deep(pre),
.doc-tabs__panel :deep(code),
.doc-tabs__panel :deep(.line) {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
