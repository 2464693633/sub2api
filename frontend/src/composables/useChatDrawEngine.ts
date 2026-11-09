/**
 * 对话生图引擎(模块级单例)。
 *
 * 把生图做成聊天式多轮迭代:每一轮 = 用户提示词(+参考图) → 助手结果图(可多张)。
 * 请求方式与生图工作台一致(会话直通 /api/v1/image-studio/*,按模型分组带
 * X-Image-Studio-Group 头),但结果按"每张完成即显示"流式回填到当前轮。
 *
 * 持久化:按用户隔离的 IndexedDB `chat-draw-db-<owner>`,每轮一条记录(含图片 Blob),
 * 刷新后可回看;切换账号/登出时中止进行中的请求并清空内存状态。
 */
import { computed, ref, toRaw } from 'vue'
import { i18n } from '@/i18n'
import { useAppStore } from '@/stores/app'
import { sessionOwnerKey, StudioSessionChangedError } from './studioSession'
import {
  MAX_REFS, QUALITIES, models, model as studioModel, initError, loadModels,
  blobFromB64, compressRefFile
} from './useImageStudioEngine'
import { aiEnhancePrompt, aiKeyId, aiModel } from './useAIPrompt'

const t = i18n.global.t

// ===== 常量 =====
export { MAX_REFS, QUALITIES }
export type ChatQuality = typeof QUALITIES[number]
/** 画幅预设:与生图工作台 RATIO_PRESETS 同一套像素(长边基准) */
export const CHAT_RATIOS = [
  { value: '1:1', w: 1, h: 1, base: 1024 },
  { value: '3:2', w: 3, h: 2, base: 1216 },
  { value: '2:3', w: 2, h: 3, base: 1216 },
  { value: '4:3', w: 4, h: 3, base: 1024 },
  { value: '3:4', w: 3, h: 4, base: 1024 },
  { value: '16:9', w: 16, h: 9, base: 1280 },
  { value: '9:16', w: 9, h: 16, base: 1280 },
  { value: '21:9', w: 21, h: 9, base: 1680 }
] as const
export const CHAT_COUNTS = [1, 2, 3, 4] as const
const PARALLEL = 2
const REQUEST_TIMEOUT_MS = 300000

export function sizeForRatio(ratio: string): string {
  const r = CHAT_RATIOS.find(x => x.value === ratio)
  if (!r) return 'auto'
  const snap = (v: number) => Math.max(256, Math.min(4096, Math.round(v / 16) * 16))
  const w = r.w >= r.h ? r.base : r.base * r.w / r.h
  const h = r.w >= r.h ? r.base * r.h / r.w : r.base
  return `${snap(w)}x${snap(h)}`
}

// ===== 类型 =====
export interface ChatImage {
  id: string
  status: 'pending' | 'done' | 'failed'
  blob?: Blob
  error?: string
  actualSize?: string
  /** 该图由哪种操作产生:首轮生成 / 重绘 / 变体 */
  kind?: 'variant'
}
export interface ChatTurn {
  id: string
  ts: number
  /** 用户原始输入 */
  prompt: string
  /** AI 润色后实际发送的提示词(未润色时为空) */
  polished?: string
  refs: Blob[]
  model: string
  groupId?: number
  platform?: string
  ratio: string
  size: string
  quality: ChatQuality
  count: number
  /** 来源操作:普通发送 / 重绘 / 变体 */
  origin?: 'redraw' | 'variant'
  status: 'running' | 'done' | 'failed' | 'cancelled'
  images: ChatImage[]
  ms?: number
}
export interface DraftRef { blob: Blob; url: string }

// ===== 状态 =====
export const turns = ref<ChatTurn[]>([])
export const draftText = ref('')
export const draftRefs = ref<DraftRef[]>([])
export const ratio = ref<string>('1:1')
export const quality = ref<ChatQuality>('auto')
export const count = ref<number>(1)
export const model = ref<string>('')
export const polishEnabled = ref(false)
export const polishing = ref(false)
export const elapsed = ref(0)
export const loaded = ref(false)

export const busy = computed(() => polishing.value || turns.value.some(x => x.status === 'running'))

const controllers = new Map<string, AbortController>()
let sessionEpoch = 0
let loadedOwner = ''
let elapsedTimer: number | null = null

function appStore() {
  return useAppStore()
}
function uid(): string {
  return Date.now().toString(36) + Math.random().toString(36).slice(2, 8)
}

