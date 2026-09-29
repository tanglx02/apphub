<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Copy, ExternalLink, MoreVertical, Pencil, RotateCw, Play, Square, Star, Trash2 } from 'lucide-vue-next'
import AppIcon from './AppIcon.vue'
import StatusBadge from './StatusBadge.vue'
import type { AppItem } from '@/api/types'
import { busyIds, isBusy, restartApp, startApp, stopApp, toggleFavorite } from '@/stores/apps'
import { toastHelpers } from '@/stores/ui'

const props = defineProps<{
  app: AppItem
  selectable?: boolean
  selected?: boolean
}>()

const emit = defineEmits<{
  (e: 'toggle-select', id: number): void
  (e: 'edit', app: AppItem): void
  (e: 'delete', app: AppItem): void
}>()

const router = useRouter()
const menuOpen = ref(false)

const busy = computed(() => isBusy(props.app.id))
const action = computed(() => busyIds[props.app.id] || '')
const running = computed(() => props.app.runtime.status === 'ONLINE')

async function copyText(text: string, label: string) {
  try {
    await navigator.clipboard.writeText(text)
    toastHelpers.success('已复制', `${label}：${text}`)
  } catch {
    toastHelpers.error('复制失败', '浏览器拒绝了剪贴板访问')
  }
}

function openDetail() {
  router.push({ name: 'app-detail', params: { id: props.app.id } })
}

function openUrl(url: string) {
  window.open(url, '_blank', 'noopener,noreferrer')
}

function onCardClick(e: MouseEvent) {
  const target = e.target as HTMLElement
  if (target.closest('[data-no-nav]')) return
  openDetail()
}

function closeMenu() {
  menuOpen.value = false
}
</script>

