<template>
  <div class="grid h-full min-h-0 grid-cols-1 gap-3 lg:grid-cols-[240px_minmax(0,320px)_minmax(0,1fr)]">
    <!-- 左列:生成记录 -->
    <div class="card flex min-h-0 flex-col p-3 lg:h-full">
      <div class="mb-2 flex items-center justify-between">
        <span class="text-sm font-semibold">{{ t('canvas.genRecords') }}</span>
        <span class="text-xs text-gray-400">{{ history.length }}</span>
      </div>
      <div class="mb-2 flex items-center gap-1.5 text-[11px]">
        <button type="button" class="rounded border border-gray-200 px-1.5 py-0.5 hover:border-primary-400 dark:border-dark-600" @click="onNewRecord">{{ t('canvas.new') }}</button>
        <button type="button" class="rounded border border-gray-200 px-1.5 py-0.5 hover:border-primary-400 dark:border-dark-600" @click="toggleSelectAll">{{ selected.size ? t('canvas.deselectAll') : t('canvas.selectAll') }}</button>
        <button type="button" class="rounded border border-gray-200 px-1.5 py-0.5 text-gray-400 hover:border-red-300 hover:text-red-500 dark:border-dark-600" @click="deleteSelected">{{ t('canvas.deleteSelected') }}</button>
      </div>
      <div class="min-h-0 flex-1 space-y-2 overflow-y-auto pr-1">
        <div v-if="history.length === 0" class="flex h-40 items-center justify-center rounded-lg border border-dashed border-gray-200 text-xs text-gray-400 dark:border-dark-700">{{ t('canvas.noRecords') }}</div>
        <div
          v-for="item in history"
          :key="item.id"
          class="flex cursor-pointer items-center gap-2 rounded-lg border p-1.5 transition-colors"
          :class="selected.has(item.id) ? 'border-primary-400 bg-primary-50/50 dark:bg-primary-900/20' : 'border-gray-100 hover:border-primary-300 dark:border-dark-700'"
          @click="toggleSelect(item.id)"
        >
          <img v-if="objectUrlFor(item)" :src="objectUrlFor(item)" class="h-14 w-14 shrink-0 rounded object-cover" draggable="false" alt="" />
          <div class="min-w-0 flex-1">
            <div class="truncate text-[11px]">{{ item.prompt || t('imageStudio.prompt') }}</div>
            <div class="truncate text-[10px] text-gray-400">{{ item.model }} · {{ item.actualSize || item.size }}</div>
          </div>
        </div>
      </div>
    </div>

    <!-- 中列:参数面板 -->
    <div class="card flex min-h-0 flex-col gap-3 overflow-y-auto p-3 lg:h-full">
      <span class="text-sm font-semibold">{{ t('canvas.imageStudio') }}</span>
      <div>
        <div class="mb-1 flex items-center justify-between">
          <label class="text-xs font-medium text-gray-500">{{ t('imageStudio.prompt') }}</label>
        </div>
        <textarea
          v-model="prompt"
          rows="6"
          maxlength="4000"
          :placeholder="t('canvas.imagePromptPlaceholder')"
          class="w-full rounded-lg border border-gray-300 bg-white p-2.5 text-sm outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800"
        ></textarea>
      </div>

      <!-- 参考图 -->
      <div>
        <div class="mb-1 flex items-center justify-between">
          <label class="text-xs font-medium text-gray-500">{{ t('canvas.refImages') }}</label>
          <div class="flex gap-1">
            <button type="button" class="rounded border border-gray-200 px-1.5 py-0.5 text-[11px] hover:border-primary-400 dark:border-dark-600" @click="pasteFromClipboard">{{ t('canvas.clipboard') }}</button>
            <label class="cursor-pointer rounded border border-gray-200 px-1.5 py-0.5 text-[11px] hover:border-primary-400 dark:border-dark-600">
              {{ t('canvas.upload') }}
              <input type="file" accept="image/png,image/jpeg,image/webp" multiple class="hidden" @change="onRefFilesChange" />
            </label>
          </div>
        </div>
        <div
          class="flex min-h-[72px] flex-wrap items-center gap-2 rounded-lg border border-dashed border-gray-300 p-2 dark:border-dark-600"
          @dragover.prevent
          @drop.prevent="e => { const f = e.dataTransfer?.files; if (f?.length) addRefFiles(Array.from(f)) }"
        >
          <div v-for="(r, i) in refItems" :key="r.url" class="group relative h-14 w-14 overflow-hidden rounded border border-gray-200 dark:border-dark-600">
            <img :src="r.url" class="h-full w-full object-cover" draggable="false" alt="" />
            <button type="button" class="absolute right-0 top-0 hidden h-4 w-4 items-center justify-center rounded-bl bg-black/60 text-[9px] text-white group-hover:flex" @click="removeRefItem(i)">✕</button>
          </div>
          <span v-if="refItems.length === 0" class="text-[11px] text-gray-400">{{ t('canvas.refDropHint') }}</span>
        </div>
      </div>

      <!-- 模型 -->
      <div>
        <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('imageStudio.model') }}</label>
        <select v-model="model" class="w-full rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-sm outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800">
          <option v-for="mo in models" :key="mo.id + ':' + mo.group_id" :value="mo.id">{{ mo.id }}{{ mo.group_name ? ' · ' + mo.group_name : '' }}</option>
        </select>
      </div>

      <!-- 质量 -->
      <div>
        <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('imageStudio.quality') }}</label>
        <div class="grid grid-cols-4 gap-1">
          <button v-for="q in QUALITIES" :key="q" type="button" class="rounded-md border px-1 py-1 text-xs transition-colors" :class="quality === q ? 'border-primary-500 bg-primary-50 text-primary-600 dark:bg-primary-900/30' : 'border-gray-200 text-gray-500 hover:border-primary-300 dark:border-dark-600'" @click="quality = q">
            {{ q === 'auto' ? t('canvas.qualityAuto') : q === 'high' ? t('canvas.qualityHigh') : q === 'medium' ? t('canvas.qualityMedium') : t('canvas.qualityLow') }}
          </button>
        </div>
      </div>

      <!-- 尺寸 W/H -->
      <div>
        <div class="mb-1 flex items-center justify-between">
          <label class="text-xs font-medium text-gray-500">{{ t('videoStudio.videoSize') }}</label>
          <label class="flex items-center gap-1 text-[11px] text-gray-500">
            {{ t('canvas.align16') }}
            <button type="button" class="relative h-4 w-7 rounded-full transition-colors" :class="align16 ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600'" @click="align16 = !align16">
              <span class="absolute top-0.5 h-3 w-3 rounded-full bg-white transition-all" :class="align16 ? 'left-3.5' : 'left-0.5'"></span>
            </button>
          </label>
        </div>
        <div class="flex items-center gap-1.5">
          <input v-model.number="widthInput" type="number" min="256" max="4096" step="16" class="w-full rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-sm outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800" @change="applySize" />
          <span class="text-gray-400">↔</span>
          <input v-model.number="heightInput" type="number" min="256" max="4096" step="16" class="w-full rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-sm outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800" @change="applySize" />
        </div>
      </div>

      <!-- 宽高比 -->
      <div>
        <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('canvas.aspectRatio') }}</label>
        <div class="grid grid-cols-4 gap-1">
          <button v-for="r in RATIO_PRESETS" :key="r.label" type="button" class="flex flex-col items-center gap-0.5 rounded-md border px-1 py-1.5 text-[10px] transition-colors" :class="ratioActive(r) ? 'border-primary-500 bg-primary-50 text-primary-600 dark:bg-primary-900/30' : 'border-gray-200 text-gray-500 hover:border-primary-300 dark:border-dark-600'" @click="applyRatio(r)">
            <span class="inline-block border border-current" :style="{ width: Math.min(20, 16 * r.w / Math.max(r.w, r.h)) + 'px', height: Math.min(20, 16 * r.h / Math.max(r.w, r.h)) + 'px' }"></span>
            {{ r.label }}
          </button>
        </div>
      </div>

      <button
        type="button"
        class="mt-auto rounded-lg bg-primary-600 px-4 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-primary-700 disabled:cursor-not-allowed disabled:opacity-50"
        :disabled="generating || !!initError || !prompt.trim()"
        @click="generateBatch"
      >
        <span v-if="generating" class="mr-1 inline-block h-3.5 w-3.5 animate-spin rounded-full border-2 border-white/40 border-t-white"></span>
        {{ generating ? t('imageStudio.generating') : t('canvas.startGenerate') }}
      </button>
    </div>

    <!-- 右列:生成结果 -->
    <div class="card flex min-h-0 flex-col p-3 lg:h-full">
      <div class="mb-2 flex shrink-0 items-center justify-between">
        <span class="text-sm font-semibold">{{ t('canvas.genResults') }}</span>
        <button v-if="batch.some(s => s.status === 'failed')" type="button" class="text-xs text-gray-400 hover:text-red-500" @click="clearFailed">{{ t('imageStudio.clearFailed') }}</button>
      </div>
      <div class="flex min-h-0 flex-1 flex-col overflow-y-auto">
        <div v-if="batch.length === 0" class="flex flex-1 flex-col items-center justify-center gap-3 text-sm text-gray-400">
          <span class="text-3xl">🖼️</span>
          {{ t('canvas.noResults') }}
        </div>
        <div v-else class="grid grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-3">
          <div v-for="slot in batch" :key="slot.slotId" class="group relative overflow-hidden rounded-lg border border-gray-200 bg-gray-100 dark:border-dark-700 dark:bg-dark-800/60">
            <template v-if="slot.status === 'done' && slot.url">
              <img :src="slot.url" class="aspect-square w-full cursor-zoom-in select-none object-contain" :alt="slot.prompt.slice(0, 30)" draggable="false" @click="viewer = { url: slot.url!, prompt: slot.prompt }" />
              <div class="absolute inset-x-0 bottom-0 flex flex-wrap items-center justify-center gap-x-2 bg-black/60 px-2 py-1 opacity-0 transition-opacity group-hover:opacity-100">
                <button type="button" class="text-xs text-white hover:underline" @click="sendToCanvas(slot.slotId)">{{ t('canvas.toCanvas') }}</button>
                <a :href="slot.url" :download="`image-${slot.slotId}.png`" class="text-xs text-white hover:underline">{{ t('imageStudio.download') }}</a>
                <button type="button" class="text-xs text-white" @click="removeSlot(slot.slotId)">✕</button>
              </div>
              <span class="absolute right-1 top-1 rounded bg-green-500/80 px-1 text-[10px] text-white">{{ t('imageStudio.doneTag') }}</span>
            </template>
            <template v-else-if="slot.status === 'failed'">
              <div class="flex aspect-square w-full flex-col items-center justify-center gap-1 bg-red-50 p-2 text-center dark:bg-red-900/20">
                <span class="line-clamp-3 text-[10px] text-red-500">{{ slot.error || t('imageStudio.generateFailed') }}</span>
                <button type="button" class="text-[10px] text-primary-500 underline" @click="retrySlot(slot.slotId)">{{ t('imageStudio.retry') }}</button>
              </div>
            </template>
            <template v-else>
              <div class="flex aspect-square w-full flex-col items-center justify-center gap-2 bg-gray-50 dark:bg-dark-800">
                <span class="inline-block h-5 w-5 animate-spin rounded-full border-2 border-primary-300 border-t-primary-600"></span>
                <span class="text-[11px] text-gray-500 dark:text-gray-300">{{ t('imageStudio.elapsed', { n: elapsed }) }}</span>
              </div>
            </template>
          </div>
        </div>
      </div>
    </div>

    <!-- 灯箱 -->
    <div v-if="viewer" class="fixed inset-0 z-[100] flex items-center justify-center bg-black/85 p-6" @click="viewer = null">
      <img :src="viewer.url" class="max-h-[90vh] max-w-[92vw] rounded-lg object-contain" alt="" @click.stop />
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import {
  MODES, QUALITIES, IMAGE_MODEL_PATTERN,
  models, model, initError, retryInit,
  quality,
  prompt, refItems, addRefFiles, removeRefItem,
  batch, generating, elapsed, generateBatch, clearFailed, removeSlot, retrySlot,
  history, objectUrlFor, loadHistory,
} from '@/composables/useImageStudioEngine'
import { addImageToCurrentCanvas } from '@/composables/useInfiniteCanvas'

