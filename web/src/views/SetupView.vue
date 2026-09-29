<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { Package, ShieldCheck } from 'lucide-vue-next'
import { ApiRequestError } from '@/api/client'
import { setup } from '@/stores/auth'
import { toastHelpers } from '@/stores/ui'

const router = useRouter()
const username = ref('admin')
const password = ref('')
const confirmPwd = ref('')
const siteName = ref('AppHub')
const submitting = ref(false)
const error = ref('')

async function submit() {
  error.value = ''
  if (password.value.length < 8) {
    error.value = '密码长度至少 8 位'
    return
  }
  if (password.value !== confirmPwd.value) {
    error.value = '两次输入的密码不一致'
    return
  }
  submitting.value = true
  try {
    await setup(username.value.trim(), password.value, siteName.value.trim())
    toastHelpers.success('初始化完成', '请使用新账户登录')
    router.replace({ name: 'login' })
  } catch (e) {
    error.value = (e as ApiRequestError).message || '初始化失败'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="min-h-full flex items-center justify-center px-5 py-10 bg-canvas">
    <div class="w-full max-w-[400px] animate-fade-up">
      <div class="flex flex-col items-center mb-7">
        <div class="w-12 h-12 rounded-2xl bg-accent text-white flex items-center justify-center mb-4">
          <Package :size="22" />
        </div>
        <h1 class="text-[19px] font-semibold text-ink tracking-tight">欢迎使用 AppHub</h1>
        <p class="text-[13px] text-muted mt-1.5 text-center">
          创建管理员账户，开始管理你的服务器应用
        </p>
      </div>

      <div class="card p-6">
        <form @submit.prevent="submit">
          <label class="label">系统名称</label>
          <input v-model="siteName" class="input mb-4" placeholder="AppHub" />

          <label class="label">管理员用户名</label>
          <input v-model="username" class="input mb-4" placeholder="admin" autocomplete="username" />

          <label class="label">密码</label>
          <input
            v-model="password"
            type="password"
            class="input mb-4"
            placeholder="至少 8 位"
            autocomplete="new-password"
          />

          <label class="label">确认密码</label>
          <input
            v-model="confirmPwd"
            type="password"
            class="input"
            placeholder="再次输入密码"
            autocomplete="new-password"
          />

          <p v-if="error" class="mt-3 text-[12.5px] text-danger">{{ error }}</p>

          <button type="submit" class="btn btn-lg btn-primary w-full mt-5" :disabled="submitting">
            <ShieldCheck :size="16" />
            {{ submitting ? '创建中…' : '创建管理员账户' }}
          </button>
        </form>
      </div>

      <p class="text-center text-[12px] text-faint mt-6">
        密码使用 Argon2id 加密存储，不支持找回，请妥善保管
      </p>
    </div>
  </div>
</template>
