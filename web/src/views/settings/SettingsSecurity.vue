<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { KeyRound, LogOut, RefreshCw, Server, ShieldAlert } from 'lucide-vue-next'
import { api, ApiRequestError } from '@/api/client'
import { auth, changePassword, logout } from '@/stores/auth'
import { confirmDialog, toastHelpers } from '@/stores/ui'
import { useRouter } from 'vue-router'

const router = useRouter()
const pwd = reactive({ old: '', next: '', confirm: '' })
const submitting = ref(false)

const sessions = ref<{ id: string; username: string; ip: string; created_at: string; expires_at: string }[]>([])
const service = ref<{ available: boolean; installed?: boolean; active?: string; enabled?: string } | null>(null)
const serviceBusy = ref('')

async function loadSessions() {
  try {
    sessions.value = await api.get('/api/v1/auth/sessions')
  } catch {
    /* ignore */
  }
}

async function loadService() {
  try {
    service.value = await api.get('/api/v1/system/service')
  } catch {
    service.value = null
  }
}

async function submitPassword() {
  if (pwd.next.length < 8) {
    toastHelpers.error('新密码长度至少 8 位')
    return
  }
  if (pwd.next !== pwd.confirm) {
    toastHelpers.error('两次输入的新密码不一致')
    return
  }
  const ok = await confirmDialog({
    title: '修改管理员密码',
    description: '修改后所有已登录设备都会被迫重新登录。',
    confirmText: '确认修改',
    danger: true,
  })
  if (!ok) return
  submitting.value = true
  try {
    await changePassword(pwd.old, pwd.next)
    toastHelpers.success('密码已修改，请重新登录')
    await logout()
    router.push({ name: 'login' })
  } catch (e) {
    toastHelpers.error('修改失败', (e as ApiRequestError).message)
  } finally {
    submitting.value = false
  }
}

