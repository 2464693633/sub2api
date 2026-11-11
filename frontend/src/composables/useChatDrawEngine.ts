/**
 * 对话生图引擎(模块级单例,多会话)。
 *
 * 把生图做成聊天式多轮迭代:每一轮 = 用户提示词(+参考图) → 助手结果图(可多张)。
 * 请求方式与生图工作台一致(会话直通 /api/v1/image-studio/*,按模型分组带
 * X-Image-Studio-Group 头),结果按"每张完成即显示"流式回填到所属会话的轮次。
 *
 * 多会话:
 *   - sessions 表记录会话元信息(标题=首轮提示词截断 / 轮数 / 生图数 / lastTs)
 *   - 每个会话的轮次按需从 IndexedDB 懒加载进 turnStore;`turns` 是当前会话的视图
 *   - 切走的会话若仍在生成/润色,会被"钉住"留在内存里继续跑(后台完成后打角标)
 *   - 新建会话先只存在于内存,发出首轮后才落库;空的未保存会话切走即丢弃
 *
 * 持久化:按用户隔离的 IndexedDB `chat-draw-db-<owner>`;切换账号/登出时中止请求并清空内存。
 *
 * 代际:
 *   - sessionEpoch:登出/切账号时递增,旧账号的任何异步回调都不得再改共享状态
 *   - sessionGen[sid]:清空/删除某会话时递增,该会话旧轮次的 worker 不得再写库/改状态
 */
import { computed, ref, toRaw } from 'vue'
import { i18n } from '@/i18n'
import { useAppStore } from '@/stores/app'
import { sessionOwnerKey, StudioSessionChangedError } from './studioSession'
import {
  MAX_REFS, models, model as studioModel, initError, loadModels,
  blobFromB64, compressRefFile
} from './useImageStudioEngine'
import { aiEnhancePrompt, aiKeyId, aiModel } from './useAIPrompt'

const t = i18n.global.t

// ===== 常量 =====
export { MAX_REFS }
export const QUALITIES = ['auto', 'low', 'medium', 'high'] as const
export type ChatQuality = typeof QUALITIES[number]
export type ChatFormat = 'auto' | 'png' | 'jpeg' | 'webp'
export const CHAT_RESOLUTIONS = ['1k', '2k', '4k'] as const
export type ChatResolution = typeof CHAT_RESOLUTIONS[number]
const RESOLUTION_BASES: Record<ChatResolution, number> = { '1k': 1024, '2k': 2048, '4k': 3840 }
/** 画幅预设:长边像素由分辨率档位决定 */
export const CHAT_RATIOS = [
  { value: '1:1', w: 1, h: 1 },
  { value: '3:2', w: 3, h: 2 },
  { value: '2:3', w: 2, h: 3 },
  { value: '4:3', w: 4, h: 3 },
  { value: '3:4', w: 3, h: 4 },
  { value: '16:9', w: 16, h: 9 },
  { value: '9:16', w: 9, h: 16 },
  { value: '21:9', w: 21, h: 9 }
] as const
export const CHAT_COUNTS = [1, 2, 3, 4] as const
export const MAX_CHAT_COUNT = 8
const PARALLEL = 2
const REQUEST_TIMEOUT_MS = 300000
/** 每个会话首次只加载最近 N 轮,避免全量读出所有图片 Blob */
export const LOAD_LIMIT = 50
/** 会话标题 = 首轮提示词截断 */
export const TITLE_MAX = 18
/** v2 之前的单会话数据迁移到这个会话 */
export const LEGACY_SESSION_ID = 'legacy'

export function sizeForRatio(ratio: string, resolution: ChatResolution = '1k'): string {
  const r = CHAT_RATIOS.find(x => x.value === ratio)
  if (!r) return 'auto'
  const snap = (v: number) => Math.max(256, Math.min(4096, Math.round(v / 16) * 16))
  const base = RESOLUTION_BASES[resolution]
  const w = r.w >= r.h ? base : base * r.w / r.h
  const h = r.w >= r.h ? base * r.h / r.w : base
  return `${snap(w)}x${snap(h)}`
}

export function buildSessionTitle(prompt: string): string {
  const s = prompt.trim().replace(/\s+/g, ' ')
  return s.length > TITLE_MAX ? `${s.slice(0, TITLE_MAX)}…` : s
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
  /** 所属会话 */
  sessionId: string
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
  format: ChatFormat
  /** 来源操作:普通发送 / 重绘 / 变体 */
  origin?: 'redraw' | 'variant'
  status: 'running' | 'done' | 'failed' | 'cancelled'
  images: ChatImage[]
  ms?: number
  /** 本次运行开始时间(用于计时显示) */
  startedAt?: number
}
export interface ChatSession {
  id: string
  /** 空串 = 尚未发首轮,视图显示"新对话" */
  title: string
  createdAt: number
  lastTs: number
  turnCount: number
  /** 该会话成功生成的图片数 */
  imageCount: number
}
export type SessionNotice = 'done' | 'error'
export interface DraftRef { blob: Blob; url: string }

