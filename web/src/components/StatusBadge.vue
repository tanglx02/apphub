<script setup lang="ts">
import { computed } from 'vue'
import type { AppStatus } from '@/api/types'

const props = withDefaults(defineProps<{ status: AppStatus; size?: 'sm' | 'md'; showDot?: boolean }>(), {
  size: 'sm',
  showDot: true,
})

const meta = computed(() => {
  switch (props.status) {
    case 'ONLINE':
      return { text: '在线', color: 'text-ok', bg: 'bg-ok', label: 'ONLINE' }
    case 'OFFLINE':
      return { text: '已停止', color: 'text-faint', bg: 'bg-faint', label: 'OFFLINE' }
    case 'STARTING':
      return { text: '启动中', color: 'text-info', bg: 'bg-info', label: 'STARTING' }
    case 'STOPPING':
      return { text: '停止中', color: 'text-warn', bg: 'bg-warn', label: 'STOPPING' }
    case 'ERROR':
      return { text: '异常', color: 'text-danger', bg: 'bg-danger', label: 'ERROR' }
    default:
      return { text: '未知', color: 'text-faint', bg: 'bg-faint', label: 'UNKNOWN' }
  }
})
</script>

<template>
  <span
    class="inline-flex items-center gap-1.5 font-medium tabular-nums"
    :class="[meta.color, size === 'md' ? 'text-[13px]' : 'text-[12px]']"
  >
    <span v-if="showDot" class="status-dot shrink-0" :class="[meta.bg, status === 'ONLINE' && 'animate-pulse-dot']" />
    <span>{{ meta.text }}</span>
    <span v-if="size === 'md'" class="text-faint font-mono text-[11px] tracking-wide">{{ meta.label }}</span>
  </span>
</template>
