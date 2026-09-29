<script setup lang="ts">
import { computed, ref } from 'vue'
import { Check, Copy, ExternalLink, MonitorSmartphone, Globe, Star } from 'lucide-vue-next'
import AppIcon from '@/components/AppIcon.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import type { PublicApp } from '@/api/types'
import { toastHelpers } from '@/stores/ui'

// 前台只读应用卡片：仅展示 + 导航，无任何管理操作。
// 卡片带"本地应用 / 非本地应用"类型小标签，帮助用户区分服务归属。
const props = defineProps<{
  app: PublicApp
  favorite: boolean
  allowFavorite: boolean
}>()

const emit = defineEmits<{
  (e: 'open-detail', app: PublicApp): void
  (e: 'toggle-favorite', id: number): void
}>()

const copied = ref('')

const isExternal = computed(() => props.app.type === 'EXTERNAL')
const primary = computed(() => props.app.endpoints?.[0] || null)
const hasAnyURL = computed(() => (props.app.endpoints?.length || 0) > 0)

async function copyText(text: string, label: string) {
  try {
    await navigator.clipboard.writeText(text)
    copied.value = text
    toastHelpers.success('已复制', `${label}：${text}`)
    window.setTimeout(() => {
      if (copied.value === text) copied.value = ''
    }, 1600)
  } catch {
    toastHelpers.error('复制失败', '浏览器拒绝了剪贴板访问')
  }
}

function openURL(url: string) {
  window.open(url, '_blank', 'noopener,noreferrer')
}

function prettyHost(url: string) {
  return url.replace(/^https?:\/\//, '').replace(/\/$/, '')
}

function onCardClick(e: MouseEvent) {
  const target = e.target as HTMLElement
  if (target.closest('[data-no-nav]')) return
  emit('open-detail', props.app)
}
</script>

<template>
  <article
    class="card card-hover relative flex flex-col p-4 sm:p-[18px] cursor-pointer animate-fade-up"
    @click="onCardClick"
  >
    <!-- 头部 -->
    <header class="flex items-start gap-3">
      <div
        class="w-11 h-11 rounded-xl flex items-center justify-center border border-line bg-elevated text-xl shrink-0"
        :style="app.category_color ? { color: app.category_color } : undefined"
      >
        <AppIcon :name="app.icon || app.category_icon || 'Box'" :size="22" />
      </div>
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-1.5 min-w-0">
          <h3 class="font-semibold text-[15px] leading-tight truncate text-ink">{{ app.name }}</h3>
          <span
            class="shrink-0 text-[10.5px] px-1.5 py-0.5 rounded-md border"
            :class="
              isExternal
                ? 'border-info/40 text-info bg-info/8'
                : 'border-ok/40 text-ok bg-ok/8'
            "
          >
            {{ isExternal ? '非本地' : '本地应用' }}
          </span>
        </div>
        <p class="text-[13px] text-muted mt-1 line-clamp-2 leading-snug">
          {{ app.description || '暂无描述' }}
        </p>
      </div>
      <button
        v-if="allowFavorite"
        class="w-7 h-7 rounded-lg flex items-center justify-center shrink-0 transition-colors"
        :class="favorite ? 'text-warn' : 'text-faint hover:text-warn'"
        :title="favorite ? '取消收藏' : '收藏'"
        data-no-nav
        @click.stop="emit('toggle-favorite', app.id)"
      >
        <Star :size="16" :class="favorite && 'fill-warn'" />
      </button>
    </header>

    <!-- 状态 -->
    <div class="mt-3 flex items-center justify-between gap-2">
      <StatusBadge :status="app.status" />
      <span v-if="app.category" class="chip">{{ app.category }}</span>
    </div>

    <!-- 地址（只读 + 复制 + 跳转） -->
    <div class="mt-3 space-y-1.5" data-no-nav>
      <div
        v-for="(ep, idx) in app.endpoints?.slice(0, 2) || []"
        :key="ep.url"
        class="flex items-center gap-2"
      >
        <MonitorSmartphone v-if="!isExternal && idx === 0" :size="13" class="text-faint shrink-0" />
        <Globe v-else :size="13" class="text-faint shrink-0" />
        <span class="text-[11px] text-faint w-9 shrink-0">{{ ep.name }}</span>
        <button
          class="flex-1 min-w-0 text-left text-[12.5px] text-muted hover:text-accent truncate font-mono transition-colors"
          :title="ep.url"
          @click.stop="openURL(ep.url)"
        >
          {{ prettyHost(ep.url) }}
        </button>
        <button
          class="w-6 h-6 rounded-md text-faint hover:text-ink hover:bg-elevated flex items-center justify-center shrink-0 transition-colors"
          :title="`复制${ep.name}地址`"
          @click.stop="copyText(ep.url, ep.name)"
        >
          <Check v-if="copied === ep.url" :size="13" class="text-ok" />
          <Copy v-else :size="13" />
        </button>
      </div>

      <p v-if="!hasAnyURL" class="text-[12px] text-faint py-1">暂无访问地址</p>
    </div>

    <!-- 打开按钮（唯一动作：跳转） -->
    <footer class="mt-4 pt-3 border-t border-line" data-no-nav>
      <button v-if="primary" class="btn btn-sm btn-primary w-full" @click.stop="openURL(primary.url)">
        <ExternalLink :size="14" />
        {{ app.endpoints && app.endpoints.length > 1 ? '打开应用' : isExternal ? '打开' : '进入应用' }}
      </button>
      <div v-else class="text-center text-[12px] text-faint py-1.5">该应用暂不可直接打开</div>
    </footer>
  </article>
</template>
