<template>
  <AppLayout>
    <div class="mx-auto max-w-[1600px] space-y-4 p-4 text-gray-900 dark:text-gray-100">
    <!-- 页头 -->
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 class="text-xl font-bold">{{ t('imageStudio.title') }}</h1>
        <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('imageStudio.subtitle') }}</p>
      </div>
      <div class="flex items-center gap-2">
        <label class="text-xs text-gray-500">{{ t('imageStudio.apiKey') }}</label>
        <select
          v-model="selectedKeyId"
          class="h-9 max-w-56 rounded-lg border border-gray-300 bg-white px-2 text-sm dark:border-dark-600 dark:bg-dark-800"
        >
          <option v-for="k in keys" :key="k.id" :value="k.id">{{ k.name }}（{{ maskKey(k.key) }}）</option>
        </select>
      </div>
    </div>

    <div v-if="keys.length === 0" class="card p-6 text-center text-sm text-gray-500">
      {{ t('imageStudio.noKeys') }}
      <router-link to="/keys" class="text-primary-500 hover:underline">{{ t('imageStudio.goCreateKey') }}</router-link>
    </div>

    <div v-else class="grid gap-4 xl:grid-cols-[320px_1fr_300px]">
      <!-- 左侧参数面板 -->
      <div class="card space-y-4 p-4">
        <div class="grid grid-cols-2 gap-1.5">
          <button
            type="button"
            class="rounded-lg border px-2 py-1.5 text-sm transition-colors"
            :class="mode === 't2i'
              ? 'border-primary-500 bg-primary-50 font-medium text-primary-600 dark:bg-primary-900/20 dark:text-primary-300'
              : 'border-gray-200 text-gray-600 dark:border-dark-600 dark:text-gray-400'"
            @click="mode = 't2i'"
          >{{ t('imageStudio.modeText2Image') }}</button>
          <button
            type="button"
            class="rounded-lg border px-2 py-1.5 text-sm transition-colors"
            :class="mode === 'i2i'
              ? 'border-primary-500 bg-primary-50 font-medium text-primary-600 dark:bg-primary-900/20 dark:text-primary-300'
              : 'border-gray-200 text-gray-600 dark:border-dark-600 dark:text-gray-400'"
            @click="mode = 'i2i'"
          >{{ t('imageStudio.modeImage2Image') }}</button>
        </div>

        <div>
          <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('imageStudio.model') }}</label>
          <select
            v-model="model"
            class="h-9 w-full rounded-lg border border-gray-300 bg-white px-2 text-sm outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800"
          >
            <option v-for="m in models" :key="m" :value="m">{{ m }}</option>
          </select>
        </div>

        <details open class="rounded-lg border border-gray-200 dark:border-dark-700">
          <summary class="cursor-pointer px-3 py-2 text-xs font-medium text-gray-500">{{ t('imageStudio.advancedParams') }}</summary>
          <div class="space-y-3 p-3 pt-1">
            <div>
              <label class="mb-1 block text-xs text-gray-500">{{ t('imageStudio.quality') }}</label>
              <select v-model="quality" class="h-8 w-full rounded-lg border border-gray-300 bg-white px-2 text-sm dark:border-dark-600 dark:bg-dark-800">
                <option value="auto">Auto</option>
                <option value="high">High</option>
                <option value="medium">Medium</option>
                <option value="low">Low</option>
              </select>
            </div>
            <div>
              <label class="mb-1 block text-xs text-gray-500">{{ t('imageStudio.outputFormat') }}</label>
              <select v-model="outputFormat" class="h-8 w-full rounded-lg border border-gray-300 bg-white px-2 text-sm dark:border-dark-600 dark:bg-dark-800">
                <option value="png">PNG</option>
                <option value="jpeg">JPEG</option>
                <option value="webp">WebP</option>
              </select>
            </div>
          </div>
        </details>

        <div>
          <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('imageStudio.clarity') }}</label>
          <div class="grid grid-cols-3 gap-1.5">
            <button
              v-for="c in clarityOptions"
              :key="c"
              type="button"
              class="rounded-lg border py-1.5 text-sm transition-colors"
              :class="clarity === c
                ? 'border-primary-500 bg-primary-50 text-primary-600 dark:bg-primary-900/20 dark:text-primary-300'
                : 'border-gray-200 text-gray-600 dark:border-dark-600 dark:text-gray-400'"
              @click="clarity = c"
            >
              {{ c }}
            </button>
          </div>
        </div>

        <div>
          <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('imageStudio.size') }}</label>
          <div class="grid grid-cols-4 gap-1.5">
            <button
              v-for="r in ratios"
              :key="r.label"
              type="button"
              class="rounded-lg border py-1.5 text-center text-[11px] transition-colors"
              :class="ratio === r.label
                ? 'border-primary-500 bg-primary-50 text-primary-600 dark:bg-primary-900/20 dark:text-primary-300'
                : 'border-gray-200 text-gray-600 hover:border-primary-300 dark:border-dark-600 dark:text-gray-400'"
              @click="ratio = r.label"
            >
              <div class="mx-auto mb-0.5 rounded-sm border border-gray-400 dark:border-gray-500" :style="ratioBoxStyle(r)"></div>
              <div class="font-medium">{{ r.label }}</div>
            </button>
          </div>
          <p class="mt-1 text-center text-[11px] text-gray-400">{{ computedSize }}</p>
        </div>

        <div>
          <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('imageStudio.quantity') }}</label>
          <div class="flex flex-wrap gap-1.5">
            <button
              v-for="q in quantityOptions"
              :key="q"
              type="button"
              class="h-8 flex-1 rounded-lg border text-sm transition-colors"
              :class="quantity === q
                ? 'border-primary-500 bg-primary-50 text-primary-600 dark:bg-primary-900/20 dark:text-primary-300'
                : 'border-gray-200 text-gray-600 dark:border-dark-600 dark:text-gray-400'"
              @click="quantity = q"
            >
              {{ q }}
            </button>
          </div>
        </div>

        <div class="flex items-center justify-between rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-800/60">
          <span class="text-xs text-gray-500">{{ t('imageStudio.estimate') }}</span>
          <span class="text-sm font-semibold text-amber-500">${{ estimate.toFixed(2) }}</span>
        </div>
        <p class="text-xs text-gray-400">{{ t('imageStudio.feeNote') }}</p>
      </div>

      <!-- 中间:当前批次 + 提示词 -->
      <div class="space-y-4">
        <div class="card p-4">
          <div class="mb-3 flex items-center justify-between">
            <span class="text-sm font-semibold">{{ t('imageStudio.currentBatch') }}</span>
            <button
              v-if="batch.some(s => s.status === 'failed')"
              type="button"
              class="text-xs text-gray-400 hover:text-red-500"
              @click="clearFailed"
            >{{ t('imageStudio.clearFailed') }}</button>
          </div>
          <div v-if="batch.length === 0" class="flex min-h-64 items-center justify-center text-sm text-gray-400">
            {{ t('imageStudio.emptyResult') }}
          </div>
          <div v-else class="grid grid-cols-2 gap-3 md:grid-cols-3">
            <div v-for="slot in batch" :key="slot.slotId" class="group relative overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700">
              <template v-if="slot.status === 'done' && slot.url">
                <img :src="slot.url" class="aspect-square w-full object-cover" :alt="slot.prompt.slice(0, 30)" />
                <div class="absolute inset-x-0 bottom-0 flex items-center justify-center gap-2 bg-black/60 px-2 py-1 opacity-0 transition-opacity group-hover:opacity-100">
                  <a :href="slot.url" :download="`image-${slot.slotId}.png`" class="text-[10px] text-white underline">{{ t('imageStudio.download') }}</a>
                  <button type="button" class="text-[10px] text-amber-300" :title="t('imageStudio.star')" @click="starSlot(slot.slotId, slot.blob)">★</button>
                  <button type="button" class="text-[10px] text-white" @click="removeSlot(slot.slotId)">✕</button>
                </div>
                <span class="absolute right-1 top-1 rounded bg-green-500/80 px-1 text-[10px] text-white">{{ t('imageStudio.doneShort') }}</span>
              </template>
              <template v-else-if="slot.status === 'failed'">
                <div class="flex aspect-square w-full flex-col items-center justify-center gap-1 bg-red-50 p-2 text-center dark:bg-red-900/20">
                  <span class="line-clamp-3 text-[10px] text-red-500">{{ slot.error || t('imageStudio.generateFailed') }}</span>
                  <button type="button" class="text-[10px] text-primary-500 underline" @click="retrySlot(slot.slotId)">{{ t('imageStudio.retry') }}</button>
                </div>
                <span class="absolute right-1 top-1 rounded bg-red-500/80 px-1 text-[10px] text-white">{{ t('imageStudio.failedShort') }}</span>
              </template>
              <template v-else>
                <div class="flex aspect-square w-full items-center justify-center bg-gray-50 dark:bg-dark-800">
                  <span class="inline-block h-5 w-5 animate-spin rounded-full border-2 border-primary-300 border-t-primary-600"></span>
                </div>
                <span class="absolute bottom-1 left-1/2 -translate-x-1/2 rounded bg-black/50 px-1.5 text-[10px] text-white">{{ slot.status === 'running' ? t('imageStudio.generating') : t('imageStudio.queued') }}</span>
              </template>
            </div>
          </div>
        </div>

        <div class="card p-4">
          <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('imageStudio.prompt') }}</label>
          <textarea
            v-model="prompt"
            rows="4"
            maxlength="4000"
            :placeholder="t('imageStudio.promptPlaceholder')"
            class="w-full rounded-lg border border-gray-300 bg-white p-3 text-sm outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800"
          ></textarea>
          <div class="mt-1 text-right text-[11px] text-gray-400">{{ remainingCharsText }}</div>
          <div class="mt-2 flex items-center justify-between gap-2">
            <div class="flex items-center gap-2">
              <button
                type="button"
                class="h-8 rounded-lg border px-3 text-xs transition-colors"
                :class="mode === 't2i' ? 'border-primary-400 text-primary-600 dark:text-primary-300' : 'border-gray-200 text-gray-500 dark:border-dark-600 dark:text-gray-400'"
                @click="mode = 't2i'"
              >{{ t('imageStudio.modeText2Image') }}</button>
              <button
                type="button"
                class="h-8 rounded-lg border px-3 text-xs transition-colors"
                :class="mode === 'i2i' ? 'border-primary-400 text-primary-600 dark:text-primary-300' : 'border-gray-200 text-gray-500 dark:border-dark-600 dark:text-gray-400'"
                @click="mode = 'i2i'"
              >{{ t('imageStudio.modeImage2Image') }}</button>
              <label v-if="mode === 'i2i'" class="btn btn-secondary h-8 cursor-pointer px-3 text-xs">
                {{ t('imageStudio.refImage') }} {{ refFiles.length }}/8
                <input type="file" accept="image/png,image/jpeg,image/webp" multiple class="hidden" @change="onRefFilesChange" />
              </label>
            </div>
            <button
              type="button"
              class="btn btn-primary min-w-40"
              :disabled="generating || !prompt.trim() || !selectedKeyId"
              @click="generateBatch"
            >
              <span v-if="generating" class="mr-1 inline-block h-3 w-3 animate-spin rounded-full border-2 border-white/40 border-t-white"></span>
              {{ generating ? t('imageStudio.generating') : t('imageStudio.generateN', { n: quantity }) }}
            </button>
          </div>
          <p v-if="errorMessage" class="mt-2 text-sm text-red-500">{{ errorMessage }}</p>
        </div>
      </div>

      <!-- 右侧:本地历史 + 收藏栏 + 存储 -->
      <div class="card flex max-h-[76vh] flex-col p-4 lg:sticky lg:top-20">
        <div class="mb-2 flex items-center justify-between">
          <span class="text-sm font-semibold">{{ t('imageStudio.history') }}</span>
          <button type="button" class="text-xs text-gray-400 hover:text-blue-500" :title="t('common.refresh')" @click="loadHistory">⟳</button>
        </div>
        <input
          v-model="historySearch"
          :placeholder="t('imageStudio.historySearch')"
          class="mb-2 h-8 w-full rounded-lg border border-gray-300 bg-white px-2 text-xs outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800"
        />

        <div class="mb-1 mt-2 text-xs font-semibold text-gray-500">{{ t('imageStudio.starredBar') }}</div>
        <div class="mb-2 max-h-40 space-y-1.5 overflow-y-auto">
          <div v-if="starredItems.length === 0" class="py-2 text-center text-[11px] text-gray-400">{{ t('imageStudio.starredEmpty') }}</div>
          <div v-for="item in starredItems" :key="item.id" class="flex items-center gap-2 rounded-lg border border-amber-200/60 bg-amber-50/50 p-1.5 dark:border-amber-500/30 dark:bg-amber-900/10">
            <img v-if="item.images.length && item.images[0].blob.size > 0" :src="objectUrlFor(item)" class="h-9 w-9 rounded object-cover" alt="" />
            <span class="min-w-0 flex-1 truncate text-[11px]">{{ item.prompt }}</span>
            <button type="button" class="shrink-0 text-amber-500" :title="t('imageStudio.unstar')" @click="toggleStar(item.id)">★</button>
          </div>
        </div>

        <div class="mb-1 mt-2 text-xs font-semibold text-gray-500">{{ t('imageStudio.history') }}</div>
        <div class="min-h-0 flex-1 space-y-2 overflow-y-auto">
          <div v-if="filteredHistory.length === 0" class="py-6 text-center text-xs text-gray-400">
            {{ t('imageStudio.historyEmpty') }}
          </div>
          <div
            v-for="item in filteredHistory"
            :key="item.id"
            class="cursor-pointer rounded-lg border border-gray-200 p-2 transition-colors hover:border-primary-300 dark:border-dark-600"
            @click="restoreBatch(item)"
          >
            <div class="flex items-center justify-between gap-2">
              <span class="truncate text-xs font-medium">{{ item.prompt }}</span>
              <button type="button" class="shrink-0 text-gray-300 hover:text-red-500" :aria-label="t('common.delete')" @click.stop="deleteHistory(item.id)">✕</button>
              <button type="button" class="shrink-0 text-amber-400 hover:text-amber-500" :title="t('imageStudio.star')" @click.stop="toggleStar(item.id)">★</button>
            </div>
            <div class="mt-0.5 text-[10px] text-gray-400">{{ formatHistoryTime(item.ts) }} · {{ item.model }} · {{ item.quality.toUpperCase() }} · {{ item.size }}</div>
          </div>
        </div>

        <div class="mt-3 border-t border-gray-200 pt-2 dark:border-dark-700">
          <div class="mb-1 flex items-center justify-between text-[11px] text-gray-400">
            <span>{{ t('imageStudio.localStorage') }}</span>
            <span>{{ formatBytes(storageUsedBytes) }}</span>
          </div>
          <div class="h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
            <div class="h-full rounded-full bg-primary-500" :style="{ width: storagePercent + '%' }"></div>
          </div>
        </div>

        <div class="mt-3 flex gap-1.5">
          <button type="button" class="flex-1 rounded-lg border border-gray-200 px-2 py-1.5 text-[11px] hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-800" @click="exportZip">{{ t('imageStudio.exportZip') }}</button>
          <label class="flex-1 cursor-pointer rounded-lg border border-gray-200 px-2 py-1.5 text-center text-[11px] hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-800">
            {{ t('imageStudio.importZip') }}
            <input type="file" accept=".zip" class="hidden" @change="onImportZipChange" />
          </label>
          <button type="button" class="flex-1 rounded-lg border border-red-200 px-2 py-1.5 text-[11px] text-red-500 hover:bg-red-50 dark:border-red-500/30 dark:hover:bg-red-900/10" @click="clearHistory">{{ t('imageStudio.clearAll') }}</button>
        </div>
        <p class="mt-2 text-[10px] leading-relaxed text-gray-400">{{ t('imageStudio.historyNote') }}</p>
      </div>
    </div>
  </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import JSZip from 'jszip'
