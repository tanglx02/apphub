import type { ApiError, ApiResponse } from './types'

// API 客户端：统一处理 CSRF、错误、401 跳转

let csrfToken = ''
const listeners = new Set<() => void>()

export function setCsrfToken(token: string) {
  csrfToken = token
  try {
    localStorage.setItem('apphub.csrf', token)
  } catch {
    /* 隐私模式下忽略 */
  }
}

export function getCsrfToken(): string {
  if (csrfToken) return csrfToken
  try {
    csrfToken = localStorage.getItem('apphub.csrf') || ''
  } catch {
    csrfToken = ''
  }
  return csrfToken
}

export function clearCsrfToken() {
  csrfToken = ''
  try {
    localStorage.removeItem('apphub.csrf')
  } catch {
    /* ignore */
  }
}

export function onUnauthorized(fn: () => void) {
  listeners.add(fn)
  return () => listeners.delete(fn)
}

export class ApiRequestError extends Error {
  code: string
  status: number
  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

interface RequestOptions {
  method?: string
  body?: unknown
  query?: Record<string, string | number | undefined>
  signal?: AbortSignal
  raw?: boolean
}

function buildURL(path: string, query?: Record<string, string | number | undefined>) {
  const url = path.startsWith('http') ? path : path
  if (!query) return url
  const params = new URLSearchParams()
  for (const [k, v] of Object.entries(query)) {
    if (v !== undefined && v !== '') params.append(k, String(v))
  }
  const qs = params.toString()
  return qs ? `${url}?${qs}` : url
}

export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = 'GET', body, query, signal } = options
  const headers: Record<string, string> = {
    Accept: 'application/json',
  }
  const init: RequestInit = {
    method,
    headers,
    credentials: 'same-origin',
    signal,
  }

  if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
    init.body = JSON.stringify(body)
  }
  if (method !== 'GET' && method !== 'HEAD') {
    const token = getCsrfToken()
    if (token) headers['X-CSRF-Token'] = token
  }

  let resp: Response
  try {
    resp = await fetch(buildURL(path, query), init)
  } catch (e) {
    if ((e as Error).name === 'AbortError') throw e
    throw new ApiRequestError(0, 'NETWORK', '网络请求失败，请检查与服务端的连接')
  }

  if (resp.status === 401) {
    listeners.forEach((fn) => fn())
    throw new ApiRequestError(401, 'UNAUTHORIZED', '登录状态已失效，请重新登录')
  }

  const text = await resp.text()
  let payload: ApiResponse<T> | null = null
  try {
    payload = text ? JSON.parse(text) : null
  } catch {
    /* 非 JSON 响应 */
  }

  if (!payload) {
    throw new ApiRequestError(resp.status, 'BAD_RESPONSE', `服务端返回异常（HTTP ${resp.status}）`)
  }
  if (!payload.success) {
    const err = payload.data as unknown as ApiError
    throw new ApiRequestError(resp.status, err?.code || 'UNKNOWN', err?.message || payload.message || '操作失败')
  }
  return payload.data as T
}

export const api = {
  get: <T>(path: string, query?: Record<string, string | number | undefined>, signal?: AbortSignal) =>
    request<T>(path, { method: 'GET', query, signal }),
  post: <T>(path: string, body?: unknown) => request<T>(path, { method: 'POST', body }),
  put: <T>(path: string, body?: unknown) => request<T>(path, { method: 'PUT', body }),
  del: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
}