// ===== 状态 =====
const sessionRecords = ref<ChatSession[]>([])
/** 会话列表,按最近活动倒序 */
export const sessions = computed(() => [...sessionRecords.value].sort((a, b) => b.lastTs - a.lastTs || b.createdAt - a.createdAt))
export const activeSessionId = ref('')
export const activeSession = computed(() => sessionRecords.value.find(s => s.id === activeSessionId.value) ?? null)
/** 已加载进内存的会话轮次(sid → turns) */
const turnStore = ref<Record<string, ChatTurn[]>>({})
const EMPTY: ChatTurn[] = []
/** 当前会话的轮次 */
export const turns = computed<ChatTurn[]>(() => turnStore.value[activeSessionId.value] ?? EMPTY)
const olderMap = ref<Record<string, boolean>>({})
/** 当前会话在库里还有比已加载更早的轮次 */
export const hasOlder = computed(() => Boolean(olderMap.value[activeSessionId.value]))
/** 后台会话完成/失败的角标 */
export const sessionNotices = ref<Record<string, SessionNotice>>({})
/** 累计成功生图张数(所有会话) */
export const totalImages = computed(() => sessionRecords.value.reduce((n, s) => n + (s.imageCount || 0), 0))

export const draftText = ref('')
export const draftRefs = ref<DraftRef[]>([])
export const ratio = ref<string>('1:1')
export const resolution = ref<ChatResolution>('1k')
export const quality = ref<ChatQuality>('auto')
export const count = ref<number>(1)
export const format = ref<ChatFormat>('auto')
export const model = ref<string>('')
export const polishEnabled = ref(false)
export const polishing = ref(false)
/** 每秒跳动的时钟(仅在有进行中的轮次时运行) */
const now = ref(Date.now())

const sessionsLoaded = ref(false)
/** 当前账号的会话列表与当前会话的轮次是否已读完(读完前禁止发送,防止被首次读取覆盖) */
export const loaded = computed(() => sessionsLoaded.value && Boolean(turnStore.value[activeSessionId.value]))

/** 正在生成的会话 id */
export const runningSessionIds = computed(() => {
  const out = new Set<string>()
  for (const [sid, list] of Object.entries(turnStore.value)) {
    if (list.some(x => x.status === 'running')) out.add(sid)
  }
  return out
})
export const busy = computed(() => !loaded.value || polishing.value || turns.value.some(x => x.status === 'running'))

/** 当前会话进行中轮次已用秒数 */
export const elapsed = computed(() => {
  const running = turns.value.filter(x => x.status === 'running' && x.startedAt)
  const start = Math.max(0, ...running.map(x => x.startedAt!))
  return start ? Math.max(0, Math.floor((now.value - start) / 1000)) : 0
})
export function elapsedOf(turn: ChatTurn): number {
  return turn.startedAt ? Math.max(0, Math.floor((now.value - turn.startedAt) / 1000)) : 0
}

const controllers = new Map<string, AbortController>()
let sessionEpoch = 0
const sessionGen = new Map<string, number>()
let polishSeq = 0
/** 正在压缩中的参考图名额(并发添加时一并计入上限) */
let pendingRefs = 0
let tickTimer: number | null = null
let initOwner = ''
let initPromise: Promise<void> | null = null
/** 已创建但尚未落库(未发首轮)的会话 */
const unsavedIds = new Set<string>()
/** 正在润色等结果的会话(引用计数),期间不可被逐出内存 */
const pins = new Map<string, number>()
const loadPromises = new Map<string, Promise<void>>()

function appStore() {
  return useAppStore()
}
function uid(): string {
  return Date.now().toString(36) + Math.random().toString(36).slice(2, 8)
}
function genOf(sid: string): number {
  return sessionGen.get(sid) ?? 0
}
function bumpGen(sid: string) {
  sessionGen.set(sid, genOf(sid) + 1)
}
function findSession(sid: string): ChatSession | undefined {
  return sessionRecords.value.find(s => s.id === sid)
}

// ===== 账号 / 代际 =====
interface Session { owner: string; epoch: number }
/** 某一次生成任务的身份:账号 + 账号代际 + 会话 + 会话代际 */
interface Run extends Session { sid: string; gen: number }
function currentSession(): Session {
  return { owner: sessionOwnerKey(), epoch: sessionEpoch }
}
function runFor(sid: string): Run {
  return { ...currentSession(), sid, gen: genOf(sid) }
}
function isCurrent(s: Session): boolean {
  return s.epoch === sessionEpoch && s.owner === sessionOwnerKey()
}
function isLive(r: Run): boolean {
  return isCurrent(r) && r.gen === genOf(r.sid) && Boolean(turnStore.value[r.sid])
}
function authHeaders(s: Session): Record<string, string> {
  if (!isCurrent(s)) throw new StudioSessionChangedError()
  return { Authorization: `Bearer ${localStorage.getItem('auth_token') || ''}` }
}

