<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { AlertTriangle, Plus, Terminal, Trash2, Upload } from 'lucide-vue-next'
import BaseModal from './BaseModal.vue'
import { api, ApiRequestError, getCsrfToken } from '@/api/client'
import type { AppItem, AppScope } from '@/api/types'
import { categories } from '@/stores/apps'
import { toastHelpers } from '@/stores/ui'

const props = defineProps<{ open: boolean; app: AppItem | null }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'saved'): void }>()

interface EndpointDraft {
  name: string
  url: string
}

interface FormState {
  id?: number
  type: AppScope
  name: string
  slug: string
  description: string
  icon: string
  category_id: number
  tags: string
  internal_url: string
  external_url: string
  app_type: 'systemd' | 'command'
  systemd_unit: string
  start_command: string
  stop_command: string
  restart_command: string
  shell_mode: boolean
  start_args: string
  stop_args: string
  restart_args: string
  status_type: 'http' | 'tcp' | 'process' | 'systemd' | 'none'
  status_target: string
  expected_codes: string
  work_dir: string
  environment: string
  enabled: boolean
  auto_start: boolean
  favorite: boolean
  public_visible: boolean
  timeout_seconds: number
  check_timeout_sec: number
  endpoints: EndpointDraft[]
}

function emptyForm(): FormState {
  return {
    type: 'LOCAL',
    name: '',
    slug: '',
    description: '',
    icon: '',
    category_id: 0,
    tags: '',
    internal_url: '',
    external_url: '',
    app_type: 'command',
    systemd_unit: '',
    start_command: '',
    stop_command: '',
    restart_command: '',
    shell_mode: false,
    start_args: '',
    stop_args: '',
    restart_args: '',
    status_type: 'http',
    status_target: '',
    expected_codes: '',
    work_dir: '',
    environment: '',
    enabled: true,
    auto_start: false,
    favorite: false,
    public_visible: true,
    timeout_seconds: 60,
    check_timeout_sec: 5,
    endpoints: [],
  }
}

const form = ref<FormState>(emptyForm())
const saving = ref(false)
const testing = ref('')
const error = ref('')
const tab = ref<'basic' | 'control' | 'detect'>('basic')
const uploadInput = ref<HTMLInputElement | null>(null)

const presetIcons = ['📦', '🔐', '🤖', '📊', '🛠️', '⬇️', '🔑', '💻', '🗄️', '🎬', '🌐', '⚙️', '🚀', '🧩']

const isExternal = computed(() => form.value.type === 'EXTERNAL')

// 标签页：非本地应用没有"运行控制"
const tabs = computed(() => {
  const list: { key: 'basic' | 'control' | 'detect'; label: string }[] = [{ key: 'basic', label: '基本信息' }]
  if (!isExternal.value) list.push({ key: 'control', label: '运行控制' })
  list.push({ key: 'detect', label: '状态检测' })
  return list
})

const subtitle = computed(() => {
  if (isEdit.value) return '修改配置不会影响应用自身的程序文件'
  return isExternal.value
    ? '非本地应用：仅用于导航与在线检测，不含任何启停管理配置'
    : '本地应用：可由 AppHub 启动、停止和管理的应用'
})

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    error.value = ''
    tab.value = 'basic'
    if (props.app) {
      const a = props.app
      form.value = {
        id: a.id,
        type: (a.type as AppScope) || 'LOCAL',
        name: a.name,
        slug: a.slug,
        description: a.description,
        icon: a.icon,
        category_id: a.category_id,
        tags: a.tags,
        internal_url: a.internal_url,
        external_url: a.external_url,
        app_type: a.app_type,
        systemd_unit: a.systemd_unit,
        start_command: a.start_command,
        stop_command: a.stop_command,
        restart_command: a.restart_command,
        shell_mode: a.shell_mode,
        start_args: a.start_args,
        stop_args: a.stop_args,
        restart_args: a.restart_args,
        status_type: a.status_type,
        status_target: a.status_target,
        expected_codes: a.expected_codes,
        work_dir: a.work_dir,
        environment: a.environment,
        enabled: a.enabled,
        auto_start: a.auto_start,
        favorite: a.favorite,
        public_visible: a.public_visible !== false,
        timeout_seconds: a.timeout_seconds || 60,
        check_timeout_sec: a.check_timeout_sec || 5,
        endpoints: [],
      }
      // 加载附加访问入口
      try {
        const list = await api.get<{ name: string; url: string }[]>(`/api/v1/apps/${a.id}/endpoints`)
        form.value.endpoints = (list || []).map((e) => ({ name: e.name, url: e.url }))
      } catch {
        /* 忽略 */
      }
    } else {
      form.value = emptyForm()
    }
  },
  { immediate: true },
)

