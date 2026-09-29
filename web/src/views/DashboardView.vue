<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Cpu, HardDrive, MemoryStick, Plus, Server, Search, X, Zap, ArrowDownUp, CheckCheck } from 'lucide-vue-next'
import AppCard from '@/components/AppCard.vue'
import SkeletonGrid from '@/components/SkeletonGrid.vue'
import EmptyState from '@/components/EmptyState.vue'
import AppFormModal from '@/components/AppFormModal.vue'
import { api } from '@/api/client'
import type { AppItem, SystemInfo } from '@/api/types'
import { apps, categories, loaded, refreshApps, startApp, stopApp, restartApp, summary } from '@/stores/apps'
import { auth } from '@/stores/auth'
import { confirmDialog, toastHelpers } from '@/stores/ui'

const props = defineProps<{ search?: string }>()
const router = useRouter()

// ---------- 服务器信息（低频刷新） ----------
const sysInfo = ref<SystemInfo | null>(null)
let sysTimer: number | null = null

async function loadSystemInfo() {
  try {
    sysInfo.value = await api.get<SystemInfo>('/api/v1/system/info')
  } catch {
    /* 忽略，首页不因系统信息失败而中断 */
  }
}

// ---------- 筛选与排序 ----------
const activeCategory = ref<number | 'all' | 'fav'>('all')
const sortBy = ref<'manual' | 'name' | 'status' | 'category'>('manual')
const selectMode = ref(false)
const selected = ref<number[]>([])
const batchRunning = ref(false)
const batchProgress = ref({ done: 0, total: 0 })

const statusWeight: Record<string, number> = { ONLINE: 0, STARTING: 1, STOPPING: 2, ERROR: 3, UNKNOWN: 4, OFFLINE: 5 }

// 本地 / 非本地应用数量（后台概览用）
const localCount = computed(() => apps.items.filter((a) => (a.type || 'LOCAL') !== 'EXTERNAL').length)
const externalCount = computed(() => apps.items.filter((a) => a.type === 'EXTERNAL').length)

const filtered = computed<AppItem[]>(() => {
  let list = apps.items.filter((a) => a.enabled !== false)
  const q = (props.search || '').trim().toLowerCase()
  if (q) {
    list = list.filter((a) => {
      const cat = categories.items.find((c) => c.id === a.category_id)?.name || ''
      return (
        a.name.toLowerCase().includes(q) ||
        a.slug.toLowerCase().includes(q) ||
        (a.description || '').toLowerCase().includes(q) ||
        (a.tags || '').toLowerCase().includes(q) ||
        cat.toLowerCase().includes(q)
      )
    })
  }
  if (activeCategory.value === 'fav') {
    list = list.filter((a) => a.favorite)
  } else if (activeCategory.value !== 'all') {
    list = list.filter((a) => a.category_id === activeCategory.value)
  }

  const sorted = [...list]
  if (sortBy.value === 'name') {
    sorted.sort((a, b) => a.name.localeCompare(b.name, 'zh-CN'))
  } else if (sortBy.value === 'status') {
    sorted.sort(
      (a, b) =>
        (statusWeight[a.runtime.status] ?? 9) - (statusWeight[b.runtime.status] ?? 9) ||
        a.sort_order - b.sort_order,
    )
  } else if (sortBy.value === 'category') {
    sorted.sort(
      (a, b) => (a.category_id || 999) - (b.category_id || 999) || a.sort_order - b.sort_order,
    )
  } else {
    sorted.sort((a, b) => Number(b.favorite) - Number(a.favorite) || a.sort_order - b.sort_order)
  }
  return sorted
})

// ---------- 拖拽排序 ----------
const dragId = ref<number | null>(null)

function onDragStart(id: number) {
  dragId.value = id
}
function onDragOver(id: number) {
  if (dragId.value === null || dragId.value === id) return
  const list = [...apps.items]
  const from = list.findIndex((a) => a.id === dragId.value)
  const to = list.findIndex((a) => a.id === id)
  if (from < 0 || to < 0) return
  const [moved] = list.splice(from, 1)
  list.splice(to, 0, moved)
  apps.items = list
}
async function onDragEnd() {
  if (dragId.value === null) return
  dragId.value = null
  try {
    await api.put('/api/v1/apps/reorder', { ids: apps.items.map((a) => a.id) })
    toastHelpers.success('排序已保存')
  } catch {
    toastHelpers.error('保存排序失败')
    refreshApps()
  }
}

