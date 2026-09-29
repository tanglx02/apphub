<script setup lang="ts">
import { AlertTriangle, Check, Info, X, XCircle } from 'lucide-vue-next'
import { dismissToast, toasts } from '@/stores/ui'

function iconFor(type: string) {
  if (type === 'success') return Check
  if (type === 'error') return XCircle
  if (type === 'warning') return AlertTriangle
  return Info
}
function colorFor(type: string) {
  if (type === 'success') return 'text-ok'
  if (type === 'error') return 'text-danger'
  if (type === 'warning') return 'text-warn'
  return 'text-info'
}
</script>

<template>
  <div class="fixed z-[60] top-4 right-4 left-4 sm:left-auto sm:w-[360px] flex flex-col gap-2 pointer-events-none">
    <TransitionGroup name="toast">
      <div
        v-for="t in toasts"
        :key="t.id"
        class="pointer-events-auto card flex items-start gap-2.5 p-3 pr-2 animate-fade-up"
        :style="{ boxShadow: '0 12px 32px rgb(0 0 0 / 0.16)' }"
      >
        <component :is="iconFor(t.type)" :size="16" class="mt-0.5 shrink-0" :class="colorFor(t.type)" />
        <div class="min-w-0 flex-1">
          <p class="text-[13.5px] font-medium text-ink leading-snug">{{ t.title }}</p>
          <p v-if="t.description" class="text-[12.5px] text-muted mt-0.5 break-words leading-snug">
            {{ t.description }}
          </p>
        </div>
        <button
          class="w-6 h-6 rounded-md text-faint hover:text-ink hover:bg-elevated flex items-center justify-center shrink-0"
          @click="dismissToast(t.id)"
        >
          <X :size="13" />
        </button>
      </div>
    </TransitionGroup>
  </div>
</template>

<style scoped>
.toast-enter-active,
.toast-leave-active {
  transition: all 0.22s cubic-bezier(0.22, 1, 0.36, 1);
}
.toast-enter-from {
  opacity: 0;
  transform: translateX(12px) scale(0.98);
}
.toast-leave-to {
  opacity: 0;
  transform: translateX(12px) scale(0.98);
}
</style>