const isEdit = computed(() => !!form.value.id)

async function save() {
  const f = form.value
  if (isExternal.value) {
    if (!f.external_url.trim() && !f.endpoints.some((e) => e.url.trim())) {
      error.value = '非本地应用必须配置至少一个访问地址'
      return
    }
    if (!f.status_target.trim() && f.status_type !== 'none') {
      f.status_type = 'http'
      f.status_target = f.external_url.trim()
    }
  }
  saving.value = true
  error.value = ''
  try {
    if (isEdit.value) {
      await api.put(`/api/v1/apps/${f.id}`, f)
      toastHelpers.success('已保存', `${f.name} 的配置已更新`)
    } else {
      await api.post('/api/v1/apps', f)
      toastHelpers.success('已添加', `${f.name} 已加入应用中心`)
    }
    emit('saved')
  } catch (e) {
    error.value = (e as ApiRequestError).message || '保存失败'
  } finally {
    saving.value = false
  }
}

async function testCommand(kind: 'start' | 'stop' | 'status') {
  if (!form.value.id) {
    toastHelpers.info('请先保存应用', '测试命令需要应用已存在')
    return
  }
  testing.value = kind
  try {
    const res = await api.post<{ exit_code: number; stdout: string; stderr: string; error?: string }>(
      `/api/v1/apps/${form.value.id}/test`,
      { kind },
    )
    const out = (res.stdout || '') + (res.stderr ? '\n' + res.stderr : '')
    toastHelpers.success(
      `测试${kind === 'start' ? '启动' : kind === 'stop' ? '停止' : '状态'}完成`,
      (out || res.error || `退出码 ${res.exit_code}`).slice(0, 300),
    )
  } catch (e) {
    toastHelpers.error('测试失败', (e as ApiRequestError).message)
  } finally {
    testing.value = ''
  }
}

async function uploadIcon(file: File) {
  const body = new FormData()
  body.append('file', file)
  try {
    const resp = await fetch('/api/v1/upload', {
      method: 'POST',
      body,
      headers: { 'X-CSRF-Token': getCsrfToken() },
    })
    const json = await resp.json()
    if (!json.success) throw new Error(json.data?.message || '上传失败')
    form.value.icon = json.data.url
    toastHelpers.success('图标已上传')
  } catch (e) {
    toastHelpers.error('上传失败', (e as Error).message)
  }
}

function onFileChange(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (file) uploadIcon(file)
  input.value = ''
}

function addEndpoint() {
  form.value.endpoints.push({ name: '', url: '' })
}

function removeEndpoint(idx: number) {
  form.value.endpoints.splice(idx, 1)
}

const statusHint = computed(() => {
  switch (form.value.status_type) {
    case 'http':
      return '填写完整检查地址，留空则使用访问地址'
    case 'tcp':
      return '填写 host:port，例如 127.0.0.1:8080'
    case 'process':
      return '填写进程 PID，留空则使用 AppHub 记录的 PID'
    case 'systemd':
      return '填写服务名，例如 myapp.service'
    default:
      return '不检测状态，始终显示未知'
  }
})
</script>