async function revokeAll() {
  const ok = await confirmDialog({
    title: '使全部会话失效',
    description: '当前所有设备（含本机）都将被强制退出登录。',
    confirmText: '确认',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del('/api/v1/auth/sessions')
    toastHelpers.success('已使全部会话失效')
    await logout()
    router.push({ name: 'login' })
  } catch (e) {
    toastHelpers.error('操作失败', (e as ApiRequestError).message)
  }
}

async function serviceAction(action: string) {
  const danger = action === 'stop' || action === 'restart' || action === 'disable'
  const ok = await confirmDialog({
    title: `确认操作：${action}`,
    description:
      action === 'stop'
        ? '停止后 AppHub 将无法访问，需通过脚本或 systemctl 重新启动。'
        : action === 'restart'
          ? '重启期间 AppHub 会短暂不可用。'
          : `将对 apphub.service 执行 systemctl ${action}。`,
    confirmText: '确认执行',
    danger,
  })
  if (!ok) return
  serviceBusy.value = action
  try {
    await api.post('/api/v1/system/service', { action })
    toastHelpers.success('操作已提交', `systemctl ${action} apphub.service`)
    setTimeout(loadService, 1500)
  } catch (e) {
    toastHelpers.error('操作失败', (e as ApiRequestError).message)
  } finally {
    serviceBusy.value = ''
  }
}

function fmtTime(t: string) {
  return new Date(t).toLocaleString('zh-CN', { hour12: false })
}

onMounted(() => {
  loadSessions()
  loadService()
})
</script>

<template>
  <div class="space-y-4">
    <!-- 修改密码 -->
    <section class="card p-5">
      <div class="flex items-center gap-2 mb-4">
        <KeyRound :size="15" class="text-faint" />
        <h2 class="text-[14px] font-semibold text-ink">修改密码</h2>
      </div>
      <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
        <div>
          <label class="label">当前密码</label>
          <input v-model="pwd.old" type="password" class="input" autocomplete="current-password" />
        </div>
        <div>
          <label class="label">新密码</label>
          <input v-model="pwd.next" type="password" class="input" autocomplete="new-password" />
        </div>
        <div>
          <label class="label">确认新密码</label>
          <input v-model="pwd.confirm" type="password" class="input" autocomplete="new-password" />
        </div>
      </div>
      <div class="mt-4 flex items-center gap-3">
        <button class="btn btn-md btn-primary" :disabled="submitting" @click="submitPassword">
          {{ submitting ? '提交中…' : '修改密码' }}
        </button>
        <span class="text-[12.5px] text-faint">密码使用 Argon2id 加密，永远不会明文保存或返回</span>
      </div>
    </section>

    <!-- 会话 -->
    <section class="card p-5">
      <div class="flex items-center gap-2 mb-4">
        <ShieldAlert :size="15" class="text-faint" />
        <h2 class="text-[14px] font-semibold text-ink">活跃会话</h2>
        <div class="flex-1" />
        <button class="btn btn-sm btn-outline" @click="loadSessions"><RefreshCw :size="13" />刷新</button>
        <button class="btn btn-sm btn-outline !text-danger" @click="revokeAll">
          <LogOut :size="13" />全部踢出
        </button>
      </div>
      <div class="overflow-x-auto">
        <table class="w-full text-[13px]">
          <thead>
            <tr class="text-left text-[12px] text-faint border-b border-line">
              <th class="font-medium py-2">用户</th>
              <th class="font-medium py-2">IP</th>
              <th class="font-medium py-2">登录时间</th>
              <th class="font-medium py-2">过期时间</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="s in sessions" :key="s.id" class="border-b border-line/60 last:border-0">
              <td class="py-2 text-ink">{{ s.username }}</td>
              <td class="py-2 font-mono text-muted">{{ s.ip }}</td>
              <td class="py-2 font-mono text-muted text-[12px]">{{ fmtTime(s.created_at) }}</td>
              <td class="py-2 font-mono text-muted text-[12px]">{{ fmtTime(s.expires_at) }}</td>
            </tr>
            <tr v-if="!sessions.length">
              <td colspan="4" class="py-6 text-center text-muted">暂无活跃会话</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <!-- 服务管理 -->
    <section class="card p-5">
      <div class="flex items-center gap-2 mb-4">
        <Server :size="15" class="text-faint" />
        <h2 class="text-[14px] font-semibold text-ink">AppHub 服务管理</h2>
        <div class="flex-1" />
        <span v-if="service?.active" class="chip font-mono">{{ service.active }}</span>
      </div>
      <p v-if="service && !service.available" class="text-[13px] text-muted">
        当前环境不支持 systemd，请使用 scripts/manage.sh 管理服务。
      </p>
      <template v-else>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 text-[13px] mb-4">
          <div class="flex items-center justify-between rounded-xl bg-elevated px-3 py-2.5">
            <span class="text-faint">运行状态</span>
            <span class="font-mono text-ink">{{ service?.active || '—' }}</span>
          </div>
          <div class="flex items-center justify-between rounded-xl bg-elevated px-3 py-2.5">
            <span class="text-faint">开机自启</span>
            <span class="font-mono text-ink">{{ service?.enabled || '—' }}</span>
          </div>
        </div>
        <div class="flex flex-wrap gap-2">
          <button class="btn btn-sm btn-outline" :disabled="!!serviceBusy" @click="serviceAction('start')">启动</button>
          <button class="btn btn-sm btn-outline" :disabled="!!serviceBusy" @click="serviceAction('stop')">停止</button>
          <button class="btn btn-sm btn-outline" :disabled="!!serviceBusy" @click="serviceAction('restart')">重启</button>
          <button class="btn btn-sm btn-outline" :disabled="!!serviceBusy" @click="serviceAction('enable')">启用开机自启</button>
          <button class="btn btn-sm btn-outline" :disabled="!!serviceBusy" @click="serviceAction('disable')">取消开机自启</button>
        </div>
      </template>
    </section>

    <p class="text-[12px] text-faint">
      提示：若通过 tailscale / frp / cloudflared 暴露到公网，请务必开启 HTTPS 并使用强密码，
      避免将命令执行接口匿名暴露。
    </p>
  </div>
</template>