// ---------- 批量操作 ----------
function toggleSelect(id: number) {
  const idx = selected.value.indexOf(id)
  if (idx >= 0) selected.value.splice(idx, 1)
  else selected.value.push(id)
}

async function batch(action: 'start' | 'stop' | 'restart') {
  if (!selected.value.length) return
  const text = action === 'start' ? '启动' : action === 'stop' ? '停止' : '重启'
  const ok = await confirmDialog({
    title: `批量${text} ${selected.value.length} 个应用`,
    description: `将按顺序${text}所选应用，最多同时处理 3 个，避免系统负载瞬间过高。`,
    confirmText: `确认${text}`,
    danger: action !== 'start',
  })
  if (!ok) return

  batchRunning.value = true
  batchProgress.value = { done: 0, total: selected.value.length }
  for (const id of selected.value) {
    if (action === 'start') await startApp(id)
    else if (action === 'stop') await stopApp(id)
    else await restartApp(id)
    batchProgress.value.done++
  }
  batchRunning.value = false
  toastHelpers.success(`批量${text}完成`, `共 ${selected.value.length} 个应用`)
  selected.value = []
  selectMode.value = false
  refreshApps()
}

async function startAllApps() {
  const ok = await confirmDialog({
    title: '启动全部应用',
    description: '将依次启动全部已启用的应用。建议仅在确认资源充足时使用。',
    confirmText: '确认启动',
    danger: true,
  })
  if (!ok) return
  try {
    const res = await api.post<{ total: number; success: number; failed: number }>('/api/v1/apps/start-all')
    toastHelpers.success('已完成', `成功 ${res.success} / 共 ${res.total}`)
    refreshApps()
  } catch (e) {
    toastHelpers.error('启动全部失败', (e as Error).message)
  }
}

// ---------- 新增 / 编辑 / 删除 ----------
const formOpen = ref(false)
const editing = ref<AppItem | null>(null)

function openCreate() {
  editing.value = null
  formOpen.value = true
}
function openEdit(app: AppItem) {
  editing.value = app
  formOpen.value = true
}