import { keysAPI } from '@/api'
import { useAppStore } from '@/stores/app'
import AppLayout from '@/components/layout/AppLayout.vue'

const { t } = useI18n()
const appStore = useAppStore()

interface KeyItem { id: number; name: string; key: string; status: string }
interface HistoryImage { blob: Blob; failed: boolean }
interface HistoryItem {
  id: number
  ts: number
  model: string
  quality: string
  format: string
  size: string
  prompt: string
  mode: 't2i' | 'i2i'
  images: HistoryImage[]
  starred: boolean
}
interface BatchSlot {
  slotId: number
  status: 'queued' | 'running' | 'done' | 'failed'
  url?: string
  blob?: Blob
  prompt: string
  size: string
  error?: string
}

const keys = ref<KeyItem[]>([])
const selectedKeyId = ref<number | null>(null)
const models = ref<string[]>([])
const model = ref('gpt-image-2')
const mode = ref<'t2i' | 'i2i'>('t2i')
const refFiles = ref<File[]>([])
const quality = ref('auto')
const outputFormat = ref('png')
const clarity = ref('1K')
const ratio = ref('1:1')
const size = ref('1024x1024')
const quantity = ref(1)
const prompt = ref('')
const generating = ref(false)
const errorMessage = ref('')
const history = ref<HistoryItem[]>([])
const historySearch = ref('')
const storageUsedBytes = ref(0)
const batch = ref<BatchSlot[]>([])
let slotSeq = 0
const objectUrlCache = new Map<number, string>()

