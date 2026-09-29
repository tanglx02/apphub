<script setup lang="ts">
import AppIcon from './AppIcon.vue'

withDefaults(
  defineProps<{
    icon?: string
    title: string
    description?: string
    actionText?: string
  }>(),
  { icon: 'Box' },
)
const emit = defineEmits<{ (e: 'action'): void }>()
</script>

<template>
  <div class="flex flex-col items-center justify-center py-16 px-6 text-center animate-fade-in">
    <div class="w-14 h-14 rounded-2xl bg-elevated border border-line flex items-center justify-center text-faint mb-4">
      <AppIcon :name="icon" :size="24" />
    </div>
    <h3 class="text-[15px] font-semibold text-ink">{{ title }}</h3>
    <p v-if="description" class="text-[13px] text-muted mt-1.5 max-w-[380px] leading-relaxed">
      {{ description }}
    </p>
    <button v-if="actionText" class="btn btn-md btn-primary mt-5" @click="emit('action')">
      {{ actionText }}
    </button>
  </div>
</template>
