<script setup lang="ts">
import { Moon, Monitor, Sun } from 'lucide-vue-next'
import { setTheme, theme } from '@/stores/ui'

const options = [
  { value: 'light', icon: Sun, label: '浅色' },
  { value: 'dark', icon: Moon, label: '深色' },
  { value: 'system', icon: Monitor, label: '跟随系统' },
] as const
</script>

<template>
  <div class="inline-flex items-center gap-0.5 p-0.5 rounded-xl bg-elevated border border-line">
    <button
      v-for="opt in options"
      :key="opt.value"
      class="w-7 h-7 rounded-lg flex items-center justify-center transition-colors"
      :class="theme === opt.value ? 'bg-surface text-ink shadow-sm' : 'text-faint hover:text-ink'"
      :title="opt.label"
      @click="setTheme(opt.value)"
    >
      <component :is="opt.icon" :size="14" />
    </button>
  </div>
</template>
