/**
 * 视频工作台引擎(模块级单例)。
 *
 * 与 useImageStudioEngine 同一模式:任务队列、历史、草稿常驻 SPA 生命周期,
 * 切换页面不丢进度。视频生成为异步任务:提交(POST generations)拿 task id,
 * 轮询状态(GET tasks/:id),done 后取内容(GET tasks/:id/content)存 Blob。
 */
import { computed, ref } from 'vue'
import { i18n } from '@/i18n'
import { useAppStore } from '@/stores/app'

const t = i18n.global.t

// ===== 常量 =====
export const MODES = [
  { value: 't2v' as const, label: 'videoStudio.modeText2Video' },
  { value: 'i2v' as const, label: 'videoStudio.modeImage2Video' }
]
export const RESOLUTIONS = ['480p', '720p', '1080p'] as const
export const SECONDS = [6, 10, 12, 15]
export const SIZE_PRESETS = [
  { value: 'auto', label: 'auto' },
  { value: '1280x720', label: 'videoStudio.sizeLandscape' },
  { value: '720x1280', label: 'videoStudio.sizePortrait' },
  { value: '1024x1024', label: 'videoStudio.sizeSquare' },
  { value: '1792x1024', label: 'videoStudio.sizeWide' },
  { value: '1024x1792', label: 'videoStudio.sizeTall' }
]
export const QUANTITIES = [1, 2, 3, 4]
export const FALLBACK_VIDEO_MODELS = ['grok-imagine-video-1.5', 'grok-imagine-video']
const VIDEO_MODEL_PATTERN = /(video|imagine|seedance|veo|sora|kling)/i
const POLL_INTERVAL_MS = 5000
const POLL_TIMEOUT_MS = 15 * 60 * 1000

// ===== 类型 =====
export interface VideoTask {
  taskId: number
  remoteId: string
  status: 'pending' | 'processing' | 'done' | 'failed'
  prompt: string
  model: string
  seconds: number
  resolution: string
  submittedAt: number
  finishedAt?: number
  videoUrl?: string
  blob?: Blob
  error?: string
  billingNote?: string
}
export interface VideoHistoryItem {
  id: number
  ts: number
  model: string
  prompt: string
  seconds: number
  resolution: string
  blob: Blob
  sizeMB: number
  starred: boolean
}
export interface RefImage { file: File; url: string }

// ===== 设置状态 =====
export const models = ref<string[]>([...FALLBACK_VIDEO_MODELS])
export const model = ref(FALLBACK_VIDEO_MODELS[0])
export const initError = ref('')
export const mode = ref<'t2v' | 'i2v'>('t2v')
export const resolution = ref<'480p' | '720p' | '1080p'>('720p')
export const videoSize = ref('auto')
export const seconds = ref(6)
export const quantity = ref(1)
export const prompt = ref('')

// ===== 参考图(图生视频首帧,v1 单张) =====
export const refImages = ref<RefImage[]>([])

// ===== 任务与历史 =====
export const tasks = ref<VideoTask[]>([])
export const generating = ref(false)
export const history = ref<VideoHistoryItem[]>([])
export const historySearch = ref('')
export const storageUsed = ref(0)
export const storageQuota = ref(0)
let taskSeq = 0
const objectUrlCache = new Map<number, string>()

function appStore() {
  return useAppStore()
}
function sessionAuthHeader(): { Authorization: string } {
  const token = localStorage.getItem('auth_token') || ''
  return { Authorization: `Bearer ${token}` }
}
function api(path: string) {
  return `${window.location.origin}${path}`
}

export const filteredHistory = computed(() => {
  const kw = historySearch.value.trim().toLowerCase()
  if (!kw) return history.value
  return history.value.filter(it => it.prompt.toLowerCase().includes(kw) || it.model.toLowerCase().includes(kw))
})
export const starredItems = computed(() => history.value.filter(it => it.starred))
export const storagePercent = computed(() => (storageQuota.value > 0 ? Math.min(100, (storageUsed.value / storageQuota.value) * 100) : 0))
export function fmtMB(n: number): string {
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

// ===== 模型列表 =====
export function retryInit() {
  initError.value = ''
  void loadModels()
}
export async function loadModels() {
  initError.value = ''
  try {
    const controller = new AbortController()
    const timer = window.setTimeout(() => controller.abort(), 15000)
    const res = await fetch(api('/api/v1/video-studio/models'), {
      headers: sessionAuthHeader(),
      signal: controller.signal
    })
    window.clearTimeout(timer)
    const body = await res.json().catch(() => null) as { message?: string; error?: { message?: string }; data?: Array<{ id: string }> } | null
    if (!res.ok) {
      throw new Error(body?.message || body?.error?.message || `HTTP ${res.status}`)
    }
    const ids: string[] = (body?.data || []).map(m => m.id)
    const videoModels = ids.filter(id => VIDEO_MODEL_PATTERN.test(id))
    if (videoModels.length) {
      models.value = videoModels
    } else {
      models.value = [...FALLBACK_VIDEO_MODELS]
    }
    if (!models.value.includes(model.value)) model.value = models.value[0] || FALLBACK_VIDEO_MODELS[0]
  } catch (err: unknown) {
    const msg = err instanceof DOMException && err.name === 'AbortError'
      ? t('imageStudio.timeout')
      : (err instanceof Error ? err.message : t('videoStudio.initFailed'))
    initError.value = msg
  }
}

// ===== 参考图(可拖入多张,最多 9 张;提交时取第一张作为视频首帧) =====
const MAX_REF_IMAGES = 9
export function addRefImages(incoming: File[]) {
  const images = incoming.filter(f => f.type.startsWith('image/'))
  if (!images.length) return
  const room = MAX_REF_IMAGES - refImages.value.length
  if (room <= 0) {
    appStore().showError(t('videoStudio.maxRefImages'))
    return
  }
  const accepted = images.slice(0, room)
  for (const f of accepted) {
    refImages.value.push({ file: f, url: URL.createObjectURL(f) })
  }
  mode.value = 'i2v'
  if (images.length > accepted.length) appStore().showError(t('videoStudio.maxRefImages'))
}
export function removeRefImage(index = 0) {
  const item = refImages.value[index]
  if (!item) return
  URL.revokeObjectURL(item.url)
  refImages.value = refImages.value.filter((_, i) => i !== index)
}

// ===== 提交 / 轮询 / 内容 =====
async function fileToDataUrl(file: File): Promise<string> {
  return await new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result))
    reader.onerror = () => reject(reader.error)
    reader.readAsDataURL(file)
  })
}