const { t } = useI18n()
const appStore = useAppStore()
void MODES

const viewer = ref<{ url: string; prompt: string } | null>(null)
const selected = ref(new Set<number>())
const align16 = ref(true)
const widthInput = ref(1024)
const heightInput = ref(1024)

const RATIO_PRESETS = [
  { label: '1:1', w: 1, h: 1, base: 1024 },
  { label: '3:2', w: 3, h: 2, base: 1216 },
  { label: '2:3', w: 2, h: 3, base: 1216 },
  { label: '4:3', w: 4, h: 3, base: 1024 },
  { label: '3:4', w: 3, h: 4, base: 1024 },
  { label: '16:9', w: 16, h: 9, base: 1280 },
  { label: '9:16', w: 9, h: 16, base: 1280 },
  { label: '1:1(2k)', w: 1, h: 1, base: 2048 },
  { label: '21:9', w: 21, h: 9, base: 1680 },
  { label: '9:21', w: 9, h: 21, base: 1680 }
]

function snap(v: number): number {
  if (!align16.value) return Math.max(256, Math.min(4096, Math.round(v)))
  return Math.max(256, Math.min(4096, Math.round(v / 16) * 16))
}
function applySize() {
  const w = snap(Number(widthInput.value) || 1024)
  const h = snap(Number(heightInput.value) || 1024)
  widthInput.value = w
  heightInput.value = h
  size.value = `${w}x${h}`
}
function applyRatio(r: { w: number; h: number; base: number }) {
  let w: number, h: number
  if (r.w >= r.h) {
    w = r.base
    h = Math.round(r.base * r.h / r.w)
  } else {
    h = r.base
    w = Math.round(r.base * r.w / r.h)
  }
  widthInput.value = snap(w)
  heightInput.value = snap(h)
  applySize()
}
function ratioActive(r: { w: number; h: number }): boolean {
  if (size.value === 'auto') return false
  const [w, h] = size.value.split('x').map(Number)
  if (!w || !h) return false
  return Math.abs(w / h - r.w / r.h) < 0.02
}
// 引擎的 size ref 在组件内以别名使用
import { size } from '@/composables/useImageStudioEngine'