const ratios = [
  { label: '1:1', w: 1, h: 1 },
  { label: '16:9', w: 16, h: 9 },
  { label: '9:16', w: 9, h: 16 },
  { label: '4:3', w: 4, h: 3 },
  { label: '3:4', w: 3, h: 4 },
  { label: '3:2', w: 3, h: 2 },
  { label: '2:3', w: 2, h: 3 },
  { label: '21:9', w: 21, h: 9 }
]
const clarityOptions = ['1K', '2K', '4K']
const quantityOptions = [1, 4, 8, 16, 32, 50]

const CLARITY_BASE: Record<string, number> = { '1K': 1024, '2K': 2048, '4K': 4096 }

const computedSize = computed(() => {
  const r = ratios.find(x => x.label === ratio.value) || ratios[0]
  const base = CLARITY_BASE[clarity.value]
  let w: number
  let h: number
  if (r.w >= r.h) {
    w = base
    h = Math.round((base * r.h) / r.w)
  } else {
    h = base
    w = Math.round((base * r.w) / r.h)
  }
  return `${w} × ${h}`
})

function ratioBoxStyle(r: { w: number; h: number }) {
  const max = 18
  const scale = Math.min(max / r.w, max / r.h)
  return { width: `${Math.max(4, r.w * scale)}px`, height: `${Math.max(4, r.h * scale)}px` }
}

