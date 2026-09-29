<script setup lang="ts">
import { computed } from 'vue'
import { Copy, ExternalLink } from 'lucide-vue-next'
import AppIcon from '@/components/AppIcon.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import BaseModal from '@/components/BaseModal.vue'
import type { PublicApp } from '@/api/types'
import { toastHelpers } from '@/stores/ui'

// 前台轻量详情弹窗：仅查看与打开应用，无任何管理操作
const props = defineProps<{
  open: boolean
  app: PublicApp | null
}>()
const emit = defineEmits<{ (e: 'close'): void }>()

const isExternal = computed(() => props.app?.type === 'EXTERNAL')

const tags = computed(() =>
  (props.app?.tags || '')
    .split(/[,，;；]/)
    .map((t) => t.trim())
    .filter(Boolean),
)

async function copyText(text: string, label: string) {
  try {
    await navigator.clipboard.writeText(text)
    toastHelpers.success('已复制', `${label}：${text}`)
  } catch {
    toastHelpers.error('复制失败')
  }
}

function openURL(url: string) {
  window.open(url, '_blank', 'noopener,noreferrer')
}
</script>

<template>
  <BaseModal :open="open" :title="app?.name || ''" width="480px" @close="emit('close')">
    <div v-if="app">
      <div class="flex items-start gap-3.5">
        <div
          class="w-14 h-14 rounded-2xl bg-elevated border border-line flex items-center justify-center text-2xl shrink-0"
          :style="app.category_color ? { color: app.category_color } : undefined"
        >
          <AppIcon :name="app.icon || app.category_icon || 'Box'" :size="26" />
        </div>
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2 flex-wrap">
            <StatusBadge :status="app.status" size="md" />
            <span
              class="text-[11px] px-1.5 py-0.5 rounded-md border"
              :class="isExternal ? 'border-info/40 text-info bg-info/8' : 'border-ok/40 text-ok bg-ok/8'"
            >
              {{ isExternal ? '非本地应用' : '本地应用' }}
            </span>
          </div>
          <p class="text-[13.5px] text-muted mt-2 leading-relaxed">{{ app.description || '暂无描述' }}</p>
          <div v-if="tags.length" class="flex flex-wrap gap-1.5 mt-2.5">
            <span v-for="t in tags" :key="t" class="chip">#{{ t }}</span>
          </div>
        </div>
      </div>

      <div class="divider my-4" />

      <div class="space-y-3">
        <div v-for="ep in app.endpoints || []" :key="ep.url" class="rounded-xl border border-line p-3.5">
          <div class="flex items-center justify-between gap-2">
            <span class="text-[12.5px] text-faint">{{ ep.name }}</span>
            <button class="btn btn-sm btn-ghost" :title="`复制${ep.name}地址`" @click="copyText(ep.url, ep.name)">
              <Copy :size="13" />
            </button>
          </div>
          <a
            :href="ep.url"
            target="_blank"
            rel="noopener noreferrer"
            class="block mt-1 font-mono text-[13px] text-accent hover:underline break-all"
          >
            {{ ep.url }}
          </a>
          <button class="btn btn-sm btn-primary mt-2.5 w-full" @click="openURL(ep.url)">
            <ExternalLink :size="14" />打开{{ ep.name }}
          </button>
        </div>

        <p v-if="!app.endpoints?.length" class="text-[13px] text-faint text-center py-4">暂无访问地址</p>
      </div>

      <p v-if="!isExternal" class="text-[11.5px] text-faint mt-4 text-center">
        如需启动 / 停止等管理操作，请进入管理后台
      </p>
      <p v-else class="text-[11.5px] text-faint mt-4 text-center">
        非本地应用仅提供导航与在线状态，由外部环境自行运行
      </p>
    </div>
    <template #footer>
      <button class="btn btn-md btn-outline" @click="emit('close')">关闭</button>
    </template>
  </BaseModal>
</template>