<template>
  <BaseModal
    :open="open"
    :title="isEdit ? `编辑应用 · ${form.name}` : '添加应用'"
    :subtitle="subtitle"
    width="680px"
    @close="emit('close')"
  >
    <!-- 应用类型 -->
    <div class="mb-4 p-3 rounded-xl border border-line bg-elevated">
      <p class="label mb-2">应用类型 *</p>
      <div class="flex gap-2">
        <button
          type="button"
          class="flex-1 h-11 rounded-xl border text-left px-3 transition-colors"
          :class="form.type === 'LOCAL' ? 'border-accent bg-accent/8' : 'border-line hover:border-accent/50'"
          @click="form.type = 'LOCAL'"
        >
          <span class="block text-[13.5px] font-medium" :class="form.type === 'LOCAL' ? 'text-accent' : 'text-ink'">本地应用</span>
          <span class="block text-[11.5px] text-faint">由本机运行，可启动 / 停止 / 管理</span>
        </button>
        <button
          type="button"
          class="flex-1 h-11 rounded-xl border text-left px-3 transition-colors"
          :class="form.type === 'EXTERNAL' ? 'border-info bg-info/8' : 'border-line hover:border-info/50'"
          @click="form.type = 'EXTERNAL'"
        >
          <span class="block text-[13.5px] font-medium" :class="form.type === 'EXTERNAL' ? 'text-info' : 'text-ink'">非本地应用</span>
          <span class="block text-[11.5px] text-faint">仅导航 + 在线检测，无启停管理</span>
        </button>
      </div>
    </div>

    <!-- 标签页 -->
    <div class="flex items-center gap-1 p-0.5 rounded-xl bg-elevated border border-line mb-4">
      <button
        v-for="t in tabs"
        :key="t.key"
        class="flex-1 h-8 rounded-lg text-[13px] transition-colors"
        :class="tab === t.key ? 'bg-surface text-ink shadow-sm font-medium' : 'text-faint hover:text-ink'"
        @click="tab = t.key"
      >
        {{ t.label }}
      </button>
    </div>

    <form @submit.prevent="save">
      <!-- 基本信息 -->
      <div v-if="tab === 'basic'" class="space-y-4">
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label class="label">应用名称 *</label>
            <input v-model="form.name" class="input" placeholder="例如：Vaultwarden" />
          </div>
          <div>
            <label class="label">英文标识</label>
            <input v-model="form.slug" class="input font-mono text-[13px]" placeholder="留空自动生成" />
          </div>
        </div>

        <div>
          <label class="label">描述</label>
          <input v-model="form.description" class="input" placeholder="例如：私有密码管理" />
        </div>

        <div>
          <label class="label">图标</label>
          <div class="flex items-center gap-2">
            <div class="w-10 h-10 rounded-xl bg-elevated border border-line flex items-center justify-center text-lg shrink-0">
              {{ form.icon || '📦' }}
            </div>
            <input v-model="form.icon" class="input flex-1" placeholder="可直接粘贴 Emoji，或上传图片" />
            <input ref="uploadInput" type="file" accept=".png,.svg,.jpg,.jpeg,.webp,.gif,.ico" class="hidden" @change="onFileChange" />
            <button type="button" class="btn btn-md btn-outline shrink-0" @click="uploadInput?.click()">
              <Upload :size="14" />上传
            </button>
          </div>
          <div class="flex flex-wrap gap-1.5 mt-2">
            <button
              v-for="e in presetIcons"
              :key="e"
              type="button"
              class="w-8 h-8 rounded-lg bg-elevated border border-line hover:border-accent text-base transition-colors"
              @click="form.icon = e"
            >
              {{ e }}
            </button>
          </div>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label class="label">分类</label>
            <select v-model="form.category_id" class="input">
              <option :value="0">未分类</option>
              <option v-for="c in categories.items" :key="c.id" :value="c.id">{{ c.name }}</option>
            </select>
          </div>
          <div>
            <label class="label">标签（逗号分隔）</label>
            <input v-model="form.tags" class="input" placeholder="密码, 私有化" />
          </div>
        </div>

        <!-- 地址：本地 = 内网/公网；非本地 = 访问地址 + 附加入口 -->
        <div v-if="!isExternal" class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label class="label">内网地址</label>
            <input v-model="form.internal_url" class="input font-mono text-[13px]" placeholder="http://192.168.1.72:8080" />
          </div>
          <div>
            <label class="label">公网地址</label>
            <input v-model="form.external_url" class="input font-mono text-[13px]" placeholder="https://vault.example.com" />
          </div>
        </div>
        <p v-if="!isExternal" class="text-[12px] text-faint -mt-1">公网地址由你手动配置，AppHub 不会自动推断</p>

        <template v-if="isExternal">
          <div>
            <label class="label">访问地址 *</label>
            <input v-model="form.external_url" class="input font-mono text-[13px]" placeholder="https://example.com" />
          </div>

          <div>
            <div class="flex items-center justify-between mb-1.5">
              <label class="label !mb-0">附加访问入口（可选）</label>
              <button type="button" class="btn btn-sm btn-outline" @click="addEndpoint">
                <Plus :size="13" />添加入口
              </button>
            </div>
            <div v-if="!form.endpoints.length" class="text-[12px] text-faint py-1">
              例如同一服务的 Web / 管理后台 / API 等多个入口
            </div>
            <div v-for="(ep, idx) in form.endpoints" :key="idx" class="flex items-center gap-2 mb-2">
              <input v-model="ep.name" class="input !h-9 w-28 shrink-0" placeholder="名称" />
              <input v-model="ep.url" class="input !h-9 flex-1 font-mono text-[13px]" placeholder="https://…" />
              <button
                type="button"
                class="w-9 h-9 rounded-xl text-faint hover:text-danger hover:bg-danger/10 flex items-center justify-center shrink-0"
                @click="removeEndpoint(idx)"
              >
                <Trash2 :size="14" />
              </button>
            </div>
          </div>
        </template>
      </div>

      <!-- 运行控制（仅本地应用） -->
      <div v-else-if="tab === 'control'" class="space-y-4">
        <div>
          <label class="label">管理方式</label>
          <div class="flex gap-2">
            <button
              type="button"
              class="flex-1 h-10 rounded-xl border text-[13px] transition-colors"
              :class="form.app_type === 'systemd' ? 'border-accent bg-accent/8 text-accent font-medium' : 'border-line text-muted hover:text-ink'"
              @click="form.app_type = 'systemd'"
            >
              systemd 服务
            </button>
            <button
              type="button"
              class="flex-1 h-10 rounded-xl border text-[13px] transition-colors"
              :class="form.app_type === 'command' ? 'border-accent bg-accent/8 text-accent font-medium' : 'border-line text-muted hover:text-ink'"
              @click="form.app_type = 'command'"
            >
              自定义命令
            </button>
          </div>
        </div>

        <template v-if="form.app_type === 'systemd'">
          <div>
            <label class="label">systemd 服务名 *</label>
            <input v-model="form.systemd_unit" class="input font-mono text-[13px]" placeholder="vaultwarden.service" />
            <p class="text-[12px] text-faint mt-1.5">
              AppHub 将以结构化方式调用 systemctl，服务名需以 .service / .socket / .timer 等结尾
            </p>
          </div>
        </template>

        <template v-else>
          <div>
            <label class="label">启动命令 *</label>
            <input v-model="form.start_command" class="input font-mono text-[13px]" placeholder="/opt/apps/myapp/start.sh" />
          </div>
          <div v-if="!form.shell_mode">
            <label class="label">启动参数（JSON 数组，可选）</label>
            <input v-model="form.start_args" class="input font-mono text-[13px]" placeholder='["--port","8080"]' />
          </div>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label class="label">停止命令</label>
              <input v-model="form.stop_command" class="input font-mono text-[13px]" placeholder="/opt/apps/myapp/stop.sh" />
            </div>
            <div>
              <label class="label">重启命令</label>
              <input v-model="form.restart_command" class="input font-mono text-[13px]" placeholder="留空则执行 停止 + 启动" />
            </div>
          </div>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label class="label">工作目录</label>
              <input v-model="form.work_dir" class="input font-mono text-[13px]" placeholder="/opt/apps/myapp" />
            </div>
            <div>
              <label class="label">启停超时（秒）</label>
              <input v-model.number="form.timeout_seconds" type="number" min="1" max="600" class="input" />
            </div>
          </div>
          <div>
            <label class="label">环境变量（每行 KEY=VALUE）</label>
            <textarea v-model="form.environment" rows="3" class="textarea" placeholder="PORT=8080&#10;LOG_LEVEL=info" />
          </div>

          <label
            class="flex items-start gap-3 p-3 rounded-xl border cursor-pointer transition-colors"
            :class="form.shell_mode ? 'border-warn bg-warn/8' : 'border-line hover:border-accent/60'"
          >
            <input v-model="form.shell_mode" type="checkbox" class="mt-0.5 w-4 h-4 accent-current" />
            <span class="text-[13px]">
              <span class="font-medium text-ink">启用 Shell 命令模式</span>
              <span class="block text-faint mt-0.5 leading-relaxed">
                启用后命令将通过 /bin/sh -c 执行，支持管道与重定向，但该应用具备执行系统命令的能力。
                请确保只有可信管理员能够访问此后台。
              </span>
            </span>
          </label>
        </template>

        <div v-if="isEdit" class="flex items-center gap-2 pt-1">
          <span class="text-[12.5px] text-faint flex items-center gap-1.5"><Terminal :size="13" />命令测试</span>
          <button type="button" class="btn btn-sm btn-outline" :disabled="!!testing" @click="testCommand('start')">
            {{ testing === 'start' ? '测试中…' : '测试启动' }}
          </button>
          <button type="button" class="btn btn-sm btn-outline" :disabled="!!testing" @click="testCommand('stop')">
            {{ testing === 'stop' ? '测试中…' : '测试停止' }}
          </button>
          <button type="button" class="btn btn-sm btn-outline" :disabled="!!testing" @click="testCommand('status')">
            {{ testing === 'status' ? '测试中…' : '测试状态' }}
          </button>
        </div>
      </div>

      <!-- 状态检测 -->
      <div v-else class="space-y-4">
        <div>
          <label class="label">检测方式</label>
          <div class="grid gap-2" :class="isExternal ? 'grid-cols-3' : 'grid-cols-2 sm:grid-cols-5'">
            <button
              v-for="t in isExternal ? ['http', 'tcp', 'none'] : ['http', 'tcp', 'process', 'systemd', 'none']"
              :key="t"
              type="button"
              class="h-9 rounded-xl border text-[12.5px] font-mono transition-colors"
              :class="form.status_type === t ? 'border-accent bg-accent/8 text-accent' : 'border-line text-muted hover:text-ink'"
              @click="form.status_type = t as FormState['status_type']"
            >
              {{ t }}
            </button>
          </div>
          <p v-if="isExternal" class="text-[12px] text-faint mt-1.5">
            非本地应用仅支持 http / tcp 在线检测；检测目标留空时默认使用访问地址
          </p>
        </div>

        <div>
          <label class="label">检测目标</label>
          <input v-model="form.status_target" class="input font-mono text-[13px]" :placeholder="statusHint" />
          <p class="text-[12px] text-faint mt-1.5">{{ statusHint }}</p>
        </div>

        <div v-if="form.status_type === 'http'">
          <label class="label">期望状态码（可选，JSON 数组）</label>
          <input v-model="form.expected_codes" class="input font-mono text-[13px]" placeholder="留空表示 2xx / 3xx 均可" />
        </div>

        <div>
          <label class="label">检测超时（秒）</label>
          <input v-model.number="form.check_timeout_sec" type="number" min="1" max="60" class="input" />
        </div>

        <div class="divider" />

        <div class="space-y-2.5">
          <label class="flex items-center justify-between gap-3 cursor-pointer">
            <span class="text-[13.5px] text-ink">启用该应用</span>
            <input v-model="form.enabled" type="checkbox" class="w-4 h-4" />
          </label>
          <label v-if="!isExternal" class="flex items-center justify-between gap-3 cursor-pointer">
            <span class="text-[13.5px] text-ink">
              开机自动启动
              <span class="block text-[12px] text-faint">默认关闭，仅在 AppHub 启动时拉起</span>
            </span>
            <input v-model="form.auto_start" type="checkbox" class="w-4 h-4" />
          </label>
          <label class="flex items-center justify-between gap-3 cursor-pointer">
            <span class="text-[13.5px] text-ink">
              在前台导航显示
              <span class="block text-[12px] text-faint">关闭后仅后台可见，前台导航不再展示该应用</span>
            </span>
            <input v-model="form.public_visible" type="checkbox" class="w-4 h-4" />
          </label>
          <label v-if="!isExternal" class="flex items-center justify-between gap-3 cursor-pointer">
            <span class="text-[13.5px] text-ink">收藏应用（首页优先显示）</span>
            <input v-model="form.favorite" type="checkbox" class="w-4 h-4" />
          </label>
        </div>

        <div v-if="form.auto_start && !isExternal" class="flex items-start gap-2 p-3 rounded-xl bg-warn/8 border border-warn/40">
          <AlertTriangle :size="15" class="text-warn shrink-0 mt-0.5" />
          <p class="text-[12.5px] text-ink leading-relaxed">
            开启后该应用会在 AppHub 启动时自动拉起。若希望按需启动，请保持关闭。
          </p>
        </div>
      </div>

      <p v-if="error" class="mt-4 text-[12.5px] text-danger">{{ error }}</p>
    </form>

    <template #footer>
      <button class="btn btn-md btn-outline" @click="emit('close')">取消</button>
      <button class="btn btn-md btn-primary" :disabled="saving" @click="save">
        {{ saving ? '保存中…' : isEdit ? '保存修改' : '添加应用' }}
      </button>
    </template>
  </BaseModal>
</template>
