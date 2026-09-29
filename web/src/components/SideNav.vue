<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import AppIcon from './AppIcon.vue'
import { categories } from '@/stores/apps'
import { summary } from '@/stores/apps'

const route = useRoute()

const navItems = [
  { name: 'dashboard', label: '控制中心', icon: 'LayoutGrid', to: { name: 'dashboard' } },
  { name: 'settings-apps', label: '应用管理', icon: 'Package', to: { name: 'settings-apps' } },
  { name: 'settings-categories', label: '分类管理', icon: 'Folder', to: { name: 'settings-categories' } },
  { name: 'settings-system', label: '系统设置', icon: 'Settings', to: { name: 'settings-system' } },
  { name: 'settings-backup', label: '备份恢复', icon: 'HardDrive', to: { name: 'settings-backup' } },
  { name: 'settings-audit', label: '审计日志', icon: 'Activity', to: { name: 'settings-audit' } },
  { name: 'settings-security', label: '安全设置', icon: 'Shield', to: { name: 'settings-security' } },
]

const activeName = computed(() => route.name as string)
</script>

<template>
  <aside class="hidden lg:flex w-[228px] shrink-0 flex-col border-r border-line bg-surface">
    <nav class="flex-1 px-3 py-4 space-y-0.5 overflow-y-auto">
      <router-link
        v-for="item in navItems"
        :key="item.name"
        :to="item.to"
        class="flex items-center gap-2.5 h-9 px-2.5 rounded-xl text-[13.5px] transition-all duration-150"
        :class="
          activeName === item.name
            ? 'bg-accent/10 text-accent font-medium'
            : 'text-muted hover:text-ink hover:bg-elevated'
        "
      >
        <AppIcon :name="item.icon" :size="16" />
        <span>{{ item.label }}</span>
      </router-link>

      <template v-if="categories.items.length">
        <div class="section-title px-2.5 pt-5 pb-2">分类</div>
        <div class="space-y-0.5">
          <div
            v-for="c in categories.items"
            :key="c.id"
            class="flex items-center gap-2.5 h-8 px-2.5 rounded-lg text-[13px] text-muted"
          >
            <span class="w-1.5 h-1.5 rounded-full shrink-0" :style="{ background: c.color || 'currentColor' }" />
            <span class="truncate">{{ c.name }}</span>
          </div>
        </div>
      </template>
    </nav>

    <div class="px-4 py-3.5 border-t border-line">
      <div class="flex items-center justify-between text-[12px] text-faint">
        <span>运行中</span>
        <span class="font-mono text-ink">{{ summary.online }} / {{ summary.total }}</span>
      </div>
      <div class="mt-2 h-1 rounded-full bg-elevated overflow-hidden">
        <div
          class="h-full rounded-full bg-ok transition-all duration-500"
          :style="{ width: (summary.rate || 0) + '%' }"
        />
      </div>
    </div>
  </aside>
</template>
