<template>
  <component :is="standalone ? 'div' : AppLayout" :class="standalone ? 'min-h-screen bg-gray-50 p-3 dark:bg-dark-950' : ''">
    <div class="flex w-full flex-col gap-3 text-gray-900 dark:text-gray-100">
    <div v-if="standalone" class="flex items-center justify-between px-1">
      <span class="text-sm font-semibold">{{ t('videoStudio.title') }} <span class="ml-1 text-xs font-normal text-gray-400">{{ t('imageStudio.standaloneTag') }}</span></span>
      <button type="button" class="text-xs text-gray-400 hover:text-gray-600 dark:hover:text-gray-200" @click="reloadPage">{{ t('imageStudio.reload') }}</button>
    </div>
    <div v-else class="flex justify-end">
      <button
        type="button"
        class="flex items-center gap-1 rounded-md border border-gray-200 px-2 py-1 text-xs text-gray-500 transition-colors hover:border-primary-400 hover:text-primary-600 dark:border-dark-600"
        @click="openStandalone"
      >⧉ {{ t('imageStudio.openStandalone') }}</button>
    </div>

    <!-- 初始化失败降级 -->
    <div v-if="initError" class="card flex flex-wrap items-center justify-between gap-3 border border-red-200 p-3 dark:border-red-900/50">
      <span class="text-sm text-red-500">{{ initError }}</span>
      <button
        type="button"
        class="rounded-md border border-red-200 px-3 py-1 text-xs font-medium text-red-500 transition-colors hover:bg-red-50 dark:border-red-900/60 dark:hover:bg-red-900/20"
        @click="retryInit"
      >{{ t('imageStudio.retryInit') }}</button>
    </div>

    <div class="grid grid-cols-1 gap-4 xl:h-[calc(100vh-9rem)] xl:grid-cols-[250px_minmax(0,300px)_minmax(0,1fr)_300px]" :class="standalone ? 'xl:!h-[calc(100vh-5rem)]' : ''">
      <!-- 左列:参数 -->
      <div class="card space-y-4 p-4 xl:h-full xl:overflow-y-auto">
        <div class="grid grid-cols-2 gap-1 rounded-lg bg-gray-100 p-1 dark:bg-dark-800">
          <button
            v-for="m in MODES"
            :key="m.value"
            type="button"
            class="rounded-md px-2 py-1.5 text-xs font-medium transition-colors"
            :class="mode === m.value ? 'bg-white text-primary-600 shadow dark:bg-dark-700' : 'text-gray-500 hover:text-gray-800 dark:hover:text-gray-200'"
            @click="mode = m.value"
          >{{ t(m.label) }}</button>
        </div>

        <div>
          <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('imageStudio.model') }}</label>
          <select v-model="model" class="w-full rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-sm outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800">
            <option v-for="m in models" :key="m" :value="m">{{ m }}</option>
          </select>
        </div>

        <div>
          <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('videoStudio.resolution') }}</label>
          <div class="grid grid-cols-3 gap-1">
            <button
              v-for="r in RESOLUTIONS"
              :key="r"
              type="button"
              class="rounded-md border px-1 py-1 text-xs transition-colors"
              :class="resolution === r ? 'border-primary-500 bg-primary-50 text-primary-600 dark:bg-primary-900/30' : 'border-gray-200 text-gray-500 hover:border-primary-300 dark:border-dark-600'"
              @click="resolution = r"
            >{{ r }}</button>
          </div>
        </div>

        <div>
          <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('videoStudio.videoSize') }}</label>
          <div class="grid grid-cols-3 gap-1">
            <button
              v-for="s in SIZE_PRESETS"
              :key="s.value"
              type="button"
              class="rounded-md border px-1 py-1 text-xs transition-colors"
              :class="videoSize === s.value ? 'border-primary-500 bg-primary-50 text-primary-600 dark:bg-primary-900/30' : 'border-gray-200 text-gray-500 hover:border-primary-300 dark:border-dark-600'"
              @click="videoSize = s.value"
            >{{ s.value === 'auto' ? 'auto' : t(s.label) }}</button>
          </div>
        </div>

        <div>
          <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('videoStudio.seconds') }}</label>
          <div class="grid grid-cols-4 gap-1">
            <button
              v-for="s in SECONDS"
              :key="s"
              type="button"
              class="rounded-md border px-1 py-1 text-xs transition-colors"
              :class="seconds === s ? 'border-primary-500 bg-primary-50 text-primary-600 dark:bg-primary-900/30' : 'border-gray-200 text-gray-500 hover:border-primary-300 dark:border-dark-600'"
              @click="seconds = s"
            >{{ s }}s</button>
          </div>
        </div>

        <div>
          <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('imageStudio.quantity') }}</label>
          <div class="grid grid-cols-4 gap-1">
            <button
              v-for="q in QUANTITIES"
              :key="q"
              type="button"
              class="rounded-md border px-1 py-1 text-xs transition-colors"
              :class="quantity === q ? 'border-primary-500 bg-primary-50 text-primary-600 dark:bg-primary-900/30' : 'border-gray-200 text-gray-500 hover:border-primary-300 dark:border-dark-600'"
              @click="quantity = q"
            >{{ q }}</button>
          </div>
        </div>

        <p class="text-xs text-gray-400">{{ t('videoStudio.feeNote') }}</p>
      </div>

      <!-- 提示词列 -->
      <div class="card flex flex-col gap-3 p-4 xl:h-full xl:overflow-y-auto">
        <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('imageStudio.prompt') }}</label>
        <textarea
          v-model="prompt"
          rows="8"
          maxlength="4000"
          :placeholder="t('videoStudio.promptPlaceholder')"
          class="w-full h-[280px] min-h-[160px] resize-y rounded-lg border border-gray-300 bg-white p-3 text-sm outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800"
        ></textarea>
        <div class="mt-1 text-right text-[11px] text-gray-400">{{ 4000 - prompt.length }} / 4000</div>

        <!-- 首帧图(图生视频) -->
        <div v-if="mode === 'i2v'" class="mt-2">
          <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('videoStudio.firstFrame') }}</label>
          <div class="flex flex-wrap gap-2">
            <div v-if="refImages.length" class="group relative h-20 w-20 overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
              <img :src="refImages[0].url" class="h-full w-full object-cover" draggable="false" alt="first frame" />
              <button
                type="button"
                class="absolute right-0.5 top-0.5 hidden h-5 w-5 items-center justify-center rounded-full bg-black/60 text-[10px] text-white group-hover:flex"
                @click="removeRefImage"
              >✕</button>
            </div>
            <label
              v-else
              class="flex h-20 w-20 cursor-pointer flex-col items-center justify-center gap-1 rounded-lg border-2 border-dashed border-gray-300 text-gray-400 transition-colors hover:border-primary-400 hover:text-primary-500 dark:border-dark-600"
              @dragover.prevent
              @drop.prevent="onDrop"
            >
              <span class="text-lg leading-none">+</span>
              <span class="px-1 text-center text-[10px] leading-tight">{{ t('videoStudio.firstFrameHint') }}</span>
              <input type="file" accept="image/png,image/jpeg,image/webp" class="hidden" @change="onRefFilesChange" />
            </label>
          </div>
          <p class="mt-1 text-[11px] text-gray-400">{{ t('videoStudio.firstFrameNote') }}</p>
        </div>

        <!-- 提交/停止 -->
        <div class="mt-3 flex items-center gap-3">
          <button
            v-if="!generating"
            type="button"
            class="flex-1 rounded-lg bg-primary-600 px-4 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-primary-700 disabled:cursor-not-allowed disabled:opacity-50"
            :disabled="!!initError || !prompt.trim()"
            @click="submitBatch"
          >
            {{ t('videoStudio.generateN', { n: quantity }) }}
          </button>
          <button
            v-else
            type="button"
            class="flex-1 rounded-lg border border-red-300 bg-transparent px-4 py-2.5 text-sm font-semibold text-red-500 transition-colors hover:bg-red-50 dark:border-red-900/60 dark:hover:bg-red-900/20"
            @click="cancelPending"
          >{{ t('videoStudio.stopBatch') }}</button>
        </div>
        <p v-if="generating" class="mt-2 text-center text-[11px] text-gray-400">{{ t('videoStudio.timeHint') }}</p>
      </div>

      <!-- 任务区 -->
      <div class="card flex min-h-0 flex-col p-4 xl:h-full">
        <div class="mb-3 flex shrink-0 items-center justify-between">
          <span class="text-sm font-semibold">{{ t('videoStudio.tasks') }}</span>
          <button
            v-if="tasks.some(tk => tk.status === 'failed')"
            type="button"
            class="text-xs text-gray-400 hover:text-red-500"
            @click="clearFailed"
          >{{ t('imageStudio.clearFailed') }}</button>
        </div>
        <div class="flex min-h-0 flex-1 flex-col xl:overflow-y-auto">
          <div v-if="tasks.length === 0" class="flex flex-1 items-center justify-center text-sm text-gray-400">
            {{ t('videoStudio.emptyTasks') }}
          </div>
          <div v-else class="grid grid-cols-[repeat(auto-fit,minmax(280px,1fr))] gap-3">
            <div v-for="task in tasks" :key="task.taskId" class="group relative overflow-hidden rounded-lg border border-gray-200 bg-gray-100 dark:border-dark-700 dark:bg-dark-800/60">
              <template v-if="task.status === 'done' && task.videoUrl">
                <video :src="task.videoUrl" controls preload="metadata" class="aspect-video w-full bg-black object-contain"></video>
                <div class="absolute inset-x-0 top-0 flex flex-wrap items-center justify-center gap-x-2 gap-y-0.5 bg-black/60 px-2 py-1 opacity-0 transition-opacity group-hover:opacity-100">
                  <a :href="task.videoUrl" :download="`video-${task.taskId}.mp4`" class="text-xs text-white hover:underline">{{ t('imageStudio.download') }}</a>
                  <button type="button" class="text-xs text-white hover:underline" @click="removeTask(task.taskId)">✕</button>
                </div>
                <div class="absolute bottom-1 left-1 flex items-center gap-1">
                  <span class="rounded bg-black/50 px-1 text-[10px] text-white">{{ task.seconds }}s · {{ task.resolution }}</span>
                  <span v-if="task.finishedAt" class="rounded bg-black/50 px-1 text-[10px] text-white">{{ Math.round((task.finishedAt - task.submittedAt) / 1000) }}s</span>
                </div>
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
                  <span class="text-[11px] font-medium text-gray-500 dark:text-gray-300">{{ t('videoStudio.waiting', { n: Math.floor((Date.now() - task.submittedAt) / 1000) }) }}</span>
                  <span class="text-[10px] leading-tight text-gray-400">{{ t('videoStudio.timeHint') }}</span>
                </div>
                <span class="absolute bottom-1 left-1/2 -translate-x-1/2 rounded bg-black/50 px-1.5 text-[10px] text-white">{{ task.status === 'processing' ? t('videoStudio.processing') : t('videoStudio.queued') }}</span>
              </template>
            </div>
          </div>
        </div>
      </div>

      <!-- 右列:本地历史 -->
      <div class="card flex flex-col space-y-3 p-4 xl:h-full xl:min-h-0">
        <div class="flex items-center justify-between">
          <span class="text-sm font-semibold">{{ t('videoStudio.history') }}</span>
          <span class="text-xs text-gray-400">{{ history.length }}</span>
        </div>
        <input
          v-model="historySearch"
          type="text"
          :placeholder="t('imageStudio.historySearch')"
          class="w-full rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-xs outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800"
        />

        <div class="max-h-[420px] min-h-0 flex-1 space-y-2 overflow-y-auto pr-1 xl:max-h-none">
          <div v-if="filteredHistory.length === 0" class="py-6 text-center text-xs text-gray-400">
            {{ t('videoStudio.historyEmpty') }}
          </div>
          <div
            v-for="item in filteredHistory"
            :key="item.id"
            class="group flex cursor-pointer items-center gap-2 rounded-lg border border-gray-100 p-2 transition-colors hover:border-primary-300 dark:border-dark-700"
          >
            <div class="min-w-0 flex-1" @click="playHistory(item)">
              <div class="truncate text-xs">{{ item.prompt || t('imageStudio.prompt') }}</div>
              <div class="truncate text-[10px] text-gray-400">
                {{ item.model }} · {{ item.seconds }}s · {{ item.resolution }} · {{ item.sizeMB }}MB · {{ new Date(item.ts).toLocaleString() }}
              </div>
            </div>
            <div class="flex shrink-0 flex-col items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100">
              <a :href="historyVideoUrl(item)" :download="`lyozc-video-${item.ts}.mp4`" class="text-xs text-gray-400 hover:text-primary-400" :title="t('imageStudio.download')">⬇</a>
              <button type="button" class="text-xs" :class="item.starred ? 'text-amber-400' : 'text-gray-300 hover:text-amber-400'" @click.stop="toggleHistoryStar(item)">★</button>
              <button type="button" class="text-xs text-gray-400 hover:text-red-500" @click.stop="deleteHistory(item.id)">✕</button>
            </div>
          </div>
        </div>

        <div>
          <div class="mb-1 flex items-center justify-between text-[11px] text-gray-400">
            <span>{{ t('imageStudio.localStorage') }}</span>
            <span>{{ fmtMB(storageUsed) }} / {{ fmtMB(storageQuota) }}</span>
          </div>
          <div class="h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
            <div class="h-full rounded-full bg-primary-500 transition-all" :style="{ width: `${storagePercent}%` }"></div>
          </div>
        </div>
        <p class="text-[11px] text-gray-400">{{ t('videoStudio.historyNote') }}</p>
        <button type="button" class="rounded-lg border border-red-200 px-2 py-1.5 text-xs text-red-500 transition-colors hover:bg-red-50 dark:border-red-900/50 dark:hover:bg-red-900/20" @click="clearAllHistory">{{ t('imageStudio.clearAll') }}</button>
      </div>
    </div>

    <!-- 历史视频播放灯箱 -->
    <div v-if="player" class="fixed inset-0 z-[100] flex items-center justify-center bg-black/85 p-6" @click="player = null">
      <button
        type="button"
        class="absolute right-4 top-4 flex h-9 w-9 items-center justify-center rounded-full bg-white/10 text-lg text-white transition-colors hover:bg-white/20"
        @click="player = null"
      >✕</button>
      <video :src="player.url" controls autoplay class="max-h-[90vh] max-w-[92vw] rounded-lg shadow-2xl" @click.stop></video>
      <div class="absolute inset-x-0 bottom-5 mx-auto max-w-[80vw] truncate rounded-lg bg-black/60 px-4 py-2 text-center text-xs text-white/90">
        {{ player.prompt }}
      </div>
    </div>
    </div>
  </component>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import {
  MODES, RESOLUTIONS, SECONDS, SIZE_PRESETS, QUANTITIES,
  models, model, initError, retryInit,
  mode, resolution, videoSize, seconds, quantity, prompt,
  refImages, addRefImages, removeRefImage,
  tasks, generating, submitBatch, cancelPending, retryTask, removeTask, clearFailed,
  history, historySearch, filteredHistory, storageUsed, storageQuota, storagePercent, fmtMB,
  loadHistory, updateStorageMeter, historyVideoUrl, toggleHistoryStar, deleteHistory, clearAllHistory,
} from '@/composables/useVideoStudioEngine'
import type { VideoHistoryItem } from '@/composables/useVideoStudioEngine'

const { t } = useI18n()
const route = useRoute()

const standalone = computed(() => route.query.standalone === '1')
function openStandalone() {
  window.open(`${window.location.origin}/video-studio?standalone=1`, 'lyozc-video-studio', 'width=1500,height=960')
}
function reloadPage() {
  window.location.reload()
}

const player = ref<{ url: string; prompt: string } | null>(null)
function playHistory(item: VideoHistoryItem) {
  player.value = { url: historyVideoUrl(item), prompt: item.prompt }
}

function onRefFilesChange(event: Event) {
  const input = event.target as HTMLInputElement
  if (input.files?.length) addRefImages(Array.from(input.files))
  input.value = ''
}
function onDrop(event: DragEvent) {
  const files = event.dataTransfer?.files
  if (files?.length) addRefImages(Array.from(files))
}

let tickTimer: number | null = null
onMounted(() => {
  void retryInit()
  void loadHistory()
  updateStorageMeter()
  // 10s 心跳驱动"已等待 N 秒"显示(视频任务动辄数分钟)
  tickTimer = window.setInterval(() => { tasks.value = [...tasks.value] }, 10000)
})
onUnmounted(() => {
  if (tickTimer !== null) window.clearInterval(tickTimer)
})
</script>
