<template>
  <component :is="standalone ? 'div' : AppLayout" :class="standalone ? 'min-h-screen bg-gray-50 p-3 dark:bg-dark-950' : ''">
    <div class="flex h-[calc(100vh-9rem)] min-h-[560px] flex-col overflow-hidden rounded-xl border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-900" :class="standalone ? 'h-[calc(100vh-1.5rem)]' : ''">
      <!-- 顶部 tab 栏 -->
      <div class="flex flex-wrap items-center gap-1 border-b border-gray-200 px-3 py-2 dark:border-dark-700">
        <span class="mr-2 flex items-center gap-1 text-sm font-bold"><span class="text-primary-500">▲</span> {{ t('canvas.appName') }}</span>
        <button
          v-for="tab in TABS"
          :key="tab.value"
          type="button"
          class="rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors"
          :class="activeTab === tab.value ? 'bg-gray-200/70 text-gray-900 dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-800 dark:hover:text-gray-200'"
          @click="switchTab(tab.value)"
        >{{ t(tab.label) }}</button>
        <button
          type="button"
          class="ml-auto rounded-md bg-gray-900 px-3 py-1.5 text-xs font-medium text-white hover:bg-gray-700 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-200"
          @click="openStandalone"
        >⧉ {{ t('imageStudio.openStandalone') }}</button>
      </div>

      <!-- 内容区 -->
      <div class="min-h-0 flex-1 p-3">
        <CanvasBoard v-if="activeTab === 'board' && currentCanvas" :doc="currentCanvas" />
        <div v-else-if="activeTab === 'board'" class="flex h-full items-center justify-center text-sm text-gray-400">{{ t('canvas.noCanvases') }}</div>
        <CanvasGallery v-else-if="activeTab === 'gallery'" />
        <CanvasImageStudio v-else-if="activeTab === 'image'" class="h-full" />
        <CanvasVideoStudio v-else-if="activeTab === 'video'" class="h-full" />
        <CanvasPromptLibrary v-else-if="activeTab === 'prompts'" class="h-full" />
        <CanvasAssets v-else-if="activeTab === 'assets'" class="h-full" />
        <CanvasConfig v-else-if="activeTab === 'config'" />
      </div>
    </div>
  </component>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import CanvasBoard from '@/components/canvas/CanvasBoard.vue'
import CanvasGallery from '@/components/canvas/CanvasGallery.vue'
import CanvasImageStudio from '@/components/canvas/CanvasImageStudio.vue'
import CanvasVideoStudio from '@/components/canvas/CanvasVideoStudio.vue'
import CanvasPromptLibrary from '@/components/canvas/CanvasPromptLibrary.vue'
import CanvasAssets from '@/components/canvas/CanvasAssets.vue'
import CanvasConfig from '@/components/canvas/CanvasConfig.vue'
import {
  activeTab, canvases, currentCanvas, createCanvas, loadCanvases,
} from '@/composables/useInfiniteCanvas'

const { t } = useI18n()
const route = useRoute()

const standalone = computed(() => route.query.standalone === '1')
function openStandalone() {
  window.open(`${window.location.origin}/infinite-canvas?standalone=1`, 'lyozc-canvas', 'width=1560,height=960')
}

const TABS = [
  { value: 'board' as const, label: 'canvas.tabBoard' },
  { value: 'gallery' as const, label: 'canvas.tabGallery' },
  { value: 'image' as const, label: 'canvas.tabImage' },
  { value: 'video' as const, label: 'canvas.tabVideo' },
  { value: 'prompts' as const, label: 'canvas.tabPrompts' },
  { value: 'assets' as const, label: 'canvas.tabAssets' },
  { value: 'config' as const, label: 'canvas.tabConfig' }
]

function switchTab(v: typeof activeTab.value) {
  activeTab.value = v
}

onMounted(async () => {
  await loadCanvases()
  // 完全没有画布时才自动创建(避免每次进入生成重复画布)
  if (canvases.value.length === 0) await createCanvas()
})
</script>