interface Session { owner: string; epoch: number }
function currentSession(): Session {
  return { owner: sessionOwnerKey(), epoch: sessionEpoch }
}
function isCurrent(s: Session): boolean {
  return s.epoch === sessionEpoch && s.owner === sessionOwnerKey()
}
function authHeaders(s: Session): Record<string, string> {
  if (!isCurrent(s)) throw new StudioSessionChangedError()
  return { Authorization: `Bearer ${localStorage.getItem('auth_token') || ''}` }
}

// ===== 设置持久化(按用户) =====
const LS_SETTINGS = 'chat_draw_settings'
function loadSettings(owner: string) {
  try {
    const raw = JSON.parse(localStorage.getItem(`${LS_SETTINGS}:${owner}`) || 'null') as Partial<{
      ratio: string; quality: ChatQuality; count: number; model: string; polish: boolean
    }> | null
    if (!raw) return
    if (raw.ratio && CHAT_RATIOS.some(r => r.value === raw.ratio)) ratio.value = raw.ratio
    if (raw.quality && (QUALITIES as readonly string[]).includes(raw.quality)) quality.value = raw.quality
    if (raw.count && (CHAT_COUNTS as readonly number[]).includes(raw.count)) count.value = raw.count
    if (raw.model) model.value = raw.model
    polishEnabled.value = Boolean(raw.polish)
  } catch { /* 损坏的设置忽略 */ }
}
export function saveSettings() {
  localStorage.setItem(`${LS_SETTINGS}:${sessionOwnerKey()}`, JSON.stringify({
    ratio: ratio.value, quality: quality.value, count: count.value, model: model.value, polish: polishEnabled.value
  }))
}

// ===== IndexedDB =====
function openDb(owner: string): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(`chat-draw-db-${owner}`, 1)
    req.onupgradeneeded = () => {
      if (!req.result.objectStoreNames.contains('turns')) req.result.createObjectStore('turns', { keyPath: 'id' })
    }
    req.onsuccess = () => {
      req.result.onversionchange = () => req.result.close()
      resolve(req.result)
    }
    req.onerror = () => reject(req.error)
  })
}
async function dbRun(owner: string, mode: IDBTransactionMode, run: (s: IDBObjectStore) => IDBRequest | void): Promise<unknown> {
  const db = await openDb(owner)
  try {
    return await new Promise((resolve, reject) => {
      const tx = db.transaction('turns', mode)
      const req = run(tx.objectStore('turns'))
      tx.oncomplete = () => resolve(req ? req.result : undefined)
      tx.onerror = () => reject(tx.error)
      tx.onabort = () => reject(tx.error)
    })
  } finally {
    db.close()
  }
}
/** 去掉 Vue 代理,保证结构化克隆可写入 IndexedDB */
function plain(turn: ChatTurn): ChatTurn {
  const raw = toRaw(turn)
  return {
    ...raw,
    refs: toRaw(raw.refs).map(b => toRaw(b)),
    images: toRaw(raw.images).map(img => ({ ...toRaw(img), blob: img.blob ? toRaw(img.blob) : undefined }))
  }
}
function persist(turn: ChatTurn, s: Session) {
  if (!isCurrent(s)) return
  void dbRun(s.owner, 'readwrite', st => st.put(plain(turn))).catch(() => undefined)
}

// ===== 对象 URL 缓存(Blob → blob: 地址) =====
let urlCache = new WeakMap<Blob, string>()
const liveUrls = new Set<string>()
export function blobUrl(blob?: Blob): string {
  if (!blob) return ''
  const raw = toRaw(blob)
  let url = urlCache.get(raw)
  if (!url) {
    url = URL.createObjectURL(raw)
    urlCache.set(raw, url)
    liveUrls.add(url)
  }
  return url
}
function revokeAllUrls() {
  liveUrls.forEach(u => URL.revokeObjectURL(u))
  liveUrls.clear()
  urlCache = new WeakMap()
}

