<template>
  <div
    class="flex h-full w-72 shrink-0 flex-col border-r border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900 transition-all z-10 overflow-hidden"
    :class="{ 'hidden': !open }"
  >
    <!-- 顶部 Tabs -->
    <div class="flex border-b border-gray-200 bg-gray-50/70 p-1.5 dark:border-dark-700 dark:bg-dark-800/60">
      <button
        v-for="tab in tabs"
        :key="tab.key"
        type="button"
        class="flex-1 rounded-md py-1 text-center text-xs font-medium transition-colors"
        :class="activeTab === tab.key ? 'bg-white text-primary-600 shadow-sm dark:bg-dark-700 dark:text-primary-400 font-semibold' : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'"
        @click="activeTab = tab.key"
      >
        {{ tab.label }}
      </button>
    </div>

    <!-- 1. 画布元素 Tab -->
    <div v-if="activeTab === 'elements'" class="flex min-h-0 flex-1 flex-col p-2.5">
      <!-- 次级栏: 标题与类型筛选 -->
      <div class="mb-2 flex items-center justify-between gap-1">
        <span class="text-xs font-semibold text-gray-700 dark:text-gray-200">{{ t('canvas.sidePanel.elements') }}</span>
        <div class="flex items-center gap-0.5 rounded-md bg-gray-100 p-0.5 text-[11px] dark:bg-dark-800">
          <button
            v-for="f in filterOptions"
            :key="f.key"
            type="button"
            class="rounded px-1.5 py-0.5 transition-colors"
            :class="filterType === f.key ? 'bg-white font-medium text-gray-900 shadow-xs dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-800 dark:text-gray-400'"
            @click="filterType = f.key"
          >
            {{ f.label }}
          </button>
        </div>
      </div>

      <!-- 搜索框 -->
      <div class="relative mb-2.5">
        <input
          v-model="nodeSearch"
          type="text"
          :placeholder="t('canvas.sidePanel.searchNodes')"
          class="w-full rounded-md border border-gray-200 bg-gray-50/70 py-1 pl-7 pr-6 text-xs outline-none focus:border-primary-500 focus:bg-white dark:border-dark-700 dark:bg-dark-800 dark:focus:border-primary-400"
        />
        <span class="pointer-events-none absolute left-2 top-1.5 text-xs text-gray-400">🔍</span>
        <button
          v-if="nodeSearch"
          type="button"
          class="absolute right-1.5 top-1 text-xs text-gray-400 hover:text-gray-600 dark:hover:text-gray-200"
          @click="nodeSearch = ''"
        >✕</button>
      </div>

      <!-- 元素列表 -->
      <div class="min-h-0 flex-1 overflow-y-auto space-y-1.5 pr-0.5">
        <div
          v-if="filteredNodes.length === 0"
          class="flex h-40 flex-col items-center justify-center text-center text-xs text-gray-400 dark:text-gray-500"
        >
          {{ t('canvas.sidePanel.noNodes') }}
        </div>

        <div
          v-for="node in filteredNodes"
          :key="node.id"
          class="group relative flex cursor-pointer items-center gap-2 rounded-lg border p-1.5 transition-all"
          :class="[
            selection.has(node.id)
              ? 'border-primary-500 bg-primary-50/60 dark:border-primary-500 dark:bg-primary-950/30 ring-1 ring-primary-400/40'
              : hoveredNodeId === node.id
                ? 'border-primary-300 bg-gray-50 dark:border-dark-600 dark:bg-dark-800/70'
                : 'border-gray-200 hover:border-gray-300 bg-white dark:border-dark-700 dark:bg-dark-800 dark:hover:border-dark-600'
          ]"
          @click="emit('selectNode', node)"
          @mouseenter="emit('hoverNode', node.id)"
          @mouseleave="emit('hoverNode', null)"
        >
          <!-- 缩略图 -->
          <div class="relative h-10 w-10 shrink-0 overflow-hidden rounded bg-gray-100 dark:bg-dark-700 flex items-center justify-center">
            <img
              v-if="node.type === 'image' && nodeUrls.get(node.assetId || '')"
              :src="nodeUrls.get(node.assetId || '')"
              class="h-full w-full object-cover"
              alt=""
            />
            <video
              v-else-if="node.type === 'video' && nodeUrls.get(node.assetId || '')"
              :src="nodeUrls.get(node.assetId || '')"
              class="h-full w-full object-cover"
              preload="metadata"
              muted
            ></video>
            <span v-else-if="node.type === 'text'" class="font-serif text-sm font-bold text-gray-500 dark:text-gray-400">T</span>
            <span v-else class="text-xs text-gray-400">📄</span>

            <!-- 节点类型小标签 -->
            <span
              class="absolute bottom-0 right-0 rounded-tl px-1 text-[8px] font-semibold text-white"
              :class="node.type === 'image' ? 'bg-sky-500' : node.type === 'video' ? 'bg-violet-500' : 'bg-emerald-600'"
            >
              {{ node.type === 'image' ? '图' : node.type === 'video' ? '视' : '文' }}
            </span>
          </div>

          <!-- 文本/描述与尺寸 -->
          <div class="min-w-0 flex-1">
            <div class="truncate text-xs font-medium text-gray-800 dark:text-gray-200">
              {{ nodeTitle(node) }}
            </div>
            <div class="mt-0.5 text-[10px] text-gray-400 dark:text-gray-500 font-mono">
              {{ Math.round(node.w) }} × {{ Math.round(node.h) }}
            </div>
          </div>

          <!-- 操作按钮 (定位 & 删除) -->
          <div class="flex items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100">
            <button
              type="button"
              class="rounded p-1 text-gray-400 hover:text-primary-600 dark:hover:text-primary-400"
              :title="t('canvas.sidePanel.focusNode')"
              @click.stop="emit('selectNode', node)"
            >
              ⊙
            </button>
            <button
              type="button"
              class="rounded p-1 text-gray-400 hover:text-red-500"
              :title="t('common.delete')"
              @click.stop="emit('deleteNode', node.id)"
            >
              🗑
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- 2. 资产 Tab -->
    <div v-else-if="activeTab === 'assets'" class="flex min-h-0 flex-1 flex-col p-2.5">
      <!-- 搜索框 -->
      <div class="relative mb-2.5">
        <input
          v-model="assetSearch"
          type="text"
          :placeholder="t('canvas.sidePanel.searchAssets')"
          class="w-full rounded-md border border-gray-200 bg-gray-50/70 py-1 pl-7 pr-6 text-xs outline-none focus:border-primary-500 focus:bg-white dark:border-dark-700 dark:bg-dark-800 dark:focus:border-primary-400"
        />
        <span class="pointer-events-none absolute left-2 top-1.5 text-xs text-gray-400">🔍</span>
        <button
          v-if="assetSearch"
          type="button"
          class="absolute right-1.5 top-1 text-xs text-gray-400 hover:text-gray-600 dark:hover:text-gray-200"
          @click="assetSearch = ''"
        >✕</button>
      </div>

      <div class="min-h-0 flex-1 overflow-y-auto space-y-2 pr-0.5">
        <div
          v-if="assetItems.length === 0"
          class="flex h-40 flex-col items-center justify-center text-center text-xs text-gray-400 dark:text-gray-500"
        >
          {{ t('canvas.sidePanel.noAssets') }}
        </div>

        <div
          v-for="item in assetItems"
          :key="item.kind + item.id"
          class="group relative overflow-hidden rounded-lg border border-gray-200 bg-white transition-colors dark:border-dark-700 dark:bg-dark-800"
        >
          <div class="relative aspect-video w-full bg-gray-100 dark:bg-dark-700">
            <img
              v-if="item.kind === 'image'"
              :src="item.url"
              class="h-full w-full object-contain"
              alt=""
            />
            <video
              v-else
              :src="item.url"
              class="h-full w-full object-contain"
              preload="metadata"
              muted
            ></video>

            <span
              class="absolute left-1.5 top-1.5 rounded px-1 text-[9px] font-medium text-white"
              :class="item.kind === 'image' ? 'bg-sky-500/85' : 'bg-violet-500/85'"
            >
              {{ item.kind === 'image' ? t('canvas.kindImage') : t('canvas.kindVideo') }}
            </span>
          </div>

          <div class="p-2">
            <p class="line-clamp-2 text-[11px] text-gray-600 dark:text-gray-300">
              {{ item.prompt || (item.kind === 'image' ? t('canvas.kindImage') : t('canvas.kindVideo')) }}
            </p>
            <div class="mt-2 flex items-center justify-between border-t border-gray-100 pt-1.5 text-xs dark:border-dark-700">
              <button
                type="button"
                class="rounded bg-primary-50 px-2 py-0.5 text-[11px] font-medium text-primary-600 hover:bg-primary-100 dark:bg-primary-950/40 dark:text-primary-400"
                @click="onInsertAsset(item)"
              >
                {{ t('canvas.sidePanel.insertToCanvas') }}
              </button>
              <button
                type="button"
                class="text-[11px] text-gray-400 hover:text-red-500"
                @click="item.remove()"
              >
                {{ t('common.delete') }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 3. 提示词库 Tab -->
    <div v-else-if="activeTab === 'prompts'" class="flex min-h-0 flex-1 flex-col p-2.5">
      <!-- 模板源选择 -->
      <div class="mb-2">
        <select
          :value="activeSourceId"
          class="w-full rounded-md border border-gray-200 bg-gray-50/80 px-2 py-1 text-xs outline-none focus:border-primary-500 dark:border-dark-700 dark:bg-dark-800 text-gray-700 dark:text-gray-200"
          @change="onSourceChange(($event.target as HTMLSelectElement).value)"
        >
          <option v-for="s in sources" :key="s.id" :value="s.id">
            {{ s.name }} ({{ s.items.length }})
          </option>
        </select>
      </div>

      <!-- 搜索框 -->
      <div class="relative mb-2.5">
        <input
          v-model="onlineSearch"
          type="text"
          :placeholder="t('canvas.sidePanel.searchPrompts')"
          class="w-full rounded-md border border-gray-200 bg-gray-50/70 py-1 pl-7 pr-6 text-xs outline-none focus:border-primary-500 focus:bg-white dark:border-dark-700 dark:bg-dark-800 dark:focus:border-primary-400"
        />
        <span class="pointer-events-none absolute left-2 top-1.5 text-xs text-gray-400">🔍</span>
        <button
          v-if="onlineSearch"
          type="button"
          class="absolute right-1.5 top-1 text-xs text-gray-400 hover:text-gray-600 dark:hover:text-gray-200"
          @click="onlineSearch = ''"
        >✕</button>
      </div>

      <!-- 提示词列表 -->
      <div class="min-h-0 flex-1 overflow-y-auto space-y-2 pr-0.5">
        <div v-if="activeSource && activeSource.loading && !activeSource.items.length" class="flex h-40 items-center justify-center text-xs text-gray-400">
          <span class="inline-block h-4 w-4 animate-spin rounded-full border-2 border-primary-300 border-t-primary-600 mr-2"></span>
          {{ t('canvas.onlineLoading') }}
        </div>

        <div v-else-if="filteredOnline.length === 0" class="flex h-40 flex-col items-center justify-center text-center text-xs text-gray-400 dark:text-gray-500">
          {{ t('canvas.sidePanel.noPrompts') }}
        </div>

        <div
          v-for="p in filteredOnline"
          :key="p.id"
          class="rounded-lg border border-gray-200 bg-white p-2.5 dark:border-dark-700 dark:bg-dark-800"
        >
          <div class="truncate text-xs font-semibold text-gray-800 dark:text-gray-200">{{ p.title }}</div>
          <p class="mt-1 line-clamp-3 text-[11px] leading-relaxed text-gray-500 dark:text-gray-400 whitespace-pre-wrap">{{ p.prompt }}</p>
          <div class="mt-2 flex items-center justify-between border-t border-gray-100 pt-1.5 dark:border-dark-700">
            <button
              type="button"
              class="rounded bg-primary-50 px-2 py-0.5 text-[11px] font-medium text-primary-600 hover:bg-primary-100 dark:bg-primary-950/40 dark:text-primary-400"
              @click="emit('insertText', p.prompt)"
            >
              {{ t('canvas.toCanvas') }}
            </button>
            <button
              type="button"
              class="text-[11px] text-gray-400 hover:text-primary-500"
              @click="copyPrompt(p.prompt)"
            >
              {{ t('canvas.copy') }}
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import type { CanvasNode } from '@/composables/useInfiniteCanvas'
import {
  history as imageHistory, objectUrlFor as imageObjectUrl, deleteHistory as deleteImageHistory,
  loadHistory as loadImageHistory,
} from '@/composables/useImageStudioEngine'
import {
  history as videoHistory, historyVideoUrl, deleteHistory as deleteVideoHistory,
  loadHistory as loadVideoHistory,
} from '@/composables/useVideoStudioEngine'
import {
  sources, activeSourceId, activeSource, onlineSearch, filteredOnline,
  loadSource,
} from '@/composables/useOnlinePrompts'

const props = defineProps<{
  nodes: CanvasNode[]
  selection: Set<string>
  hoveredNodeId: string | null
  nodeUrls: Map<string, string>
  open: boolean
}>()

const emit = defineEmits<{
  (e: 'update:open', val: boolean): void
  (e: 'selectNode', node: CanvasNode): void
  (e: 'hoverNode', nodeId: string | null): void
  (e: 'deleteNode', nodeId: string): void
  (e: 'insertMedia', blob: Blob, kind: 'image' | 'video'): void
  (e: 'insertText', text: string): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

const activeTab = ref<'elements' | 'assets' | 'prompts'>('elements')

const tabs = computed(() => [
  { key: 'elements' as const, label: t('canvas.sidePanel.elements') },
  { key: 'assets' as const, label: t('canvas.sidePanel.assets') },
  { key: 'prompts' as const, label: t('canvas.sidePanel.prompts') }
])

// 1. 元素列表
const nodeSearch = ref('')
const filterType = ref<'all' | 'image' | 'video' | 'text'>('all')
const filterOptions = computed(() => [
  { key: 'all' as const, label: t('canvas.sidePanel.filterAll') },
  { key: 'image' as const, label: t('canvas.sidePanel.filterImage') },
  { key: 'video' as const, label: t('canvas.sidePanel.filterVideo') },
  { key: 'text' as const, label: t('canvas.sidePanel.filterText') }
])

function nodeTitle(node: CanvasNode): string {
  if (node.type === 'text') {
    return node.text?.trim() ? node.text.slice(0, 30) : t('canvas.sidePanel.filterText')
  }
  if (node.type === 'image') return t('canvas.sidePanel.filterImage')
  if (node.type === 'video') return t('canvas.sidePanel.filterVideo')
  return '节点'
}

const filteredNodes = computed(() => {
  const kw = nodeSearch.value.trim().toLowerCase()
  return props.nodes.filter(n => {
    if (filterType.value !== 'all' && n.type !== filterType.value) return false
    if (!kw) return true
    if (n.type.includes(kw)) return true
    if (n.type === 'text' && n.text?.toLowerCase().includes(kw)) return true
    return false
  })
})

// 2. 资产列表
const assetSearch = ref('')
interface AssetItem {
  kind: 'image' | 'video'
  id: number
  url: string
  prompt: string
  remove: () => void
}

const assetItems = computed<AssetItem[]>(() => {
  const kw = assetSearch.value.trim().toLowerCase()
  const out: AssetItem[] = []
  for (const item of imageHistory.value) {
    if (kw && !item.prompt.toLowerCase().includes(kw)) continue
    const url = imageObjectUrl(item)
    if (!url) continue
    out.push({
      kind: 'image', id: item.id, url, prompt: item.prompt,
      remove: () => void deleteImageHistory(item.id)
    })
  }
  for (const item of videoHistory.value) {
    if (kw && !item.prompt.toLowerCase().includes(kw)) continue
    out.push({
      kind: 'video', id: item.id, url: historyVideoUrl(item), prompt: item.prompt,
      remove: () => void deleteVideoHistory(item.id)
    })
  }
  return out.sort((a, b) => b.id - a.id)
})

function onInsertAsset(item: AssetItem) {
  const blob = item.kind === 'image'
    ? imageHistory.value.find(i => i.id === item.id)?.images[0]?.blob
    : videoHistory.value.find(i => i.id === item.id)?.blob
  if (blob) emit('insertMedia', blob, item.kind)
}

// 3. 提示词库
function onSourceChange(id: string) {
  void loadSource(id)
}

async function copyPrompt(content: string) {
  try {
    await navigator.clipboard.writeText(content)
    appStore.showSuccess(t('canvas.sidePanel.promptCopied'))
  } catch {
    appStore.showError(t('common.copyFailed'))
  }
}

onMounted(() => {
  void loadImageHistory()
  void loadVideoHistory()
  if (!activeSourceId.value && sources.value.length > 0) {
    void loadSource(sources.value[0].id)
  }
})
</script>
