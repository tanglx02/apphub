<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const tabs = [
  { name: 'settings-apps', label: '应用管理' },
  { name: 'settings-categories', label: '分类管理' },
  { name: 'settings-system', label: '系统设置' },
  { name: 'settings-security', label: '安全设置' },
  { name: 'settings-backup', label: '备份恢复' },
  { name: 'settings-audit', label: '审计日志' },
]
const active = computed(() => route.name as string)
</script>

<template>
  <div class="px-4 sm:px-6 lg:px-7 py-5 sm:py-6 max-w-[1280px] mx-auto">
    <div class="flex items-center gap-1 overflow-x-auto no-scrollbar border-b border-line mb-5">
      <router-link
        v-for="t in tabs"
        :key="t.name"
        :to="{ name: t.name }"
        class="shrink-0 h-10 px-3 text-[13.5px] border-b-2 -mb-px transition-colors"
        :class="
          active === t.name
            ? 'border-accent text-accent font-medium'
            : 'border-transparent text-muted hover:text-ink'
        "
      >
        {{ t.label }}
      </router-link>
    </div>
    <router-view />
  </div>
</template>
