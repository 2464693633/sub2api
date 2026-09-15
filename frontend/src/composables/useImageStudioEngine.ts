/**
 * 生图工作台引擎(模块级单例)。
 *
 * SPA 路由切换会销毁页面组件,但模块实例常驻 —— 把批量生成任务、
 * 历史记录、提示词草稿等状态全部放在这里,用户切走再回来时进度原样恢复,
 * 进行中的生成在后台照常完成并自动入库。页面组件只是本引擎的"显示器"。
 */
import { computed, ref } from 'vue'
import JSZip from 'jszip'
import { i18n } from '@/i18n'
import { useAppStore } from '@/stores/app'

const t = i18n.global.t

// ===== 常量 =====
export const MODES = [
  { value: 't2i' as const, label: 'imageStudio.modeText2Image' },
  { value: 'i2i' as const, label: 'imageStudio.modeImage2Image' }
]
export const QUALITIES = ['auto', 'high', 'medium', 'low'] as const
export const FORMATS = ['png', 'jpeg', 'webp'] as const
export const CLARITIES = [
  { value: '1k' as const, label: '1K' },
  { value: '2k' as const, label: '2K' },
  { value: '4k' as const, label: '4K' }
]
export const RATIOS = [
  { value: '1:1', w: 1, h: 1 },
  { value: '16:9', w: 16, h: 9 },
  { value: '9:16', w: 9, h: 16 },
  { value: '4:3', w: 4, h: 3 },
  { value: '3:4', w: 3, h: 4 },
  { value: '3:2', w: 3, h: 2 },
  { value: '2:3', w: 2, h: 3 },
  { value: '21:9', w: 21, h: 9 }
]
export const QUANTITIES = [1, 4, 8, 16, 32, 50]
const MAX_REFS = 8
const MAX_PROMPT_BOXES = 8
export const IMAGE_MODEL_PATTERN = /(image|dall|flux|seedream|banana|diffusion)/i
export const FALLBACK_MODELS = ['gpt-image-2.5', 'gpt-image-2', 'gpt-image-1', 'dall-e-3']
const CLARITY_BASE: Record<string, number> = { '1k': 1024, '2k': 2048, '4k': 4096 }

// ===== 类型 =====
export interface PromptBox { id: number; text: string; status: 'draft' | 'done' }
export interface BatchSlot {
  slotId: number
  status: 'running' | 'done' | 'failed'
  prompt: string
  size: string
  actualSize?: string
  blob?: Blob
  url?: string
  error?: string
  ms?: number
}
export interface HistoryImage { blob: Blob; failed: boolean }
export interface HistoryItem {
  id: number
  ts: number
  model: string
  quality: string
  format: string
  size: string
  actualSize?: string
  prompt: string
  mode: 't2i' | 'i2i'
  images: HistoryImage[]
  starred: boolean
}
export interface RefItem { file: File; url: string }

// ===== 设置状态 =====
export const models = ref<string[]>([...FALLBACK_MODELS])
export const model = ref(FALLBACK_MODELS[0])
export const initError = ref('')
export const mode = ref<'t2i' | 'i2i'>('t2i')
export const quality = ref<'auto' | 'high' | 'medium' | 'low'>('auto')
export const outputFormat = ref<'png' | 'jpeg' | 'webp'>('png')
export const clarity = ref<'1k' | '2k' | '4k'>('1k')
export const size = ref<string>('auto')
export const quantity = ref(1)

// ===== 提示词框 =====
export const promptBoxes = ref<PromptBox[]>([{ id: 1, text: '', status: 'draft' }])
export const activeBoxId = ref(1)
let boxSeq = 1
export const activeBox = computed(() => promptBoxes.value.find(b => b.id === activeBoxId.value))
export const prompt = computed({
  get: () => activeBox.value?.text ?? '',
  set: (v: string) => { if (activeBox.value) activeBox.value.text = v }
})

// ===== 参考图 =====
export const refItems = ref<RefItem[]>([])

// ===== 批量生成 =====
export const batch = ref<BatchSlot[]>([])
export const generating = ref(false)
export const batchCancelRequested = ref(false)
export const elapsed = ref(0)
let slotSeq = 0
let elapsedTimer: number | null = null

// ===== 历史 =====
export const history = ref<HistoryItem[]>([])
export const historySearch = ref('')
const objectUrlCache = new Map<number, string>()
export const storageUsed = ref(0)
export const storageQuota = ref(0)

function appStore() {
  return useAppStore()
}

// 每次调用现读 localStorage:token 刷新后旧值不能被缓存(computed 会永久缓存导致 401)
function sessionAuthHeader(): { Authorization: string } {
  const token = localStorage.getItem('auth_token') || ''
  return { Authorization: `Bearer ${token}` }
}

