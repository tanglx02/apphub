<script setup lang="ts">
import { Cpu, MemoryStick } from 'lucide-vue-next'
import type { PublicConfig, PublicSummary } from '@/api/types'

// 前台服务器概览：应用全部关闭 ≠ 服务器离线
defineProps<{
  config: PublicConfig
  summary: PublicSummary
}>()
</script>

<template>
  <section class="card p-5">
    <div class="flex items-start justify-between gap-4 flex-wrap">
      <div>
        <div class="flex items-center gap-2.5">
          <span class="status-dot bg-ok animate-pulse-dot" />
          <h1 class="text-[18px] font-semibold text-ink tracking-tight">{{ config.title }}</h1>
        </div>
        <p class="text-[13px] text-muted mt-1">{{ config.subtitle || '应用导航' }}</p>
      </div>
      <div class="text-right">
        <p class="text-[11.5px] text-faint">在线率</p>
        <p class="text-[22px] font-semibold text-ink tabular-nums leading-tight">{{ summary.online_rate }}%</p>
      </div>
    </div>

    <div class="mt-4 grid grid-cols-4 gap-2">
      <div class="rounded-xl bg-elevated px-3 py-2.5">
        <p class="text-[11.5px] text-faint">应用</p>
        <p class="text-[19px] font-semibold text-ink tabular-nums">{{ summary.total }}</p>
      </div>
      <div class="rounded-xl bg-ok/10 px-3 py-2.5">
        <p class="text-[11.5px] text-ok/80">运行中</p>
        <p class="text-[19px] font-semibold text-ok tabular-nums">{{ summary.online }}</p>
      </div>
      <div class="rounded-xl bg-elevated px-3 py-2.5">
        <p class="text-[11.5px] text-faint">已停止</p>
        <p class="text-[19px] font-semibold text-muted tabular-nums">{{ summary.offline }}</p>
      </div>
      <div class="rounded-xl bg-danger/10 px-3 py-2.5">
        <p class="text-[11.5px] text-danger/80">异常</p>
        <p class="text-[19px] font-semibold text-danger tabular-nums">{{ summary.error }}</p>
      </div>
    </div>

    <!-- 系统资源（后台可关闭；低频刷新） -->
    <div
      v-if="config.show_resources && (summary.mem_total_mb || summary.cpu_percent)"
      class="mt-3 pt-3 border-t border-line flex items-center gap-4 text-[12px] text-faint flex-wrap"
    >
      <span class="flex items-center gap-1.5">
        <Cpu :size="13" />
        CPU
        <span class="text-ink font-mono tabular-nums">{{ (summary.cpu_percent || 0).toFixed(0) }}%</span>
      </span>
      <span class="flex items-center gap-1.5">
        <MemoryStick :size="13" />
        内存
        <span class="text-ink font-mono tabular-nums">
          {{ summary.mem_used_mb || 0 }} / {{ summary.mem_total_mb || 0 }} MB
        </span>
      </span>
      <span class="ml-auto hidden sm:inline">服务器在线</span>
    </div>
  </section>
</template>
