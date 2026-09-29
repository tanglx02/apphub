<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Search, Settings, X, Package, CheckCheck, Star } from 'lucide-vue-next'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import SkeletonGrid from '@/components/SkeletonGrid.vue'
import PublicAppCard from '@/components/public/PublicAppCard.vue'
import PublicServerOverview from '@/components/public/PublicServerOverview.vue'
import PublicAppModal from '@/components/public/PublicAppModal.vue'
import AppIcon from '@/components/AppIcon.vue'
import type { PublicApp } from '@/api/types'
import {
  pub,
  publicLoading,
  isFavorite,
  toggleFavorite,
  loadOverview,
} from '@/stores/publicStore'
import { theme, setTheme } from '@/stores/ui'

// 前台只读导航：只查看 / 搜索 / 分类 / 收藏（本地）/ 打开应用。
// 不提供任何管理操作；管理入口在右上角，不与内容竞争注意力。

const query = ref('')
const scopeFilter = ref<'all' | 'LOCAL' | 'EXTERNAL'>('all')
const activeCategory = ref<'all' | 'fav' | number>('all')
const sortBy = ref<'manual' | 'name' | 'status' | 'category'>('manual')
const detailOpen = ref(false)
const detailApp = ref<PublicApp | null>(null)

const statusWeight: Record<string, number> = {
  ONLINE: 0,
  STARTING: 1,
  STOPPING: 2,
  ERROR: 3,
  UNKNOWN: 4,
  OFFLINE: 5,
}

const filtered = computed(() => {
  let list = [...pub.apps]
  const q = query.value.trim().toLowerCase()
  if (q) {
    list = list.filter(
      (a) =>
        a.name.toLowerCase().includes(q) ||
        (a.description || '').toLowerCase().includes(q) ||
        (a.tags || '').toLowerCase().includes(q) ||
        (a.category || '').toLowerCase().includes(q) ||
        // 类型中文关键词搜索：输入"本地"匹配本地应用，"非本地"匹配非本地应用
        (q === '本地' && a.type === 'LOCAL') ||
        (q === '非本地' && a.type === 'EXTERNAL'),
    )
  }
  if (scopeFilter.value !== 'all') {
    list = list.filter((a) => a.type === scopeFilter.value)
  }
  if (activeCategory.value === 'fav') {
    list = list.filter((a) => isFavorite(a.id))
  } else if (activeCategory.value !== 'all') {
    list = list.filter((a) => (a.category || '') === categoryName(activeCategory.value as number))
  }

  if (sortBy.value === 'name') {
    list.sort((a, b) => a.name.localeCompare(b.name, 'zh-CN'))
  } else if (sortBy.value === 'status') {
    list.sort(
      (a, b) =>
        (statusWeight[a.status] ?? 9) - (statusWeight[b.status] ?? 9) || a.sort_order - b.sort_order,
    )
  } else if (sortBy.value === 'category') {
    list.sort((a, b) => (a.category || '~').localeCompare(b.category || '~', 'zh-CN') || a.sort_order - b.sort_order)
  } else {
    list.sort((a, b) => a.sort_order - b.sort_order)
  }
  return list
})

function categoryName(id: number) {
  return pub.categories.find((c) => c.id === id)?.name || ''
}

function openDetail(app: PublicApp) {
  detailApp.value = app
  detailOpen.value = true
}

// 主题初始化：跟随配置默认值（用户手动切换后写入 localStorage 覆盖）
onMounted(() => {
  loadOverview(false)
  if (!localStorage.getItem('apphub.theme') && pub.config.default_theme) {
    const t = pub.config.default_theme
    if (t === 'light' || t === 'dark' || t === 'system') setTheme(t)
  }
})
onUnmounted(() => {
  // 轮询由路由守卫统一管理
})

// 供模板使用的主题状态（避免未使用告警同时支持响应式切换）
const themeRef = theme
void themeRef
void setTheme
</script>

