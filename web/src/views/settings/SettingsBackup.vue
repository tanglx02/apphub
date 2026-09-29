<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Download, HardDrive, RotateCcw, Trash2 } from 'lucide-vue-next'
import { api, ApiRequestError } from '@/api/client'
import type { BackupRecord } from '@/api/types'
import { confirmDialog, toastHelpers } from '@/stores/ui'

const items = ref<BackupRecord[]>([])
const dir = ref('')
const loading = ref(false)
const note = ref('')
const restoreTarget = ref('')
const restoreOpts = ref({ config: true, db: true, uploads: true })

async function load() {
  loading.value = true
  try {
    const res = await api.get<{ items: BackupRecord[]; dir: string }>('/api/v1/backups')
    items.value = res.items || []
    dir.value = res.dir
  } catch (e) {
    toastHelpers.error('读取备份失败', (e as ApiRequestError).message)
  } finally {
    loading.value = false
  }
}

async function createBackup() {
  try {
    await api.post('/api/v1/backups', { note: note.value })
    toastHelpers.success('备份已创建')
    note.value = ''
    load()
  } catch (e) {
    toastHelpers.error('创建备份失败', (e as ApiRequestError).message)
  }
}

async function restore() {
  if (!restoreTarget.value) {
    toastHelpers.error('请选择要恢复的备份')
    return
  }
  const ok = await confirmDialog({
    title: '确认恢复备份',
    description: '恢复前会自动备份当前状态。恢复完成后需要重启 AppHub 服务才能生效。',
    confirmText: '确认恢复',
    danger: true,
  })
  if (!ok) return
  try {
    const res = await api.post<{ message: string }>('/api/v1/backups/restore', {
      name: restoreTarget.value,
      restore_config: restoreOpts.value.config,
      restore_db: restoreOpts.value.db,
      restore_uploads: restoreOpts.value.uploads,
    })
    toastHelpers.success('恢复完成', res.message)
  } catch (e) {
    toastHelpers.error('恢复失败', (e as ApiRequestError).message)
  }
}

async function remove(b: BackupRecord) {
  const ok = await confirmDialog({
    title: `删除备份「${b.name}」`,
    description: '该操作不可撤销。',
    confirmText: '删除',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/api/v1/backups/${b.name}`)
    toastHelpers.success('已删除')
    load()
  } catch (e) {
    toastHelpers.error('删除失败', (e as ApiRequestError).message)
  }
}

function fmtSize(bytes: number) {
  if (!bytes) return '—'
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / 1024 / 1024).toFixed(2) + ' MB'
}

function fmtTime(t: string) {
  return new Date(t).toLocaleString('zh-CN', { hour12: false })
}

const hasItems = computed(() => items.value.length > 0)

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <section class="card p-5">
      <div class="flex items-center gap-2 mb-4">
        <HardDrive :size="15" class="text-faint" />
        <h2 class="text-[14px] font-semibold text-ink">创建备份</h2>
      </div>
      <p class="text-[13px] text-muted mb-3">
        备份包含 config.yaml、SQLite 数据库一致性快照、上传的图标与分类排序。保存在
        <span class="font-mono">{{ dir || './backups' }}</span>
      </p>
      <div class="flex flex-col sm:flex-row gap-2">
        <input v-model="note" class="input flex-1" placeholder="备注（可选），例如：升级前" />
        <button class="btn btn-md btn-primary sm:w-32" @click="createBackup">创建备份</button>
      </div>
    </section>

    <section class="card p-5">
      <h2 class="text-[14px] font-semibold text-ink mb-4">恢复备份</h2>
      <div class="space-y-3">
        <div>
          <label class="label">选择备份文件</label>
          <select v-model="restoreTarget" class="input">
            <option value="">请选择…</option>
            <option v-for="b in items" :key="b.name" :value="b.name">
              {{ b.name }} · {{ fmtSize(b.size_bytes) }}
            </option>
          </select>
        </div>
        <div class="flex flex-wrap gap-4">
          <label class="flex items-center gap-2 text-[13px] text-ink">
            <input v-model="restoreOpts.db" type="checkbox" class="w-4 h-4" />应用与设置
          </label>
          <label class="flex items-center gap-2 text-[13px] text-ink">
            <input v-model="restoreOpts.config" type="checkbox" class="w-4 h-4" />config.yaml
          </label>
          <label class="flex items-center gap-2 text-[13px] text-ink">
            <input v-model="restoreOpts.uploads" type="checkbox" class="w-4 h-4" />图标资源
          </label>
        </div>
        <button class="btn btn-md btn-outline" :disabled="!restoreTarget" @click="restore">
          <RotateCcw :size="15" />恢复并重启提示
        </button>
        <p class="text-[12.5px] text-faint">
          恢复后请执行 <span class="font-mono">sudo systemctl restart apphub</span> 使配置生效。
        </p>
      </div>
    </section>

    <section class="card overflow-hidden">
      <header class="px-5 py-3.5 border-b border-line flex items-center justify-between">
        <h2 class="text-[14px] font-semibold text-ink">备份列表</h2>
        <button class="btn btn-sm btn-ghost" @click="load">刷新</button>
      </header>
      <div v-if="loading" class="p-8 text-center text-[13px] text-muted">加载中…</div>
      <div v-else-if="!hasItems" class="p-10 text-center text-[13px] text-muted">暂无备份</div>
      <div v-else class="divide-y divide-line">
        <div v-for="b in items" :key="b.name" class="p-3.5 flex items-center gap-3 flex-wrap">
          <div class="min-w-0 flex-1">
            <p class="text-[13.5px] font-medium text-ink font-mono truncate">{{ b.name }}</p>
            <p class="text-[11.5px] text-faint mt-0.5">
              {{ fmtSize(b.size_bytes) }} · {{ fmtTime(b.created_at) }}
              <span v-if="b.note"> · {{ b.note }}</span>
            </p>
          </div>
          <a :href="`/api/v1/backups/${encodeURIComponent(b.name)}/download`" class="btn btn-sm btn-outline">
            <Download :size="13" />下载
          </a>
          <button class="btn btn-sm btn-outline !text-danger" @click="remove(b)">
            <Trash2 :size="13" />删除
          </button>
        </div>
      </div>
    </section>
  </div>
</template>
