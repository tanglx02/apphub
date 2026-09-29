<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { LogIn, Package } from 'lucide-vue-next'
import { api, ApiRequestError } from '@/api/client'
import { auth, bootstrap, login } from '@/stores/auth'
import { ensurePolling } from '@/router'
import { toastHelpers } from '@/stores/ui'

const router = useRouter()
const route = useRoute()

const username = ref('admin')
const password = ref('')
const submitting = ref(false)
const error = ref('')
const needSetup = ref(false)

onMounted(async () => {
  const res = await bootstrap()
  if (res === 'authed') {
    ensurePolling()
    router.replace({ path: '/admin' })
    return
  }
  // 判断是否已初始化：尝试用探测用户数量
  try {
    await api.post('/api/v1/auth/login', { username: '__probe__', password: '__probe__' })
  } catch (e) {
    const err = e as ApiRequestError
    if (err.code === 'NOT_INITIALIZED' || err.message.includes('尚未创建管理员')) {
      needSetup.value = true
      router.replace({ name: 'setup' })
    }
  }
})

async function submit() {
  if (!username.value || !password.value) {
    error.value = '请输入用户名和密码'
    return
  }
  submitting.value = true
  error.value = ''
  try {
    await login(username.value.trim(), password.value)
    await bootstrap()
    ensurePolling()
    const redirect = (route.query.redirect as string) || '/admin'
    router.replace(redirect)
  } catch (e) {
    const err = e as ApiRequestError
    if (err.code === 'NOT_INITIALIZED') {
      needSetup.value = true
      router.replace({ name: 'setup' })
      return
    }
    error.value = err.message || '登录失败'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="min-h-full flex items-center justify-center px-5 py-10 bg-canvas">
    <div class="w-full max-w-[360px] animate-fade-up">
      <div class="flex flex-col items-center mb-8">
        <div class="w-12 h-12 rounded-2xl bg-accent text-white flex items-center justify-center mb-4">
          <Package :size="22" />
        </div>
        <h1 class="text-[19px] font-semibold text-ink tracking-tight">AppHub</h1>
        <p class="text-[13px] text-muted mt-1.5">服务器应用控制中心</p>
      </div>

      <div class="card p-6">
        <form @submit.prevent="submit">
          <label class="label">用户名</label>
          <input v-model="username" class="input mb-4" autocomplete="username" placeholder="admin" />

          <label class="label">密码</label>
          <input
            v-model="password"
            type="password"
            class="input"
            autocomplete="current-password"
            placeholder="请输入密码"
            @keyup.enter="submit"
          />

          <p v-if="error" class="mt-3 text-[12.5px] text-danger">{{ error }}</p>

          <button
            type="submit"
            class="btn btn-lg btn-primary w-full mt-5"
            :disabled="submitting"
          >
            <span
              v-if="submitting"
              class="w-4 h-4 rounded-full border-2 border-white/40 border-t-white animate-spin"
            />
            <LogIn v-else :size="16" />
            {{ submitting ? '登录中…' : '登录' }}
          </button>
        </form>
      </div>

      <p class="text-center text-[12px] text-faint mt-6">
        仅限授权管理员访问 · 所有操作均记录审计日志
      </p>
    </div>
  </div>
</template>