<template>
  <div class="min-h-full bg-canvas">
    <!-- 顶部 -->
    <header
      class="h-14 sticky top-0 z-30 border-b border-line bg-surface/85 backdrop-blur flex items-center gap-3 px-4 sm:px-6"
    >
      <div class="flex items-center gap-2.5 min-w-0">
        <div class="w-8 h-8 rounded-xl bg-accent text-white flex items-center justify-center shrink-0">
          <Package :size="16" />
        </div>
        <div class="min-w-0">
          <p class="text-[15px] font-semibold text-ink leading-tight truncate">
            {{ pub.config.title || 'AppHub' }}
          </p>
          <p class="text-[11px] text-faint leading-tight truncate hidden sm:block">
            {{ pub.config.subtitle || '我的服务器应用' }}
          </p>
        </div>
      </div>

      <!-- 搜索 -->
      <div v-if="pub.config.show_search" class="flex-1 max-w-[420px] ml-auto relative">
        <Search :size="15" class="absolute left-3 top-1/2 -translate-y-1/2 text-faint pointer-events-none" />
        <input v-model="query" class="input h-9 pl-9 pr-8 text-[13px]" placeholder="搜索应用…" />
        <button v-if="query" class="absolute right-2.5 top-1/2 -translate-y-1/2 text-faint hover:text-ink" @click="query = ''">
          <X :size="14" />
        </button>
      </div>
      <div v-else class="flex-1" />

      <ThemeSwitch />

      <!-- 后台入口（不突出，右上角） -->
      <router-link
        to="/admin"
        class="btn btn-sm btn-outline shrink-0"
        title="管理后台"
      >
        <Settings :size="14" />
        <span class="hidden sm:inline">管理后台</span>
      </router-link>
    </header>

    <main class="px-4 sm:px-6 lg:px-7 py-5 sm:py-6 max-w-[1680px] mx-auto pb-16">
      <!-- 前台已停用 -->
      <div v-if="pub.disabled" class="card p-12 flex flex-col items-center text-center mt-10">
        <div class="w-14 h-14 rounded-2xl bg-elevated border border-line flex items-center justify-center text-faint mb-4">
          <Settings :size="24" />
        </div>
        <h2 class="text-[16px] font-semibold text-ink">前台导航未启用</h2>
        <p class="text-[13px] text-muted mt-2">管理员可在后台「系统设置 → 前台导航」中开启。</p>
        <router-link to="/admin" class="btn btn-md btn-primary mt-5">进入管理后台</router-link>
      </div>

      <template v-else>
        <!-- 服务器概览 -->
        <PublicServerOverview :config="pub.config" :summary="pub.summary" />

        <!-- 类型筛选 -->
        <section v-if="pub.apps.length" class="mt-5 flex items-center gap-1.5 flex-wrap">
          <button
            class="chip h-7 px-2.5 transition-colors"
            :class="scopeFilter === 'all' ? '!bg-accent !text-white !border-accent' : 'hover:text-ink'"
            @click="scopeFilter = 'all'"
          >
            全部 {{ pub.summary.total }}
          </button>
          <button
            class="chip h-7 px-2.5 transition-colors"
            :class="scopeFilter === 'LOCAL' ? '!bg-ok !text-white !border-ok' : 'hover:text-ink'"
            @click="scopeFilter = 'LOCAL'"
          >
            本地应用 {{ pub.summary.local_apps }}
          </button>
          <button
            class="chip h-7 px-2.5 transition-colors"
            :class="scopeFilter === 'EXTERNAL' ? '!bg-info !text-white !border-info' : 'hover:text-ink'"
            @click="scopeFilter = 'EXTERNAL'"
          >
            非本地 {{ pub.summary.external_apps }}
          </button>
        </section>

        <!-- 分类 + 排序 -->
        <section
          v-if="pub.config.show_categories && pub.categories.length"
          class="mt-3 flex items-center gap-2 flex-wrap"
        >
          <div class="flex-1 min-w-0 flex items-center gap-1.5 overflow-x-auto no-scrollbar py-0.5">
            <button
              class="chip shrink-0 h-7 px-2.5 transition-colors"
              :class="activeCategory === 'all' ? '!bg-accent !text-white !border-accent' : 'hover:text-ink'"
              @click="activeCategory = 'all'"
            >
              全部
            </button>
            <button
              v-if="pub.config.allow_favorite"
              class="chip shrink-0 h-7 px-2.5 transition-colors"
              :class="activeCategory === 'fav' ? '!bg-warn !text-white !border-warn' : 'hover:text-ink'"
              @click="activeCategory = 'fav'"
            >
              <Star :size="11" />收藏
            </button>
            <button
              v-for="c in pub.categories"
              :key="c.id"
              class="chip shrink-0 h-7 px-2.5 transition-colors"
              :class="activeCategory === c.id ? '!bg-accent !text-white !border-accent' : 'hover:text-ink'"
              @click="activeCategory = c.id"
            >
              <span class="w-1.5 h-1.5 rounded-full" :style="{ background: c.color || 'currentColor' }" />
              {{ c.name }}
            </button>
          </div>

          <select
            v-model="sortBy"
            class="h-8 rounded-lg bg-elevated border border-line text-[12px] text-ink px-2 outline-none shrink-0"
          >
            <option value="manual">默认排序</option>
            <option value="name">按名称</option>
            <option value="status">按状态</option>
            <option value="category">按分类</option>
          </select>
        </section>

        <!-- 应用网格 -->
        <section class="mt-4">
          <SkeletonGrid v-if="publicLoading && !pub.loaded" :count="6" />

          <!-- 空状态 -->
          <div v-else-if="pub.loaded && !pub.apps.length" class="card py-16 flex flex-col items-center text-center mt-2">
            <div class="w-14 h-14 rounded-2xl bg-elevated border border-line flex items-center justify-center text-faint mb-4">
              <AppIcon name="Package" :size="24" />
            </div>
            <h3 class="text-[15px] font-semibold text-ink">还没有配置应用</h3>
            <p class="text-[13px] text-muted mt-1.5">请进入管理后台添加应用。</p>
            <router-link to="/admin" class="btn btn-md btn-primary mt-5">进入管理后台</router-link>
          </div>

          <!-- 筛选无结果 -->
          <div v-else-if="!filtered.length" class="card py-14 flex flex-col items-center justify-center text-center">
            <Search :size="22" class="text-faint mb-3" />
            <p class="text-[14px] font-medium text-ink">没有匹配的应用</p>
            <button
              class="btn btn-sm btn-outline mt-4"
              @click="query = ''; activeCategory = 'all'"
            >
              <X :size="14" />清除筛选
            </button>
          </div>

          <div
            v-else
            class="grid gap-3.5 sm:gap-4 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5"
          >
            <PublicAppCard
              v-for="app in filtered"
              :key="app.id"
              :app="app"
              :favorite="isFavorite(app.id)"
              :allow-favorite="pub.config.allow_favorite"
              @open-detail="openDetail"
              @toggle-favorite="toggleFavorite"
            />
          </div>
        </section>

        <!-- 底部提示 -->
        <p
          v-if="pub.apps.length"
          class="text-center text-[11.5px] text-faint mt-8 flex items-center justify-center gap-1.5"
        >
          <CheckCheck :size="12" />
          状态每 5 秒自动刷新 · 收藏仅保存在本设备浏览器中
        </p>
      </template>
    </main>

    <!-- 详情弹窗（仅查看 + 打开应用） -->
    <PublicAppModal :open="detailOpen" :app="detailApp" @close="detailOpen = false" />
  </div>
</template>
