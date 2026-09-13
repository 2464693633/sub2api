<template>
  <div class="mx-auto max-w-7xl space-y-4 p-4 text-gray-900 dark:text-gray-100">
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

    <!-- 未选密钥提示 -->
    <div v-if="keys.length === 0" class="card p-6 text-center text-sm text-gray-500">
      {{ t('imageStudio.noKeys') }}
      <router-link to="/keys" class="text-primary-500 hover:underline">{{ t('imageStudio.goCreateKey') }}</router-link>
    </div>

    <div v-else class="grid gap-4 lg:grid-cols-[320px_1fr_280px]">
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
          <input
            v-model="model"
            list="studio-model-options"
            class="h-9 w-full rounded-lg border border-gray-300 bg-white px-2 text-sm outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800"
          />
          <datalist id="studio-model-options">
            <option v-for="m in models" :key="m" :value="m" />
          </datalist>
        </div>

        <div>
          <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('imageStudio.size') }}</label>
          <div class="grid grid-cols-3 gap-1.5">
            <button
              v-for="preset in sizePresets"
              :key="preset.value"
              type="button"
              class="rounded-lg border px-1 py-1.5 text-center text-[11px] transition-colors"
              :class="size === preset.value
                ? 'border-primary-500 bg-primary-50 text-primary-600 dark:bg-primary-900/20 dark:text-primary-300'
                : 'border-gray-200 text-gray-600 hover:border-primary-300 dark:border-dark-600 dark:text-gray-400'"
              @click="size = preset.value"
            >
              <div class="font-medium">{{ preset.ratio }}</div>
              <div class="text-[10px] text-gray-400">{{ preset.value }}</div>
            </button>
          </div>
        </div>

        <div v-if="mode === 't2i'">
          <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('imageStudio.quantity') }}</label>
          <div class="flex gap-1.5">
            <button
              v-for="q in [1, 2, 4]"
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

        <p class="text-xs text-gray-400">{{ t('imageStudio.feeNote') }}</p>
      </div>

      <!-- 中间:提示词 + 当前批次结果 -->
      <div class="space-y-4">
        <div class="card p-4">
          <div v-if="mode === 'i2i'" class="mb-2 flex items-center gap-2">
            <label class="btn btn-secondary h-8 cursor-pointer px-3 text-xs">
              {{ t('imageStudio.refImage') }}
              <input
                type="file"
                accept="image/png,image/jpeg,image/webp"
                class="hidden"
                @change="onRefFileChange"
              />
            </label>
            <span v-if="refFile" class="text-xs text-gray-500">{{ refFile.name }}</span>
          </div>
          <label class="mb-1 block text-xs font-medium text-gray-500">{{ t('imageStudio.prompt') }}</label>
          <textarea
            v-model="prompt"
            rows="4"
            maxlength="4000"
            :placeholder="t('imageStudio.promptPlaceholder')"
            class="w-full rounded-lg border border-gray-300 bg-white p-3 text-sm outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800"
          ></textarea>
          <div class="mt-3 flex items-center justify-between gap-2">
            <p v-if="errorMessage" class="text-sm text-red-500">{{ errorMessage }}</p>
            <span v-else class="text-xs text-gray-400">{{ t('imageStudio.feeNote2') }}</span>
            <button
              type="button"
              class="btn btn-primary min-w-32"
              :disabled="generating || !prompt.trim()"
              @click="generate"
            >
              <span v-if="generating" class="mr-1 inline-block h-3 w-3 animate-spin rounded-full border-2 border-white/40 border-t-white"></span>
              {{ generating ? t('imageStudio.generating') : t('imageStudio.generate') }}
            </button>
          </div>
        </div>

        <div class="card min-h-72 p-4">
          <div v-if="generating" class="flex min-h-56 items-center justify-center text-sm text-gray-400">
            <span class="mr-2 inline-block h-4 w-4 animate-spin rounded-full border-2 border-primary-300 border-t-primary-600"></span>
            {{ t('imageStudio.generatingWait') }}
          </div>
          <div v-else-if="results.length" class="grid grid-cols-2 gap-3 xl:grid-cols-3">
            <div v-for="(img, idx) in results" :key="idx" class="group relative overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700">
              <img :src="img.url" class="w-full object-cover" :alt="img.prompt.slice(0, 30)" />
              <div class="absolute inset-x-0 bottom-0 flex items-center justify-between bg-black/60 px-2 py-1 opacity-0 transition-opacity group-hover:opacity-100">
                <span class="text-[10px] text-white/80">{{ img.size }}</span>
                <a :href="img.url" :download="`image-${batchTs}-${idx + 1}.png`" class="text-[10px] text-white underline">{{ t('imageStudio.download') }}</a>
              </div>
            </div>
          </div>
          <div v-else class="flex min-h-56 items-center justify-center text-sm text-gray-400">
            {{ t('imageStudio.emptyResult') }}
          </div>
        </div>
      </div>

      <!-- 右侧:本地历史 -->
      <div class="card flex max-h-[70vh] flex-col p-4 lg:sticky lg:top-20">
        <div class="mb-2 flex items-center justify-between">
          <span class="text-sm font-semibold">{{ t('imageStudio.history') }}</span>
          <button type="button" class="text-xs text-red-500 hover:underline" @click="clearHistory">{{ t('imageStudio.clearAll') }}</button>
        </div>
        <input
          v-model="historySearch"
          :placeholder="t('imageStudio.historySearch')"
          class="mb-2 h-8 w-full rounded-lg border border-gray-300 bg-white px-2 text-xs outline-none focus:border-primary-500 dark:border-dark-600 dark:bg-dark-800"
        />
        <div class="min-h-0 flex-1 space-y-2 overflow-y-auto">
          <div v-if="filteredHistory.length === 0" class="py-6 text-center text-xs text-gray-400">
            {{ t('imageStudio.historyEmpty') }}
          </div>
          <div
            v-for="item in filteredHistory"
            :key="item.id"
            class="cursor-pointer rounded-lg border border-gray-200 p-2 transition-colors hover:border-primary-300 dark:border-dark-600"
            @click="restoreFromHistory(item)"
          >
            <div class="flex items-center justify-between gap-2">
              <span class="truncate text-xs font-medium">{{ item.prompt }}</span>
              <button
                type="button"
                class="shrink-0 text-gray-300 hover:text-red-500"
                :aria-label="t('common.delete')"
                @click.stop="deleteHistory(item.id)"
              >✕</button>
            </div>
            <div class="mt-0.5 text-[10px] text-gray-400">{{ formatHistoryTime(item.ts) }} · {{ item.model }} · {{ item.size }}</div>
          </div>
        </div>
        <p class="mt-2 text-[10px] leading-relaxed text-gray-400">{{ t('imageStudio.historyNote') }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { keysAPI } from '@/api'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const appStore = useAppStore()

interface KeyItem { id: number; name: string; key: string; status: string }
interface HistoryItem { id: number; ts: number; model: string; size: string; prompt: string; mode: string; blobs: Blob[] }
interface GeneratedImage { url: string; prompt: string; size: string }

const keys = ref<KeyItem[]>([])
const selectedKeyId = ref<number | null>(null)
const models = ref<string[]>([])
const model = ref('')
const size = ref('1024x1024')
const quantity = ref(1)
const mode = ref<'t2i' | 'i2i'>('t2i')
const refFile = ref<File | null>(null)
const prompt = ref('')
const generating = ref(false)
const errorMessage = ref('')
const results = ref<GeneratedImage[]>([])
const history = ref<HistoryItem[]>([])
const historySearch = ref('')
const batchTs = ref(Date.now())

const sizePresets = [
  { ratio: '1:1', value: '1024x1024' },
  { ratio: '3:2', value: '1536x1024' },
  { ratio: '2:3', value: '1024x1536' },
  { ratio: '16:9', value: '1792x1024' },
  { ratio: '9:16', value: '1024x1792' },
  { ratio: 'auto', value: 'auto' }
]

const selectedKey = computed(() => keys.value.find(k => k.id === selectedKeyId.value) || null)
const gatewayBase = computed(() => appStore.cachedPublicSettings?.api_base_url || window.location.origin)
const filteredHistory = computed(() => {
  const q = historySearch.value.trim().toLowerCase()
  if (!q) return history.value
  return history.value.filter(h => h.prompt.toLowerCase().includes(q) || h.model.toLowerCase().includes(q))
})

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
  } catch {
    history.value = []
  }
}

