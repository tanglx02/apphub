import { reactive, ref } from 'vue'

export type ThemeMode = 'light' | 'dark' | 'system'

export interface Toast {
  id: number
  type: 'success' | 'error' | 'info' | 'warning'
  title: string
  description?: string
}

const THEME_KEY = 'apphub.theme'

export const theme = ref<ThemeMode>(loadTheme())
const media = window.matchMedia('(prefers-color-scheme: dark)')

function loadTheme(): ThemeMode {
  try {
    const v = localStorage.getItem(THEME_KEY)
    if (v === 'light' || v === 'dark' || v === 'system') return v
  } catch {
    /* ignore */
  }
  return 'system'
}

export function applyTheme(mode: ThemeMode = theme.value) {
  const isDark = mode === 'dark' || (mode === 'system' && media.matches)
  document.documentElement.classList.toggle('dark', isDark)
  const meta = document.querySelector('meta[name="theme-color"]')
  if (meta) meta.setAttribute('content', isDark ? '#0B1120' : '#F4F6FA')
  theme.value = mode
}

export function setTheme(mode: ThemeMode) {
  applyTheme(mode)
  try {
    localStorage.setItem(THEME_KEY, mode)
  } catch {
    /* ignore */
  }
}

media.addEventListener('change', () => {
  if (theme.value === 'system') applyTheme('system')
})

// ---------- Toast ----------

export const toasts = reactive<Toast[]>([])
let seq = 0

export function toast(type: Toast['type'], title: string, description?: string, duration = 3600) {
  const id = ++seq
  toasts.push({ id, type, title, description })
  if (duration > 0) {
    window.setTimeout(() => dismissToast(id), duration)
  }
  return id
}

export function dismissToast(id: number) {
  const idx = toasts.findIndex((t) => t.id === id)
  if (idx >= 0) toasts.splice(idx, 1)
}

export const toastHelpers = {
  success: (title: string, description?: string) => toast('success', title, description),
  error: (title: string, description?: string) => toast('error', title, description, 5200),
  info: (title: string, description?: string) => toast('info', title, description),
  warning: (title: string, description?: string) => toast('warning', title, description, 4600),
}

// ---------- 确认对话框 ----------

export interface ConfirmOptions {
  title: string
  description?: string
  confirmText?: string
  cancelText?: string
  danger?: boolean
}

export const confirmState = reactive<{
  open: boolean
  options: ConfirmOptions
  resolve: ((v: boolean) => void) | null
}>({
  open: false,
  options: { title: '' },
  resolve: null,
})

export function confirmDialog(options: ConfirmOptions): Promise<boolean> {
  confirmState.options = options
  confirmState.open = true
  return new Promise<boolean>((resolve) => {
    confirmState.resolve = resolve
  })
}

export function resolveConfirm(value: boolean) {
  confirmState.open = false
  confirmState.resolve?.(value)
  confirmState.resolve = null
}
