import { createRouter, createWebHistory, RouteRecordRaw } from 'vue-router'
import { auth, bootstrap } from '@/stores/auth'
import { initAppsPolling } from '@/stores/apps'
import { startPublicPolling, stopPublicPolling } from '@/stores/publicStore'

// 路由结构：
//   /            前台只读导航（无需登录）
//   /admin/**    后台管理（需管理员登录）
//   /home、/login、/settings 等旧地址自动重定向，保证旧链接可用
const routes: RouteRecordRaw[] = [
  {
    path: '/',
    name: 'public-home',
    component: () => import('@/views/PublicHome.vue'),
    meta: { public: true, title: '首页' },
  },
  { path: '/home', redirect: { name: 'public-home' } },
  {
    path: '/admin/login',
    name: 'login',
    component: () => import('@/views/LoginView.vue'),
    meta: { public: true, title: '登录' },
  },
  {
    path: '/setup',
    name: 'setup',
    component: () => import('@/views/SetupView.vue'),
    meta: { public: true, title: '初始化' },
  },
  {
    path: '/admin',
    component: () => import('@/views/AppShell.vue'),
    children: [
      { path: '', name: 'dashboard', component: () => import('@/views/DashboardView.vue'), meta: { title: '控制中心' } },
      {
        path: 'apps/:id',
        name: 'app-detail',
        component: () => import('@/views/AppDetailView.vue'),
        meta: { title: '应用详情' },
      },
      {
        path: 'settings',
        component: () => import('@/views/settings/SettingsLayout.vue'),
        children: [
          { path: '', redirect: { name: 'settings-apps' } },
          {
            path: 'apps',
            name: 'settings-apps',
            component: () => import('@/views/settings/SettingsApps.vue'),
            meta: { title: '应用管理' },
          },
          {
            path: 'categories',
            name: 'settings-categories',
            component: () => import('@/views/settings/SettingsCategories.vue'),
            meta: { title: '分类管理' },
          },
          {
            path: 'security',
            name: 'settings-security',
            component: () => import('@/views/settings/SettingsSecurity.vue'),
            meta: { title: '安全设置' },
          },
          {
            path: 'system',
            name: 'settings-system',
            component: () => import('@/views/settings/SettingsSystem.vue'),
            meta: { title: '系统设置' },
          },
          {
            path: 'backup',
            name: 'settings-backup',
            component: () => import('@/views/settings/SettingsBackup.vue'),
            meta: { title: '备份与恢复' },
          },
          {
            path: 'audit',
            name: 'settings-audit',
            component: () => import('@/views/settings/SettingsAudit.vue'),
            meta: { title: '审计日志' },
          },
        ],
      },
    ],
  },
  // 旧地址兼容重定向
  { path: '/login', redirect: { name: 'login' } },
  { path: '/apps/:id', redirect: (to) => ({ name: 'app-detail', params: to.params }) },
  { path: '/settings/:pathMatch(.*)*', redirect: (to) => ({ path: `/admin/settings/${to.params.pathMatch}` }) },
  { path: '/admin/settings', redirect: { name: 'settings-apps' } },
  { path: '/:pathMatch(.*)*', redirect: { name: 'public-home' } },
]

export const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach(async (to) => {
  if (to.name !== 'public-home') {
    // 离开前台页面时停止公共轮询
    stopPublicPolling()
  }
  if (to.name === 'public-home') {
    return true
  }
  if (auth.checking && !auth.user) {
    await bootstrap()
  }
  if (to.meta.public) {
    if (to.name === 'login' && auth.user) return { path: '/admin' }
    return true
  }
  if (!auth.user) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  return true
})

router.afterEach((to) => {
  const title = (to.meta.title as string) || ''
  const site = auth.settings?.site_name || 'AppHub'
  document.title = title ? `${title} · ${site}` : site
  if (to.name === 'public-home') {
    startPublicPolling()
  }
})

// 首次登录成功后启动后台轮询
let pollingStarted = false
export function ensurePolling() {
  if (pollingStarted || !auth.user) return
  pollingStarted = true
  initAppsPolling(auth.settings?.refresh_interval || 5)
}

export function resetPollingFlag() {
  pollingStarted = false
}
