<template>
  <div class="flex h-full min-h-0 flex-col gap-3">
    <div class="flex flex-wrap items-center gap-2">
      <input v-model="search" type="text" :placeholder="t('imageStudio.historySearch')" class="w-56 rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-xs outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800" />
      <span class="text-xs text-gray-400">{{ t('canvas.assetsHint') }}</span>
    </div>
    <div class="min-h-0 flex-1 overflow-y-auto pr-1">
      <div v-if="items.length === 0" class="flex h-48 items-center justify-center text-sm text-gray-400">{{ t('canvas.noAssets') }}</div>
      <div v-else class="grid grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-3">
        <div v-for="item in items" :key="item.kind + item.id" class="group relative overflow-hidden rounded-lg border border-gray-200 bg-gray-100 dark:border-dark-700 dark:bg-dark-800/60">
          <template v-if="item.kind === 'image'">
            <img :src="item.url" class="aspect-square w-full cursor-zoom-in select-none object-contain" draggable="false" alt="" @click="viewer = item.url" />
          </template>
          <template v-else>
            <video :src="item.url" controls preload="metadata" class="aspect-video w-full bg-black object-contain"></video>
          </template>
          <div class="absolute inset-x-0 bottom-0 flex items-center justify-center gap-2 bg-black/60 py-1 opacity-0 transition-opacity group-hover:opacity-100">
            <button v-if="item.kind === 'image'" type="button" class="text-[11px] text-white hover:underline" @click="toCanvas(item)">{{ t('canvas.toCanvas') }}</button>
            <a :href="item.url" :download="item.download" class="text-[11px] text-white hover:underline">{{ t('imageStudio.download') }}</a>
            <button type="button" class="text-[11px] text-red-300 hover:underline" @click="item.remove()">{{ t('common.delete') }}</button>
          </div>
          <span class="absolute left-1 top-1 rounded px-1 text-[10px]" :class="item.kind === 'image' ? 'bg-sky-500/80 text-white' : 'bg-violet-500/80 text-white'">
            {{ item.kind === 'image' ? t('canvas.kindImage') : t('canvas.kindVideo') }}
          </span>
        </div>
      </div>
    </div>
    <div v-if="viewer" class="fixed inset-0 z-[100] flex items-center justify-center bg-black/85 p-6" @click="viewer = null">
      <img :src="viewer" class="max-h-[90vh] max-w-[92vw] rounded-lg object-contain" alt="" @click.stop />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  history as imageHistory, objectUrlFor as imageObjectUrl, deleteHistory as deleteImageHistory,
  loadHistory as loadImageHistory,
} from '@/composables/useImageStudioEngine'
import {
  history as videoHistory, historyVideoUrl, deleteHistory as deleteVideoHistory,
  loadHistory as loadVideoHistory,
} from '@/composables/useVideoStudioEngine'
import { addImageToCurrentCanvas } from '@/composables/useInfiniteCanvas'

const { t } = useI18n()

const search = ref('')
const viewer = ref<string | null>(null)

interface AssetItem {
  kind: 'image' | 'video'
  id: number
  url: string
  prompt: string
  download: string
  remove: () => void
}

const items = computed<AssetItem[]>(() => {
  const kw = search.value.trim().toLowerCase()
  const out: AssetItem[] = []
  for (const item of imageHistory.value) {
    if (kw && !item.prompt.toLowerCase().includes(kw)) continue
    const url = imageObjectUrl(item)
    if (!url) continue
    out.push({
      kind: 'image', id: item.id, url, prompt: item.prompt,
      download: `image-${item.ts}.png`,
      remove: () => void deleteImageHistory(item.id)
    })
  }
  for (const item of videoHistory.value) {
    if (kw && !item.prompt.toLowerCase().includes(kw)) continue
    out.push({
      kind: 'video', id: item.id, url: historyVideoUrl(item), prompt: item.prompt,
      download: `video-${item.ts}.mp4`,
      remove: () => void deleteVideoHistory(item.id)
    })
  }
  return out.sort((a, b) => b.id - a.id)
})

function toCanvas(item: AssetItem) {
  const asset = item.kind === 'image'
    ? imageHistory.value.find(i => i.id === item.id)?.images[0]?.blob
    : videoHistory.value.find(i => i.id === item.id)?.blob
  if (asset) void addImageToCurrentCanvas(asset)
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && viewer.value) viewer.value = null
}
onMounted(() => {
  window.addEventListener('keydown', onKeydown)
  void loadImageHistory()
  void loadVideoHistory()
})
onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown)
})
</script>
