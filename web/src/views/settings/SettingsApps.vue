<script setup lang="ts">
import { computed, ref } from 'vue'
import { Download, Pencil, Plus, Trash2, Upload } from 'lucide-vue-next'
import AppIcon from '@/components/AppIcon.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import AppFormModal from '@/components/AppFormModal.vue'
import { api, ApiRequestError } from '@/api/client'
import type { AppItem } from '@/api/types'
import { apps, categories, refreshApps } from '@/stores/apps'
import { confirmDialog, toastHelpers } from '@/stores/ui'

const keyword = ref('')
const scopeFilter = ref<'all' | 'LOCAL' | 'EXTERNAL'>('all')
const formOpen = ref(false)
const editing = ref<AppItem | null>(null)

const filtered = computed(() => {
  let list = apps.items
  if (scopeFilter.value !== 'all') {
    list = list.filter((a) => (a.type || 'LOCAL') === scopeFilter.value)
  }
  const q = keyword.value.trim().toLowerCase()
  if (!q) return list
  return list.filter(
    (a) => a.name.toLowerCase().includes(q) || (a.description || '').toLowerCase().includes(q),
  )
})

function scopeText(t?: string) {
  return t === 'EXTERNAL' ? '非本地' : '本地应用'
}

function catName(id: number) {
  return categories.items.find((c) => c.id === id)?.name || '未分类'
}

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
    title: `删除「${app.name}」`,
    description: '仅删除 AppHub 中的应用定义，服务器上该应用的程序和数据不会被删除。',
    confirmText: '删除',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/api/v1/apps/${app.id}`)
    toastHelpers.success('已删除')
    refreshApps()
  } catch (e) {
    toastHelpers.error('删除失败', (e as ApiRequestError).message)
  }
}

async function exportConfig() {
  try {
    const data = await api.get('/api/v1/apps/export')
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `apphub-apps-${new Date().toISOString().slice(0, 10)}.json`
    a.click()
    URL.revokeObjectURL(url)
    toastHelpers.success('已导出应用配置')
  } catch (e) {
    toastHelpers.error('导出失败', (e as ApiRequestError).message)
  }
}

const importInput = ref<HTMLInputElement | null>(null)

async function onImport(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  try {
    const text = await file.text()
    const payload = JSON.parse(text)
    const ok = await confirmDialog({
      title: '导入应用配置',
      description: `将导入 ${payload.apps?.length || 0} 个应用。已存在的同名应用会自动使用新的标识，不会覆盖现有条目。`,
      confirmText: '导入',
    })
    if (!ok) return
    const res = await api.post<{ imported: number }>('/api/v1/apps/import', payload)
    toastHelpers.success('导入完成', `成功导入 ${res.imported} 个应用`)
    refreshApps()
  } catch (err) {
    toastHelpers.error('导入失败', (err as Error).message)
  } finally {
    input.value = ''
  }
}
</script>

<template>
  <div>
    <div class="flex items-center gap-2 flex-wrap mb-4">
      <input v-model="keyword" class="input h-9 max-w-[280px] text-[13px]" placeholder="搜索应用…" />
      <select
        v-model="scopeFilter"
        class="input h-9 max-w-[140px] text-[13px]"
      >
        <option value="all">全部类型</option>
        <option value="LOCAL">本地应用</option>
        <option value="EXTERNAL">非本地应用</option>
      </select>
      <div class="flex-1" />
      <input ref="importInput" type="file" accept=".json" class="hidden" @change="onImport" />
      <button class="btn btn-md btn-outline" @click="importInput?.click()"><Upload :size="14" />导入</button>
      <button class="btn btn-md btn-outline" @click="exportConfig"><Download :size="14" />导出</button>
      <button class="btn btn-md btn-primary" @click="openCreate"><Plus :size="15" />添加应用</button>
    </div>

    <div class="card overflow-hidden">
      <!-- 桌面端表格 -->
      <div class="hidden md:block overflow-x-auto">
        <table class="w-full text-[13px]">
          <thead>
            <tr class="border-b border-line text-left text-[12px] text-faint">
              <th class="font-medium px-4 py-2.5">应用</th>
              <th class="font-medium px-4 py-2.5">分类</th>
              <th class="font-medium px-4 py-2.5">管理</th>
              <th class="font-medium px-4 py-2.5">类型</th>
              <th class="font-medium px-4 py-2.5">状态</th>
              <th class="font-medium px-4 py-2.5">内网地址</th>
              <th class="font-medium px-4 py-2.5">自启</th>
              <th class="font-medium px-4 py-2.5 w-24"></th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="a in filtered"
              :key="a.id"
              class="border-b border-line/60 last:border-0 hover:bg-elevated/60 transition-colors"
            >
              <td class="px-4 py-2.5">
                <div class="flex items-center gap-2.5">
                  <div class="w-8 h-8 rounded-lg bg-elevated border border-line flex items-center justify-center text-base shrink-0">
                    <AppIcon :name="a.icon || 'Box'" :size="16" />
                  </div>
                  <div class="min-w-0">
                    <p class="font-medium text-ink truncate">{{ a.name }}</p>
                    <p class="text-[11.5px] text-faint truncate max-w-[220px]">{{ a.description || '—' }}</p>
                  </div>
                </div>
              </td>
              <td class="px-4 py-2.5 text-muted">{{ catName(a.category_id) }}</td>
              <td class="px-4 py-2.5">
                <span v-if="a.type === 'EXTERNAL'" class="chip text-info">非本地</span>
                <span v-else class="chip text-ok">本地应用</span>
              </td>
              <td class="px-4 py-2.5">
                <span v-if="a.type === 'EXTERNAL'" class="text-faint text-[12px]">—</span>
                <span v-else class="chip font-mono">{{ a.app_type }}</span>
              </td>
              <td class="px-4 py-2.5"><StatusBadge :status="a.runtime.status" /></td>
              <td class="px-4 py-2.5 font-mono text-[12px] text-muted truncate max-w-[200px]">
                {{ a.internal_url || '—' }}
              </td>
              <td class="px-4 py-2.5">
                <span :class="a.auto_start ? 'text-warn' : 'text-faint'">{{ a.auto_start ? '开启' : '关闭' }}</span>
              </td>
              <td class="px-4 py-2.5">
                <div class="flex items-center justify-end gap-1">
                  <button class="btn btn-sm btn-ghost" @click="openEdit(a)"><Pencil :size="14" /></button>
                  <button class="btn btn-sm btn-ghost !text-danger" @click="removeApp(a)"><Trash2 :size="14" /></button>
                </div>
              </td>
            </tr>
            <tr v-if="!filtered.length">
              <td colspan="8" class="px-4 py-10 text-center text-muted text-[13px]">暂无应用</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 移动端卡片列表 -->
      <div class="md:hidden divide-y divide-line">
        <div v-for="a in filtered" :key="a.id" class="p-3.5 flex items-center gap-3">
          <div class="w-9 h-9 rounded-lg bg-elevated border border-line flex items-center justify-center text-base shrink-0">
            <AppIcon :name="a.icon || 'Box'" :size="18" />
          </div>
          <div class="min-w-0 flex-1">
            <p class="text-[13.5px] font-medium text-ink truncate">{{ a.name }}</p>
            <div class="flex items-center gap-2 mt-1 flex-wrap">
              <StatusBadge :status="a.runtime.status" />
              <span class="text-[11.5px] text-faint">{{ catName(a.category_id) }}</span>
              <span class="text-[11px] px-1.5 rounded-md" :class="a.type === 'EXTERNAL' ? 'text-info bg-info/10' : 'text-ok bg-ok/10'">
                {{ scopeText(a.type) }}
              </span>
            </div>
          </div>
          <button class="btn btn-sm btn-ghost" @click="openEdit(a)"><Pencil :size="14" /></button>
          <button class="btn btn-sm btn-ghost !text-danger" @click="removeApp(a)"><Trash2 :size="14" /></button>
        </div>
        <p v-if="!filtered.length" class="p-8 text-center text-muted text-[13px]">暂无应用</p>
      </div>
    </div>

    <AppFormModal :open="formOpen" :app="editing" @close="formOpen = false" @saved="refreshApps()" />
  </div>
</template>