function gatewayBase(): string {
  return window.location.origin
}

export const computedSize = computed(() => {
  if (size.value === 'auto') {
    // 画幅 auto 时清晰度也要生效:2K/4K 请求方形大图(1K 保持 auto 由上游默认)
    if (clarity.value !== '1k') {
      const base = CLARITY_BASE[clarity.value] || 1024
      return `${base}x${base}`
    }
    return 'auto'
  }
  const [wStr, hStr] = size.value.split(':')
  const w = Number(wStr)
  const h = Number(hStr)
  if (!w || !h) return 'auto'
  const base = CLARITY_BASE[clarity.value] || 1024
  const ratio = w / h
  let width = base
  let height = base
  if (ratio >= 1) height = Math.round(base / ratio)
  else width = Math.round(base * ratio)
  return `${width}x${height}`
})

export const filteredHistory = computed(() => {
  const kw = historySearch.value.trim().toLowerCase()
  if (!kw) return history.value
  return history.value.filter(it => it.prompt.toLowerCase().includes(kw) || it.model.toLowerCase().includes(kw))
})
export const starredItems = computed(() => history.value.filter(it => it.starred))
export const storagePercent = computed(() => (storageQuota.value > 0 ? Math.min(100, (storageUsed.value / storageQuota.value) * 100) : 0))

function startElapsed() {
  elapsed.value = 0
  stopElapsed()
  elapsedTimer = window.setInterval(() => { elapsed.value++ }, 1000)
}
function stopElapsed() {
  if (elapsedTimer !== null) {
    window.clearInterval(elapsedTimer)
    elapsedTimer = null
  }
}

// ===== 提示词框 =====
export function addPromptBox() {
  if (promptBoxes.value.length >= MAX_PROMPT_BOXES) return
  const id = ++boxSeq
  promptBoxes.value = [...promptBoxes.value, { id, text: '', status: 'draft' }]
  activeBoxId.value = id
}
export function removePromptBox(id: number) {
  if (promptBoxes.value.length <= 1) return
  const idx = promptBoxes.value.findIndex(b => b.id === id)
  promptBoxes.value = promptBoxes.value.filter(b => b.id !== id)
  if (activeBoxId.value === id) {
    const next = promptBoxes.value[Math.max(0, idx - 1)]
    activeBoxId.value = next ? next.id : promptBoxes.value[0]?.id ?? 1
  }
}

// ===== 参考图 =====
export function addRefFiles(incoming: File[]) {
  const images = incoming.filter(f => f.type.startsWith('image/'))
  if (!images.length) return
  const room = MAX_REFS - refItems.value.length
  if (room <= 0) {
    appStore().showError(t('imageStudio.maxRefs'))
    return
  }
  const accepted = images.slice(0, room)
  for (const f of accepted) {
    refItems.value.push({ file: f, url: URL.createObjectURL(f) })
  }
  mode.value = 'i2i'
  if (images.length > accepted.length) appStore().showError(t('imageStudio.maxRefs'))
}
export function removeRefItem(idx: number) {
  const item = refItems.value[idx]
  if (!item) return
  URL.revokeObjectURL(item.url)
  refItems.value = refItems.value.filter((_, i) => i !== idx)
}

// ===== 模型(会话直通,无需 API Key) =====
export function retryInit() {
  initError.value = ''
  void loadModels()
}
export async function loadModels() {
  initError.value = ''
  try {
    const controller = new AbortController()
    const timer = window.setTimeout(() => controller.abort(), 15000)
    const res = await fetch(`${gatewayBase()}/api/v1/image-studio/models`, {
      headers: sessionAuthHeader(),
      signal: controller.signal
    })
    window.clearTimeout(timer)
    const body = await res.json().catch(() => null) as { message?: string; error?: { message?: string }; data?: Array<{ id: string }> } | null
    if (!res.ok) {
      throw new Error(body?.message || body?.error?.message || `HTTP ${res.status}`)
    }
    const ids: string[] = (body?.data || []).map((m) => m.id)
    const imageModels = ids.filter(id => IMAGE_MODEL_PATTERN.test(id))
    if (!imageModels.length) {
      initError.value = t('imageStudio.noImageModels')
      return
    }
    models.value = imageModels
    if (!models.value.includes(model.value)) model.value = models.value[0] || FALLBACK_MODELS[0]
  } catch (err: unknown) {
    const msg = err instanceof DOMException && err.name === 'AbortError'
      ? t('imageStudio.timeout')
      : (err instanceof Error ? err.message : t('imageStudio.noKeys'))
    initError.value = msg
    models.value = FALLBACK_MODELS
    model.value = FALLBACK_MODELS[0]
  }
}