// 预估单价表(USD/张,按清晰度),按需调整
const UNIT_ESTIMATE: Record<string, Record<string, number>> = {
  'gpt-image-2.5': { '1K': 0.05, '2K': 0.2, '4K': 0.5 },
  'gpt-image-2': { '1K': 0.05, '2K': 0.2, '4K': 0.5 },
  default: { '1K': 0.05, '2K': 0.2, '4K': 0.5 }
}
const estimate = computed(() => {
  const per = (UNIT_ESTIMATE[model.value] || UNIT_ESTIMATE.default)[clarity.value] ?? 0.15
  return per * quantity.value
})

const selectedKey = computed(() => keys.value.find(k => k.id === selectedKeyId.value) || null)
const gatewayBase = computed(() => appStore.cachedPublicSettings?.api_base_url || window.location.origin)
const filteredHistory = computed(() => {
  const q = historySearch.value.trim().toLowerCase()
  if (!q) return history.value
  return history.value.filter(h => h.prompt.toLowerCase().includes(q) || h.model.toLowerCase().includes(q))
})
const starredItems = computed(() => history.value.filter(h => h.starred))
const remainingCharsText = computed(() => t('imageStudio.remainingChars', { n: 4000 - prompt.value.length }))
const storagePercent = computed(() => Math.min(100, (storageUsedBytes.value / (10 * 1024 * 1024 * 1024)) * 100))

