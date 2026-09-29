<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Info, Save } from 'lucide-vue-next'
import { api, ApiRequestError } from '@/api/client'
import type { AppConfig } from '@/api/types'
import { auth, bootstrap } from '@/stores/auth'
import { setRefreshInterval } from '@/stores/apps'
import { toastHelpers } from '@/stores/ui'

const loading = ref(true)
const saving = ref(false)
const needRestart = ref(false)
const paths = ref<Record<string, unknown>>({})
const runtime = ref<Record<string, any>>({
  site_name: '',
  logo: '',
  allow_batch_start: false,
  refresh_interval: 5,
  public_enabled: true,
  public_title: '',
  public_subtitle: '',
  public_show_resources: true,
  public_show_categories: true,
  public_show_search: true,
  public_allow_favorite: true,
  public_default_theme: 'system',
})
const cfg = reactive<AppConfig>({
  server: {
    host: '0.0.0.0',
    port: 18080,
    https: false,
    redirect_http: false,
    http_port: 18081,
    trusted_proxies: '',
    site_name: 'AppHub',
    logo: '',
    timezone: 'Asia/Shanghai',
    read_timeout_sec: 30,
    write_timeout_sec: 60,
  },
  database: { path: './data/apphub.db', busy_timeout_ms: 5000 },
  logging: { level: 'info', retention_days: 30, max_size_mb: 20, console: true },
  security: {
    session_timeout: 86400,
    login_max_fails: 5,
    login_lock_minutes: 15,
    rate_limit_per_min: 120,
    csrf_enabled: true,
    cookie_secure: 'auto',
  },
  status: { interval: 5, timeout: 5, concurrency: 4, start_timeout: 60, stop_timeout: 30, restart_timeout: 90 },
  tls: { enabled: false, self_signed: false, cert_file: './data/certs/server.crt', key_file: './data/certs/server.key', auto_generate: true },
  backup: { directory: './backups', retention_days: 30, auto_enabled: false },
  executor: { max_output_bytes: 1048576, kill_process_tree: true, grace_period_sec: 10 },
})