async function submitTask(promptText: string): Promise<{ remoteId: string }> {
  const payload: Record<string, unknown> = {
    model: model.value,
    prompt: promptText,
    seconds: seconds.value,
    resolution: resolution.value
  }
  if (videoSize.value !== 'auto') payload.resolution = resolution.value
  if (mode.value === 'i2v' && refImages.value.length) {
    payload.image_url = await fileToDataUrl(refImages.value[0].file)
  }
  const controller = new AbortController()
  const timer = window.setTimeout(() => controller.abort(), 120000)
  try {
    const res = await fetch(api('/api/v1/video-studio/generations'), {
      method: 'POST',
      headers: { ...sessionAuthHeader(), 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
      signal: controller.signal
    })
    const body = await res.json().catch(() => null) as { error?: { message?: string }; message?: string; id?: string; request_id?: string; task_id?: string } | null
    if (!res.ok) {
      throw new Error(body?.error?.message || body?.message || t('videoStudio.submitFailed'))
    }
    // 网关/中转的响应 id 字段不统一:xAI 原生为 id,sub2api 链路为 request_id
    const remoteId = body?.id || body?.request_id || body?.task_id
    if (!remoteId) throw new Error(t('videoStudio.submitFailed'))
    return { remoteId }
  } finally {
    window.clearTimeout(timer)
  }
}

interface TaskStatus {
  status?: string
  state?: string
  error?: string | { message?: string }
  video?: { url?: string }
}

function taskPhase(st: TaskStatus): 'pending' | 'processing' | 'done' | 'failed' {
  const s = String(st.status || st.state || '').toLowerCase()
  if (s === 'done' || s === 'succeeded' || s === 'completed' || s === 'success') return 'done'
  if (s === 'failed' || s === 'error' || s === 'expired' || s === 'cancelled') return 'failed'
  if (s === 'processing' || s === 'running' || s === 'generating') return 'processing'
  return 'pending'
}

async function pollTask(remoteId: string): Promise<{ phase: VideoTask['status']; statusRaw: TaskStatus }> {
  const res = await fetch(api(`/api/v1/video-studio/tasks/${encodeURIComponent(remoteId)}`), {
    headers: sessionAuthHeader()
  })
  const body = await res.json().catch(() => null) as (TaskStatus & { error?: { message?: string }; message?: string }) | null
  if (!res.ok) {
    throw new Error(body?.error?.message || body?.message || `HTTP ${res.status}`)
  }
  return { phase: taskPhase(body || {}), statusRaw: body || {} }
}

async function fetchVideoBlob(remoteId: string): Promise<Blob> {
  const controller = new AbortController()
  const timer = window.setTimeout(() => controller.abort(), 300000)
  try {
    const res = await fetch(api(`/api/v1/video-studio/tasks/${encodeURIComponent(remoteId)}/content`), {
      headers: sessionAuthHeader(),
      signal: controller.signal
    })
    if (!res.ok) {
      const body = await res.json().catch(() => null) as { error?: { message?: string }; message?: string } | null
      throw new Error(body?.error?.message || body?.message || t('videoStudio.contentFailed'))
    }
    return await res.blob()
  } finally {
    window.clearTimeout(timer)
  }
}

function errText(e: unknown): string {
  return e instanceof Error ? e.message : t('videoStudio.submitFailed')
}

// runTask:提交 + 轮询 + 取内容 + 入库。独立 async,不阻塞批量循环。
async function runTask(task: VideoTask) {
  try {
    const { remoteId } = await submitTask(task.prompt)
    task.remoteId = remoteId
    tasks.value = [...tasks.value]
    const deadline = Date.now() + POLL_TIMEOUT_MS
    for (;;) {
      if (Date.now() > deadline) throw new Error(t('videoStudio.pollTimeout'))
      await new Promise(r => setTimeout(r, POLL_INTERVAL_MS))
      const { phase, statusRaw } = await pollTask(remoteId)
      if (phase === 'done') {
        const blob = await fetchVideoBlob(remoteId)
        task.blob = blob
        task.videoUrl = URL.createObjectURL(blob)
        task.status = 'done'
        task.finishedAt = Date.now()
        if (statusRaw.video?.url) task.billingNote = ''
        tasks.value = [...tasks.value]
        await saveHistory(task)
        return
      }
      if (phase === 'failed') {
        const raw = statusRaw.error
        task.error = typeof raw === 'string' ? raw : (raw?.message || t('videoStudio.generateFailed'))
        task.status = 'failed'
        tasks.value = [...tasks.value]
        return
      }
      task.status = phase
      tasks.value = [...tasks.value]
    }
  } catch (e) {
    task.status = 'failed'
    task.error = errText(e)
    tasks.value = [...tasks.value]
  }
}

export function cancelPending() {
  // 前端标记取消:仅停止后续轮询提示;已提交的上游任务无法撤销,完成后照常入库
  tasks.value.forEach(tk => {
    if (tk.status === 'pending') {
      tk.status = 'failed'
      tk.error = t('videoStudio.cancelled')
    }
  })
  tasks.value = [...tasks.value]
}

export function submitBatch() {
  if (initError.value || !prompt.value.trim() || generating.value) return
  const promptText = prompt.value.trim()
  const n = quantity.value
  generating.value = true
  const created: VideoTask[] = []
  for (let i = 0; i < n; i++) {
    const task: VideoTask = {
      taskId: ++taskSeq,
      remoteId: '',
      status: 'pending',
      prompt: promptText,
      model: model.value,
      seconds: seconds.value,
      resolution: resolution.value,
      submittedAt: Date.now()
    }
    created.push(task)
  }
  tasks.value = [...created, ...tasks.value]
  // 并发提交:每张独立异步跑,引擎单例保证切页继续
  void Promise.all(created.map(tk => runTask(tk))).finally(() => {
    generating.value = false
    const ok = created.filter(tk => tk.status === 'done').length
    if (ok > 0) appStore().showSuccess(t('videoStudio.batchDone', { ok, total: n }))
  })
}

export function retryTask(taskId: number) {
  const task = tasks.value.find(tk => tk.taskId === taskId)
  if (!task || task.status !== 'failed') return
  task.status = 'pending'
  task.error = undefined
  tasks.value = [...tasks.value]
  void runTask(task)
}

export function removeTask(taskId: number) {
  const task = tasks.value.find(tk => tk.taskId === taskId)
  if (task?.videoUrl) URL.revokeObjectURL(task.videoUrl)
  tasks.value = tasks.value.filter(tk => tk.taskId !== taskId)
}

export function clearFailed() {
  tasks.value = tasks.value.filter(tk => tk.status !== 'failed')
}

// ===== IndexedDB(视频库) =====
function openIdb(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open('video-studio-db', 1)
    req.onupgradeneeded = () => {
      if (!req.result.objectStoreNames.contains('history')) {
        req.result.createObjectStore('history', { keyPath: 'id' })
      }
    }
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
  })
}
async function idbPut(item: VideoHistoryItem) {
  const db = await openIdb()
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction('history', 'readwrite')
    tx.objectStore('history').put(item)
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
  db.close()
}
async function idbAll(): Promise<VideoHistoryItem[]> {
  const db = await openIdb()
  const items = await new Promise<VideoHistoryItem[]>((resolve, reject) => {
    const req = db.transaction('history', 'readonly').objectStore('history').getAll()
    req.onsuccess = () => resolve(req.result as VideoHistoryItem[])
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

async function saveHistory(task: VideoTask) {
  if (!task.blob) return
  const item: VideoHistoryItem = {
    id: Date.now(),
    ts: Date.now(),
    model: task.model,
    prompt: task.prompt,
    seconds: task.seconds,
    resolution: task.resolution,
    blob: task.blob,
    sizeMB: Number((task.blob.size / 1024 / 1024).toFixed(1)),
    starred: false
  }
  await idbPut(item).catch(() => undefined)
  history.value = [item, ...history.value]
  updateStorageMeter()
}

export async function loadHistory() {
  try {
    const items = await idbAll()
    history.value = items
      .filter(it => it && it.blob instanceof Blob)
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

export function historyVideoUrl(item: VideoHistoryItem): string {
  const cached = objectUrlCache.get(item.id)
  if (cached) return cached
  const url = URL.createObjectURL(item.blob)
  objectUrlCache.set(item.id, url)
  return url
}

export async function toggleHistoryStar(item: VideoHistoryItem) {
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
  if (!window.confirm(t('videoStudio.clearAllConfirm'))) return
  objectUrlCache.forEach(url => URL.revokeObjectURL(url))
  objectUrlCache.clear()
  history.value = []
  await idbClear().catch(() => undefined)
  updateStorageMeter()
}
