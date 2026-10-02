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
import { sessionOwnerKey, StudioSessionChangedError } from './studioSession'
import { isGrokImageModel } from './studioModels'
import { openStudioHistoryDb } from './studioHistory'

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
const POLL_TIMEOUT_MS = 8 * 60 * 1000
// 无进展早停:状态与进度都无变化超过 3 分钟,判定上游卡死
const NO_PROGRESS_TIMEOUT_MS = 3 * 60 * 1000
// 连续轮询错误容忍次数(网络抖动不立即判死)
const MAX_POLL_ERRORS = 3

// ===== 类型 =====
// 模型选项带所属分组:工作台密钥按分组各一把(多个 grok/composite 视频分组同台),
// 生成时经 X-Video-Studio-Group 头路由到对应分组。
export interface VideoModelOption { id: string; group_id: number; group_name: string }

function toOptions(ids: string[]): VideoModelOption[] {
  return ids.map(id => ({ id, group_id: 0, group_name: '' }))
}

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
  groupId?: number
  upstreamDone?: boolean
  upstreamFailed?: boolean
  videoSize?: string
  mode?: 't2v' | 'i2v'
  refImage?: File
  owner?: string
  sessionEpoch?: number
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
export const models = ref<VideoModelOption[]>(toOptions(FALLBACK_VIDEO_MODELS))
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
let sessionEpoch = 0
const objectUrlCache = new Map<number, string>()

function appStore() {
  return useAppStore()
}
interface StudioSession { owner?: string; sessionEpoch?: number }

function currentSession(): StudioSession {
  return { owner: sessionOwnerKey(), sessionEpoch }
}

function isCurrentSession(session: StudioSession): boolean {
  return session.sessionEpoch === sessionEpoch && session.owner === sessionOwnerKey()
}