async function removeApp(app: AppItem) {
  const ok = await confirmDialog({
    title: `删除应用「${app.name}」`,
    description: '仅删除 AppHub 中的应用定义，不会删除服务器上的实际程序或数据。',
    confirmText: '删除',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/api/v1/apps/${app.id}`)
    toastHelpers.success('已删除', `${app.name} 的应用定义已移除`)
    refreshApps()
  } catch (e) {
    toastHelpers.error('删除失败', (e as Error).message)
  }
}

function onSaved() {
  formOpen.value = false
  refreshApps()
}

function fmtUptime(sec: number) {
  if (!sec) return '—'
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  if (d) return `${d}天 ${h}小时`
  if (h) return `${h}小时 ${m}分`
  return `${m}分钟`
}

onMounted(() => {
  loadSystemInfo()
  sysTimer = window.setInterval(() => {
    if (document.visibilityState === 'visible') loadSystemInfo()
  }, 30000)
})
onUnmounted(() => {
  if (sysTimer) window.clearInterval(sysTimer)
})

watch(selectMode, (v) => {
  if (!v) selected.value = []
})
</script>

<template>
  <div class="px-4 sm:px-6 lg:px-7 py-5 sm:py-6 max-w-[1680px] mx-auto">
    <!-- 概览 -->
    <section class="grid grid-cols-1 lg:grid-cols-[1fr_auto] gap-4">
      <div class="card p-5">
        <div class="flex items-start justify-between gap-4 flex-wrap">
          <div>
            <h2 class="text-[17px] font-semibold text-ink tracking-tight">
              {{ auth.settings?.site_name || 'AppHub' }}
            </h2>
            <p class="text-[13px] text-muted mt-1">
              服务器<span class="text-ok font-medium">运行正常</span>
              <span v-if="sysInfo" class="text-faint"> · {{ sysInfo.hostname }} · {{ sysInfo.arch }}</span>
            </p>
          </div>
          <div class="text-right">
            <p class="text-[11.5px] text-faint">在线率</p>
            <p class="text-[22px] font-semibold text-ink tabular-nums leading-tight">{{ summary.rate }}%</p>
          </div>
        </div>

        <div class="mt-4 grid grid-cols-4 gap-2">
          <div class="rounded-xl bg-elevated px-3 py-2.5">
            <p class="text-[11.5px] text-faint">应用总数</p>
            <p class="text-[19px] font-semibold text-ink tabular-nums">{{ summary.total }}</p>
          </div>
          <div class="rounded-xl bg-ok/10 px-3 py-2.5">
            <p class="text-[11.5px] text-ok/80">运行中</p>
            <p class="text-[19px] font-semibold text-ok tabular-nums">{{ summary.online }}</p>
          </div>
          <div class="rounded-xl bg-elevated px-3 py-2.5">
            <p class="text-[11.5px] text-faint">已停止</p>
            <p class="text-[19px] font-semibold text-muted tabular-nums">{{ summary.offline }}</p>
          </div>
          <div class="rounded-xl bg-danger/10 px-3 py-2.5">
            <p class="text-[11.5px] text-danger/80">异常</p>
            <p class="text-[19px] font-semibold text-danger tabular-nums">{{ summary.error }}</p>
          </div>
        </div>

        <div class="mt-2.5 flex items-center gap-2 text-[12px] text-faint">
          <span class="chip">本地应用 {{ localCount }}</span>
          <span class="chip">非本地 {{ externalCount }}</span>
        </div>
      </div>

      <!-- 服务器信息 -->
      <div v-if="sysInfo" class="card p-5 lg:w-[340px]">
        <div class="flex items-center gap-2 mb-3">
          <Server :size="15" class="text-faint" />
          <span class="section-title">服务器</span>
        </div>
        <div class="space-y-2 text-[12.5px]">
          <div class="flex items-center justify-between gap-3">
            <span class="text-faint flex items-center gap-1.5"><Cpu :size="13" />CPU</span>
            <span class="text-ink font-mono tabular-nums">{{ sysInfo.cpu_percent.toFixed(0) }}% · {{ sysInfo.cpu_cores }} 核</span>
          </div>
          <div class="flex items-center justify-between gap-3">
            <span class="text-faint flex items-center gap-1.5"><MemoryStick :size="13" />内存</span>
            <span class="text-ink font-mono tabular-nums">
              {{ sysInfo.mem_used_mb }} / {{ sysInfo.mem_total_mb }} MB
            </span>
          </div>
          <div class="flex items-center justify-between gap-3">
            <span class="text-faint flex items-center gap-1.5"><HardDrive :size="13" />磁盘</span>
            <span class="text-ink font-mono tabular-nums">
              {{ sysInfo.disk_used_gb }} / {{ sysInfo.disk_total_gb }} GB
            </span>
          </div>
          <div class="flex items-center justify-between gap-3">
            <span class="text-faint">运行时间</span>
            <span class="text-ink font-mono tabular-nums">{{ fmtUptime(sysInfo.uptime_seconds) }}</span>
          </div>
          <div class="flex items-center justify-between gap-3">
            <span class="text-faint">当前 IP</span>
            <span class="text-ink font-mono">{{ sysInfo.ip }}</span>
          </div>
          <div class="flex items-center justify-between gap-3">
            <span class="text-faint">版本</span>
            <span class="text-ink font-mono">v{{ sysInfo.version }} · DB {{ sysInfo.db_version }}</span>
          </div>
        </div>
      </div>
    </section>

    <!-- 工具条 -->
    <section class="mt-5 flex items-center gap-2 flex-wrap">
      <div class="flex-1 min-w-0 flex items-center gap-1.5 overflow-x-auto no-scrollbar py-0.5">
        <button
          class="chip shrink-0 h-7 px-2.5 transition-colors"
          :class="activeCategory === 'all' ? '!bg-accent !text-white !border-accent' : 'hover:text-ink'"
          @click="activeCategory = 'all'"
        >
          全部
        </button>
        <button
          class="chip shrink-0 h-7 px-2.5 transition-colors"
          :class="activeCategory === 'fav' ? '!bg-warn !text-white !border-warn' : 'hover:text-ink'"
          @click="activeCategory = 'fav'"
        >
          收藏
        </button>
        <button
          v-for="c in categories.items"
          :key="c.id"
          class="chip shrink-0 h-7 px-2.5 transition-colors"
          :class="activeCategory === c.id ? '!bg-accent !text-white !border-accent' : 'hover:text-ink'"
          @click="activeCategory = c.id"
        >
          <span class="w-1.5 h-1.5 rounded-full" :style="{ background: c.color || 'currentColor' }" />
          {{ c.name }}
        </button>
      </div>

      <div class="flex items-center gap-2 shrink-0">
        <select
          v-model="sortBy"
          class="h-8 rounded-lg bg-elevated border border-line text-[12px] text-ink px-2 outline-none"
        >
          <option value="manual">手动排序</option>
          <option value="name">按名称</option>
          <option value="status">按状态</option>
          <option value="category">按分类</option>
        </select>

        <button
          class="btn btn-sm btn-outline"
          :class="selectMode && '!border-accent !text-accent'"
          @click="selectMode = !selectMode"
        >
          <CheckCheck :size="14" />{{ selectMode ? '退出多选' : '多选' }}
        </button>

        <button class="btn btn-sm btn-primary" @click="openCreate">
          <Plus :size="15" />添加应用
        </button>
      </div>
    </section>

    <!-- 批量操作条 -->
    <section
      v-if="selectMode"
      class="mt-3 card p-3 flex items-center gap-2 flex-wrap animate-fade-in"
    >
      <span class="text-[13px] text-muted">已选 {{ selected.length }} 个</span>
      <div class="flex-1" />
      <button class="btn btn-sm btn-outline" :disabled="!selected.length || batchRunning" @click="batch('start')">
        批量启动
      </button>
      <button class="btn btn-sm btn-outline" :disabled="!selected.length || batchRunning" @click="batch('stop')">
        批量停止
      </button>
      <button class="btn btn-sm btn-outline" :disabled="!selected.length || batchRunning" @click="batch('restart')">
        批量重启
      </button>
      <button class="btn btn-sm btn-ghost" @click="selected = []">清空</button>
      <span v-if="batchRunning" class="text-[12px] text-accent font-mono">
        {{ batchProgress.done }} / {{ batchProgress.total }} 已完成
      </span>
    </section>

    <!-- 应用网格 -->
    <section class="mt-4">
      <SkeletonGrid v-if="!loaded" :count="6" />

      <EmptyState
        v-else-if="!filtered.length && !apps.items.length"
        icon="Package"
        title="还没有任何应用"
        description="添加第一个应用，之后就可以在这里一键启动、直接访问，不用再记 IP 和端口。"
        action-text="添加应用"
        @action="openCreate"
      />

      <div
        v-else-if="!filtered.length"
        class="card py-14 flex flex-col items-center justify-center text-center"
      >
        <Search :size="22" class="text-faint mb-3" />
        <p class="text-[14px] font-medium text-ink">没有匹配的应用</p>
        <p class="text-[13px] text-muted mt-1">试试其他关键词，或清除筛选条件</p>
        <button class="btn btn-sm btn-outline mt-4" @click="activeCategory = 'all'">
          <X :size="14" />清除筛选
        </button>
      </div>

      <div
        v-else
        class="grid gap-3.5 sm:gap-4 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5"
      >
        <div
          v-for="app in filtered"
          :key="app.id"
          :draggable="sortBy === 'manual' && !selectMode"
          @dragstart="onDragStart(app.id)"
          @dragover.prevent="onDragOver(app.id)"
          @dragend="onDragEnd"
        >
          <AppCard
            :app="app"
            :selectable="selectMode"
            :selected="selected.includes(app.id)"
            @toggle-select="toggleSelect"
            @edit="openEdit"
            @delete="removeApp"
          />
        </div>
      </div>
    </section>

    <!-- 快捷启动全部（需设置开启） -->
    <section
      v-if="auth.settings?.allow_batch_start && apps.items.length"
      class="mt-6 flex items-center justify-between card p-4"
    >
      <div>
        <p class="text-[13.5px] font-medium text-ink">快捷启动全部应用</p>
        <p class="text-[12.5px] text-muted mt-0.5">已在设置中开启，将按队列依次启动，避免负载突增</p>
      </div>
      <button class="btn btn-md btn-outline" @click="startAllApps">
        <Zap :size="15" />启动全部
      </button>
    </section>

    <p class="text-center text-[11.5px] text-faint mt-8 mb-2 flex items-center justify-center gap-1.5">
      <ArrowDownUp :size="12" />
      手动排序模式下可直接拖拽卡片调整顺序
    </p>

    <AppFormModal :open="formOpen" :app="editing" @close="formOpen = false" @saved="onSaved" />
  </div>
</template>