<template>
  <article
    class="card card-hover relative flex flex-col p-4 sm:p-[18px] cursor-pointer group animate-fade-up"
    @click="onCardClick"
    @mouseleave="menuOpen = false"
  >
    <!-- 头部：图标 + 名称 + 状态 -->
    <header class="flex items-start gap-3">
      <div class="relative shrink-0">
        <div
          class="w-11 h-11 rounded-xl flex items-center justify-center border border-line bg-elevated text-xl"
          :style="app.category_color ? { color: app.category_color } : undefined"
        >
          <AppIcon :name="app.icon || app.category_icon || 'Box'" :size="22" />
        </div>
        <span
          v-if="app.favorite"
          class="absolute -top-1.5 -right-1.5 w-4 h-4 rounded-full bg-warn/15 flex items-center justify-center"
        >
          <Star :size="10" class="text-warn fill-warn" />
        </span>
      </div>

      <div class="min-w-0 flex-1">
        <div class="flex items-start justify-between gap-2">
          <h3 class="font-semibold text-[15px] leading-tight truncate text-ink">{{ app.name }}</h3>
          <div class="flex items-center gap-1 shrink-0" data-no-nav>
            <button
              v-if="selectable"
              class="w-5 h-5 rounded-md border flex items-center justify-center transition-colors"
              :class="selected ? 'bg-accent border-accent text-white' : 'border-line hover:border-accent'"
              @click.stop="emit('toggle-select', app.id)"
            >
              <svg v-if="selected" viewBox="0 0 12 12" class="w-3 h-3"><path d="M2 6l3 3 5-6" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>
            </button>
            <button
              class="w-7 h-7 rounded-lg text-faint hover:text-ink hover:bg-elevated flex items-center justify-center transition-colors"
              title="更多操作"
              @click.stop="menuOpen = !menuOpen"
            >
              <MoreVertical :size="16" />
            </button>
          </div>
        </div>
        <p class="text-[13px] text-muted mt-1 line-clamp-2 leading-snug">
          {{ app.description || '暂无描述' }}
        </p>
      </div>
    </header>

    <!-- 状态栏 -->
    <div class="mt-3.5 flex items-center justify-between gap-2">
      <StatusBadge :status="app.runtime.status" />
      <div class="flex items-center gap-2 text-[11px] text-faint">
        <span v-if="app.category_name" class="chip">{{ app.category_name }}</span>
        <span v-if="app.app_type === 'systemd'" class="chip font-mono">systemd</span>
        <span v-else-if="app.shell_mode" class="chip font-mono text-warn">shell</span>
      </div>
    </div>

    <!-- 地址 -->
    <div class="mt-3 space-y-1.5" data-no-nav>
      <div v-if="app.internal_url" class="flex items-center gap-2 group/link">
        <span class="text-[11px] text-faint w-8 shrink-0">内网</span>
        <button
          class="flex-1 min-w-0 text-left text-[12.5px] text-muted hover:text-accent truncate font-mono transition-colors"
          :title="app.internal_url"
          @click.stop="openUrl(app.internal_url)"
        >
          {{ app.internal_url }}
        </button>
        <button
          class="w-6 h-6 rounded-md text-faint hover:text-ink hover:bg-elevated flex items-center justify-center shrink-0 transition-colors"
          title="复制内网地址"
          @click.stop="copyText(app.internal_url, '内网地址')"
        >
          <Copy :size="13" />
        </button>
        <button
          class="w-6 h-6 rounded-md text-faint hover:text-accent hover:bg-elevated flex items-center justify-center shrink-0 transition-colors"
          title="新标签页打开"
          @click.stop="openUrl(app.internal_url)"
        >
          <ExternalLink :size="13" />
        </button>
      </div>

      <div v-if="app.external_url" class="flex items-center gap-2">
        <span class="text-[11px] text-faint w-8 shrink-0">公网</span>
        <button
          class="flex-1 min-w-0 text-left text-[12.5px] text-muted hover:text-accent truncate font-mono transition-colors"
          :title="app.external_url"
          @click.stop="openUrl(app.external_url)"
        >
          {{ app.external_url }}
        </button>
        <button
          class="w-6 h-6 rounded-md text-faint hover:text-ink hover:bg-elevated flex items-center justify-center shrink-0 transition-colors"
          title="复制公网地址"
          @click.stop="copyText(app.external_url, '公网地址')"
        >
          <Copy :size="13" />
        </button>
        <button
          class="w-6 h-6 rounded-md text-faint hover:text-accent hover:bg-elevated flex items-center justify-center shrink-0 transition-colors"
          title="新标签页打开"
          @click.stop="openUrl(app.external_url)"
        >
          <ExternalLink :size="13" />
        </button>
      </div>

      <div v-if="!app.internal_url && !app.external_url" class="text-[12px] text-faint py-1">
        未配置访问地址
      </div>
    </div>

    <!-- 错误提示 -->
    <p
      v-if="app.runtime.status === 'ERROR' && app.runtime.detail"
      class="mt-2 text-[12px] text-danger line-clamp-2"
    >
      {{ app.runtime.detail }}
    </p>

    <!-- 操作区 -->
    <footer class="mt-4 pt-3 border-t border-line flex items-center gap-2" data-no-nav>
      <button
        v-if="!running && !busy"
        class="btn btn-sm btn-primary flex-1"
        :disabled="busy"
        @click.stop="startApp(app.id)"
      >
        <Play :size="14" />
        启动
      </button>
      <button
        v-else-if="busy"
        class="btn btn-sm btn-outline flex-1"
        disabled
      >
        <span class="w-3.5 h-3.5 rounded-full border-2 border-current border-t-transparent animate-spin" />
        {{ action === 'start' ? '启动中' : action === 'stop' ? '停止中' : '重启中' }}
      </button>
      <template v-else>
        <button
          v-if="app.internal_url || app.external_url"
          class="btn btn-sm btn-primary flex-1"
          @click.stop="openUrl(app.internal_url || app.external_url)"
        >
          <ExternalLink :size="14" />
          打开
        </button>
        <button class="btn btn-sm btn-outline" title="停止" @click.stop="stopApp(app.id)">
          <Square :size="13" />
        </button>
        <button class="btn btn-sm btn-outline" title="重启" @click.stop="restartApp(app.id)">
          <RotateCw :size="13" />
        </button>
      </template>
    </footer>

    <!-- 下拉菜单 -->
    <div
      v-if="menuOpen"
      class="absolute right-3 top-12 w-44 card py-1 z-40 animate-fade-in"
      :style="{ boxShadow: '0 12px 32px rgb(0 0 0 / 0.16)' }"
      @click.stop="closeMenu"
    >
        <button class="w-full px-3 h-9 flex items-center gap-2 text-[13px] text-ink hover:bg-elevated" @click="toggleFavorite(app.id)">
          <Star :size="14" />{{ app.favorite ? '取消收藏' : '收藏应用' }}
        </button>
        <button class="w-full px-3 h-9 flex items-center gap-2 text-[13px] text-ink hover:bg-elevated" @click="router.push({ name: 'app-detail', params: { id: app.id } })">
          <ExternalLink :size="14" />查看详情
        </button>
        <button class="w-full px-3 h-9 flex items-center gap-2 text-[13px] text-ink hover:bg-elevated" @click="emit('edit', app)">
          <Pencil :size="14" />编辑应用
        </button>
        <div class="divider my-1" />
        <button class="w-full px-3 h-9 flex items-center gap-2 text-[13px] text-danger hover:bg-danger/10" @click="emit('delete', app)">
          <Trash2 :size="14" />删除应用
        </button>
    </div>
  </article>
</template>