async function load() {
  loading.value = true
  try {
    const res = await api.get<{ config: AppConfig; runtime: typeof runtime.value; paths: Record<string, unknown> }>(
      '/api/v1/settings',
    )
    Object.assign(cfg, res.config)
    Object.assign(runtime.value, res.runtime)
    paths.value = res.paths
  } catch (e) {
    toastHelpers.error('读取设置失败', (e as ApiRequestError).message)
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    const res = await api.put<{ need_restart: boolean }>('/api/v1/settings', {
      config: cfg,
      runtime: {
        site_name: runtime.value.site_name,
        logo: runtime.value.logo,
        allow_batch_start: String(runtime.value.allow_batch_start),
        refresh_interval: String(runtime.value.refresh_interval),
        public_enabled: String(runtime.value.public_enabled),
        public_title: runtime.value.public_title || '',
        public_subtitle: runtime.value.public_subtitle || '',
        public_show_resources: String(runtime.value.public_show_resources),
        public_show_categories: String(runtime.value.public_show_categories),
        public_show_search: String(runtime.value.public_show_search),
        public_allow_favorite: String(runtime.value.public_allow_favorite),
        public_default_theme: runtime.value.public_default_theme || 'system',
      },
    })
    needRestart.value = res.need_restart
    setRefreshInterval(runtime.value.refresh_interval)
    await bootstrap()
    toastHelpers.success(
      '设置已保存',
      res.need_restart ? '监听地址或 HTTPS 变更需要重启服务后生效' : '已即时生效',
    )
  } catch (e) {
    toastHelpers.error('保存失败', (e as ApiRequestError).message)
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <div v-if="loading" class="card p-10 text-center text-[13px] text-muted">正在读取设置…</div>

  <div v-else class="space-y-4">
    <!-- 基本 -->
    <section class="card p-5">
      <h2 class="text-[14px] font-semibold text-ink mb-4">站点与监听</h2>
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div>
          <label class="label">系统名称</label>
          <input v-model="runtime.site_name" class="input" />
        </div>
        <div>
          <label class="label">Logo 地址（留空使用默认）</label>
          <input v-model="runtime.logo" class="input font-mono text-[13px]" placeholder="/uploads/logo.png" />
        </div>
        <div>
          <label class="label">监听地址</label>
          <input v-model="cfg.server.host" class="input font-mono text-[13px]" placeholder="0.0.0.0" />
        </div>
        <div>
          <label class="label">监听端口</label>
          <input v-model.number="cfg.server.port" type="number" class="input" />
        </div>
        <div>
          <label class="label">时区</label>
          <input v-model="cfg.server.timezone" class="input" placeholder="Asia/Shanghai" />
        </div>
        <div>
          <label class="label">可信反向代理（逗号分隔 CIDR）</label>
          <input v-model="cfg.server.trusted_proxies" class="input font-mono text-[13px]" placeholder="127.0.0.1/32" />
        </div>
      </div>
    </section>

    <!-- HTTPS -->
    <section class="card p-5">
      <h2 class="text-[14px] font-semibold text-ink mb-4">HTTPS</h2>
      <div class="space-y-3">
        <label class="flex items-center justify-between gap-3 cursor-pointer">
          <span class="text-[13.5px] text-ink">启用 HTTPS</span>
          <input v-model="cfg.tls.enabled" type="checkbox" class="w-4 h-4" />
        </label>
        <label class="flex items-center justify-between gap-3 cursor-pointer">
          <span class="text-[13.5px] text-ink">HTTP 自动重定向到 HTTPS</span>
          <input v-model="cfg.server.redirect_http" type="checkbox" class="w-4 h-4" />
        </label>
        <div v-if="cfg.server.redirect_http" class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label class="label">HTTP 监听端口</label>
            <input v-model.number="cfg.server.http_port" type="number" class="input" />
          </div>
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label class="label">证书文件路径</label>
            <input v-model="cfg.tls.cert_file" class="input font-mono text-[13px]" />
          </div>
          <div>
            <label class="label">私钥文件路径</label>
            <input v-model="cfg.tls.key_file" class="input font-mono text-[13px]" />
          </div>
        </div>
        <label class="flex items-center justify-between gap-3 cursor-pointer">
          <span class="text-[13.5px] text-ink">
            自动生成自签名证书
            <span class="block text-[12px] text-faint">证书不存在时自动生成，适合内网与穿透场景</span>
          </span>
          <input v-model="cfg.tls.auto_generate" type="checkbox" class="w-4 h-4" />
        </label>
      </div>
    </section>

    <!-- 状态检测与安全 -->
    <section class="card p-5">
      <h2 class="text-[14px] font-semibold text-ink mb-4">状态检测</h2>
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <div>
          <label class="label">检测间隔（秒）</label>
          <input v-model.number="cfg.status.interval" type="number" min="3" class="input" />
        </div>
        <div>
          <label class="label">单次检测超时（秒）</label>
          <input v-model.number="cfg.status.timeout" type="number" min="1" class="input" />
        </div>
        <div>
          <label class="label">检测并发数</label>
          <input v-model.number="cfg.status.concurrency" type="number" min="1" class="input" />
        </div>
        <div>
          <label class="label">页面刷新间隔（秒）</label>
          <select v-model.number="runtime.refresh_interval" class="input">
            <option :value="3">3 秒</option>
            <option :value="5">5 秒</option>
            <option :value="10">10 秒</option>
            <option :value="30">30 秒</option>
          </select>
        </div>
        <div>
          <label class="label">启动命令超时（秒）</label>
          <input v-model.number="cfg.status.start_timeout" type="number" class="input" />
        </div>
        <div>
          <label class="label">停止命令超时（秒）</label>
          <input v-model.number="cfg.status.stop_timeout" type="number" class="input" />
        </div>
        <div>
          <label class="label">命令最大输出（字节）</label>
          <input v-model.number="cfg.executor.max_output_bytes" type="number" class="input" />
        </div>
        <div>
          <label class="label">优雅停止等待（秒）</label>
          <input v-model.number="cfg.executor.grace_period_sec" type="number" class="input" />
        </div>
      </div>
      <label class="flex items-center justify-between gap-3 cursor-pointer mt-3">
        <span class="text-[13.5px] text-ink">停止时清理子进程组（避免残留）</span>
        <input v-model="cfg.executor.kill_process_tree" type="checkbox" class="w-4 h-4" />
      </label>
      <label class="flex items-center justify-between gap-3 cursor-pointer mt-3">
        <span class="text-[13.5px] text-ink">
          允许一键启动全部应用
          <span class="block text-[12px] text-faint">默认关闭，避免同时拉起大量应用</span>
        </span>
        <input v-model="runtime.allow_batch_start" type="checkbox" class="w-4 h-4" />
      </label>
    </section>

    <!-- 安全与日志 -->
    <section class="card p-5">
      <h2 class="text-[14px] font-semibold text-ink mb-4">安全与日志</h2>
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <div>
          <label class="label">会话有效期（秒）</label>
          <input v-model.number="cfg.security.session_timeout" type="number" class="input" />
        </div>
        <div>
          <label class="label">登录失败锁定次数</label>
          <input v-model.number="cfg.security.login_max_fails" type="number" class="input" />
        </div>
        <div>
          <label class="label">锁定时长（分钟）</label>
          <input v-model.number="cfg.security.login_lock_minutes" type="number" class="input" />
        </div>
        <div>
          <label class="label">每分钟请求上限</label>
          <input v-model.number="cfg.security.rate_limit_per_min" type="number" class="input" />
        </div>
        <div>
          <label class="label">日志级别</label>
          <select v-model="cfg.logging.level" class="input">
            <option value="debug">debug</option>
            <option value="info">info</option>
            <option value="warn">warn</option>
            <option value="error">error</option>
          </select>
        </div>
        <div>
          <label class="label">日志保留天数</label>
          <input v-model.number="cfg.logging.retention_days" type="number" class="input" />
        </div>
        <div>
          <label class="label">单文件最大（MB）</label>
          <input v-model.number="cfg.logging.max_size_mb" type="number" class="input" />
        </div>
        <div>
          <label class="label">备份保留天数</label>
          <input v-model.number="cfg.backup.retention_days" type="number" class="input" />
        </div>
      </div>
      <label class="flex items-center justify-between gap-3 cursor-pointer mt-3">
        <span class="text-[13.5px] text-ink">启用 CSRF 防护</span>
        <input v-model="cfg.security.csrf_enabled" type="checkbox" class="w-4 h-4" />
      </label>
    </section>

    <!-- 前台导航设置 -->
    <section class="card p-5">
      <h2 class="text-[14px] font-semibold text-ink mb-4">前台导航</h2>
      <p class="text-[12.5px] text-muted mb-4">
        前台（系统首页）是只读应用导航：仅可查看状态、打开应用，无任何管理操作。
      </p>
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div>
          <label class="label">前台标题</label>
          <input v-model="runtime.public_title" class="input" placeholder="我的服务器" />
        </div>
        <div>
          <label class="label">副标题</label>
          <input v-model="runtime.public_subtitle" class="input" placeholder="应用导航" />
        </div>
        <div>
          <label class="label">默认主题（未手动切换时生效）</label>
          <select v-model="runtime.public_default_theme" class="input">
            <option value="system">跟随系统</option>
            <option value="light">浅色</option>
            <option value="dark">深色</option>
          </select>
        </div>
      </div>
      <div class="mt-3 space-y-2.5">
        <label class="flex items-center justify-between gap-3 cursor-pointer">
          <span class="text-[13.5px] text-ink">
            启用前台导航
            <span class="block text-[12px] text-faint">关闭后访问系统首页将提示"前台未启用"</span>
          </span>
          <input v-model="runtime.public_enabled" type="checkbox" class="w-4 h-4" />
        </label>
        <label class="flex items-center justify-between gap-3 cursor-pointer">
          <span class="text-[13.5px] text-ink">显示服务器资源（CPU / 内存）</span>
          <input v-model="runtime.public_show_resources" type="checkbox" class="w-4 h-4" />
        </label>
        <label class="flex items-center justify-between gap-3 cursor-pointer">
          <span class="text-[13.5px] text-ink">显示分类筛选</span>
          <input v-model="runtime.public_show_categories" type="checkbox" class="w-4 h-4" />
        </label>
        <label class="flex items-center justify-between gap-3 cursor-pointer">
          <span class="text-[13.5px] text-ink">显示搜索框</span>
          <input v-model="runtime.public_show_search" type="checkbox" class="w-4 h-4" />
        </label>
        <label class="flex items-center justify-between gap-3 cursor-pointer">
          <span class="text-[13.5px] text-ink">允许前台收藏（仅保存在访客浏览器本地）</span>
          <input v-model="runtime.public_allow_favorite" type="checkbox" class="w-4 h-4" />
        </label>
      </div>
    </section>

    <!-- 路径信息 -->
    <section class="card p-5">
      <div class="flex items-center gap-2 mb-3">
        <Info :size="15" class="text-faint" />
        <h2 class="text-[14px] font-semibold text-ink">路径与版本</h2>
      </div>
      <dl class="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-2 text-[12.5px] font-mono">
        <div v-for="(v, k) in paths" :key="k" class="flex items-center justify-between gap-3 border-b border-line/50 pb-1.5">
          <dt class="text-faint">{{ k }}</dt>
          <dd class="text-ink truncate max-w-[320px]" :title="String(v)">{{ v }}</dd>
        </div>
      </dl>
      <p class="text-[12.5px] text-faint mt-3">
        当前版本 v{{ auth.settings?.version?.replace('v', '') }} · 数据库版本 {{ auth.settings?.db_version }}
      </p>
    </section>

    <div class="flex items-center gap-3 sticky bottom-4">
      <button class="btn btn-md btn-primary" :disabled="saving" @click="save">
        <Save :size="15" />{{ saving ? '保存中…' : '保存设置' }}
      </button>
      <span v-if="needRestart" class="text-[12.5px] text-warn">
        监听地址或 HTTPS 相关变更需要重启服务后生效：systemctl restart apphub
      </span>
    </div>
  </div>
</template>
