import { computed, reactive, ref } from 'vue'
import { api, ApiRequestError } from '@/api/client'
import type { AppItem, Category, Summary } from '@/api/types'
import { toastHelpers } from './ui'

export const apps = reactive<{ items: AppItem[] }>({ items: [] })
export const categories = reactive<{ items: Category[] }>({ items: [] })
export const summary = reactive<Summary>({ total: 0, online: 0, offline: 0, error: 0, rate: 0, updated_at: '' })

export const loading = ref(false)
export const loaded = ref(false)
export const busyIds = reactive<Record<number, string>>({})

let timer: number | null = null
let intervalMs = 5000

export function setRefreshInterval(sec: number) {
  intervalMs = Math.max(3000, sec * 1000)
  restartPolling()
}

export function isBusy(id: number) {
  return !!busyIds[id]
}

function mergeRuntime(next: AppItem[]) {
  const map = new Map(apps.items.map((a) => [a.id, a]))
  for (const item of next) {
    const prev = map.get(item.id)
    if (prev && (prev.runtime.status === 'STARTING' || prev.runtime.status === 'STOPPING')) {
      const s = prev.runtime.status
      // 本地过渡态优先，避免服务端缓存滞后造成闪烁
      if (item.runtime.status === 'ONLINE' && s === 'STOPPING') continue
      if (item.runtime.status === 'OFFLINE' && s === 'STARTING') continue
    }
  }
  apps.items = next
}

export async function refreshApps(silent = true) {
  if (!silent) loading.value = true
  try {
    const res = await api.get<{ apps: AppItem[]; summary: Summary }>('/api/v1/apps')
    mergeRuntime(res.apps)
    Object.assign(summary, res.summary)
    loaded.value = true
  } catch (e) {
    if (!(e instanceof ApiRequestError)) throw e
    if (e.code === 'UNAUTHORIZED') throw e
  } finally {
    loading.value = false
  }
}

export async function refreshCategories() {
  try {
    const res = await api.get<Category[]>('/api/v1/categories')
    categories.items = res
  } catch {
    /* ignore */
  }
}

function startPolling() {
  if (timer !== null) return
  timer = window.setInterval(() => {
    if (document.visibilityState === 'visible') refreshApps()
  }, intervalMs)
}

function stopPolling() {
  if (timer !== null) {
    window.clearInterval(timer)
    timer = null
  }
}

function restartPolling() {
  stopPolling()
  startPolling()
}

export function initAppsPolling(sec?: number) {
  if (sec) intervalMs = Math.max(3000, sec * 1000)
  refreshApps(false)
  refreshCategories()
  restartPolling()
}

export function stopAppsPolling() {
  stopPolling()
}

// ---------- 控制操作 ----------

async function control(id: number, action: 'start' | 'stop' | 'restart') {
  const app = apps.items.find((a) => a.id === id)
  if (!app) return
  if (isBusy(id)) {
    toastHelpers.info(app.name, '操作正在进行中')
    return
  }
  const runtime = apps.items.find((a) => a.id === id)!.runtime
  if (action === 'start' && runtime.status === 'STARTING') {
    toastHelpers.info(app.name, '应用正在启动，请稍候')
    return
  }
  if (action === 'stop' && runtime.status === 'STOPPING') {
    toastHelpers.info(app.name, '应用正在停止，请稍候')
    return
  }

  busyIds[id] = action
  // 乐观更新：立即反馈，不等服务端
  runtime.status = action === 'start' ? 'STARTING' : 'STOPPING'
  try {
    const res = await api.post<{ message: string; status: string }>(`/api/v1/apps/${id}/${action}`)
    toastHelpers.success(app.name, res.message)
  } catch (e) {
    const err = e as ApiRequestError
    toastHelpers.error(`${app.name} ${actionText(action)}失败`, err.message)
  } finally {
    delete busyIds[id]
    // 连续两次刷新：立即 + 延迟，覆盖慢启动应用
    setTimeout(() => refreshApps(), 700)
    setTimeout(() => refreshApps(), 3000)
  }
}

function actionText(action: string) {
  return action === 'start' ? '启动' : action === 'stop' ? '停止' : '重启'
}

export const startApp = (id: number) => control(id, 'start')
export const stopApp = (id: number) => control(id, 'stop')
export const restartApp = (id: number) => control(id, 'restart')

export async function toggleFavorite(id: number) {
  const app = apps.items.find((a) => a.id === id)
  if (!app) return
  try {
    const res = await api.post<{ favorite: boolean }>(`/api/v1/apps/${id}/favorite`)
    app.favorite = res.favorite
    toastHelpers.info(app.name, res.favorite ? '已加入收藏' : '已取消收藏')
  } catch (e) {
    toastHelpers.error('操作失败', (e as ApiRequestError).message)
  }
}

export async function removeApp(id: number) {
  await api.del(`/api/v1/apps/${id}`)
  apps.items = apps.items.filter((a) => a.id !== id)
}

// ---------- 派生数据 ----------

export const visibleApps = computed(() => apps.items.filter((a) => a.enabled !== false))
export const favoriteApps = computed(() => apps.items.filter((a) => a.favorite))

export function appById(id: number) {
  return apps.items.find((a) => a.id === id)
}