const IMAGE_MODEL_PATTERN = /(image|dall|flux|seedream|banana|diffusion)/i
const FALLBACK_MODELS = ['gpt-image-2.5', 'gpt-image-2', 'gpt-image-1', 'dall-e-3']

function maskKey(key: string): string {
  if (!key || key.length < 12) return key
  return `${key.slice(0, 8)}...${key.slice(-6)}`
}

function formatHistoryTime(ts: number): string {
  const d = new Date(ts)
  return `${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

function formatBytes(bytes: number): string {
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

// ===== IndexedDB 本地历史 =====
const IDB_NAME = 'image-studio-db'
const IDB_STORE = 'history'

function openDB(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(IDB_NAME, 1)
    req.onupgradeneeded = () => {
      if (!req.result.objectStoreNames.contains(IDB_STORE)) {
        req.result.createObjectStore(IDB_STORE, { keyPath: 'id' })
      }
    }
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
  })
}

async function idbPut(item: HistoryItem): Promise<void> {
  const db = await openDB()
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction(IDB_STORE, 'readwrite')
    tx.objectStore(IDB_STORE).put(item)
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
  db.close()
}

async function idbAll(): Promise<HistoryItem[]> {
  const db = await openDB()
  const items = await new Promise<HistoryItem[]>((resolve, reject) => {
    const req = db.transaction(IDB_STORE, 'readonly').objectStore(IDB_STORE).getAll()
    req.onsuccess = () => resolve(req.result || [])
    req.onerror = () => reject(req.error)
  })
  db.close()
  return items.sort((a, b) => b.ts - a.ts)
}

async function idbPutMany(items: HistoryItem[]): Promise<void> {
  const db = await openDB()
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction(IDB_STORE, 'readwrite')
    for (const item of items) tx.objectStore(IDB_STORE).put(item)
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
  db.close()
}

async function idbDelete(id: number): Promise<void> {
  const db = await openDB()
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction(IDB_STORE, 'readwrite')
    tx.objectStore(IDB_STORE).delete(id)
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
  db.close()
}

async function idbClear(): Promise<void> {
  const db = await openDB()
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction(IDB_STORE, 'readwrite')
    tx.objectStore(IDB_STORE).clear()
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
  db.close()
}

async function loadHistory() {
  try {
    history.value = await idbAll()
    updateStorageMeter()
  } catch {
    history.value = []
  }
}

function updateStorageMeter() {
  let bytes = 0
  for (const h of history.value) {
    for (const img of h.images) bytes += img.blob.size
  }
  storageUsedBytes.value = bytes
}

async function toggleStar(id: number) {
  const item = history.value.find(h => h.id === id)
  if (!item) return
  item.starred = !item.starred
  await idbPut(item)
  history.value = [...history.value]
}

async function deleteHistory(id: number) {
  await idbDelete(id)
  history.value = history.value.filter(h => h.id !== id)
  updateStorageMeter()
}

async function clearHistory() {
  await idbClear()
  history.value = []
  updateStorageMeter()
}

function restoreBatch(item: HistoryItem) {
  batch.value.forEach(s => { if (s.url) URL.revokeObjectURL(s.url) })
  batch.value = item.images
    .filter(img => !img.failed)
    .map((img, i) => ({
      slotId: item.ts + i,
      status: 'done' as const,
      url: URL.createObjectURL(img.blob),
      blob: img.blob,
      prompt: item.prompt,
      size: item.size
    }))
  mode.value = item.mode
  model.value = item.model
  prompt.value = item.prompt
}

async function exportZip() {
  if (!history.value.length) return
  const zip = new JSZip()
  const metas: Record<string, unknown>[] = []
  for (const h of history.value) {
    const folder = zip.folder(String(h.ts))!
    const meta = {
      id: h.id, ts: h.ts, model: h.model, quality: h.quality, format: h.format,
      size: h.size, prompt: h.prompt, mode: h.mode, starred: h.starred
    }
    metas.push(meta)
    folder.file('meta.json', JSON.stringify(meta, null, 2))
    h.images.forEach((img, i) => {
      if (!img.failed) folder.file(`image-${i + 1}.png`, img.blob)
    })
  }
  zip.file('meta.json', JSON.stringify(metas, null, 2))
  const blob = await zip.generateAsync({ type: 'blob' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `image-studio-${Date.now()}.zip`
  a.click()
  window.setTimeout(() => URL.revokeObjectURL(url), 30000)
}

async function onImportZipChange(event: Event) {
  const input = event.target as HTMLInputElement
  if (!input.files || !input.files[0]) return
  try {
    const zip = await JSZip.loadAsync(input.files[0])
    let metas: Record<string, unknown>[] = []
    const metaFile = zip.file('meta.json')
    if (metaFile) {
      try { metas = JSON.parse(await metaFile.async('string')) } catch { metas = [] }
    }
    const folders = new Set<string>()
    Object.keys(zip.files).forEach(path => {
      const seg = path.split('/')[0]
      if (/^\d+$/.test(seg)) folders.add(seg)
    })
    const items: HistoryItem[] = []
    for (const folder of folders) {
      const meta = metas.find(m => String(m.ts) === folder) || {}
      const images: HistoryImage[] = []
      for (const path of Object.keys(zip.files)) {
        if (!path.startsWith(`${folder}/image-`) || zip.files[path].dir) continue
        const blob = await zip.files[path].async('blob')
        images.push({ blob, failed: path.includes('-failed') })
      }
      if (!images.length) continue
      items.push({
        id: Number(folder) || Date.now() + items.length,
        ts: Number(meta.ts) || Date.now(),
        model: String(meta.model || 'unknown'),
        quality: String(meta.quality || 'auto'),
        format: String(meta.format || 'png'),
        size: String(meta.size || '1024x1024'),
        prompt: String(meta.prompt || ''),
        mode: (meta.mode as 't2i' | 'i2i') || 't2i',
        images,
        starred: Boolean(meta.starred)
      })
    }
    if (items.length) {
      await idbPutMany(items)
      await loadHistory()
      appStore.showSuccess(t('imageStudio.importDone', { n: items.length }))
    }
  } catch {
    appStore.showError(t('imageStudio.importFailed'))
  }
  input.value = ''
}

function objectUrlFor(item: HistoryItem): string {
  const cached = objectUrlCache.get(item.id)
  if (cached) return cached
  const url = item.images.length && !item.images[0].failed
    ? URL.createObjectURL(item.images[0].blob)
    : ''
  if (url) objectUrlCache.set(item.id, url)
  return url
}

function blobFromB64(b64: string): Blob {
  const bin = atob(b64)
  const bytes = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
  return new Blob([bytes], { type: 'image/png' })
}

async function blobFromUrl(url: string): Promise<Blob> {
  const res = await fetch(url)
  return await res.blob()
}

async function loadKeys() {
  try {
    const res = await keysAPI.list(1, 100, { status: 'active' })
    keys.value = (res.items || []).map(k => ({ id: k.id, name: k.name, key: k.key, status: k.status }))
    const saved = Number(localStorage.getItem('image_studio_key_id'))
    selectedKeyId.value = keys.value.some(k => k.id === saved)
      ? saved
      : (keys.value[0]?.id ?? null)
  } catch {
    keys.value = []
  }
}

async function loadModels() {
  if (!selectedKey.value) return
  try {
    const controller = new AbortController()
    const timer = window.setTimeout(() => controller.abort(), 15000)
    const res = await fetch(`${gatewayBase.value}/v1/models`, {
      headers: { Authorization: `Bearer ${selectedKey.value.key}` },
      signal: controller.signal
    })
    window.clearTimeout(timer)
    if (!res.ok) throw new Error(String(res.status))
    const j = await res.json()
    const ids: string[] = (j.data || []).map((m: { id: string }) => m.id)
    const imageModels = ids.filter(id => IMAGE_MODEL_PATTERN.test(id))
    models.value = imageModels.length ? imageModels : FALLBACK_MODELS
    if (!models.value.includes(model.value)) model.value = models.value[0] || FALLBACK_MODELS[0]
  } catch {
    models.value = FALLBACK_MODELS
    model.value = FALLBACK_MODELS[0]
  }
}

// ===== 生成(逐张循环,每张一个 n=1 请求,支持逐张状态/重试) =====
async function generateOne(promptText: string, refList: File[]): Promise<{ blob: Blob | null; error?: string }> {
  const controller = new AbortController()
  const timer = window.setTimeout(() => controller.abort(), 300000)
  try {
    let res: Response
    if (mode.value === 'i2i' && refList.length) {
      const form = new FormData()
      form.append('model', model.value)
      form.append('prompt', promptText)
      if (size.value !== 'auto') form.append('size', size.value)
      if (quality.value !== 'auto') form.append('quality', quality.value)
      form.append('output_format', outputFormat.value)
      for (const f of refList) form.append('image', f)
      res = await fetch(`${gatewayBase.value}/v1/images/edits`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${selectedKey.value!.key}` },
        body: form,
        signal: controller.signal
      })
    } else {
      res = await fetch(`${gatewayBase.value}/v1/images/generations`, {
        method: 'POST',
        headers: {
          Authorization: `Bearer ${selectedKey.value!.key}`,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          model: model.value,
          prompt: promptText,
          size: size.value === 'auto' ? undefined : size.value,
          quality: quality.value === 'auto' ? undefined : quality.value,
          output_format: outputFormat.value,
          n: 1
        }),
        signal: controller.signal
      })
    }
    const body = await res.json().catch(() => null)
    if (!res.ok) {
      const msg = body?.error?.message || body?.message || t('imageStudio.generateFailed')
      return { blob: null, error: msg }
    }
    const data: Array<{ b64_json?: string; url?: string }> = body?.data || []
    if (!data.length) return { blob: null, error: t('imageStudio.generateFailed') }
    const first = data[0]
    const blob = first.b64_json
      ? blobFromB64(first.b64_json)
      : (first.url ? await blobFromUrl(first.url) : null)
    if (!blob) return { blob: null, error: t('imageStudio.generateFailed') }
    return { blob }
  } catch (err: unknown) {
    if (err instanceof DOMException && err.name === 'AbortError') {
      return { blob: null, error: t('imageStudio.timeout') }
    }
    return { blob: null, error: err instanceof Error ? err.message : t('imageStudio.generateFailed') }
  } finally {
    window.clearTimeout(timer)
  }
}

