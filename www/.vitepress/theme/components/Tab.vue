<script setup lang="ts">
defineOptions({ name: 'Tab' })

defineProps<{
  title: string
  value?: string
}>()
</script>

<template>
  <slot />
</template>
