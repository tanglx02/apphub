<script setup lang="ts">
import { AlertTriangle } from 'lucide-vue-next'
import { confirmState, resolveConfirm } from '@/stores/ui'
</script>

<template>
  <Transition name="fade">
    <div
      v-if="confirmState.open"
      class="fixed inset-0 z-[70] flex items-center justify-center p-4 bg-black/45 backdrop-blur-[2px]"
      @click.self="resolveConfirm(false)"
    >
      <div class="card w-full max-w-[400px] p-5 animate-fade-up" :style="{ boxShadow: '0 20px 50px rgb(0 0 0 / 0.28)' }">
        <div class="flex items-start gap-3">
          <div
            class="w-9 h-9 rounded-xl flex items-center justify-center shrink-0"
            :class="confirmState.options.danger ? 'bg-danger/12 text-danger' : 'bg-accent/12 text-accent'"
          >
            <AlertTriangle :size="18" />
          </div>
          <div class="min-w-0 flex-1">
            <h3 class="text-[15px] font-semibold text-ink">{{ confirmState.options.title }}</h3>
            <p v-if="confirmState.options.description" class="text-[13px] text-muted mt-1.5 leading-relaxed">
              {{ confirmState.options.description }}
            </p>
          </div>
        </div>
        <div class="mt-5 flex justify-end gap-2">
          <button class="btn btn-md btn-outline" @click="resolveConfirm(false)">
            {{ confirmState.options.cancelText || '取消' }}
          </button>
          <button
            class="btn btn-md"
            :class="confirmState.options.danger ? 'btn-danger' : 'btn-primary'"
            @click="resolveConfirm(true)"
          >
            {{ confirmState.options.confirmText || '确认' }}
          </button>
        </div>
      </div>
    </div>
  </Transition>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.18s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
