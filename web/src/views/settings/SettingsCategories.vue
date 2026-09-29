<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Plus, Pencil, Trash2 } from 'lucide-vue-next'
import AppIcon from '@/components/AppIcon.vue'
import BaseModal from '@/components/BaseModal.vue'
import { api, ApiRequestError } from '@/api/client'
import type { Category } from '@/api/types'
import { categories, refreshCategories } from '@/stores/apps'
import { confirmDialog, toastHelpers } from '@/stores/ui'

const open = ref(false)
const editing = ref<Category | null>(null)
const form = reactive({ name: '', slug: '', icon: 'LayoutGrid', color: '#64748B' })

const presetColors = ['#8B5CF6', '#EF4444', '#06B6D4', '#64748B', '#F59E0B', '#10B981', '#3B82F6', '#0EA5E9', '#EC4899', '#14B8A6', '#A855F7', '#94A3B8']

function openCreate() {
  editing.value = null
  form.name = ''
  form.slug = ''
  form.icon = 'LayoutGrid'
  form.color = '#64748B'
  open.value = true
}

function openEdit(c: Category) {
  editing.value = c
  form.name = c.name
  form.slug = c.slug
  form.icon = c.icon
  form.color = c.color || '#64748B'
  open.value = true
}

async function save() {
  if (!form.name.trim()) {
    toastHelpers.error('分类名称不能为空')
    return
  }
  try {
    if (editing.value) {
      await api.put(`/api/v1/categories/${editing.value.id}`, form)
      toastHelpers.success('已保存分类')
    } else {
      await api.post('/api/v1/categories', form)
      toastHelpers.success('已创建分类')
    }
    open.value = false
    refreshCategories()
  } catch (e) {
    toastHelpers.error('保存失败', (e as ApiRequestError).message)
  }
}

async function remove(c: Category) {
  const ok = await confirmDialog({
    title: `删除分类「${c.name}」`,
    description: '使用该分类的应用不会被删除，只会变为未分类。',
    confirmText: '删除',
    danger: true,
  })
  if (!ok) return
  try {
    await api.del(`/api/v1/categories/${c.id}`)
    toastHelpers.success('已删除分类')
    refreshCategories()
  } catch (e) {
    toastHelpers.error('删除失败', (e as ApiRequestError).message)
  }
}

onMounted(() => refreshCategories())
</script>

<template>
  <div>
    <div class="flex items-center justify-between gap-3 mb-4">
      <p class="text-[13px] text-muted">分类用于在首页筛选和归类应用</p>
      <button class="btn btn-md btn-primary" @click="openCreate"><Plus :size="15" />新建分类</button>
    </div>

    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
      <div v-for="c in categories.items" :key="c.id" class="card p-4 flex items-center gap-3">
        <div
          class="w-10 h-10 rounded-xl bg-elevated border border-line flex items-center justify-center shrink-0"
          :style="{ color: c.color || undefined }"
        >
          <AppIcon :name="c.icon || 'Folder'" :size="18" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-[14px] font-medium text-ink truncate">{{ c.name }}</p>
          <p class="text-[11.5px] text-faint font-mono truncate">{{ c.slug }}</p>
        </div>
        <button class="btn btn-sm btn-ghost" @click="openEdit(c)"><Pencil :size="14" /></button>
        <button class="btn btn-sm btn-ghost !text-danger" @click="remove(c)"><Trash2 :size="14" /></button>
      </div>
    </div>

    <BaseModal :open="open" :title="editing ? '编辑分类' : '新建分类'" width="480px" @close="open = false">
      <div class="space-y-4">
        <div>
          <label class="label">分类名称 *</label>
          <input v-model="form.name" class="input" placeholder="例如：安全" />
        </div>
        <div>
          <label class="label">标识（留空自动生成）</label>
          <input v-model="form.slug" class="input font-mono text-[13px]" placeholder="security" />
        </div>
        <div>
          <label class="label">图标名称（Lucide）</label>
          <input v-model="form.icon" class="input font-mono text-[13px]" placeholder="Shield" />
        </div>
        <div>
          <label class="label">颜色</label>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="c in presetColors"
              :key="c"
              type="button"
              class="w-7 h-7 rounded-lg border-2 transition-transform"
              :class="form.color === c ? 'border-ink scale-110' : 'border-transparent'"
              :style="{ background: c }"
              @click="form.color = c"
            />
          </div>
        </div>
      </div>
      <template #footer>
        <button class="btn btn-md btn-outline" @click="open = false">取消</button>
        <button class="btn btn-md btn-primary" @click="save">保存</button>
      </template>
    </BaseModal>
  </div>
</template>
