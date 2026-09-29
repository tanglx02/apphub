<script setup lang="ts">
import { useRoute } from 'vue-router'
import { computed } from 'vue'
import AppIcon from './AppIcon.vue'

const route = useRoute()
const items = [
  { name: 'dashboard', label: '应用', icon: 'LayoutGrid', to: { name: 'dashboard' } },
  { name: 'settings-apps', label: '管理', icon: 'Package', to: { name: 'settings-apps' } },
  { name: 'settings-backup', label: '备份', icon: 'HardDrive', to: { name: 'settings-backup' } },
  { name: 'settings-system', label: '设置', icon: 'Settings', to: { name: 'settings-system' } },
]
const active = computed(() => route.name as string)
</script>

<template>
  <nav
    class="lg:hidden fixed bottom-0 left-0 right-0 z-40 bg-surface/95 backdrop-blur border-t border-line safe-bottom"
  >
    <div class="flex items-stretch">
      <router-link
        v-for="item in items"
        :key="item.name"
        :to="item.to"
        class="flex-1 flex flex-col items-center gap-1 py-2.5 transition-colors"
        :class="active === item.name ? 'text-accent' : 'text-faint'"
      >
        <AppIcon :name="item.icon" :size="19" />
        <span class="text-[11px]">{{ item.label }}</span>
      </router-link>
    </div>
  </nav>
</template>