// ===== IndexedDB =====
function openIdb(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open('image-studio-db', 1)
    req.onupgradeneeded = () => {
      if (!req.result.objectStoreNames.contains('history')) {
        req.result.createObjectStore('history', { keyPath: 'id' })
      }
    }
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
  })
}
async function idbPut(item: HistoryItem) {
  const db = await openIdb()
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction('history', 'readwrite')
    tx.objectStore('history').put(item)
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
  db.close()
}
async function idbPutMany(items: HistoryItem[]) {
  const db = await openIdb()
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction('history', 'readwrite')
    const store = tx.objectStore('history')
    for (const it of items) store.put(it)
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
  db.close()
}
async function idbAll(): Promise<HistoryItem[]> {
  const db = await openIdb()
  const items = await new Promise<HistoryItem[]>((resolve, reject) => {
    const req = db.transaction('history', 'readonly').objectStore('history').getAll()
    req.onsuccess = () => resolve(req.result as HistoryItem[])
    req.onerror = () => reject(req.error)
  })
  db.close()
  return items
}
async function idbDelete(id: number) {
  const db = await openIdb()
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction('history', 'readwrite')
    tx.objectStore('history').delete(id)
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
  db.close()
}
async function idbClear() {
  const db = await openIdb()
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction('history', 'readwrite')
    tx.objectStore('history').clear()
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
  db.close()
}

export async function loadHistory() {
  try {
    const items = await idbAll()
    // 过滤早期版本写入的不完整记录(缺少 images 数组会导致渲染崩溃)
    history.value = items
      .filter(it => it && Array.isArray(it.images))
      .sort((a, b) => b.ts - a.ts)
  } catch {
    history.value = []
  }
}