function sessionAuthHeader(session: StudioSession): { Authorization: string } {
  if (!isCurrentSession(session)) throw new StudioSessionChangedError()
  return { Authorization: `Bearer ${localStorage.getItem('auth_token') || ''}` }
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
  const session = currentSession()
  initError.value = ''
  try {
    const controller = new AbortController()
    const timer = window.setTimeout(() => controller.abort(), 15000)
    const res = await fetch(api('/api/v1/video-studio/models'), {
      headers: sessionAuthHeader(session),
      signal: controller.signal
    })
    window.clearTimeout(timer)
    const body = await res.json().catch(() => null) as { message?: string; error?: { message?: string }; data?: VideoModelOption[] } | null
    if (!isCurrentSession(session)) return
    if (!res.ok) {
      throw new Error(body?.message || body?.error?.message || `HTTP ${res.status}`)
    }
    const options = (body?.data || []).filter(o => o && o.id && VIDEO_MODEL_PATTERN.test(o.id) && !/image/i.test(o.id) && !isGrokImageModel(o.id))
    models.value = options.length ? options : toOptions(FALLBACK_VIDEO_MODELS)
    if (!models.value.some(o => o.id === model.value)) model.value = models.value[0]?.id || FALLBACK_VIDEO_MODELS[0]
  } catch (err: unknown) {
    if (!isCurrentSession(session)) return
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

async function submitTask(task: VideoTask): Promise<{ remoteId: string }> {
  const payload: Record<string, unknown> = {
    model: task.model,
    prompt: task.prompt,
    seconds: task.seconds,
    resolution: task.resolution
  }
  if (task.videoSize && task.videoSize !== 'auto') payload.size = task.videoSize
  if (task.mode === 'i2v' && task.refImage) {
    payload.image_url = await fileToDataUrl(task.refImage)
  }
  // 按模型所属分组路由:工作台密钥每分组一把,X 头让 relay 选对应密钥/分组
  const headers: Record<string, string> = { ...sessionAuthHeader(task), 'Content-Type': 'application/json' }
  if (task.groupId && task.groupId > 0) headers['X-Video-Studio-Group'] = String(task.groupId)
  const controller = new AbortController()
  const timer = window.setTimeout(() => controller.abort(), 120000)
  try {
    const res = await fetch(api('/api/v1/video-studio/generations'), {
      method: 'POST',
      headers,
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
  progress?: number
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

function taskGroupHeaders(task: VideoTask): Record<string, string> {
  // 多视频分组:状态/内容查询必须回到创建时的分组密钥,否则归属绑定解析失败
  return task.groupId && task.groupId > 0 ? { 'X-Video-Studio-Group': String(task.groupId) } : {}
}

async function pollTask(task: VideoTask): Promise<{ phase: VideoTask['status']; statusRaw: TaskStatus }> {
  const res = await fetch(api(`/api/v1/video-studio/tasks/${encodeURIComponent(task.remoteId)}`), {
    headers: { ...sessionAuthHeader(task), ...taskGroupHeaders(task) }
  })
  const body = await res.json().catch(() => null) as (TaskStatus & { error?: { message?: string }; message?: string }) | null
  if (!res.ok) {
    throw new Error(body?.error?.message || body?.message || `HTTP ${res.status}`)
  }
  return { phase: taskPhase(body || {}), statusRaw: body || {} }
}

async function fetchVideoBlob(task: VideoTask): Promise<Blob> {
  // 大视频(15s/1080p 可达数十 MB)经网关转发较慢:单次 10 分钟,失败自动重试 3 次
  let lastErr: unknown = new Error(t('videoStudio.contentFailed'))
  for (let attempt = 0; attempt < 3; attempt++) {
    if (!isCurrentSession(task)) throw new StudioSessionChangedError()
    if (attempt > 0) await new Promise(r => setTimeout(r, attempt * 10000))
    if (!isCurrentSession(task)) throw new StudioSessionChangedError()
    const controller = new AbortController()
    const timer = window.setTimeout(() => controller.abort(), 600000)
    try {
      const res = await fetch(api(`/api/v1/video-studio/tasks/${encodeURIComponent(task.remoteId)}/content`), {
        headers: { ...sessionAuthHeader(task), ...taskGroupHeaders(task) },
        signal: controller.signal
      })
      if (!res.ok) {
        const body = await res.json().catch(() => null) as { error?: { message?: string }; message?: string } | null
        throw new Error(body?.error?.message || body?.message || t('videoStudio.contentFailed'))
      }
      return await res.blob()
    } catch (err) {
      if (!isCurrentSession(task)) throw new StudioSessionChangedError()
      lastErr = err instanceof DOMException && err.name === 'AbortError'
        ? new Error(t('videoStudio.downloadTimeout'))
        : err
    } finally {
      window.clearTimeout(timer)
    }
  }
  throw lastErr
}

function errText(e: unknown): string {
  return e instanceof Error ? e.message : t('videoStudio.submitFailed')
}

// 手动停止等待的任务:runTask 轮询中检测到即退出
const stoppedTasks = new Set<number>()

// runTask:提交 + 轮询 + 取内容 + 入库。独立 async,不阻塞批量循环。
async function runTask(task: VideoTask) {
  let lastSignature = ''
  let lastProgressAt = Date.now()
  let pollErrors = 0
  try {
    let pollImmediately = Boolean(task.remoteId)
    if (!task.remoteId) {
      const { remoteId } = await submitTask(task)
      if (!isCurrentSession(task)) return
      task.remoteId = remoteId
      tasks.value = [...tasks.value]
    }
    const deadline = Date.now() + POLL_TIMEOUT_MS
    for (;;) {
      if (!isCurrentSession(task)) return
      if (stoppedTasks.has(task.taskId)) throw new Error(t('videoStudio.stoppedWaiting'))
      if (Date.now() > deadline) throw new Error(t('videoStudio.pollTimeout'))
      if (Date.now() - lastProgressAt > NO_PROGRESS_TIMEOUT_MS) throw new Error(t('videoStudio.noProgress'))
      if (!pollImmediately) await new Promise(r => setTimeout(r, POLL_INTERVAL_MS))
      pollImmediately = false
      if (!isCurrentSession(task)) return
      if (stoppedTasks.has(task.taskId)) throw new Error(t('videoStudio.stoppedWaiting'))
      let phase: VideoTask['status']
      let statusRaw: TaskStatus
      try {
        const polled = await pollTask(task)
        if (!isCurrentSession(task)) return
        if (stoppedTasks.has(task.taskId)) throw new Error(t('videoStudio.stoppedWaiting'))
        phase = polled.phase
        statusRaw = polled.statusRaw
        pollErrors = 0
      } catch (pollErr) {
        if (!isCurrentSession(task)) return
        // 网络抖动容错:连续多次失败才判死
        pollErrors++
        if (pollErrors >= MAX_POLL_ERRORS) throw pollErr
        continue
      }
      const signature = `${phase}:${String(statusRaw.progress ?? '')}:${String(statusRaw.status || '')}`
      if (signature !== lastSignature) {
        lastSignature = signature
        lastProgressAt = Date.now()
      }
      if (phase === 'done') {
        // 视频已生成并计费:下载失败只影响取片,不把上游任务整个判死
        task.upstreamDone = true
        tasks.value = [...tasks.value]
        await finishDownload(task)
        return
      }
      if (phase === 'failed') {
        task.upstreamFailed = true
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
    if (!isCurrentSession(task)) return
    task.status = 'failed'
    task.error = errText(e)
    tasks.value = [...tasks.value]
  } finally {
    stoppedTasks.delete(task.taskId)
  }
}

/** 手动停止等待:轮询循环在 5 秒内退出并把任务标记为失败 */
export function stopWaiting(taskId: number) {
  const task = tasks.value.find(tk => tk.taskId === taskId)
  if (!task || (task.status !== 'pending' && task.status !== 'processing')) return
  stoppedTasks.add(taskId)
  pruneStoppedTasks()
}

/** 修剪 stoppedTasks 中已不在任务列表里的 id,防长期驻留泄漏 */
function pruneStoppedTasks() {
  if (stoppedTasks.size < 64) return
  const alive = new Set(tasks.value.map(tk => tk.taskId))
  for (const id of stoppedTasks) {
    if (!alive.has(id)) stoppedTasks.delete(id)
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
  const selectedOption = models.value.find(o => o.id === model.value)
  const owner = sessionOwnerKey()
  const batchEpoch = sessionEpoch
  const refImage = mode.value === 'i2v' ? refImages.value[0]?.file : undefined
  for (let i = 0; i < n; i++) {
    const task: VideoTask = {
      taskId: ++taskSeq,
      remoteId: '',
      status: 'pending',
      prompt: promptText,
      model: model.value,
      seconds: seconds.value,
      resolution: resolution.value,
      submittedAt: Date.now(),
      groupId: selectedOption && selectedOption.group_id > 0 ? selectedOption.group_id : undefined,
      videoSize: videoSize.value,
      mode: mode.value,
      refImage,
      owner,
      sessionEpoch: batchEpoch
    }
    created.push(task)
  }
  tasks.value = [...created, ...tasks.value]
  // 并发提交:每张独立异步跑,引擎单例保证切页继续
  void Promise.all(created.map(tk => runTask(tk))).finally(() => {
    if (!isCurrentSession({ owner, sessionEpoch: batchEpoch })) return
    generating.value = false
    const ok = created.filter(tk => tk.status === 'done').length
    if (ok > 0) appStore().showSuccess(t('videoStudio.batchDone', { ok, total: n }))
  })
}

/** 上游已完成任务的下载+入库;下载失败仅标记失败,不影响已计费的生成结果 */
async function finishDownload(task: VideoTask) {
  try {
    const blob = await fetchVideoBlob(task)
    if (!isCurrentSession(task)) return
    task.blob = blob
    task.videoUrl = URL.createObjectURL(blob)
    task.status = 'done'
    task.finishedAt = Date.now()
    tasks.value = [...tasks.value]
    await saveHistory(task)
  } catch (e) {
    if (!isCurrentSession(task)) return
    task.status = 'failed'
    task.error = errText(e)
    tasks.value = [...tasks.value]
  }
}

export function retryTask(taskId: number) {
  const task = tasks.value.find(tk => tk.taskId === taskId)
  if (!task || task.status !== 'failed') return
  // Tasks created by older builds do not have session snapshot fields.
  task.sessionEpoch ??= sessionEpoch
  task.owner ||= sessionOwnerKey()
  if (!isCurrentSession(task)) return
  task.videoSize ||= 'auto'
  task.mode ||= 't2v'
  // Only a confirmed upstream terminal failure requires a new generation.
  // Polling/network failures retain the remote task to avoid duplicate billing.
  if (task.upstreamFailed) {
    task.remoteId = ''
    task.upstreamDone = false
    task.upstreamFailed = false
  }
  // 上游已生成(已计费)的任务:重试仅重新下载,不会重复提交计费
  if (task.upstreamDone && task.remoteId) {
    task.status = 'processing'
    task.error = undefined
    tasks.value = [...tasks.value]
    void finishDownload(task)
    return
  }
  task.status = 'pending'
  task.error = undefined
  tasks.value = [...tasks.value]
  void runTask(task)
}

export function removeTask(taskId: number) {
  const task = tasks.value.find(tk => tk.taskId !== undefined && tk.taskId === taskId)
  if (task?.videoUrl) URL.revokeObjectURL(task.videoUrl)
  // 未完成任务被移除时同步停止其后台轮询
  if (task && (task.status === 'pending' || task.status === 'processing')) stoppedTasks.add(taskId)
  tasks.value = tasks.value.filter(tk => tk.taskId !== taskId)
}

export function clearFailed() {
  tasks.value = tasks.value.filter(tk => tk.status !== 'failed')
}

// ===== IndexedDB(视频库) =====
function openIdb(owner = sessionOwnerKey()): Promise<IDBDatabase> {
  return openStudioHistoryDb('video', owner)
}
async function idbPut(item: VideoHistoryItem, owner = sessionOwnerKey()) {
  const db = await openIdb(owner)
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction('history', 'readwrite')
    tx.objectStore('history').put(item)
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
  db.close()
}
async function idbAll(owner = sessionOwnerKey()): Promise<VideoHistoryItem[]> {
  const db = await openIdb(owner)
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
  if (!task.blob || !isCurrentSession(task)) return
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
  const owner = task.owner || sessionOwnerKey()
  await idbPut(item, owner).catch(() => undefined)
  if (!isCurrentSession(task)) return
  history.value = [item, ...history.value]
  updateStorageMeter()
}

export async function loadHistory() {
  const session = currentSession()
  try {
    const items = await idbAll(session.owner)
    if (!isCurrentSession(session)) return
    history.value = items
      .filter(it => it && it.blob instanceof Blob)
      .sort((a, b) => b.ts - a.ts)
  } catch {
    if (isCurrentSession(session)) history.value = []
  }
}

export function updateStorageMeter() {
  const session = currentSession()
  if (navigator.storage?.estimate) {
    void navigator.storage.estimate().then(est => {
      if (!isCurrentSession(session)) return
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

/** Stop the old user's polling and remove all in-memory studio data. */
export function resetVideoStudioSession() {
  sessionEpoch++
  tasks.value.forEach(task => stoppedTasks.add(task.taskId))
  tasks.value.forEach(task => { if (task.videoUrl) URL.revokeObjectURL(task.videoUrl) })
  tasks.value = []
  generating.value = false
  storageUsed.value = 0
  storageQuota.value = 0
  refImages.value.forEach(item => URL.revokeObjectURL(item.url))
  refImages.value = []
  objectUrlCache.forEach(url => URL.revokeObjectURL(url))
  objectUrlCache.clear()
  history.value = []
  historySearch.value = ''
  prompt.value = ''
  initError.value = ''
  models.value = toOptions(FALLBACK_VIDEO_MODELS)
  model.value = FALLBACK_VIDEO_MODELS[0]
  pruneStoppedTasks()
}

if (typeof window !== 'undefined') {
  window.addEventListener('studio-session-ended', resetVideoStudioSession)
}
