<script setup lang="ts">
defineProps<{ count?: number }>()
</script>

<template>
  <div class="grid gap-3.5 sm:gap-4 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5">
    <div v-for="i in count || 6" :key="i" class="card p-4 sm:p-[18px]">
      <div class="flex items-start gap-3">
        <div class="skeleton w-11 h-11 rounded-xl shrink-0" />
        <div class="flex-1 space-y-2 pt-1">
          <div class="skeleton h-3.5 w-2/3" />
          <div class="skeleton h-3 w-full" />
        </div>
      </div>
      <div class="mt-4 flex items-center justify-between">
        <div class="skeleton h-3 w-16" />
        <div class="skeleton h-4 w-14 rounded-lg" />
      </div>
      <div class="mt-3 space-y-2">
        <div class="skeleton h-3 w-full" />
        <div class="skeleton h-3 w-4/5" />
      </div>
      <div class="mt-4 pt-3 border-t border-line">
        <div class="skeleton h-8 w-full rounded-xl" />
      </div>
    </div>
  </div>
</template>
