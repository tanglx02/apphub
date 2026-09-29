<script setup lang="ts">
import { computed } from 'vue'
import {
  Activity,
  AlertTriangle,
  Box,
  Check,
  ChevronRight,
  CircleDot,
  Clock,
  Code,
  Copy,
  Cpu,
  Database,
  Download,
  ExternalLink,
  Film,
  Folder,
  Globe,
  HardDrive,
  KeyRound,
  LayoutGrid,
  Loader2,
  Network,
  Package,
  Pencil,
  Play,
  RotateCw,
  Server,
  Settings,
  Shield,
  Sparkles,
  Square,
  Terminal,
  Workflow,
  Zap,
} from 'lucide-vue-next'

// 仅按需引入常用图标，控制包体与首屏体积
const registry: Record<string, unknown> = {
  Activity,
  AlertTriangle,
  Box,
  Check,
  ChevronRight,
  CircleDot,
  Clock,
  Code,
  Copy,
  Cpu,
  Database,
  Download,
  ExternalLink,
  Film,
  Folder,
  Globe,
  HardDrive,
  KeyRound,
  LayoutGrid,
  Loader2,
  Network,
  Package,
  Pencil,
  Play,
  RotateCw,
  Server,
  Settings,
  Shield,
  Sparkles,
  Square,
  Terminal,
  Workflow,
  Zap,
}

const props = withDefaults(
  defineProps<{
    name?: string
    size?: number | string
    color?: string
    strokeWidth?: number
  }>(),
  { name: '', size: 18, strokeWidth: 1.75 },
)

const isUrl = computed(() => !!props.name && (props.name.startsWith('/') || props.name.startsWith('http')))
const isEmoji = computed(
  () => !!props.name && !isUrl.value && !registry[props.name] && !/^[A-Za-z]+$/.test(props.name),
)
const component = computed(() => (registry[props.name] as never) || LayoutGrid)
</script>

<template>
  <img
    v-if="isUrl"
    :src="name"
    :style="{ width: typeof size === 'number' ? size + 'px' : size, height: typeof size === 'number' ? size + 'px' : size }"
    class="object-contain rounded-md"
    alt=""
  />
  <span
    v-else-if="isEmoji"
    :style="{ fontSize: (typeof size === 'number' ? size : 18) + 'px', lineHeight: 1 }"
    >{{ name }}</span
  >
  <component
    :is="component"
    v-else
    :size="Number(size)"
    :stroke-width="strokeWidth"
    :color="color"
  />
</template>