// ===== 设置持久化(按用户) =====
const LS_SETTINGS = 'chat_draw_settings'
const LS_ACTIVE = 'chat_draw_active'
function loadSettings(owner: string) {
  resolution.value = '1k'
  format.value = 'auto'
  count.value = 1
  try {
    const raw = JSON.parse(localStorage.getItem(`${LS_SETTINGS}:${owner}`) || 'null') as Partial<{
      ratio: string; resolution: ChatResolution; quality: ChatQuality; count: number; format: ChatFormat; model: string; polish: boolean
    }> | null
    if (!raw) return
    if (raw.ratio && (raw.ratio === 'auto' || CHAT_RATIOS.some(r => r.value === raw.ratio))) ratio.value = raw.ratio
    if (raw.resolution && (CHAT_RESOLUTIONS as readonly string[]).includes(raw.resolution)) resolution.value = raw.resolution
    if (raw.quality && (QUALITIES as readonly string[]).includes(raw.quality)) quality.value = raw.quality
    if (raw.count !== undefined) count.value = clampCount(raw.count)
    if (raw.format && ['auto', 'png', 'jpeg', 'webp'].includes(raw.format)) format.value = raw.format
    if (raw.model) model.value = raw.model
    polishEnabled.value = Boolean(raw.polish)
  } catch { /* 损坏的设置忽略 */ }
}
export function saveSettings() {
  count.value = clampCount(count.value)
  localStorage.setItem(`${LS_SETTINGS}:${sessionOwnerKey()}`, JSON.stringify({
    ratio: ratio.value, resolution: resolution.value, quality: quality.value, count: clampCount(count.value), format: format.value, model: model.value, polish: polishEnabled.value
  }))
}

export function clampCount(value: unknown): number {
  const n = Number(value)
  return Number.isFinite(n) ? Math.max(1, Math.min(MAX_CHAT_COUNT, Math.trunc(n))) : 1
}
function saveActive() {
  localStorage.setItem(`${LS_ACTIVE}:${sessionOwnerKey()}`, activeSessionId.value)
}

// ===== IndexedDB =====
export const DB_VERSION = 3
function openDb(owner: string): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(`chat-draw-db-${owner}`, DB_VERSION)
    req.onupgradeneeded = (ev) => {
      const db = req.result
      const tx = req.transaction!
      const turnsStore = db.objectStoreNames.contains('turns')
        ? tx.objectStore('turns')
        : db.createObjectStore('turns', { keyPath: 'id' })
      if (!turnsStore.indexNames.contains('ts')) turnsStore.createIndex('ts', 'ts')
      if (!turnsStore.indexNames.contains('session_ts')) turnsStore.createIndex('session_ts', ['sessionId', 'ts'])
      const sessStore = db.objectStoreNames.contains('sessions')
        ? tx.objectStore('sessions')
        : db.createObjectStore('sessions', { keyPath: 'id' })
      // v2 → v3:旧的单会话轮次归入 legacy 会话
      if (ev.oldVersion > 0 && ev.oldVersion < 3) {
        const legacy: ChatSession = { id: LEGACY_SESSION_ID, title: '', createdAt: 0, lastTs: 0, turnCount: 0, imageCount: 0 }
        let firstTs = Infinity
        const cur = turnsStore.openCursor()
        cur.onsuccess = () => {
          const c = cur.result
          if (c) {
            const v = c.value as ChatTurn
            if (!v.sessionId) {
              v.sessionId = LEGACY_SESSION_ID
              c.update(v)
              legacy.turnCount++
              legacy.imageCount += (v.images || []).filter(i => i.status === 'done').length
              legacy.lastTs = Math.max(legacy.lastTs, v.ts || 0)
              if ((v.ts || 0) < firstTs) { firstTs = v.ts || 0; legacy.title = buildSessionTitle(v.prompt || '') }
            }
            c.continue()
          } else if (legacy.turnCount) {
            legacy.createdAt = firstTs === Infinity ? legacy.lastTs : firstTs
            sessStore.put(legacy)
          }
        }
      }
    }
    req.onsuccess = () => {
      req.result.onversionchange = () => req.result.close()
      resolve(req.result)
    }
    req.onerror = () => reject(req.error)
    req.onblocked = () => reject(new Error('Chat draw database upgrade blocked'))
  })
}
type StoreName = 'turns' | 'sessions'
async function dbTx<T = unknown>(
  owner: string,
  stores: StoreName[],
  mode: IDBTransactionMode,
  run: (tx: IDBTransaction) => IDBRequest | (() => T) | void
): Promise<T> {
  const db = await openDb(owner)
  try {
    return await new Promise<T>((resolve, reject) => {
      const tx = db.transaction(stores, mode)
      const out = run(tx)
      tx.oncomplete = () => resolve((typeof out === 'function' ? out() : out ? out.result : undefined) as T)
      tx.onerror = () => reject(tx.error)
      tx.onabort = () => reject(tx.error)
    })
  } finally {
    db.close()
  }
}
function dbAllSessions(owner: string): Promise<ChatSession[]> {
  return dbTx<ChatSession[]>(owner, ['sessions'], 'readonly', tx => tx.objectStore('sessions').getAll())
}
/** 按时间倒序读某会话最近 limit 条 */
function dbSessionTurns(owner: string, sid: string, limit: number): Promise<ChatTurn[]> {
  return dbTx<ChatTurn[]>(owner, ['turns'], 'readonly', tx => {
    const out: ChatTurn[] = []
    const range = IDBKeyRange.bound([sid, -Infinity], [sid, Infinity])
    const req = tx.objectStore('turns').index('session_ts').openCursor(range, 'prev')
    req.onsuccess = () => {
      const c = req.result
      if (c && out.length < limit) {
        out.push(c.value as ChatTurn)
        c.continue()
      }
    }
    return () => out
  })
}
/** 删除某会话的全部轮次(可选连同会话记录) */
function dbDeleteSession(owner: string, sid: string, removeRecord: boolean) {
  return dbTx(owner, ['turns', 'sessions'], 'readwrite', tx => {
    const range = IDBKeyRange.bound([sid, -Infinity], [sid, Infinity])
    const req = tx.objectStore('turns').index('session_ts').openKeyCursor(range)
    const st = tx.objectStore('turns')
    req.onsuccess = () => {
      const c = req.result
      if (c) { st.delete(c.primaryKey); c.continue() }
    }
    if (removeRecord) tx.objectStore('sessions').delete(sid)
  })
}

