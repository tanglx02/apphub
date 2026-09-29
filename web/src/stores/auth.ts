import { computed, reactive } from 'vue'
import { api, clearCsrfToken, setCsrfToken } from '@/api/client'
import type { PublicUser, SiteSettings } from '@/api/types'

interface AuthState {
  checking: boolean
  initialized: boolean
  user: PublicUser | null
  settings: SiteSettings | null
}

export const auth = reactive<AuthState>({
  checking: true,
  initialized: true,
  user: null,
  settings: null,
})

export const isAuthed = computed(() => !!auth.user)

export async function bootstrap() {
  auth.checking = true
  try {
    const me = await api.get<{ user: PublicUser; csrf_token: string; settings: SiteSettings }>('/api/v1/auth/me')
    auth.user = me.user
    auth.settings = me.settings
    auth.initialized = true
    setCsrfToken(me.csrf_token)
    applySiteName(me.settings?.site_name)
    return 'authed'
  } catch (e) {
    const err = e as { code?: string }
    if (err.code === 'UNAUTHORIZED') {
      // 未登录：判断是否需要初始化
      auth.user = null
      auth.initialized = true
      return 'guest'
    }
    auth.initialized = true
    return 'guest'
  } finally {
    auth.checking = false
  }
}

export async function login(username: string, password: string) {
  const res = await api.post<{ user: PublicUser; csrf_token: string; settings?: SiteSettings }>(
    '/api/v1/auth/login',
    { username, password },
  )
  auth.user = res.user
  if (res.settings) auth.settings = res.settings
  setCsrfToken(res.csrf_token)
  applySiteName(res.settings?.site_name)
}

export async function setup(username: string, password: string, siteName?: string) {
  await api.post('/api/v1/auth/setup', { username, password, site_name: siteName })
}

export async function logout() {
  try {
    await api.post('/api/v1/auth/logout')
  } catch {
    /* ignore */
  }
  auth.user = null
  clearCsrfToken()
}

export async function changePassword(oldPassword: string, newPassword: string) {
  await api.post('/api/v1/auth/password', { old_password: oldPassword, new_password: newPassword })
}

function applySiteName(name?: string) {
  if (name) document.title = name
}