export function updateStorageMeter() {
  if (navigator.storage?.estimate) {
    void navigator.storage.estimate().then(est => {
      storageUsed.value = est.usage ?? 0
      storageQuota.value = est.quota ?? 0
    })
  }
}
export function fmtMB(n: number): string {
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

export function objectUrlFor(item: HistoryItem): string {
  if (!item || !Array.isArray(item.images)) return ''
  const cached = objectUrlCache.get(item.id)
  if (cached) return cached
  const first = item.images.find(img => img && !img.failed)
  if (!first) return ''
  const url = URL.createObjectURL(first.blob)
  objectUrlCache.set(item.id, url)
  return url
}

export async function toggleHistoryStar(item: HistoryItem) {
  item.starred = !item.starred
  await idbPut(item).catch(() => undefined)
}
export async function deleteHistory(id: number) {
  const cached = objectUrlCache.get(id)
  if (cached) {
    URL.revokeObjectURL(cached)
    objectUrlCache.delete(id)
  }
  history.value = history.value.filter(it => it.id !== id)
  await idbDelete(id).catch(() => undefined)
  updateStorageMeter()
}
export async function clearAllHistory() {
  if (!window.confirm(t('imageStudio.clearAllConfirm'))) return
  objectUrlCache.forEach(url => URL.revokeObjectURL(url))
  objectUrlCache.clear()
  history.value = []
  await idbClear().catch(() => undefined)
  updateStorageMeter()
}
export function restoreHistory(item: HistoryItem) {
  batch.value.forEach(s => { if (s.url) URL.revokeObjectURL(s.url) })
  batch.value = item.images.map(img => ({
    slotId: ++slotSeq,
    status: (img.failed ? 'failed' : 'done') as BatchSlot['status'],
    prompt: item.prompt,
    size: item.size,
    actualSize: img.failed ? undefined : item.actualSize,
    blob: img.failed ? undefined : img.blob,
    url: img.failed ? undefined : URL.createObjectURL(img.blob),
    error: img.failed ? t('imageStudio.generateFailed') : undefined
  }))
  if (activeBox.value) activeBox.value.text = item.prompt
  if (models.value.includes(item.model)) model.value = item.model
  appStore().showSuccess(t('imageStudio.restored'))
}

// ===== ZIP 导出 / 导入 =====
export async function exportZip() {
  if (!history.value.length) return
  const zip = new JSZip()
  const metas: Record<string, unknown>[] = []
  for (const item of history.value) {
    const folder = zip.folder(String(item.ts))
    if (!folder) continue
    for (let i = 0; i < item.images.length; i++) {
      const img = item.images[i]
      const ext = img.failed ? 'txt' : (item.format || 'png')
      if (img.failed) {
        folder.file(`image-${i + 1}-failed.txt`, t('imageStudio.generateFailed'))
      } else {
        folder.file(`image-${i + 1}.${ext}`, img.blob)
      }
    }
    metas.push({
      ts: item.ts, model: item.model, quality: item.quality, format: item.format,
      size: item.size, actualSize: item.actualSize, prompt: item.prompt, mode: item.mode, starred: item.starred
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
export async function onImportZipChange(event: Event) {
  const input = event.target as HTMLInputElement
  if (!input.files || !input.files[0]) return
  try {
    const zip = await JSZip.loadAsync(input.files[0])
    let metas: Record<string, unknown>[] = []
    const metaFile = zip.file('meta.json')
    if (metaFile) {
      try { metas = JSON.parse(await metaFile.async('string')) as Record<string, unknown>[] } catch { metas = [] }
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
        if (path.endsWith('.txt')) {
          images.push({ blob: new Blob([await zip.files[path].async('string')], { type: 'text/plain' }), failed: true })
          continue
        }
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
        actualSize: meta.actualSize ? String(meta.actualSize) : undefined,
        prompt: String(meta.prompt || ''),
        mode: (meta.mode as 't2i' | 'i2i') || 't2i',
        images,
        starred: Boolean(meta.starred)
      })
    }
    if (items.length) {
      await idbPutMany(items)
      await loadHistory()
      appStore().showSuccess(t('imageStudio.importDone', { n: items.length }))
      updateStorageMeter()
    }
  } catch {
    appStore().showError(t('imageStudio.importFailed'))
  }
  input.value = ''
}

// ===== 生成 =====
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

// 解码图片 Blob 的真实像素尺寸(请求的清晰度只是参数,实际以输出为准)
async function imageSizeOf(blob: Blob): Promise<string> {
  try {
    const bmp = await createImageBitmap(blob)
    const s = `${bmp.width}x${bmp.height}`
    bmp.close()
    return s
  } catch {
    return ''
  }
}

async function generateOne(spec: BatchSpec, promptText: string, refList: File[]): Promise<{ blob: Blob | null; error?: string }> {
  const controller = new AbortController()
  const timer = window.setTimeout(() => controller.abort(), 300000)
  try {
    let res: Response
    if (spec.mode === 'i2i' && refList.length) {
      const form = new FormData()
      form.append('model', spec.model)
      form.append('prompt', promptText)
      if (spec.size !== 'auto') form.append('size', spec.size)
      if (spec.quality !== 'auto') form.append('quality', spec.quality)
      form.append('output_format', spec.format)
      for (const f of refList) form.append('image', f)
      res = await fetch(`${gatewayBase()}/api/v1/image-studio/edits`, {
        method: 'POST',
        headers: sessionAuthHeader(),
        body: form,
        signal: controller.signal
      })
    } else {
      res = await fetch(`${gatewayBase()}/api/v1/image-studio/generations`, {
        method: 'POST',
        headers: {
          ...sessionAuthHeader(),
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          model: spec.model,
          prompt: promptText,
          size: spec.size === 'auto' ? undefined : spec.size,
          quality: spec.quality === 'auto' ? undefined : spec.quality,
          output_format: spec.format,
          n: 1
        }),
        signal: controller.signal
      })
    }
    const body = await res.json().catch(() => null) as { error?: { message?: string }; message?: string; data?: Array<{ b64_json?: string; url?: string }> } | null
    if (!res.ok) {
      const msg = body?.error?.message || body?.message || t('imageStudio.generateFailed')
      return { blob: null, error: msg }
    }
    const data = body?.data || []
    if (!data.length) return { blob: null, error: t('imageStudio.generateFailed') }
    const first = data[0]
    const blob = first?.b64_json
      ? blobFromB64(first.b64_json)
      : (first?.url ? await blobFromUrl(first.url) : null)
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

// 生成参数快照:批量开始时定格,中途改设置不影响进行中的批次
export interface BatchSpec {
  model: string
  mode: 't2i' | 'i2i'
  quality: 'auto' | 'high' | 'medium' | 'low'
  format: 'png' | 'jpeg' | 'webp'
  size: string
}
let lastSpec: BatchSpec | null = null

export function cancelBatch() {
  if (generating.value) batchCancelRequested.value = true
}

export function clearFailed() {
  batch.value = batch.value.filter(s => s.status !== 'failed')
}
export function removeSlot(slotId: number) {
  const slot = batch.value.find(s => s.slotId === slotId)
  if (slot?.url) URL.revokeObjectURL(slot.url)
  batch.value = batch.value.filter(s => s.slotId !== slotId)
}
export function starSlot(slotId: number, blob?: Blob) {
  const slot = batch.value.find(s => s.slotId === slotId)
  const b = blob || slot?.blob
  if (!b) return
  const item: HistoryItem = {
    id: Date.now(),
    ts: Date.now(),
    model: model.value,
    quality: quality.value,
    format: outputFormat.value,
    size: slot?.size || computedSize.value,
    actualSize: slot?.actualSize,
    prompt: slot?.prompt || '',
    mode: mode.value,
    images: [{ blob: b, failed: false }],
    starred: true
  }
  history.value = [item, ...history.value]
  void idbPut(item)
  updateStorageMeter()
  appStore().showSuccess(t('imageStudio.starredSaved'))
}
export async function copySlotImage(slotId: number) {
  const slot = batch.value.find(s => s.slotId === slotId)
  if (!slot?.blob) return
  try {
    const type = slot.blob.type || 'image/png'
    await navigator.clipboard.write([new ClipboardItem({ [type]: slot.blob })])
    appStore().showSuccess(t('imageStudio.copied'))
  } catch {
    appStore().showError(t('imageStudio.copyFailed'))
  }
}
export function setRefFromSlot(slotId: number) {
  const slot = batch.value.find(s => s.slotId === slotId)
  if (!slot?.blob) return
  const file = new File([slot.blob], `ref-${slotId}.${outputFormat.value}`, { type: slot.blob.type || 'image/png' })
  addRefFiles([file])
  appStore().showSuccess(t('imageStudio.refAdded'))
}
export async function retrySlot(slotId: number) {
  const slot = batch.value.find(s => s.slotId === slotId)
  if (!slot || generating.value || !lastSpec) return
  slot.status = 'running'
  const refs = lastSpec.mode === 'i2i' ? refItems.value.map(r => r.file) : []
  const started = performance.now()
  const result = await generateOne(lastSpec, slot.prompt, refs)
  slot.ms = Math.round(performance.now() - started)
  slot.status = result.blob ? 'done' : 'failed'
  slot.error = result.error
  if (result.blob) {
    slot.blob = result.blob
    slot.url = URL.createObjectURL(result.blob)
    slot.actualSize = await imageSizeOf(result.blob)
  }
  batch.value = [...batch.value]
}

export async function generateBatch() {
  const box = activeBox.value
  if (!box || initError.value || !box.text.trim() || generating.value) return
  const spec: BatchSpec = {
    model: model.value,
    mode: mode.value,
    quality: quality.value,
    format: outputFormat.value,
    size: computedSize.value
  }
  lastSpec = spec
  const n = quantity.value
  const promptText = box.text.trim()
  const refs = spec.mode === 'i2i' ? refItems.value.map(r => r.file) : []
  batch.value.forEach(s => { if (s.url) URL.revokeObjectURL(s.url) })
  batch.value = []
  generating.value = true
  batchCancelRequested.value = false
  startElapsed()
  let okCount = 0
  try {
    for (let i = 0; i < n; i++) {
      // 用户点击停止后不再发起新请求,已完成的图照常入库
      if (batchCancelRequested.value) break
      const slotId = ++slotSeq
      const slot: BatchSlot = { slotId, status: 'running', prompt: promptText, size: spec.size }
      batch.value = [...batch.value, slot]
      const started = performance.now()
      const result = await generateOne(spec, promptText, refs)
      slot.ms = Math.round(performance.now() - started)
      if (result.blob) {
        slot.blob = result.blob
        slot.url = URL.createObjectURL(result.blob)
        slot.actualSize = await imageSizeOf(result.blob)
        slot.status = 'done'
        okCount++
      } else {
        slot.status = 'failed'
        slot.error = result.error
      }
      batch.value = [...batch.value]
    }
    if (batchCancelRequested.value) {
      appStore().showSuccess(t('imageStudio.batchCancelled', { ok: okCount }))
    } else if (okCount > 0) {
      box.status = 'done'
      appStore().showSuccess(t('imageStudio.batchDone', { ok: okCount, total: n }))
    }
  } finally {
    generating.value = false
    stopElapsed()
    const done = batch.value.filter(s => s.status === 'done' && s.blob)
    if (done.length) {
      const item: HistoryItem = {
        id: Date.now(),
        ts: Date.now(),
        model: spec.model,
        quality: spec.quality,
        format: spec.format,
        size: spec.size,
        actualSize: done[0]?.actualSize,
        prompt: promptText,
        mode: spec.mode,
        images: done.map(s => ({ blob: s.blob as Blob, failed: false })),
        starred: false
      }
      await idbPut(item).catch(() => undefined)
      history.value = [item, ...history.value]
      updateStorageMeter()
    }
  }
}