// 写库串行化:每次写都各自开连接,不排队的话 delete 可能先于更早发起的 put 提交
let writeChain: Promise<unknown> = Promise.resolve()
function enqueueWrite(op: () => Promise<unknown>): Promise<unknown> {
  const next = writeChain.then(op, op)
  writeChain = next.catch(() => undefined)
  return next
}
let lastSaveErrorAt = 0
function reportSaveError() {
  // 一轮多张图会连续失败,10 秒内只提示一次
  if (Date.now() - lastSaveErrorAt < 10000) return
  lastSaveErrorAt = Date.now()
  appStore().showError(t('canvasChat.saveFailed'))
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
function persist(turn: ChatTurn, run: Run) {
  if (!isLive(run)) return
  const id = turn.id
  void enqueueWrite(async () => {
    // 排队期间会话可能被清空/删除、该轮被删除:执行时再校验一次,避免已删记录复活
    if (!isLive(run) || !turnStore.value[run.sid]?.some(x => x.id === id)) return
    await dbTx(run.owner, ['turns'], 'readwrite', tx => tx.objectStore('turns').put(plain(turn)))
  }).catch(() => { if (isCurrent(run)) reportSaveError() })
}
function persistSession(run: Run) {
  if (!isLive(run) || unsavedIds.has(run.sid)) return
  void enqueueWrite(async () => {
    const s = findSession(run.sid)
    if (!isLive(run) || !s) return
    await dbTx(run.owner, ['sessions'], 'readwrite', tx => tx.objectStore('sessions').put({ ...toRaw(s) }))
  }).catch(() => { if (isCurrent(run)) reportSaveError() })
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
/** 释放给定 Blob 的地址,但仍被内存中任何会话的轮次引用的(重绘轮共用参考图)保留 */
function releaseBlobs(blobs: Blob[]) {
  const inUse = new Set<Blob>()
  for (const list of Object.values(turnStore.value)) {
    for (const turn of list) {
      turn.refs.forEach(b => inUse.add(toRaw(b)))
      turn.images.forEach(img => { if (img.blob) inUse.add(toRaw(img.blob)) })
    }
  }
  for (const b of blobs) {
    const raw = toRaw(b)
    if (inUse.has(raw)) continue
    const url = urlCache.get(raw)
    if (!url) continue
    URL.revokeObjectURL(url)
    liveUrls.delete(url)
    urlCache.delete(raw)
  }
}
function blobsOf(turn: ChatTurn): Blob[] {
  return [...turn.refs, ...turn.images.flatMap(img => (img.blob ? [img.blob] : []))]
}
/** 把会话从内存移除并释放其不再被引用的 Blob 地址 */
function dropFromStore(sid: string) {
  const list = turnStore.value[sid]
  if (!list) return
  const next = { ...turnStore.value }
  delete next[sid]
  turnStore.value = next
  releaseBlobs(list.flatMap(blobsOf))
}
/** 会话是否必须留在内存(生成中 / 润色等结果中 / 正在读取) */
function isPinned(sid: string): boolean {
  return (pins.get(sid) ?? 0) > 0 || loadPromises.has(sid) ||
    Boolean(turnStore.value[sid]?.some(x => x.status === 'running'))
}

/** 逐出当前会话以外、未被钉住的会话缓存 */
function evictInactive() {
  for (const sid of Object.keys(turnStore.value)) {
    if (sid === activeSessionId.value || isPinned(sid)) continue
    // 未落库的空会话切走即丢弃
    if (unsavedIds.has(sid)) {
      unsavedIds.delete(sid)
      sessionRecords.value = sessionRecords.value.filter(s => s.id !== sid)
    }
    dropFromStore(sid)
  }
}

// ===== 时钟 =====
function syncTicker() {
  if (controllers.size && tickTimer === null) {
    now.value = Date.now()
    tickTimer = window.setInterval(() => { now.value = Date.now() }, 1000)
  } else if (!controllers.size && tickTimer !== null) {
    window.clearInterval(tickTimer)
    tickTimer = null
  }
}

// ===== 初始化 / 加载 =====
function newMemorySession(): ChatSession {
  const ts = Date.now()
  const s: ChatSession = { id: uid(), title: '', createdAt: ts, lastTs: ts, turnCount: 0, imageCount: 0 }
  unsavedIds.add(s.id)
  sessionRecords.value = [...sessionRecords.value, s]
  turnStore.value = { ...turnStore.value, [s.id]: [] }
  return s
}

/** 懒加载某会话的轮次;同一会话共享一次读取 */
function ensureSessionLoaded(sid: string): Promise<void> {
  if (turnStore.value[sid]) return Promise.resolve()
  const existing = loadPromises.get(sid)
  if (existing) return existing
  const run = runFor(sid)
  const load = (async () => {
    try {
      // 先等排队中的写入落盘,读到的才是最新状态
      await writeChain
      const recent = await dbSessionTurns(run.owner, sid, LOAD_LIMIT + 1)
      if (!isCurrent(run) || run.gen !== genOf(sid) || !findSession(sid) || turnStore.value[sid]) return
      const items = recent
        .slice(0, LOAD_LIMIT)
        .filter(it => it && Array.isArray(it.images))
        .map(it => ({
          ...it,
          count: clampCount(it.count),
          format: (it.format === 'png' || it.format === 'jpeg' || it.format === 'webp') ? it.format : 'auto' as const
        }))
        .sort((a, b) => a.ts - b.ts)
      // 库里仍标记为进行中、但内存里没有对应请求的轮次已无法继续:标记中断,保留已完成的图
      const interrupted: ChatTurn[] = []
      for (const it of items) {
        if (it.status !== 'running' || controllers.has(it.id)) continue
        it.status = 'cancelled'
        it.images.forEach(img => {
          if (img.status === 'pending') { img.status = 'failed'; img.error = t('canvasChat.interrupted') }
        })
        interrupted.push(it)
      }
      olderMap.value = { ...olderMap.value, [sid]: recent.length > LOAD_LIMIT }
      turnStore.value = { ...turnStore.value, [sid]: items }
      const live = runFor(sid)
      interrupted.forEach(it => persist(it, live))
    } catch {
      if (!isCurrent(run)) return
      appStore().showError(t('canvasChat.loadFailed'))
      // 读取失败也放行为空会话,避免永久卡在加载态
      if (findSession(sid) && !turnStore.value[sid]) turnStore.value = { ...turnStore.value, [sid]: [] }
    }
  })()
  const p = load.finally(() => {
    // 账号重置会清空 loadPromises;只删除自己这一次
    if (loadPromises.get(sid) === p) loadPromises.delete(sid)
  })
  loadPromises.set(sid, p)
  return p
}

async function loadAccount(s: Session) {
  sessionsLoaded.value = false
  sessionRecords.value = []
  turnStore.value = {}
  olderMap.value = {}
  activeSessionId.value = ''
  loadSettings(s.owner)
  try {
    const list = await dbAllSessions(s.owner)
    if (!isCurrent(s)) return
    sessionRecords.value = list.filter(x => x && typeof x.id === 'string')
  } catch {
    if (isCurrent(s)) appStore().showError(t('canvasChat.loadFailed'))
  }
  if (!isCurrent(s)) return
  const saved = localStorage.getItem(`${LS_ACTIVE}:${s.owner}`) || ''
  const target = findSession(saved) ? saved : (sessions.value[0]?.id || newMemorySession().id)
  activeSessionId.value = target
  await ensureSessionLoaded(target)
}

/** 幂等:同一账号共享一次加载,切标签再回来不会重复读取,也不会提前放行发送 */
export async function initChatDraw() {
  const s = currentSession()
  if (!initPromise || initOwner !== s.owner) {
    initOwner = s.owner
    initPromise = loadAccount(s)
  }
  await initPromise
  if (!isCurrent(s)) return
  await loadModels()
  if (!isCurrent(s)) return
  if (!models.value.some(o => o.id === model.value)) {
    model.value = models.value.some(o => o.id === studioModel.value) ? studioModel.value : (models.value[0]?.id || '')
  }
  // 会话与模型都就绪后才放行发送;仅 DB 读完不算完成(模型未加载时 model 可能为空)
  if (isCurrent(s)) sessionsLoaded.value = true
}
export { initError }

// ===== 会话操作 =====
/** 切换会话;切走的会话若空闲则从内存逐出 */
export async function selectSession(sid: string) {
  if (!sessionsLoaded.value || sid === activeSessionId.value || !findSession(sid)) return
  activeSessionId.value = sid
  draftText.value = ''
  clearDraftRefs()
  clearNotice(sid)
  saveActive()
  evictInactive()
  await ensureSessionLoaded(sid)
}

/** 新对话:当前已是空的未保存会话时直接复用 */
export function newSession(): string {
  if (!sessionsLoaded.value) return activeSessionId.value
  const cur = activeSessionId.value
  if (unsavedIds.has(cur) && !(turnStore.value[cur]?.length) && !isPinned(cur)) return cur
  const s = newMemorySession()
  activeSessionId.value = s.id
  draftText.value = ''
  clearDraftRefs()
  saveActive()
  evictInactive()
  return s.id
}

export function renameSession(sid: string, title: string) {
  const s = findSession(sid)
  if (!s) return
  s.title = title.trim().slice(0, 80)
  persistSession(runFor(sid))
}

function abortSession(sid: string) {
  for (const turn of turnStore.value[sid] ?? []) controllers.get(turn.id)?.abort()
}
function forgetControllers(sid: string) {
  for (const turn of turnStore.value[sid] ?? []) controllers.delete(turn.id)
  syncTicker()
}

export async function deleteSession(sid: string) {
  if (!findSession(sid)) return
  const owner = sessionOwnerKey()
  // 递增会话代际:该会话在途 worker 返回后不再改状态/写库
  bumpGen(sid)
  abortSession(sid)
  forgetControllers(sid)
  pins.delete(sid)
  const wasUnsaved = unsavedIds.delete(sid)
  sessionRecords.value = sessionRecords.value.filter(s => s.id !== sid)
  dropFromStore(sid)
  clearNotice(sid)
  const older = { ...olderMap.value }
  delete older[sid]
  olderMap.value = older
  if (activeSessionId.value === sid) {
    activeSessionId.value = sessions.value[0]?.id || newMemorySession().id
    draftText.value = ''
    clearDraftRefs()
    saveActive()
  }
  const loading = ensureSessionLoaded(activeSessionId.value)
  if (!wasUnsaved) {
    await enqueueWrite(() => dbDeleteSession(owner, sid, true))
      .catch(() => { if (owner === sessionOwnerKey()) reportSaveError() })
  }
  await loading
}

/** 清空全部历史会话 */
export async function clearAllSessions() {
  const owner = sessionOwnerKey()
  for (const s of sessionRecords.value) bumpGen(s.id)
  controllers.forEach(c => c.abort())
  controllers.clear()
  syncTicker()
  pins.clear()
  unsavedIds.clear()
  sessionRecords.value = []
  turnStore.value = {}
  olderMap.value = {}
  sessionNotices.value = {}
  revokeAllUrls()
  draftText.value = ''
  clearDraftRefs()
  activeSessionId.value = newMemorySession().id
  saveActive()
  await enqueueWrite(() => dbTx(owner, ['turns', 'sessions'], 'readwrite', tx => {
    tx.objectStore('turns').clear()
    tx.objectStore('sessions').clear()
  })).catch(() => { if (owner === sessionOwnerKey()) appStore().showError(t('canvasChat.saveFailed')) })
}

function setNotice(sid: string, state: SessionNotice) {
  const cur = sessionNotices.value[sid]
  sessionNotices.value = { ...sessionNotices.value, [sid]: cur === 'error' && state === 'done' ? cur : state }
}
function clearNotice(sid: string) {
  if (!sessionNotices.value[sid]) return
  const next = { ...sessionNotices.value }
  delete next[sid]
  sessionNotices.value = next
}

// ===== 参考图 =====
/** 添加参考图;返回实际加入的张数。名额在每张压缩完成后重算,并发添加也不会超过 MAX_REFS */
export async function addDraftRefs(files: Blob[]): Promise<number> {
  const images = files.filter(f => f.type.startsWith('image/'))
  if (!images.length) return 0
  const s = currentSession()
  let added = 0
  let dropped = 0
  const sidAtStart = activeSessionId.value
  for (const f of images) {
    if (!isCurrent(s)) return added
    if (draftRefs.value.length + pendingRefs >= MAX_REFS) { dropped++; continue }
    pendingRefs++
    let blob: Blob
    try {
      const file = f instanceof File ? f : new File([f], `ref.${(f.type.split('/')[1] || 'png')}`, { type: f.type })
      blob = await compressRefFile(file)
    } finally {
      pendingRefs--
    }
    // 压缩期间登出/切账号/切聊天会话:丢弃,不能进入新账号或新会话的草稿
    if (!isCurrent(s) || activeSessionId.value !== sidAtStart) return added
    if (draftRefs.value.length >= MAX_REFS) { dropped++; continue }
    draftRefs.value.push({ blob, url: URL.createObjectURL(blob) })
    added++
  }
  if (dropped) appStore().showError(t('imageStudio.maxRefs'))
  return added
}
/** 手动润色当前草稿:结果直接写回输入框,由用户决定修改或发送。 */
export async function polishDraft(): Promise<void> {
  const text = draftText.value.trim()
  if (!text || polishing.value) return
  const s = currentSession()
  const sid = activeSessionId.value
  const seq = ++polishSeq
  polishing.value = true
  try {
    const result = await aiEnhancePrompt('image', text, { keyId: aiKeyId.value ?? undefined, model: aiModel.value || undefined })
    if (seq !== polishSeq || !isCurrent(s) || activeSessionId.value !== sid) return
    if (draftText.value.trim() === text) {
      draftText.value = result
    } else {
      // 等待期间用户改了输入:不覆盖,提示后可再点一次
      appStore().showError(t('canvasChat.polishInputChanged'))
    }
  } catch (err) {
    if (seq === polishSeq && isCurrent(s)) {
      appStore().showError(`${t('canvasChat.polishFailed')}: ${err instanceof Error ? err.message : ''}`)
    }
  } finally {
    if (seq === polishSeq) polishing.value = false
  }
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
  const s = currentSession()
  const added = await addDraftRefs([toRaw(img.blob)])
  if (added && isCurrent(s)) appStore().showSuccess(t('imageStudio.refAdded'))
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
  // 旧记录/兜底模型没有 platform 时,仅识别 OpenAI 图像模型。
  const isOpenAI = turn.platform ? turn.platform === 'openai' : /^(gpt-image-|dall-e-)/i.test(turn.model)
  const outputFormat = isOpenAI && turn.format && turn.format !== 'auto' ? turn.format : undefined
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
        if (outputFormat) form.append('output_format', outputFormat)
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
            ...(outputFormat ? { output_format: outputFormat } : {}),
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

function findTurn(sid: string, turnId: string): ChatTurn | undefined {
  return turnStore.value[sid]?.find(x => x.id === turnId)
}

/** 为某轮生成指定的待定图片(并发 PARALLEL),每张完成即回填并落盘 */
async function runImages(turnId: string, imageIds: string[], run: Run) {
  if (!isLive(run)) return
  const turn = findTurn(run.sid, turnId)
  if (!turn) return
  const controller = new AbortController()
  controllers.set(turnId, controller)
  turn.status = 'running'
  turn.startedAt = Date.now()
  persist(turn, run)
  syncTicker()
  const started = performance.now()
  const promptText = turn.polished || turn.prompt
  const queue = [...imageIds]
  const worker = async () => {
    while (queue.length) {
      if (!isLive(run)) return
      const id = queue.shift()!
      const img = turn.images.find(x => x.id === id)
      if (!img) continue
      if (controller.signal.aborted) {
        img.status = 'failed'
        img.error = t('canvasChat.stopped')
        continue
      }
      const result = await generateOne(turn, promptText, run, controller.signal)
      // 请求返回时会话已清空/删除或账号已切换:结果作废,不改状态、不写库
      if (!isLive(run)) return
      const actualSize = result.blob ? await imageSizeOf(result.blob) : undefined
      if (!isLive(run)) return
      if (result.blob) {
        img.actualSize = actualSize
        img.blob = result.blob
        img.status = 'done'
        img.error = undefined
        const s = findSession(run.sid)
        if (s) s.imageCount++
      } else {
        img.status = 'failed'
        img.error = result.error
      }
      persist(turn, run)
    }
  }
  try {
    await Promise.all(Array.from({ length: Math.min(PARALLEL, queue.length) }, worker))
  } catch (err) {
    if (!(err instanceof StudioSessionChangedError)) throw err
  } finally {
    if (controllers.get(turnId) === controller) controllers.delete(turnId)
    syncTicker()
  }
  if (!isLive(run)) return
  turn.ms = Math.round(performance.now() - started)
  turn.status = controller.signal.aborted
    ? 'cancelled'
    : (turn.images.some(img => img.status === 'done') ? 'done' : 'failed')
  persist(turn, run)
  const s = findSession(run.sid)
  if (s) { s.lastTs = Math.max(s.lastTs, Date.now()); persistSession(run) }
  // 后台会话完成:打角标
  if (run.sid !== activeSessionId.value && !controller.signal.aborted) {
    setNotice(run.sid, turn.images.some(img => img.status === 'failed') ? 'error' : 'done')
  }
}

function newImages(n: number, kind?: 'variant'): ChatImage[] {
  return Array.from({ length: n }, () => ({ id: uid(), status: 'pending' as const, kind }))
}

/** 新轮次入会话:更新会话元信息并落库(首轮会让未保存会话正式落库) */
function pushTurn(turn: ChatTurn, run: Run) {
  turnStore.value[run.sid].push(turn)
  const s = findSession(run.sid)
  if (s) {
    if (!s.title) s.title = buildSessionTitle(turn.prompt)
    s.turnCount++
    s.lastTs = Date.now()
  }
  unsavedIds.delete(run.sid)
  persistSession(run)
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
  count.value = clampCount(count.value)
  // 提交即快照全部草稿与参数:润色等待期间用户继续编辑的内容属于下一条,不混入本次
  const snapshot = {
    text,
    refs: draftRefs.value.map(r => toRaw(r.blob)),
    option: { ...option },
    ratio: ratio.value,
    resolution: resolution.value,
    quality: quality.value,
    count: clampCount(count.value),
    format: format.value,
    aiKeyId: aiKeyId.value ?? undefined,
    aiModel: aiModel.value || undefined
  }
  draftText.value = ''
  clearDraftRefs()
  const run = runFor(activeSessionId.value)



  const turn: ChatTurn = {
    id: uid(),
    sessionId: run.sid,
    ts: Date.now(),
    prompt: snapshot.text,
    refs: snapshot.refs,
    model: snapshot.option.id,
    groupId: snapshot.option.group_id,
    platform: snapshot.option.platform,
    ratio: snapshot.ratio,
    size: sizeForRatio(snapshot.ratio, snapshot.resolution),
    quality: snapshot.quality,
    count: snapshot.count,
    format: snapshot.format,
    status: 'running',
    images: newImages(snapshot.count)
  }
  pushTurn(turn, run)
  await runImages(turn.id, turn.images.map(i => i.id), run)
}

/** 重绘:同参数(提示词/参考图/模型/画幅/质量/张数/格式)在同一会话新开一轮 */
export async function redrawTurn(source: ChatTurn) {
  if (busy.value) return
  const run = runFor(source.sessionId)
  if (!isLive(run)) return
  const turn: ChatTurn = {
    ...plain(source),
    id: uid(),
    ts: Date.now(),
    origin: 'redraw',
    status: 'running',
    ms: undefined,
    startedAt: undefined,
    count: clampCount(source.count),
    images: newImages(clampCount(source.count))
  }
  pushTurn(turn, run)
  await runImages(turn.id, turn.images.map(i => i.id), run)
}

/** 变体:沿用该轮提示词与参考图,在同一轮追加一张新图(新随机种子) */
export async function variantOf(turn: ChatTurn) {
  if (busy.value) return
  const run = runFor(turn.sessionId)
  const extra = newImages(1, 'variant')
  turn.images.push(...extra)
  await runImages(turn.id, extra.map(i => i.id), run)
}

/** 失败图重试:原位重新生成 */
export async function retryImage(turn: ChatTurn, img: ChatImage) {
  if (busy.value || img.status !== 'failed') return
  img.status = 'pending'
  img.error = undefined
  await runImages(turn.id, [img.id], runFor(turn.sessionId))
}

/** 停止当前会话的全部进行中轮次 */
export function stopAll() {
  abortSession(activeSessionId.value)
}

export async function deleteTurn(id: string) {
  if (controllers.has(id)) return
  const sid = Object.keys(turnStore.value).find(k => turnStore.value[k].some(x => x.id === id))
  if (!sid) return
  const run = runFor(sid)
  const target = turnStore.value[sid].find(x => x.id === id)!
  turnStore.value[sid] = turnStore.value[sid].filter(x => x.id !== id)
  releaseBlobs(blobsOf(target))
  const s = findSession(sid)
  if (s) {
    s.turnCount = Math.max(0, s.turnCount - 1)
    s.imageCount = Math.max(0, s.imageCount - target.images.filter(i => i.status === 'done').length)
    persistSession(run)
  }
  await enqueueWrite(() => dbTx(run.owner, ['turns'], 'readwrite', tx => tx.objectStore('turns').delete(id)))
    .catch(() => { if (isCurrent(run)) reportSaveError() })
}

/** 清空当前会话的轮次(会话本身保留,标题重置) */
export async function clearConversation() {
  const sid = activeSessionId.value
  if (!sid) return
  // 递增会话代际:旧轮次的 worker 在请求返回后不再改状态/写库
  bumpGen(sid)
  abortSession(sid)
  forgetControllers(sid)
  const list = turnStore.value[sid] ?? []
  turnStore.value = { ...turnStore.value, [sid]: [] }
  releaseBlobs(list.flatMap(blobsOf))
  olderMap.value = { ...olderMap.value, [sid]: false }
  clearNotice(sid)
  const s = findSession(sid)
  if (s) { s.title = ''; s.turnCount = 0; s.imageCount = 0 }
  const owner = sessionOwnerKey()
  if (unsavedIds.has(sid)) return
  const record = s ? { ...toRaw(s) } : null
  await enqueueWrite(() => dbTx(owner, ['turns', 'sessions'], 'readwrite', tx => {
    const range = IDBKeyRange.bound([sid, -Infinity], [sid, Infinity])
    const st = tx.objectStore('turns')
    const req = st.index('session_ts').openKeyCursor(range)
    req.onsuccess = () => {
      const c = req.result
      if (c) { st.delete(c.primaryKey); c.continue() }
    }
    if (record) tx.objectStore('sessions').put(record)
  })).catch(() => appStore().showError(t('canvasChat.saveFailed')))
}

export function downloadImage(turn: ChatTurn, img: ChatImage) {
  if (!img.blob) return
  const ext = (img.blob.type.split('/')[1] || 'png').replace('jpeg', 'jpg')
  const a = document.createElement('a')
  a.href = blobUrl(img.blob)
  a.download = `chat-draw-${turn.ts}-${turn.images.indexOf(img) + 1}.${ext}`
  a.click()
}

/** 登出/切换账号:中止请求并清空内存中的全部会话 */
export function resetChatDrawSession() {
  sessionEpoch++
  polishSeq++
  controllers.forEach(c => c.abort())
  controllers.clear()
  syncTicker()
  pins.clear()
  unsavedIds.clear()
  loadPromises.clear()
  sessionGen.clear()
  polishing.value = false
  sessionRecords.value = []
  turnStore.value = {}
  olderMap.value = {}
  sessionNotices.value = {}
  activeSessionId.value = ''
  draftText.value = ''
  clearDraftRefs()
  revokeAllUrls()
  initOwner = ''
  initPromise = null
  sessionsLoaded.value = false
}

if (typeof window !== 'undefined') {
  window.addEventListener('studio-session-ended', resetChatDrawSession)
}
