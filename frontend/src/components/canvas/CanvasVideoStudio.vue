<template>
  <div class="grid grid-cols-1 gap-3 lg:h-full lg:min-h-0 lg:grid-cols-[240px_minmax(0,320px)_minmax(0,1fr)]">
    <!-- 左列:生成记录(移动端置于最下并限高) -->
    <div class="card order-3 flex min-h-0 flex-col p-3 lg:order-none lg:h-full">
      <div class="mb-2 flex items-center justify-between">
        <span class="text-sm font-semibold">{{ t('canvas.genRecords') }}</span>
        <span class="text-xs text-gray-400">{{ history.length }}</span>
      </div>
      <div class="max-h-44 min-h-0 flex-1 space-y-2 overflow-y-auto pr-1 lg:max-h-none">
        <div v-if="history.length === 0" class="flex h-40 items-center justify-center rounded-lg border border-dashed border-gray-200 text-xs text-gray-400 dark:border-dark-700">{{ t('canvas.noRecords') }}</div>
        <div
          v-for="item in history"
          :key="item.id"
          class="cursor-pointer rounded-lg border border-gray-100 p-2 transition-colors hover:border-primary-300 dark:border-dark-700"
          @click="player = { url: historyVideoUrl(item), prompt: item.prompt }"
        >
          <div class="truncate text-[11px]">{{ item.prompt || t('imageStudio.prompt') }}</div>
          <div class="truncate text-[10px] text-gray-400">{{ item.model }} · {{ item.seconds }}s · {{ item.sizeMB }}MB</div>
        </div>
      </div>
    </div>

    <!-- 中列:参数面板(移动端置于最前) -->
    <div class="card order-1 flex min-h-0 flex-col gap-3 overflow-y-auto p-3 lg:order-none lg:h-full">
      <span class="text-sm font-semibold">{{ t('canvas.videoStudio') }}</span>
      <span class="rounded bg-amber-50 px-2 py-1 text-[11px] text-amber-600 dark:bg-amber-900/20 dark:text-amber-400">{{ t('videoStudio.timeHint') }}</span>
      <div>
        <div class="mb-1 flex items-center justify-between gap-1">
          <label class="shrink-0 text-xs font-medium text-gray-500">{{ t('imageStudio.prompt') }}</label>
          <div class="flex min-w-0 items-center gap-1">
            <select v-model="aiKeyId" :title="t('canvas.aiPickKey')" class="max-w-28 rounded border border-gray-300 bg-white px-1 py-0.5 text-[11px] outline-none dark:border-dark-600 dark:bg-dark-800" @change="onAIKeyChange">
              <option v-if="aiKeys.length === 0" :value="null">{{ t('canvas.aiNoKeys') }}</option>
              <option v-for="k in aiKeys" :key="k.id" :value="k.id">{{ k.name }}</option>
            </select>
            <select v-model="aiModel" :title="t('canvas.aiPickModel')" :disabled="aiModels.length === 0" class="max-w-36 rounded border border-gray-300 bg-white px-1 py-0.5 text-[11px] outline-none disabled:opacity-50 dark:border-dark-600 dark:bg-dark-800" @change="rememberAIModel">
              <option v-if="aiModels.length === 0" value="">{{ t('canvas.aiNoModels') }}</option>
              <option v-for="m in aiModels" :key="m" :value="m">{{ m }}</option>
            </select>
            <button type="button" :disabled="aiLoading" class="shrink-0 rounded border border-primary-300 px-1.5 py-0.5 text-[11px] text-primary-600 hover:border-primary-500 hover:bg-primary-50 disabled:opacity-50 dark:border-primary-700 dark:text-primary-400 dark:hover:bg-primary-900/20" @click="onAIPrompt">{{ aiLoading ? t('canvas.aiPrompting') : '✨ ' + t('canvas.aiPrompt') }}</button>
          </div>
        </div>
        <textarea
          v-model="prompt"
          rows="6"
          maxlength="4000"
          :placeholder="t('videoStudio.promptPlaceholder')"
          class="w-full rounded-lg border border-gray-300 bg-white p-2.5 text-sm outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800"
        ></textarea>
      </div>

      <!-- 参考图(首帧) -->
      <div>
        <div class="mb-1 flex items-center justify-between">
          <label class="text-xs font-medium text-gray-500">{{ t('canvas.refImages') }}</label>
          <div class="flex gap-1">
            <button type="button" class="rounded border border-primary-300 px-1.5 py-0.5 text-[11px] text-primary-600 hover:border-primary-500 hover:bg-primary-50 dark:border-primary-700 dark:text-primary-400 dark:hover:bg-primary-900/20" @click="showPicker = true">{{ t('canvas.importFromImage') }}</button>
            <button type="button" class="rounded border border-gray-200 px-1.5 py-0.5 text-[11px] hover:border-primary-400 dark:border-dark-600" @click="pasteFromClipboard">{{ t('canvas.clipboard') }}</button>
            <label class="cursor-pointer rounded border border-gray-200 px-1.5 py-0.5 text-[11px] hover:border-primary-400 dark:border-dark-600">
              {{ t('canvas.upload') }}
              <input type="file" accept="image/png,image/jpeg,image/webp" class="hidden" @change="onRefFilesChange" />
            </label>
          </div>
        </div>
        <p v-if="refImages.length" class="-mt-1 text-[10px] leading-relaxed text-amber-600 dark:text-amber-400">{{ t('canvas.refImageModelNote') }}</p>
        <div
          class="flex min-h-[72px] flex-wrap items-center gap-2 rounded-lg border border-dashed border-gray-300 p-2 dark:border-dark-600"
          @dragover.prevent
          @drop.prevent="onRefDrop"
        >
          <div v-if="refImages.length" class="relative h-14 w-14 overflow-hidden rounded border border-gray-200 dark:border-dark-600">
            <img :src="refImages[0].url" class="h-full w-full object-cover" draggable="false" alt="" />
            <button type="button" class="absolute right-0 top-0 h-4 w-4 rounded-bl bg-black/60 text-[9px] text-white" @click="removeRefImage(0)">✕</button>
          </div>
          <span v-if="refImages.length === 0" class="text-[11px] text-gray-400">{{ t('canvas.refDropHintMulti') }}</span>
        </div>
      </div>

      <!-- 模型 -->
      <div>
        <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('imageStudio.model') }}</label>
        <select v-model="model" class="w-full rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-sm outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800">
          <option v-for="mo in models" :key="mo.id + ':' + mo.group_id" :value="mo.id">{{ mo.id }}{{ mo.group_name ? ' · ' + mo.group_name : '' }}</option>
        </select>
      </div>

      <!-- 清晰度 -->
      <div>
        <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('videoStudio.resolution') }}</label>
        <div class="grid grid-cols-3 gap-1">
          <button v-for="r in RESOLUTIONS" :key="r" type="button" class="rounded-md border px-1 py-1 text-xs transition-colors" :class="resolution === r ? 'border-primary-500 bg-primary-50 text-primary-600 dark:bg-primary-900/30' : 'border-gray-200 text-gray-500 hover:border-primary-300 dark:border-dark-600'" @click="resolution = r">{{ r }}</button>
        </div>
      </div>

      <!-- 尺寸 -->
      <div>
        <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('videoStudio.videoSize') }}</label>
        <div class="grid grid-cols-3 gap-1">
          <button v-for="s in SIZE_PRESETS" :key="s.value" type="button" class="rounded-md border px-1 py-1 text-[11px] transition-colors" :class="videoSize === s.value ? 'border-primary-500 bg-primary-50 text-primary-600 dark:bg-primary-900/30' : 'border-gray-200 text-gray-500 hover:border-primary-300 dark:border-dark-600'" @click="videoSize = s.value">
            {{ s.value === 'auto' ? 'auto' : t(s.label) }}
          </button>
        </div>
      </div>

      <!-- 秒数 -->
      <div>
        <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('videoStudio.seconds') }}</label>
        <div class="grid grid-cols-4 gap-1">
          <button v-for="s in SECONDS" :key="s" type="button" class="rounded-md border px-1 py-1 text-xs transition-colors" :class="seconds === s ? 'border-primary-500 bg-primary-50 text-primary-600 dark:bg-primary-900/30' : 'border-gray-200 text-gray-500 hover:border-primary-300 dark:border-dark-600'" @click="seconds = s">{{ s }}s</button>
        </div>
      </div>

      <button
        type="button"
        class="mt-auto rounded-lg bg-primary-600 px-4 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-primary-700 disabled:cursor-not-allowed disabled:opacity-50"
        :disabled="generating || !!initError || !prompt.trim()"
        @click="submitBatch"
      >
        <span v-if="generating" class="mr-1 inline-block h-3.5 w-3.5 animate-spin rounded-full border-2 border-white/40 border-t-white"></span>
        {{ generating ? t('videoStudio.stopBatch') : t('canvas.startGenerate') }}
      </button>
    </div>

    <!-- 右列:生成结果(移动端居中) -->
    <div class="card order-2 flex min-h-0 flex-col p-3 lg:order-none lg:h-full">
      <div class="mb-2 flex shrink-0 items-center justify-between">
        <span class="text-sm font-semibold">{{ t('canvas.genResults') }}</span>
        <button v-if="tasks.some(tk => tk.status === 'failed')" type="button" class="text-xs text-gray-400 hover:text-red-500" @click="clearFailed">{{ t('imageStudio.clearFailed') }}</button>
      </div>
      <div class="flex min-h-0 flex-1 flex-col overflow-y-auto">
        <div v-if="tasks.length === 0" class="flex flex-1 flex-col items-center justify-center gap-3 text-sm text-gray-400">
          <span class="text-3xl">🎬</span>
          {{ t('videoStudio.emptyTasks') }}
        </div>
        <div v-else class="grid grid-cols-[repeat(auto-fill,minmax(300px,1fr))] gap-3">
          <div v-for="task in tasks" :key="task.taskId" class="group relative overflow-hidden rounded-lg border border-gray-200 bg-gray-100 dark:border-dark-700 dark:bg-dark-800/60">
            <template v-if="task.status === 'done' && task.videoUrl">
              <video :src="task.videoUrl" controls preload="metadata" class="aspect-video w-full bg-black object-contain"></video>
              <div class="absolute inset-x-0 top-0 flex items-center justify-center gap-2 bg-black/60 py-1 opacity-0 transition-opacity group-hover:opacity-100">
                <a :href="task.videoUrl" :download="`video-${task.taskId}.mp4`" class="text-xs text-white hover:underline">{{ t('imageStudio.download') }}</a>
                <button type="button" class="text-xs text-white" @click="removeTask(task.taskId)">✕</button>
              </div>
              <span class="absolute bottom-1 left-1 rounded bg-black/50 px-1 text-[10px] text-white">{{ task.seconds }}s · {{ task.resolution }}</span>
            </template>
            <template v-else-if="task.status === 'failed'">
              <div class="flex aspect-video w-full flex-col items-center justify-center gap-1 bg-red-50 p-2 text-center dark:bg-red-900/20">
                <span class="line-clamp-3 text-[10px] text-red-500">{{ task.error || t('videoStudio.generateFailed') }}</span>
                <button type="button" class="text-[10px] text-primary-500 underline" @click="retryTask(task.taskId)">{{ t('imageStudio.retry') }}</button>
              </div>
            </template>
            <template v-else>
              <div class="flex aspect-video w-full flex-col items-center justify-center gap-2 bg-gray-50 px-3 text-center dark:bg-dark-800">
                <span class="inline-block h-5 w-5 animate-spin rounded-full border-2 border-primary-300 border-t-primary-600"></span>
                <span class="text-[11px] text-gray-500 dark:text-gray-300">{{ t('videoStudio.waiting', { n: Math.floor((Date.now() - task.submittedAt) / 1000) }) }}</span>
                <button type="button" class="text-[10px] text-gray-400 underline hover:text-red-400" @click="stopWaiting(task.taskId)">{{ t('videoStudio.stopWaiting') }}</button>
              </div>
              <button type="button" class="absolute right-1 top-1 flex h-5 w-5 items-center justify-center rounded-full bg-black/60 text-[10px] text-white" :title="t('videoStudio.stopWaiting')" @click="stopWaiting(task.taskId)">✕</button>
              <span class="absolute bottom-1 left-1/2 -translate-x-1/2 rounded bg-black/50 px-1.5 text-[10px] text-white">{{ task.status === 'processing' ? t('videoStudio.processing') : t('videoStudio.queued') }}</span>
            </template>
          </div>
        </div>
      </div>
    </div>

    <!-- 灯箱播放 -->
    <div v-if="player" class="fixed inset-0 z-[100] flex items-center justify-center bg-black/85 p-6" @click="player = null">
      <video :src="player.url" controls autoplay class="max-h-[90vh] max-w-[92vw] rounded-lg shadow-2xl" @click.stop></video>
    </div>

    <!-- 从生图记录导入参考图 -->
    <CanvasImagePicker v-if="showPicker" :max-select="9 - refImages.length" @close="showPicker = false" @import="onImportFromImage" />
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import {
  RESOLUTIONS, SECONDS, SIZE_PRESETS,
  models, model, initError, retryInit,
  resolution, videoSize, seconds, prompt,
  refImages, addRefImages, removeRefImage,
  tasks, generating, submitBatch, retryTask, removeTask, clearFailed, stopWaiting,
  history, loadHistory, historyVideoUrl,
} from '@/composables/useVideoStudioEngine'
import {
  aiEnhancePrompt,
  aiKeys, aiKeyId, aiModels, aiModel,
  initAIOptions, onAIKeyChange, rememberAIModel,
} from '@/composables/useAIPrompt'
import CanvasImagePicker from '@/components/canvas/CanvasImagePicker.vue'
import type { HistoryItem } from '@/composables/useImageStudioEngine'

