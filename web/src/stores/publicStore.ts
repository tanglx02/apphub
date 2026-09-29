import { computed, reactive, ref } from 'vue'
import { api } from '@/api/client'
import type { PublicApp, PublicConfig, PublicSummary, Category } from '@/api/types'

// 前台只读导航数据源：仅调用 /api/v1/public/* 只读接口

interface PublicState {
  loading: boolean
  loaded: boolean
  disabled: boolean
  config: PublicConfig
  summary: PublicSummary
  apps: PublicApp[]
  categories: Category[]
}

export const pub = reactive<PublicState>({
  loading: false,
  loaded: false,
  disabled: false,
  config: {
    enabled: true,
    title: '我的服务器',
    subtitle: '应用导航',
    show_resources: true,
    show_categories: true,
    show_search: true,
    allow_favorite: true,
    default_theme: 'system',
  },
  summary: { total: 0, local_apps: 0, external_apps: 0, online: 0, offline: 0, error: 0, starting: 0, online_rate: 0, server_up: true, updated_at: '' },
  apps: [],
  categories: [],
})

export const publicLoading = ref(false)

// ---------- 收藏（仅浏览器本地，不写数据库） ----------

const FAV_KEY = 'apphub.public.favorites'
const favorites = reactive<Set<number>>(new Set(loadFavorites()))

function loadFavorites(): number[] {
  try {
    const raw = localStorage.getItem(FAV_KEY)
    if (!raw) return []
    const arr = JSON.parse(raw)
    return Array.isArray(arr) ? arr.filter((n) => typeof n === 'number') : []
  } catch {
    return []
  }
}

export function isFavorite(id: number) {
  return favorites.has(id)
}

export function toggleFavorite(id: number) {
  if (favorites.has(id)) favorites.delete(id)
  else favorites.add(id)
  try {
    localStorage.setItem(FAV_KEY, JSON.stringify([...favorites]))
  } catch {
    /* 隐私模式忽略 */
  }
}

export const favoriteIds = computed(() => [...favorites])

// ---------- 数据加载 ----------

export async function loadOverview(silent = true) {
  if (!silent) publicLoading.value = true
  try {
    const res = await api.get<{
      config: PublicConfig
      summary: PublicSummary
      apps: PublicApp[]
      categories: Category[]
    }>('/api/v1/public/overview')
    pub.config = res.config
    pub.summary = res.summary
    pub.apps = res.apps
    pub.categories = res.categories
    pub.disabled = false
    pub.loaded = true
  } catch (e) {
    const err = e as { code?: string }
    if (err.code === 'PUBLIC_DISABLED') pub.disabled = true
    // 其他错误保持已有数据，避免网络抖动清空页面
  } finally {
    publicLoading.value = false
  }
}

let timer: number | null = null

export function startPublicPolling(intervalMs = 5000) {
  stopPublicPolling()
  timer = window.setInterval(() => {
    if (document.visibilityState === 'visible') loadOverview()
  }, Math.max(3000, intervalMs))
}

export function stopPublicPolling() {
  if (timer !== null) {
    window.clearInterval(timer)
    timer = null
  }
}
