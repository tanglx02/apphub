// 与后端 /api/v1 对齐的类型定义

export type AppStatus = 'ONLINE' | 'OFFLINE' | 'STARTING' | 'STOPPING' | 'ERROR' | 'UNKNOWN'
export type AppType = 'systemd' | 'command'
export type AppScope = 'LOCAL' | 'EXTERNAL'
export type StatusCheckType = 'http' | 'tcp' | 'process' | 'systemd' | 'none'

export interface AppRuntime {
  app_id: number
  status: AppStatus
  pid?: number
  started_at?: string
  uptime_seconds?: number
  last_checked: string
  last_error?: string
  detail?: string
  cpu_percent?: number
  memory_mb?: number
}

export interface AppItem {
  id: number
  name: string
  type: AppScope
  slug: string
  description: string
  icon: string
  category_id: number
  tags: string
  internal_url: string
  external_url: string
  app_type: AppType
  systemd_unit: string
  start_command: string
  stop_command: string
  restart_command: string
  shell_mode: boolean
  start_args: string
  stop_args: string
  restart_args: string
  status_type: StatusCheckType
  status_target: string
  expected_codes: string
  work_dir: string
  environment: string
  enabled: boolean
  auto_start: boolean
  favorite: boolean
  public_visible: boolean
  sort_order: number
  timeout_seconds: number
  check_timeout_sec: number
  created_at: string
  updated_at: string
  runtime: AppRuntime
  category_name?: string
  category_icon?: string
  category_color?: string
}

export interface Category {
  id: number
  name: string
  slug: string
  icon: string
  color: string
  sort_order: number
  created_at: string
}

export interface Summary {
  total: number
  online: number
  offline: number
  error: number
  rate: number
  updated_at: string
}

export interface PublicUser {
  id: number
  username: string
  role: string
  created_at: string
  last_login_at?: string
  last_login_ip?: string
}

export interface SiteSettings {
  site_name: string
  logo: string
  refresh_interval: number
  allow_batch_start: boolean
  timezone: string
  https: boolean
  version: string
  db_version: number
}

export interface SystemInfo {
  version: string
  commit: string
  build_time: string
  hostname: string
  os: string
  arch: string
  kernel: string
  uptime_seconds: number
  cpu_percent: number
  cpu_cores: number
  cpu_model: string
  mem_total_mb: number
  mem_used_mb: number
  disk_total_gb: number
  disk_used_gb: number
  load_avg: string
  ip: string
  timezone: string
  db_version: number
  api_version: string
}

export interface AuditLog {
  id: number
  user_id: number
  username: string
  ip: string
  action: string
  action_cn: string
  target: string
  result: string
  detail: string
  created_at: string
}

export interface BackupRecord {
  id: number
  name: string
  path: string
  size_bytes: number
  note: string
  created_at: string
}

export interface BatchResult {
  total: number
  success: number
  failed: number
  items: { app_id: number; name: string; ok: boolean; message?: string }[]
}

export interface ApiResponse<T> {
  success: boolean
  data: T | null
  message: string
  request_id: string
}

export interface ApiError {
  code: string
  message: string
}

export interface AppConfig {
  server: {
    host: string
    port: number
    https: boolean
    redirect_http: boolean
    http_port: number
    trusted_proxies: string
    site_name: string
    logo: string
    timezone: string
    read_timeout_sec: number
    write_timeout_sec: number
  }
  database: { path: string; busy_timeout_ms: number }
  logging: { level: string; retention_days: number; max_size_mb: number; console: boolean }
  security: {
    session_timeout: number
    login_max_fails: number
    login_lock_minutes: number
    rate_limit_per_min: number
    csrf_enabled: boolean
    cookie_secure: string
  }
  status: {
    interval: number
    timeout: number
    concurrency: number
    start_timeout: number
    stop_timeout: number
    restart_timeout: number
  }
  tls: { enabled: boolean; self_signed: boolean; cert_file: string; key_file: string; auto_generate: boolean }
  backup: { directory: string; retention_days: number; auto_enabled: boolean }
  executor: { max_output_bytes: number; kill_process_tree: boolean; grace_period_sec: number }
}

// ---------- 前台只读导航（Public API） ----------

export interface PublicEndpoint {
  name: string
  url: string
  open_new_tab: boolean
}

export interface PublicApp {
  id: number
  name: string
  type: AppScope
  description?: string
  icon?: string
  category?: string
  category_icon?: string
  category_color?: string
  status: AppStatus
  endpoints: PublicEndpoint[]
  tags?: string
  sort_order: number
}

export interface PublicConfig {
  enabled: boolean
  title: string
  subtitle: string
  show_resources: boolean
  show_categories: boolean
  show_search: boolean
  allow_favorite: boolean
  default_theme: string
}

export interface PublicSummary {
  total: number
  local_apps: number
  external_apps: number
  online: number
  offline: number
  error: number
  starting: number
  online_rate: number
  server_up: boolean
  cpu_percent?: number
  mem_total_mb?: number
  mem_used_mb?: number
  updated_at: string
}