async function deleteHistory(id: number) {
  await idbDelete(id)
  history.value = history.value.filter(h => h.id !== id)
}

async function clearHistory() {
  await idbClear()
  history.value = []
}

function restoreFromHistory(item: HistoryItem) {
  results.value.forEach(img => URL.revokeObjectURL(img.url))
  results.value = item.blobs.map(blob => ({
    url: URL.createObjectURL(blob),
    prompt: item.prompt,
    size: item.size
  }))
}

// ===== 生成 =====
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

async function generate() {
  errorMessage.value = ''
  if (!selectedKey.value || !prompt.value.trim()) return
  generating.value = true
  const controller = new AbortController()
  const timer = window.setTimeout(() => controller.abort(), 300000)
  try {
    let res: Response
    if (mode.value === 'i2i' && refFile.value) {
      const form = new FormData()
      form.append('model', model.value)
      form.append('prompt', prompt.value.trim())
      if (size.value !== 'auto') form.append('size', size.value)
      form.append('image', refFile.value)
      res = await fetch(`${gatewayBase.value}/v1/images/edits`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${selectedKey.value.key}` },
        body: form,
        signal: controller.signal
      })
    } else {
      res = await fetch(`${gatewayBase.value}/v1/images/generations`, {
        method: 'POST',
        headers: {
          Authorization: `Bearer ${selectedKey.value.key}`,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          model: model.value,
          prompt: prompt.value.trim(),
          size: size.value === 'auto' ? undefined : size.value,
          n: quantity.value
        }),
        signal: controller.signal
      })
    }
    const body = await res.json().catch(() => null)
    if (!res.ok) {
      const msg = body?.error?.message || body?.message || t('imageStudio.generateFailed')
      throw new Error(msg)
    }
    const data: Array<{ b64_json?: string; url?: string }> = body?.data || []
    if (!data.length) throw new Error(t('imageStudio.generateFailed'))
    batchTs.value = Date.now()
    const item: HistoryItem = {
      id: Date.now(),
      ts: Date.now(),
      model: model.value,
      size: size.value,
      prompt: prompt.value.trim(),
      mode: mode.value,
      blobs: []
    }
    const blobs: Blob[] = []
    for (const itemData of data) {
      const blob = itemData.b64_json
        ? blobFromB64(itemData.b64_json)
        : (itemData.url ? await blobFromUrl(itemData.url) : null)
      if (blob) {
        blobs.push(blob)
        results.value.unshift({
          url: URL.createObjectURL(blob),
          prompt: prompt.value.trim(),
          size: size.value
        })
      }
    }
    if (blobs.length) {
      item.blobs = blobs
      await idbPut(item)
      history.value = [item, ...history.value]
    }
  } catch (err: unknown) {
    if (err instanceof DOMException && err.name === 'AbortError') {
      errorMessage.value = t('imageStudio.timeout')
    } else if (err instanceof Error) {
      errorMessage.value = err.message
    } else {
      errorMessage.value = t('imageStudio.generateFailed')
    }
  } finally {
    window.clearTimeout(timer)
    generating.value = false
  }
}

function onRefFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  refFile.value = input.files && input.files[0] ? input.files[0] : null
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
  results.value.forEach(img => URL.revokeObjectURL(img.url))
})
</script>
