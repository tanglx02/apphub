<script setup lang="ts">
import { X } from 'lucide-vue-next'

defineProps<{
  open: boolean
  title: string
  subtitle?: string
  width?: string
}>()
const emit = defineEmits<{ (e: 'close'): void }>()
</script>

<template>
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="open"
        class="fixed inset-0 z-[65] flex items-end sm:items-center justify-center bg-black/45 backdrop-blur-[2px]"
        @click.self="emit('close')"
      >
        <div
          class="card w-full max-h-[92vh] flex flex-col animate-fade-up rounded-b-none sm:rounded-2xl"
          :style="{ maxWidth: width || '640px', boxShadow: '0 20px 50px rgb(0 0 0 / 0.28)' }"
        >
          <header class="flex items-start justify-between gap-3 px-5 py-4 border-b border-line shrink-0">
            <div class="min-w-0">
              <h2 class="text-[15px] font-semibold text-ink">{{ title }}</h2>
              <p v-if="subtitle" class="text-[12.5px] text-muted mt-0.5">{{ subtitle }}</p>
            </div>
            <button
              class="w-8 h-8 rounded-lg text-faint hover:text-ink hover:bg-elevated flex items-center justify-center shrink-0"
              @click="emit('close')"
            >
              <X :size="16" />
            </button>
          </header>
          <div class="flex-1 overflow-y-auto px-5 py-4">
            <slot />
          </div>
          <footer v-if="$slots.footer" class="px-5 py-3.5 border-t border-line flex justify-end gap-2 shrink-0">
            <slot name="footer" />
          </footer>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.18s ease;
}
.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}
</style>
