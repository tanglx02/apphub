<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  ArrowLeft,
  Clock,
  Copy,
  Cpu,
  ExternalLink,
  Hash,
  MemoryStick,
  Pencil,
  RotateCw,
  Play,
  Square,
  Terminal,
  Trash2,
} from 'lucide-vue-next'
import AppIcon from '@/components/AppIcon.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import AppFormModal from '@/components/AppFormModal.vue'
import { api, ApiRequestError } from '@/api/client'
import type { AppItem } from '@/api/types'
import { apps, refreshApps, restartApp, startApp, stopApp, busyIds, isBusy } from '@/stores/apps'
import { confirmDialog, toastHelpers } from '@/stores/ui'

const route = useRoute()
const router = useRouter()

const id = computed(() => Number(route.params.id))
const app = computed(() => apps.items.find((a) => a.id === id.value))
const busy = computed(() => isBusy(id.value))
const action = computed(() => busyIds[id.value] || '')

// ---------- 日志 ----------
const lines = ref(100)
const following = ref(false)
const logText = ref('')
const logLoading = ref(false)
const logSource = ref('')
let es: EventSource | null = null

async function loadLogs() {
  if (!app.value) return
  logLoading.value = true
  try {
    const res = await api.get<{ lines: string[]; source: string }>(`/api/v1/apps/${id.value}/logs`, {
      lines: lines.value,
    })
    logText.value = res.lines.join('\n')
    logSource.value = res.source
  } catch (e) {
    toastHelpers.error('读取日志失败', (e as ApiRequestError).message)
  } finally {
    logLoading.value = false
  }
}

function toggleFollow() {
  following.value = !following.value
  if (following.value) {
    startFollow()
  } else {
    stopFollow()
    loadLogs()
  }
}

function startFollow() {
  stopFollow()
  es = new EventSource(`/api/v1/apps/${id.value}/logs?follow=1&lines=100`)
  es.addEventListener('log', (e) => {
    logText.value = (e as MessageEvent).data
  })
  es.addEventListener('error', (e) => {
    const msg = (e as MessageEvent).data
    if (msg) toastHelpers.error('日志流错误', String(msg))
  })
}

function stopFollow() {
  if (es) {
    es.close()
    es = null
  }
}

onMounted(() => {
  if (!app.value) refreshApps()
  loadLogs()
})
onUnmounted(() => stopFollow())

watch(id, () => {
  stopFollow()
  following.value = false
  loadLogs()
})

// ---------- 操作 ----------
const formOpen = ref(false)