function toggleSelect(id: number) {
  const next = new Set(selected.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  selected.value = next
}
function toggleSelectAll() {
  if (selected.value.size) selected.value = new Set()
  else selected.value = new Set(history.value.map(i => i.id))
}
function deleteSelected() {
  if (!selected.value.size) return
  const ids = [...selected.value]
  void (async () => {
    for (const id of ids) await deleteHistoryById(id)
    selected.value = new Set()
  })()
}
async function onNewRecord() {
  selected.value = new Set()
  appStore.showSuccess(t('canvas.newRecordHint'))
}
function sendToCanvas(slotId: number) {
  const slot = batch.value.find(s => s.slotId === slotId)
  if (!slot?.blob) return
  void addImageToCurrentCanvas(slot.blob)
}

// 剪切板粘贴参考图
async function pasteFromClipboard() {
  try {
    const items = await navigator.clipboard.read()
    const files: File[] = []
    for (const item of items) {
      const type = item.types.find(x => x.startsWith('image/'))
      if (type) {
        const blob = await item.getType(type)
        files.push(new File([blob], `clipboard.${type.split('/')[1]}`, { type }))
      }
    }
    if (files.length) addRefFiles(files)
    else appStore.showError(t('canvas.clipboardEmpty'))
  } catch {
    appStore.showError(t('canvas.clipboardEmpty'))
  }
}
function onRefFilesChange(e: Event) {
  const input = e.target as HTMLInputElement
  if (input.files?.length) addRefFiles(Array.from(input.files))
  input.value = ''
}

import { deleteHistory as deleteHistoryById } from '@/composables/useImageStudioEngine'
// 别名,size 已在上方 import
void retryInit

onMounted(() => {
  void retryInit()
  void loadHistory()
  if (size.value === 'auto') {
    size.value = '1024x1024'
  }
  const [w, h] = size.value.split('x').map(Number)
  if (w && h) { widthInput.value = w; heightInput.value = h }
  // 首次进入若模型被历史恢复过,保证选项存在
  if (!models.value.some(o => o.id === model.value) && model.value) {
    models.value = [{ id: model.value, group_id: 0, group_name: '' }, ...models.value]
  }
})
void IMAGE_MODEL_PATTERN
</script>
