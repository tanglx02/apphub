<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { LogOut, Package, Search, Settings, Shield, User as UserIcon, X } from 'lucide-vue-next'
import AppIcon from '@/components/AppIcon.vue'
import SideNav from '@/components/SideNav.vue'
import BottomNav from '@/components/BottomNav.vue'
import ThemeSwitch from '@/components/ThemeSwitch.vue'
import { auth, logout } from '@/stores/auth'
import { setRefreshInterval } from '@/stores/apps'

const route = useRoute()
const router = useRouter()

// 顶部搜索：首页实时筛选
const query = ref('')
const menuOpen = ref(false)
const now = ref(new Date())
let clock: number | null = null

const refreshOptions = [3, 5, 10, 30]
const refreshSec = ref(auth.settings?.refresh_interval || 5)

onMounted(() => {
  clock = window.setInterval(() => (now.value = new Date()), 1000)
})
onUnmounted(() => {
  if (clock) window.clearInterval(clock)
})

defineExpose({ query })

const timeText = computed(() =>
  now.value.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
)
const dateText = computed(() =>
  now.value.toLocaleDateString('zh-CN', { month: '2-digit', day: '2-digit', weekday: 'short' }),
)

const pageTitle = computed(() => (route.meta.title as string) || '控制中心')

function changeRefresh(sec: number) {
  refreshSec.value = sec
  setRefreshInterval(sec)
}

async function doLogout() {
  await logout()
  router.push({ name: 'login' })
}
</script>

<template>
  <div class="flex h-full bg-canvas">
    <SideNav />

    <div class="flex-1 flex flex-col min-w-0">
      <!-- 顶栏 -->
      <header
        class="h-14 shrink-0 border-b border-line bg-surface/80 backdrop-blur sticky top-0 z-30 flex items-center gap-3 px-4"
      >
        <div class="lg:hidden flex items-center gap-2 min-w-0">
          <div class="w-7 h-7 rounded-lg bg-accent text-white flex items-center justify-center shrink-0">
            <Package :size="15" />
          </div>
          <span class="font-semibold text-[14px] truncate">{{ auth.settings?.site_name || 'AppHub' }}</span>
        </div>

        <div class="hidden lg:flex items-center gap-2 min-w-0">
          <h1 class="text-[15px] font-semibold text-ink truncate">{{ pageTitle }}</h1>
        </div>

        <!-- 搜索 -->
        <div v-if="route.name === 'dashboard'" class="flex-1 max-w-[420px] ml-auto relative">
          <Search :size="15" class="absolute left-3 top-1/2 -translate-y-1/2 text-faint pointer-events-none" />
          <input
            v-model="query"
            class="input h-9 pl-9 pr-8 text-[13px]"
            placeholder="搜索应用名称、描述、标签…"
          />
          <button
            v-if="query"
            class="absolute right-2.5 top-1/2 -translate-y-1/2 text-faint hover:text-ink"
            @click="query = ''"
          >
            <X :size="14" />
          </button>
        </div>
        <div v-else class="flex-1" />

        <div class="hidden sm:flex items-center gap-2 text-[12px] text-faint font-mono tabular-nums">
          <span>{{ dateText }}</span>
          <span class="text-ink">{{ timeText }}</span>
        </div>

        <div class="hidden md:flex items-center gap-1.5">
          <span class="text-[11.5px] text-faint">刷新</span>
          <select
            class="h-8 rounded-lg bg-elevated border border-line text-[12px] text-ink px-2 outline-none"
            :value="refreshSec"
            @change="changeRefresh(Number(($event.target as HTMLSelectElement).value))"
          >
            <option v-for="s in refreshOptions" :key="s" :value="s">{{ s }}s</option>
          </select>
        </div>

        <ThemeSwitch />

        <!-- 用户菜单 -->
        <div class="relative" @mouseleave="menuOpen = false">
          <button
            class="w-8 h-8 rounded-xl bg-elevated border border-line flex items-center justify-center text-ink hover:border-accent transition-colors"
            @click="menuOpen = !menuOpen"
          >
            <UserIcon :size="15" />
          </button>
          <div
            v-if="menuOpen"
            class="absolute right-0 top-10 w-52 card py-1 z-50 animate-fade-in"
            :style="{ boxShadow: '0 12px 32px rgb(0 0 0 / 0.16)' }"
          >
            <div class="px-3 py-2 border-b border-line mb-1">
              <p class="text-[13px] font-medium text-ink">{{ auth.user?.username }}</p>
              <p class="text-[11.5px] text-faint">管理员 · v{{ auth.settings?.version?.replace('v', '') }}</p>
            </div>
            <router-link
              :to="{ name: 'settings-system' }"
              class="w-full px-3 h-9 flex items-center gap-2 text-[13px] text-ink hover:bg-elevated"
            >
              <Settings :size="14" />系统设置
            </router-link>
            <router-link
              :to="{ name: 'settings-security' }"
              class="w-full px-3 h-9 flex items-center gap-2 text-[13px] text-ink hover:bg-elevated"
            >
              <Shield :size="14" />安全设置
            </router-link>
            <div class="divider my-1" />
            <button
              class="w-full px-3 h-9 flex items-center gap-2 text-[13px] text-danger hover:bg-danger/10"
              @click="doLogout"
            >
              <LogOut :size="14" />退出登录
            </button>
          </div>
        </div>
      </header>

      <!-- 内容 -->
      <main class="flex-1 overflow-y-auto pb-20 lg:pb-0">
        <router-view v-slot="{ Component }">
          <component :is="Component" :search="query" />
        </router-view>
      </main>

      <BottomNav />
    </div>
  </div>
</template>