const { t } = useI18n()
const appStore = useAppStore()

const player = ref<{ url: string; prompt: string } | null>(null)

const aiLoading = ref(false)

// 从生图记录导入参考图
const showPicker = ref(false)
function onImportFromImage(items: HistoryItem[]) {
  const files: File[] = []
  for (const it of items) {
    const blob = it.images[0]?.blob
    if (!blob || (blob.type && !blob.type.startsWith('image/'))) continue
    const ext = (blob.type.split('/')[1] || 'png').replace('jpeg', 'jpg')
    files.push(new File([blob], `ref-${it.id}.${ext}`, { type: blob.type || 'image/png' }))
  }
  if (files.length) addRefImages(files)
}

// 参考图拖放:支持本机文件,以及页面内拖动生成结果/历史缩略图(blob: 地址需取回转 File)
async function onRefDrop(e: DragEvent) {
  const dt = e.dataTransfer
  if (!dt) return
  if (dt.files?.length) {
    addRefImages(Array.from(dt.files))
    return
  }
  const uri = dt.getData('text/uri-list') || dt.getData('text/plain') || ''
  const url = uri.split('\n').map(s => s.trim()).find(u => u.startsWith('blob:') || u.startsWith(window.location.origin))
  if (!url) return
  try {
    const res = await fetch(url)
    const blob = await res.blob()
    if (!blob.type.startsWith('image/')) return
    const ext = blob.type.split('/')[1] || 'png'
    addRefImages([new File([blob], `ref-${Date.now()}.${ext}`, { type: blob.type })])
  } catch { /* 无效拖拽源,忽略 */ }
}
async function onAIPrompt() {
  if (aiLoading.value) return
  const text = prompt.value.trim()
  if (!text) {
    appStore.showError(t('canvas.aiPromptEmpty'))
    return
  }
  aiLoading.value = true
  try {
    prompt.value = await aiEnhancePrompt('video', text, { keyId: aiKeyId.value ?? undefined, model: aiModel.value || undefined })
    appStore.showSuccess(t('canvas.aiPromptDone'))
  } catch (err) {
    appStore.showError(err instanceof Error ? err.message : t('canvas.aiPromptFailed'))
  } finally {
    aiLoading.value = false
  }
}

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
    if (files.length) addRefImages(files)
    else appStore.showError(t('canvas.clipboardEmpty'))
  } catch {
    appStore.showError(t('canvas.clipboardEmpty'))
  }
}
function onRefFilesChange(e: Event) {
  const input = e.target as HTMLInputElement
  if (input.files?.length) addRefImages(Array.from(input.files))
  input.value = ''
}

let tickTimer: number | null = null
onMounted(() => {
  void retryInit()
  void loadHistory()
  void initAIOptions()
  tickTimer = window.setInterval(() => { tasks.value = [...tasks.value] }, 10000)
})
onUnmounted(() => {
  if (tickTimer !== null) window.clearInterval(tickTimer)
})
</script>
