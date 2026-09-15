<template>
  <div class="flex h-full min-h-0 flex-col gap-3">
    <div class="flex flex-wrap items-center gap-2">
      <button type="button" class="rounded-md bg-primary-600 px-3 py-1.5 text-xs font-semibold text-white hover:bg-primary-700" @click="onCreate">{{ t('canvas.newCanvas') }}</button>
      <label class="cursor-pointer rounded-md border border-gray-200 px-3 py-1.5 text-xs hover:border-primary-400 hover:text-primary-600 dark:border-dark-600">
        {{ t('canvas.importCanvas') }}
        <input type="file" accept=".json,application/json" class="hidden" @change="onImport" />
      </label>
    </div>
    <div v-if="canvases.length === 0" class="flex flex-1 items-center justify-center text-sm text-gray-400">{{ t('canvas.noCanvases') }}</div>
    <div v-else class="grid min-h-0 flex-1 grid-cols-[repeat(auto-fill,minmax(240px,1fr))] gap-3 overflow-y-auto pr-1">
      <div
        v-for="c in canvases"
        :key="c.id"
        class="group cursor-pointer rounded-lg border border-gray-200 bg-white p-3 transition-colors hover:border-primary-400 dark:border-dark-700 dark:bg-dark-800"
        :class="c.id === currentCanvasId ? 'ring-1 ring-primary-400' : ''"
        @click="openCanvas(c.id)"
      >
        <div class="flex items-start justify-between gap-2">
          <div class="min-w-0 flex-1">
            <div class="truncate text-sm font-semibold">{{ c.name }}</div>
            <div class="mt-0.5 text-[11px] text-gray-400">{{ c.nodes.length }} {{ t('canvas.nodesUnit') }} · {{ new Date(c.updatedAt).toLocaleString() }}</div>
          </div>
          <div class="flex shrink-0 items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100">
            <button type="button" class="text-xs text-gray-400 hover:text-primary-500" :title="t('common.rename')" @click.stop="onRename(c)">{{ t('common.rename') }}</button>
            <button type="button" class="text-xs text-gray-400 hover:text-primary-500" :title="t('canvas.export')" @click.stop="exportCanvas(c.id)">{{ t('canvas.export') }}</button>
            <button type="button" class="text-xs text-gray-400 hover:text-red-500" @click.stop="onDelete(c)">{{ t('common.delete') }}</button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import {
  canvases, currentCanvasId, createCanvas, deleteCanvas, exportCanvas, importCanvas,
  loadCanvases, openCanvas, renameCanvas,
} from '@/composables/useInfiniteCanvas'
import { useAppStore } from '@/stores/app'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const appStore = useAppStore()

onMounted(() => { void loadCanvases() })

async function onCreate() {
  const doc = await createCanvas()
  appStore.showSuccess(t('canvas.created', { name: doc.name }))
  openCanvas(doc.id)
}
function onRename(c: { id: string; name: string }) {
  const name = window.prompt(t('canvas.renamePrompt'), c.name)
  if (name && name.trim()) void renameCanvas(c.id, name.trim())
}
function onDelete(c: { id: string; name: string }) {
  if (window.confirm(t('canvas.deleteConfirm', { name: c.name }))) void deleteCanvas(c.id)
}
async function onImport(e: Event) {
  const input = e.target as HTMLInputElement
  if (input.files?.[0]) await importCanvas(input.files[0])
  input.value = ''
}
</script>