// ===== 初始化 =====
export async function initChatDraw() {
  const s = currentSession()
  if (loadedOwner !== s.owner) {
    loadedOwner = s.owner
    loaded.value = false
    turns.value = []
    loadSettings(s.owner)
    try {
      const items = await dbRun(s.owner, 'readonly', st => st.getAll()) as ChatTurn[]
      if (!isCurrent(s)) return
      // 刷新时仍在进行的轮次已无法继续:标记为中断,保留已完成的图
      const interrupted: ChatTurn[] = []
      for (const it of items) {
        if (it.status !== 'running') continue
        it.status = 'cancelled'
        it.images.forEach(img => {
          if (img.status === 'pending') { img.status = 'failed'; img.error = t('canvasChat.interrupted') }
        })
        interrupted.push(it)
      }
      turns.value = items.filter(it => it && Array.isArray(it.images)).sort((a, b) => a.ts - b.ts)
      interrupted.forEach(it => persist(it, s))
    } catch {
      if (isCurrent(s)) turns.value = []
    } finally {
      if (isCurrent(s)) loaded.value = true
    }
  }
  await loadModels()
  if (!isCurrent(s)) return
  if (!models.value.some(o => o.id === model.value)) {
    model.value = models.value.some(o => o.id === studioModel.value) ? studioModel.value : (models.value[0]?.id || '')
  }
}
export { initError }

// ===== 参考图 =====
export async function addDraftRefs(files: Blob[]) {
  const images = files.filter(f => f.type.startsWith('image/'))
  if (!images.length) return
  const room = MAX_REFS - draftRefs.value.length
  if (room <= 0) {
    appStore().showError(t('imageStudio.maxRefs'))
    return
  }
  for (const f of images.slice(0, room)) {
    const file = f instanceof File ? f : new File([f], `ref.${(f.type.split('/')[1] || 'png')}`, { type: f.type })
    const blob = await compressRefFile(file)
    draftRefs.value.push({ blob, url: URL.createObjectURL(blob) })
  }
  if (images.length > room) appStore().showError(t('imageStudio.maxRefs'))
}
export function removeDraftRef(idx: number) {
  const item = draftRefs.value[idx]
  if (!item) return
  URL.revokeObjectURL(item.url)
  draftRefs.value = draftRefs.value.filter((_, i) => i !== idx)
}
function clearDraftRefs() {
  draftRefs.value.forEach(r => URL.revokeObjectURL(r.url))
  draftRefs.value = []
}
export async function addImageAsRef(img: ChatImage) {
  if (!img.blob) return
  const before = draftRefs.value.length
  await addDraftRefs([toRaw(img.blob)])
  if (draftRefs.value.length > before) appStore().showSuccess(t('imageStudio.refAdded'))
}

// ===== 请求 =====
interface GenResult { blob?: Blob; error?: string }

async function fetchAsset(url: string, s: Session, signal: AbortSignal): Promise<Blob> {
  // 优先直连;上游 CDN 无 CORS 头时走后端代理(与生图工作台一致)
  try {
    const res = await fetch(url, { signal })
    if (res.ok) return await res.blob()
  } catch (err) {
    if (signal.aborted) throw err
  }
  const proxied = await fetch(`${window.location.origin}/api/v1/image-studio/asset?u=${encodeURIComponent(url)}`, {
    headers: authHeaders(s), signal
  })
  if (!proxied.ok) throw new Error(`asset proxy HTTP ${proxied.status}`)
  return await proxied.blob()
}

