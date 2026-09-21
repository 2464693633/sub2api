<template>
  <component :is="standalone ? 'div' : AppLayout" :class="standalone ? 'min-h-screen bg-gray-50 p-3 dark:bg-dark-950' : ''">
    <div class="flex flex-col lg:h-[calc(100vh-9rem)] lg:min-h-[560px] lg:overflow-hidden rounded-xl border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-900" :class="standalone ? 'lg:h-[calc(100vh-1.5rem)]' : ''">
      <!-- 顶部 tab 栏(移动端横向滑动) -->
      <div class="flex items-center gap-1 overflow-x-auto border-b border-gray-200 px-3 py-2 dark:border-dark-700">
        <span class="mr-2 flex shrink-0 items-center gap-1 text-sm font-bold"><span class="text-primary-500">▲</span> {{ t('canvas.appName') }}</span>
        <button
          v-for="tab in TABS"
          :key="tab.value"
          type="button"
          class="shrink-0 whitespace-nowrap rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors"
          :class="activeTab === tab.value ? 'bg-gray-200/70 text-gray-900 dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-800 dark:hover:text-gray-200'"
          @click="switchTab(tab.value)"
        >{{ t(tab.label) }}</button>
        <button
          type="button"
          class="ml-auto hidden shrink-0 rounded-md bg-gray-900 px-3 py-1.5 text-xs font-medium text-white hover:bg-gray-700 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-200 sm:block"
          @click="openStandalone"
        >⧉ {{ t('imageStudio.openStandalone') }}</button>
      </div>

      <!-- 内容区 -->
      <div class="min-h-0 flex-1 p-2 sm:p-3">
        <CanvasImageStudio v-if="activeTab === 'image'" class="h-full" />
        <CanvasVideoStudio v-else-if="activeTab === 'video'" class="h-full" />
        <CanvasPromptLibrary v-else-if="activeTab === 'prompts'" class="h-full" />
        <CanvasAssets v-else-if="activeTab === 'assets'" class="h-full" />
        <CanvasConfig v-else-if="activeTab === 'config'" />
      </div>
    </div>
  </component>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import CanvasImageStudio from '@/components/canvas/CanvasImageStudio.vue'
import CanvasVideoStudio from '@/components/canvas/CanvasVideoStudio.vue'
import CanvasPromptLibrary from '@/components/canvas/CanvasPromptLibrary.vue'
import CanvasAssets from '@/components/canvas/CanvasAssets.vue'
import CanvasConfig from '@/components/canvas/CanvasConfig.vue'
import { activeTab } from '@/composables/useInfiniteCanvas'

const { t } = useI18n()
const route = useRoute()

const standalone = computed(() => route.query.standalone === '1')
function openStandalone() {
  window.open(`${window.location.origin}/studio?standalone=1`, 'lyozc-canvas', 'width=1560,height=960')
}

const TABS = [
  { value: 'image' as const, label: 'canvas.tabImage' },
  { value: 'video' as const, label: 'canvas.tabVideo' },
  { value: 'prompts' as const, label: 'canvas.tabPrompts' },
  { value: 'assets' as const, label: 'canvas.tabAssets' },
  { value: 'config' as const, label: 'canvas.tabConfig' }
]

function switchTab(v: typeof activeTab.value) {
  activeTab.value = v
}


</script>
