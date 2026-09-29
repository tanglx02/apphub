<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { Activity, Trash2 } from 'lucide-vue-next'
import { api, ApiRequestError } from '@/api/client'
import type { AuditLog } from '@/api/types'
import { confirmDialog, toastHelpers } from '@/stores/ui'

const items = ref<AuditLog[]>([])
const total = ref(0)
const limit = ref(50)
const offset = ref(0)
const keyword = ref('')
const action = ref('')
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    const res = await api.get<{ items: AuditLog[]; total: number }>('/api/v1/audit', {
      limit: limit.value,
      offset: offset.value,
      keyword: keyword.value,
      action: action.value,
    })
    items.value = res.items || []
    total.value = res.total
  } catch (e) {
    toastHelpers.error('读取审计日志失败', (e as ApiRequestError).message)
  } finally {
    loading.value = false
  }
}

async function clear() {
  const ok = await confirmDialog({
    title: '清空审计日志',
    description: '将删除全部审计记录，操作本身也会被记录。该操作不可撤销。',
    confirmText: '清空',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del('/api/v1/audit')
    toastHelpers.success('已清空')
    load()
  } catch (e) {
    toastHelpers.error('操作失败', (e as ApiRequestError).message)
  }
}

function fmtTime(t: string) {
  return new Date(t).toLocaleString('zh-CN', { hour12: false })
}

const pageStart = computed(() => (total.value === 0 ? 0 : offset.value + 1))
const pageEnd = computed(() => Math.min(offset.value + limit.value, total.value))

watch([keyword, action], () => {
  offset.value = 0
  load()
})

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-center gap-2 flex-wrap">
      <input v-model="keyword" class="input h-9 max-w-[240px] text-[13px]" placeholder="搜索用户 / 应用 / IP…" />
      <select v-model="action" class="input h-9 max-w-[180px] text-[13px]">
        <option value="">全部动作</option>
        <option value="login">登录</option>
        <option value="login_failed">登录失败</option>
        <option value="app_start">启动应用</option>
        <option value="app_stop">停止应用</option>
        <option value="app_restart">重启应用</option>
        <option value="app_create">添加应用</option>
        <option value="app_update">修改应用</option>
        <option value="app_delete">删除应用</option>
        <option value="backup">备份</option>
        <option value="restore">恢复</option>
        <option value="settings_update">修改设置</option>
        <option value="password_reset">重置密码</option>
      </select>
      <div class="flex-1" />
      <button class="btn btn-md btn-outline !text-danger" @click="clear"><Trash2 :size="14" />清空</button>
    </div>

    <div class="card overflow-hidden">
      <div class="hidden md:block overflow-x-auto">
        <table class="w-full text-[13px]">
          <thead>
            <tr class="text-left text-[12px] text-faint border-b border-line">
              <th class="font-medium px-4 py-2.5">时间</th>
              <th class="font-medium px-4 py-2.5">用户</th>
              <th class="font-medium px-4 py-2.5">IP</th>
              <th class="font-medium px-4 py-2.5">动作</th>
              <th class="font-medium px-4 py-2.5">对象</th>
              <th class="font-medium px-4 py-2.5">结果</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="l in items" :key="l.id" class="border-b border-line/60 last:border-0 hover:bg-elevated/60">
              <td class="px-4 py-2.5 font-mono text-[12px] text-muted whitespace-nowrap">{{ fmtTime(l.created_at) }}</td>
              <td class="px-4 py-2.5 text-ink">{{ l.username }}</td>
              <td class="px-4 py-2.5 font-mono text-[12px] text-muted">{{ l.ip }}</td>
              <td class="px-4 py-2.5 text-ink">{{ l.action_cn || l.action }}</td>
              <td class="px-4 py-2.5 text-ink truncate max-w-[180px]" :title="l.target">{{ l.target || '—' }}</td>
              <td class="px-4 py-2.5">
                <span :class="l.result === 'success' ? 'text-ok' : 'text-danger'">
                  {{ l.result === 'success' ? '成功' : '失败' }}
                </span>
                <span v-if="l.detail" class="block text-[11.5px] text-faint truncate max-w-[220px]" :title="l.detail">
                  {{ l.detail }}
                </span>
              </td>
            </tr>
            <tr v-if="!items.length">
              <td colspan="6" class="px-4 py-10 text-center text-muted">暂无审计记录</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="md:hidden divide-y divide-line">
        <div v-for="l in items" :key="l.id" class="p-3.5">
          <div class="flex items-center justify-between gap-2">
            <span class="text-[13px] font-medium text-ink">{{ l.action_cn || l.action }}</span>
            <span :class="l.result === 'success' ? 'text-ok' : 'text-danger'" class="text-[12px]">
              {{ l.result === 'success' ? '成功' : '失败' }}
            </span>
          </div>
          <p class="text-[11.5px] text-faint font-mono mt-1">{{ fmtTime(l.created_at) }} · {{ l.username }} · {{ l.ip }}</p>
          <p v-if="l.target" class="text-[12.5px] text-muted mt-1 truncate">{{ l.target }}</p>
        </div>
        <p v-if="!items.length" class="p-8 text-center text-muted text-[13px]">暂无审计记录</p>
      </div>

      <footer class="px-4 py-3 border-t border-line flex items-center justify-between gap-3 flex-wrap">
        <span class="text-[12.5px] text-faint">
          {{ pageStart }} – {{ pageEnd }} / 共 {{ total }} 条
        </span>
        <div class="flex items-center gap-2">
          <button class="btn btn-sm btn-outline" :disabled="offset === 0" @click="offset = Math.max(0, offset - limit); load()">
            上一页
          </button>
          <button
            class="btn btn-sm btn-outline"
            :disabled="offset + limit >= total"
            @click="offset += limit; load()"
          >
            下一页
          </button>
        </div>
      </footer>
    </div>

    <p class="text-[12px] text-faint flex items-center gap-1.5">
      <Activity :size="12" />所有登录、启停、配置变更与备份操作都会被记录
    </p>
  </div>
</template>