async function generateOne(turn: ChatTurn, promptText: string, s: Session, outer: AbortSignal): Promise<GenResult> {
  const headers = authHeaders(s)
  if (turn.groupId && turn.groupId > 0) headers['X-Image-Studio-Group'] = String(turn.groupId)
  // Grok(xAI) 上游只接受 model/prompt/n,带 size/quality/output_format 会 400
  const isGrok = turn.platform ? turn.platform === 'grok' : /grok/i.test(turn.model)
  const controller = new AbortController()
  const onAbort = () => controller.abort()
  outer.addEventListener('abort', onAbort)
  let timedOut = false
  const timer = window.setTimeout(() => { timedOut = true; controller.abort() }, REQUEST_TIMEOUT_MS)
  try {
    let res: Response
    if (turn.refs.length) {
      const form = new FormData()
      form.append('model', turn.model)
      form.append('prompt', promptText)
      if (!isGrok) {
        if (turn.size !== 'auto') form.append('size', turn.size)
        if (turn.quality !== 'auto') form.append('quality', turn.quality)
        form.append('output_format', 'png')
      }
      turn.refs.forEach((b, i) => {
        const raw = toRaw(b)
        form.append('image', new File([raw], `ref-${i + 1}.${raw.type.split('/')[1] || 'png'}`, { type: raw.type || 'image/png' }))
      })
      res = await fetch(`${window.location.origin}/api/v1/image-studio/edits`, {
        method: 'POST', headers, body: form, signal: controller.signal
      })
    } else {
      const payload: Record<string, unknown> = isGrok
        ? { model: turn.model, prompt: promptText, n: 1 }
        : {
            model: turn.model,
            prompt: promptText,
            size: turn.size === 'auto' ? undefined : turn.size,
            quality: turn.quality === 'auto' ? undefined : turn.quality,
            output_format: 'png',
            n: 1
          }
      res = await fetch(`${window.location.origin}/api/v1/image-studio/generations`, {
        method: 'POST',
        headers: { ...headers, 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
        signal: controller.signal
      })
    }
    // 非 JSON 错误体(网关 502 页等)也要把真实文本显示出来
    const text = await res.text().catch(() => '')
    let body: { error?: { message?: string } | string; message?: string; data?: Array<{ b64_json?: string; url?: string }> } | null = null
    try { body = text ? JSON.parse(text) : null } catch { body = null }
    const errMsg = (typeof body?.error === 'string' ? body.error : body?.error?.message) || body?.message
    if (!res.ok) return { error: errMsg || text.slice(0, 300) || `HTTP ${res.status}` }
    const first = body?.data?.[0]
    if (!first) return { error: errMsg || t('imageStudio.generateFailed') }
    const blob = first.b64_json
      ? blobFromB64(first.b64_json)
      : (first.url ? await fetchAsset(first.url, s, controller.signal) : undefined)
    return blob ? { blob } : { error: t('imageStudio.generateFailed') }
  } catch (err) {
    if (err instanceof StudioSessionChangedError) throw err
    if (err instanceof DOMException && err.name === 'AbortError') {
      return { error: timedOut ? t('imageStudio.timeout') : t('canvasChat.stopped') }
    }
    return { error: err instanceof Error ? err.message : t('imageStudio.generateFailed') }
  } finally {
    window.clearTimeout(timer)
    outer.removeEventListener('abort', onAbort)
  }
}

async function imageSizeOf(blob: Blob): Promise<string> {
  try {
    const bmp = await createImageBitmap(blob)
    const out = `${bmp.width}x${bmp.height}`
    bmp.close()
    return out
  } catch {
    return ''
  }
}

function startElapsed() {
  stopElapsed()
  elapsed.value = 0
  elapsedTimer = window.setInterval(() => { elapsed.value++ }, 1000)
}
function stopElapsed() {
  if (elapsedTimer !== null) {
    window.clearInterval(elapsedTimer)
    elapsedTimer = null
  }
}

/** 为某轮生成指定的待定图片(并发 PARALLEL),每张完成即回填并落盘 */
async function runImages(turnId: string, imageIds: string[], s: Session) {
  const turn = turns.value.find(x => x.id === turnId)
  if (!turn) return
  const controller = new AbortController()
  controllers.set(turnId, controller)
  turn.status = 'running'
  persist(turn, s)
  startElapsed()
  const started = performance.now()
  const promptText = turn.polished || turn.prompt
  const queue = [...imageIds]
  const worker = async () => {
    while (queue.length) {
      const id = queue.shift()!
      const img = turn.images.find(x => x.id === id)
      if (!img) continue
      if (controller.signal.aborted || !isCurrent(s)) {
        img.status = 'failed'
        img.error = t('canvasChat.stopped')
        continue
      }
      const result = await generateOne(turn, promptText, s, controller.signal)
      if (!isCurrent(s)) return
      if (result.blob) {
        img.actualSize = await imageSizeOf(result.blob)
        img.blob = result.blob
        img.status = 'done'
        img.error = undefined
      } else {
        img.status = 'failed'
        img.error = result.error
      }
      persist(turn, s)
    }
  }
  try {
    await Promise.all(Array.from({ length: Math.min(PARALLEL, queue.length) }, worker))
  } catch (err) {
    if (err instanceof StudioSessionChangedError) return
    throw err
  } finally {
    if (controllers.get(turnId) === controller) controllers.delete(turnId)
  }
  if (!isCurrent(s)) return
  turn.ms = Math.round(performance.now() - started)
  turn.status = controller.signal.aborted
    ? 'cancelled'
    : (turn.images.some(img => img.status === 'done') ? 'done' : 'failed')
  if (!controllers.size) stopElapsed()
  persist(turn, s)
}

function newImages(n: number, kind?: 'variant'): ChatImage[] {
  return Array.from({ length: n }, () => ({ id: uid(), status: 'pending' as const, kind }))
}

// ===== 对外操作 =====
export async function sendDraft() {
  const text = draftText.value.trim()
  if (!text || busy.value) return
  const option = models.value.find(o => o.id === model.value)
  if (!option) {
    appStore().showError(initError.value || t('imageStudio.noImageModels'))
    return
  }
  const s = currentSession()
  let polished: string | undefined
  if (polishEnabled.value) {
    polishing.value = true
    try {
      polished = await aiEnhancePrompt('image', text, { keyId: aiKeyId.value ?? undefined, model: aiModel.value || undefined })
    } catch (err) {
      if (err instanceof StudioSessionChangedError) return
      // 润色失败不阻塞生图:提示后直接用原文
      appStore().showError(`${t('canvasChat.polishFailed')}: ${err instanceof Error ? err.message : ''}`)
    } finally {
      polishing.value = false
    }
    if (!isCurrent(s)) return
  }
  const size = sizeForRatio(ratio.value)
  const turn: ChatTurn = {
    id: uid(),
    ts: Date.now(),
    prompt: text,
    polished: polished && polished !== text ? polished : undefined,
    refs: draftRefs.value.map(r => toRaw(r.blob)),
    model: option.id,
    groupId: option.group_id,
    platform: option.platform,
    ratio: ratio.value,
    size,
    quality: quality.value,
    count: count.value,
    status: 'running',
    images: newImages(count.value)
  }
  turns.value.push(turn)
  draftText.value = ''
  clearDraftRefs()
  await runImages(turn.id, turn.images.map(i => i.id), s)
}

/** 重绘:同参数(提示词/参考图/模型/画幅/质量/张数)新开一轮 */
export async function redrawTurn(source: ChatTurn) {
  if (busy.value) return
  const s = currentSession()
  const turn: ChatTurn = {
    ...plain(source),
    id: uid(),
    ts: Date.now(),
    origin: 'redraw',
    status: 'running',
    ms: undefined,
    images: newImages(source.count)
  }
  turns.value.push(turn)
  await runImages(turn.id, turn.images.map(i => i.id), s)
}

/** 变体:沿用该轮提示词与参考图,在同一轮追加一张新图(新随机种子) */
export async function variantOf(turn: ChatTurn) {
  if (busy.value) return
  const s = currentSession()
  const extra = newImages(1, 'variant')
  turn.images.push(...extra)
  await runImages(turn.id, extra.map(i => i.id), s)
}

/** 失败图重试:原位重新生成 */
export async function retryImage(turn: ChatTurn, img: ChatImage) {
  if (busy.value || img.status !== 'failed') return
  img.status = 'pending'
  img.error = undefined
  await runImages(turn.id, [img.id], currentSession())
}

export function stopAll() {
  controllers.forEach(c => c.abort())
}

export async function deleteTurn(id: string) {
  if (controllers.has(id)) return
  turns.value = turns.value.filter(x => x.id !== id)
  await dbRun(sessionOwnerKey(), 'readwrite', st => st.delete(id)).catch(() => undefined)
}

export async function clearConversation() {
  stopAll()
  turns.value = []
  revokeAllUrls()
  await dbRun(sessionOwnerKey(), 'readwrite', st => st.clear()).catch(() => undefined)
}

export function downloadImage(turn: ChatTurn, img: ChatImage) {
  if (!img.blob) return
  const ext = (img.blob.type.split('/')[1] || 'png').replace('jpeg', 'jpg')
  const a = document.createElement('a')
  a.href = blobUrl(img.blob)
  a.download = `chat-draw-${turn.ts}-${turn.images.indexOf(img) + 1}.${ext}`
  a.click()
}

/** 登出/切换账号:中止请求并清空内存中的会话 */
export function resetChatDrawSession() {
  sessionEpoch++
  stopAll()
  controllers.clear()
  stopElapsed()
  polishing.value = false
  turns.value = []
  draftText.value = ''
  clearDraftRefs()
  revokeAllUrls()
  loadedOwner = ''
  loaded.value = false
}

if (typeof window !== 'undefined') {
  window.addEventListener('studio-session-ended', resetChatDrawSession)
}