async function removeApp() {
  if (!app.value) return
  const ok = await confirmDialog({
    title: `删除应用「${app.value.name}」`,
    description: '仅删除 AppHub 中的应用定义，不会删除服务器上的实际程序或数据。',
    confirmText: '删除',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/api/v1/apps/${id.value}`)
    toastHelpers.success('已删除')
    refreshApps()
    router.push({ name: 'dashboard' })
  } catch (e) {
    toastHelpers.error('删除失败', (e as ApiRequestError).message)
  }
}

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    toastHelpers.success('已复制', text)
  } catch {
    toastHelpers.error('复制失败')
  }
}

function fmtDuration(sec?: number) {
  if (!sec) return '—'
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = sec % 60
  if (d) return `${d} 天 ${h} 小时 ${m} 分`
  if (h) return `${h} 小时 ${m} 分 ${s} 秒`
  if (m) return `${m} 分 ${s} 秒`
  return `${s} 秒`
}

function fmtTime(t?: string) {
  if (!t) return '—'
  return new Date(t).toLocaleString('zh-CN', { hour12: false })
}
</script>

<template>
  <div class="px-4 sm:px-6 lg:px-7 py-5 sm:py-6 max-w-[1280px] mx-auto">
    <button class="btn btn-sm btn-ghost mb-4 -ml-2" @click="router.back()">
      <ArrowLeft :size="15" />返回控制中心
    </button>

    <div v-if="!app" class="card p-10 text-center text-muted text-[13px]">应用不存在或已删除</div>

    <template v-else>
      <!-- 头部 -->
      <header class="card p-5 flex flex-col sm:flex-row items-start gap-4">
        <div
          class="w-14 h-14 rounded-2xl bg-elevated border border-line flex items-center justify-center text-2xl shrink-0"
          :style="app.category_color ? { color: app.category_color } : undefined"
        >
          <AppIcon :name="app.icon || app.category_icon || 'Box'" :size="26" />
        </div>

        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-2.5 flex-wrap">
            <h1 class="text-[19px] font-semibold text-ink tracking-tight">{{ app.name }}</h1>
            <StatusBadge :status="app.runtime.status" size="md" />
          </div>
          <p class="text-[13.5px] text-muted mt-1.5">{{ app.description || '暂无描述' }}</p>
          <div class="flex items-center gap-2 mt-2.5 flex-wrap">
            <span v-if="app.category_name" class="chip">{{ app.category_name }}</span>
            <span class="chip font-mono">{{ app.app_type }}</span>
            <span v-if="app.app_type === 'systemd' && app.systemd_unit" class="chip font-mono">{{ app.systemd_unit }}</span>
            <span v-if="app.shell_mode" class="chip text-warn">shell 模式</span>
            <span v-for="t in (app.tags ? app.tags.split(',') : [])" :key="t" class="chip">#{{ t.trim() }}</span>
          </div>
        </div>

        <div class="flex items-center gap-2 shrink-0 w-full sm:w-auto">
          <button v-if="!busy && app.runtime.status !== 'ONLINE'" class="btn btn-md btn-primary flex-1 sm:flex-none" @click="startApp(app.id)">
            <Play :size="15" />启动
          </button>
          <button v-else-if="busy" class="btn btn-md btn-outline flex-1 sm:flex-none" disabled>
            <span class="w-3.5 h-3.5 rounded-full border-2 border-current border-t-transparent animate-spin" />
            {{ action === 'start' ? '启动中' : action === 'stop' ? '停止中' : '重启中' }}
          </button>
          <template v-else>
            <button class="btn btn-md btn-outline" @click="stopApp(app.id)"><Square :size="14" />停止</button>
          </template>
          <button class="btn btn-md btn-outline" :disabled="busy" @click="restartApp(app.id)">
            <RotateCw :size="14" />重启
          </button>
          <button class="btn btn-md btn-outline" @click="formOpen = true"><Pencil :size="14" />编辑</button>
          <button class="btn btn-md btn-outline !text-danger" @click="removeApp"><Trash2 :size="14" /></button>
        </div>
      </header>

      <!-- 信息网格 -->
      <div class="grid grid-cols-1 lg:grid-cols-3 gap-4 mt-4">
        <section class="card p-5 lg:col-span-1">
          <h2 class="section-title mb-3">运行状态</h2>
          <dl class="space-y-2.5 text-[13px]">
            <div class="flex items-center justify-between gap-3">
              <dt class="text-faint flex items-center gap-1.5"><Hash :size="13" />PID</dt>
              <dd class="font-mono text-ink">{{ app.runtime.pid || '—' }}</dd>
            </div>
            <div class="flex items-center justify-between gap-3">
              <dt class="text-faint flex items-center gap-1.5"><Clock :size="13" />启动时间</dt>
              <dd class="font-mono text-ink text-[12.5px]">{{ fmtTime(app.runtime.started_at) }}</dd>
            </div>
            <div class="flex items-center justify-between gap-3">
              <dt class="text-faint">运行时长</dt>
              <dd class="font-mono text-ink">{{ fmtDuration(app.runtime.uptime_seconds) }}</dd>
            </div>
            <div class="flex items-center justify-between gap-3">
              <dt class="text-faint flex items-center gap-1.5"><Cpu :size="13" />CPU</dt>
              <dd class="font-mono text-ink">{{ app.runtime.cpu_percent ? app.runtime.cpu_percent.toFixed(1) + '%' : '—' }}</dd>
            </div>
            <div class="flex items-center justify-between gap-3">
              <dt class="text-faint flex items-center gap-1.5"><MemoryStick :size="13" />内存</dt>
              <dd class="font-mono text-ink">{{ app.runtime.memory_mb ? app.runtime.memory_mb.toFixed(0) + ' MB' : '—' }}</dd>
            </div>
            <div class="flex items-center justify-between gap-3">
              <dt class="text-faint">最近检测</dt>
              <dd class="font-mono text-ink text-[12.5px]">{{ fmtTime(app.runtime.last_checked) }}</dd>
            </div>
          </dl>
          <p v-if="app.runtime.detail" class="mt-3 text-[12px] text-faint break-words">
            检测详情：{{ app.runtime.detail }}
          </p>
        </section>

        <section class="card p-5 lg:col-span-2">
          <h2 class="section-title mb-3">访问地址</h2>
          <div class="space-y-3">
            <div class="rounded-xl border border-line p-3.5">
              <div class="flex items-center justify-between gap-2">
                <span class="text-[12.5px] text-faint">内网访问</span>
                <div v-if="app.internal_url" class="flex items-center gap-1">
                  <button class="btn btn-sm btn-ghost" @click="copyText(app.internal_url)"><Copy :size="13" /></button>
                  <a :href="app.internal_url" target="_blank" rel="noopener noreferrer" class="btn btn-sm btn-ghost">
                    <ExternalLink :size="13" />
                  </a>
                </div>
              </div>
              <a
                v-if="app.internal_url"
                :href="app.internal_url"
                target="_blank"
                rel="noopener noreferrer"
                class="block mt-1 font-mono text-[13px] text-accent hover:underline break-all"
                >{{ app.internal_url }}</a
              >
              <p v-else class="mt-1 text-[13px] text-faint">未配置</p>
            </div>

            <div class="rounded-xl border border-line p-3.5">
              <div class="flex items-center justify-between gap-2">
                <span class="text-[12.5px] text-faint">公网访问</span>
                <div v-if="app.external_url" class="flex items-center gap-1">
                  <button class="btn btn-sm btn-ghost" @click="copyText(app.external_url)"><Copy :size="13" /></button>
                  <a :href="app.external_url" target="_blank" rel="noopener noreferrer" class="btn btn-sm btn-ghost">
                    <ExternalLink :size="13" />
                  </a>
                </div>
              </div>
              <a
                v-if="app.external_url"
                :href="app.external_url"
                target="_blank"
                rel="noopener noreferrer"
                class="block mt-1 font-mono text-[13px] text-accent hover:underline break-all"
                >{{ app.external_url }}</a
              >
              <p v-else class="mt-1 text-[13px] text-faint">未配置（公网地址需手动填写，系统不会自动推断）</p>
            </div>
          </div>

          <h2 class="section-title mt-5 mb-3">运行配置</h2>
          <dl class="grid grid-cols-1 sm:grid-cols-2 gap-x-4 gap-y-2.5 text-[13px]">
            <div class="flex items-center justify-between gap-3">
              <dt class="text-faint">检测方式</dt>
              <dd class="font-mono text-ink">{{ app.status_type }}</dd>
            </div>
            <div class="flex items-center justify-between gap-3">
              <dt class="text-faint">检测目标</dt>
              <dd class="font-mono text-ink truncate max-w-[180px]" :title="app.status_target">{{ app.status_target || '—' }}</dd>
            </div>
            <div class="flex items-center justify-between gap-3">
              <dt class="text-faint">开机自启</dt>
              <dd class="text-ink">{{ app.auto_start ? '已开启' : '已关闭' }}</dd>
            </div>
            <div class="flex items-center justify-between gap-3">
              <dt class="text-faint">启停超时</dt>
              <dd class="font-mono text-ink">{{ app.timeout_seconds || 60 }}s</dd>
            </div>
            <div v-if="app.work_dir" class="flex items-center justify-between gap-3 sm:col-span-2">
              <dt class="text-faint">工作目录</dt>
              <dd class="font-mono text-ink truncate max-w-[320px]" :title="app.work_dir">{{ app.work_dir }}</dd>
            </div>
            <div v-if="app.start_command" class="flex items-start justify-between gap-3 sm:col-span-2">
              <dt class="text-faint shrink-0">启动命令</dt>
              <dd class="font-mono text-ink text-[12.5px] break-all text-right">{{ app.start_command }}</dd>
            </div>
          </dl>
        </section>
      </div>

      <!-- 日志 -->
      <section class="card mt-4 overflow-hidden">
        <header class="flex items-center gap-2 px-4 py-3 border-b border-line flex-wrap">
          <Terminal :size="15" class="text-faint" />
          <span class="text-[13.5px] font-medium text-ink">应用日志</span>
          <span v-if="logSource" class="chip font-mono">{{ logSource }}</span>
          <div class="flex-1" />
          <select
            :value="lines"
            class="h-8 rounded-lg bg-elevated border border-line text-[12px] text-ink px-2 outline-none"
            @change="lines = Number(($event.target as HTMLSelectElement).value); loadLogs()"
          >
            <option :value="20">最近 20 行</option>
            <option :value="100">最近 100 行</option>
            <option :value="500">最近 500 行</option>
          </select>
          <button class="btn btn-sm btn-outline" @click="loadLogs">刷新</button>
          <button
            class="btn btn-sm"
            :class="following ? 'btn-primary' : 'btn-outline'"
            @click="toggleFollow"
          >
            <span v-if="following" class="w-2 h-2 rounded-full bg-white animate-pulse-dot" />
            {{ following ? '实时中' : '实时日志' }}
          </button>
        </header>
        <div class="p-4">
          <pre
            v-if="logText"
            class="text-[12px] font-mono text-muted whitespace-pre-wrap break-words max-h-[420px] overflow-y-auto leading-relaxed"
            >{{ logText }}</pre
          >
          <p v-else-if="logLoading" class="text-[13px] text-faint">正在读取日志…</p>
          <p v-else class="text-[13px] text-faint">暂无日志内容</p>
        </div>
      </section>

      <AppFormModal :open="formOpen" :app="app" @close="formOpen = false" @saved="refreshApps()" />
    </template>
  </div>
</template>