function clearFailed() {
  batch.value = batch.value.filter(s => s.status !== 'failed')
}

function removeSlot(slotId: number) {
  batch.value = batch.value.filter(s => s.slotId !== slotId)
}

function starSlot(slotId: number, blob?: Blob) {
  const slot = batch.value.find(s => s.slotId === slotId)
  const b = blob || slot?.blob
  if (!slot || !b) return
  const item: HistoryItem = {
    id: Date.now(),
    ts: Date.now(),
    model: model.value,
    quality: quality.value,
    format: outputFormat.value,
    size: slot.size,
    prompt: slot.prompt,
    mode: mode.value,
    images: [{ blob: b, failed: false }],
    starred: true
  }
  history.value = [item, ...history.value]
  void idbPut(item)
  updateStorageMeter()
  appStore.showSuccess(t('imageStudio.starredSaved'))
}

async function retrySlot(slotId: number) {
  const slot = batch.value.find(s => s.slotId === slotId)
  if (!slot || generating.value) return
  const target: BatchSlot = slot
  target.status = 'running'
  const result = await generateOne(slot.prompt, refFiles.value)
  target.status = result.blob ? 'done' : 'failed'
  target.error = result.error
  if (result.blob) {
    target.blob = result.blob
    target.url = URL.createObjectURL(result.blob)
  }
}

async function generateBatch() {
  errorMessage.value = ''
  if (!selectedKey.value || !prompt.value.trim()) return
  const n = quantity.value
  batch.value.forEach(s => { if (s.url) URL.revokeObjectURL(s.url) })
  batch.value = []
  generating.value = true
  let okCount = 0
  try {
    for (let i = 0; i < n; i++) {
      const slotId = ++slotSeq
      const slot: BatchSlot = { slotId, status: 'running', prompt: prompt.value.trim(), size: computedSize.value }
      batch.value = [...batch.value, slot]
      const result = await generateOne(prompt.value.trim(), refFiles.value)
      if (result.blob) {
        slot.blob = result.blob
        slot.url = URL.createObjectURL(result.blob)
        slot.status = 'done'
        okCount++
      } else {
        slot.status = 'failed'
        slot.error = result.error
      }
      batch.value = [...batch.value]
    }
    if (okCount > 0) appStore.showSuccess(t('imageStudio.batchDone', { ok: okCount, total: n }))
  } finally {
    generating.value = false
    const done = batch.value.filter(s => s.status === 'done' && s.blob)
    if (done.length) {
      const item: HistoryItem = {
        id: Date.now(),
        ts: Date.now(),
        model: model.value,
        quality: quality.value,
        format: outputFormat.value,
        size: computedSize.value,
        prompt: prompt.value.trim(),
        mode: mode.value,
        images: done.map(s => ({ blob: s.blob as Blob, failed: false })),
        starred: false
      }
      await idbPut(item)
      history.value = [item, ...history.value]
      updateStorageMeter()
    }
  }
}

function onRefFilesChange(event: Event) {
  const input = event.target as HTMLInputElement
  if (input.files) refFiles.value = Array.from(input.files).slice(0, 8)
}

watch(selectedKeyId, () => {
  localStorage.setItem('image_studio_key_id', String(selectedKeyId.value ?? ''))
  loadModels()
})

onMounted(async () => {
  await loadKeys()
  await loadHistory()
  if (selectedKeyId.value !== null) await loadModels()
})

onBeforeUnmount(() => {
  batch.value.forEach(s => { if (s.url) URL.revokeObjectURL(s.url) })
})
</script>
