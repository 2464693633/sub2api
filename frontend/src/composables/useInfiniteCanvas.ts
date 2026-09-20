/**
 * 创作中心数据层(模块级单例)。
 *
 * IndexedDB `canvas-db`:
 *   - canvases: { id, name, nodes, edges, viewport, updatedAt }
 *     nodes: [{ id, type: 'image'|'text'|'video', x, y, w, h, assetId?, text? }]
 *     edges: [{ id, from, to }]
 *   - assets:  { id, blob }        图片/视频节点的二进制
 *   - prompts: { id, name, category, content, builtin?, updatedAt }  提示词库
 */
import { computed, ref } from 'vue'
import { i18n } from '@/i18n'
import { useAppStore } from '@/stores/app'

const t = i18n.global.t

// ===== 类型 =====
export interface CanvasNode {
  id: string
  type: 'image' | 'text' | 'video'
  x: number
  y: number
  w: number
  h: number
  assetId?: string
  text?: string
}
export interface CanvasEdge {
  id: string
  from: string
  to: string
}
export interface CanvasDoc {
  id: string
  name: string
  nodes: CanvasNode[]
  edges: CanvasEdge[]
  viewport: { x: number; y: number; scale: number }
  updatedAt: number
}
export interface PromptItem {
  id: string
  name: string
  category: string
  content: string
  builtin?: boolean
  updatedAt: number
}

// ===== 状态 =====
export const canvases = ref<CanvasDoc[]>([])
export const currentCanvasId = ref<string | null>(null)
export const currentCanvas = computed(() => canvases.value.find(c => c.id === currentCanvasId.value) || null)
export const activeTab = ref<'board' | 'gallery' | 'image' | 'video' | 'prompts' | 'assets' | 'config'>('image')

export const prompts = ref<PromptItem[]>([])

function appStore() {
  return useAppStore()
}

export function uid(): string {
  return Date.now().toString(36) + Math.random().toString(36).slice(2, 8)
}

// ===== IndexedDB =====
function openDb(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open('canvas-db', 3)
    req.onupgradeneeded = () => {
      const db = req.result
      if (!db.objectStoreNames.contains('canvases')) db.createObjectStore('canvases', { keyPath: 'id' })
      if (!db.objectStoreNames.contains('assets')) db.createObjectStore('assets', { keyPath: 'id' })
      if (!db.objectStoreNames.contains('prompts')) db.createObjectStore('prompts', { keyPath: 'id' })
      if (!db.objectStoreNames.contains('online-prompts')) db.createObjectStore('online-prompts', { keyPath: 'id' })
    }
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
  })
}
function tx<T>(store: string, mode: IDBTransactionMode, run: (s: IDBObjectStore) => IDBRequest<T>): Promise<T> {
  return openDb().then(db => new Promise<T>((resolve, reject) => {
    const t2 = db.transaction(store, mode)
    const req = run(t2.objectStore(store))
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
    t2.oncomplete = () => db.close()
  }))
}

// ===== 画布 CRUD =====
export async function loadCanvases() {
  try {
    const items = await tx<CanvasDoc[]>('canvases', 'readonly', s => s.getAll() as IDBRequest<CanvasDoc[]>)
    canvases.value = items.sort((a, b) => b.updatedAt - a.updatedAt)
    // 恢复上次使用的画布(持久化),否则默认最近一个
    const saved = localStorage.getItem('canvas_current_id')
    if (saved && canvases.value.some(c => c.id === saved)) {
      currentCanvasId.value = saved
    } else if (!currentCanvasId.value || !canvases.value.some(c => c.id === currentCanvasId.value)) {
      currentCanvasId.value = canvases.value[0]?.id ?? null
    }
    if (currentCanvasId.value) localStorage.setItem('canvas_current_id', currentCanvasId.value)
  } catch {
    canvases.value = []
  }
}

export async function createCanvas(name?: string): Promise<CanvasDoc> {
  const doc: CanvasDoc = {
    id: uid(),
    name: name || t('canvas.untitled'),
    nodes: [],
    edges: [],
    viewport: { x: 0, y: 0, scale: 1 },
    updatedAt: Date.now()
  }
  await tx('canvases', 'readwrite', s => s.put(doc))
  await loadCanvases()
  currentCanvasId.value = doc.id
  localStorage.setItem('canvas_current_id', doc.id)
  return doc
}

export async function saveCanvas(doc: CanvasDoc) {
  doc.updatedAt = Date.now()
  // IndexedDB 不能 structured-clone Vue 响应式 Proxy,必须深拷贝成纯对象
  const plain = JSON.parse(JSON.stringify(doc)) as CanvasDoc
  await tx('canvases', 'readwrite', s => s.put(plain))
}

export async function renameCanvas(id: string, name: string) {
  const doc = canvases.value.find(c => c.id === id)
  if (!doc) return
  doc.name = name
  await saveCanvas(doc)
}

export async function deleteCanvas(id: string) {
  await tx('canvases', 'readwrite', s => s.delete(id))
  const doc = canvases.value.find(c => c.id === id)
  if (doc) {
    for (const n of doc.nodes) {
      const aid = n.assetId
      if (aid) await tx('assets', 'readwrite', s => s.delete(aid)).catch(() => undefined)
    }
  }
  if (currentCanvasId.value === id) currentCanvasId.value = null
  await loadCanvases()
}

export async function openCanvas(id: string) {
  currentCanvasId.value = id
  activeTab.value = 'board'
}

// ===== 资产 =====
export async function putAsset(id: string, blob: Blob) {
  await tx('assets', 'readwrite', s => s.put({ id, blob }))
}
export async function getAsset(id: string): Promise<Blob | undefined> {
  const rec = await tx<{ id: string; blob: Blob } | undefined>('assets', 'readonly', s => s.get(id) as IDBRequest<{ id: string; blob: Blob } | undefined>)
  return rec?.blob
}

/** 把一张图/一段视频加入当前画布(无画布时自动创建),返回是否成功 */
export async function addMediaToCurrentCanvas(blob: Blob, kind: 'image' | 'video'): Promise<boolean> {
  if (!currentCanvasId.value) {
    const doc = await createCanvas(t('canvas.myCanvas'))
    currentCanvasId.value = doc.id
  }
  const doc = canvases.value.find(c => c.id === currentCanvasId.value)
  if (!doc) return false
  const assetId = uid()
  await putAsset(assetId, blob)
  let w = 320
  let h = 320
  if (kind === 'image') {
    const bmp = await createImageBitmap(blob).catch(() => null)
    if (bmp) {
      w = Math.min(360, bmp.width)
      h = Math.round(w * bmp.height / bmp.width)
      bmp.close()
    }
  } else {
    h = Math.round(w * 9 / 16)
  }
  const count = doc.nodes.length
  doc.nodes.push({ id: uid(), type: kind, x: 80 + (count % 5) * 40, y: 80 + (count % 5) * 30, w, h, assetId })
  await saveCanvas(doc)
  appStore().showSuccess(t('canvas.addedToCanvas', { name: doc.name }))
  return true
}

/** 兼容旧调用:图片加入画布 */
export async function addImageToCurrentCanvas(blob: Blob): Promise<boolean> {
  return addMediaToCurrentCanvas(blob, 'image')
}

// ===== 导出 / 导入 =====
export async function exportCanvas(id: string) {
  const doc = canvases.value.find(c => c.id === id)
  if (!doc) return
  const assets: Record<string, string> = {}
  for (const n of doc.nodes) {
    if (n.assetId && !assets[n.assetId]) {
      const blob = await getAsset(n.assetId)
      if (blob) {
        assets[n.assetId] = await new Promise<string>(resolve => {
          const reader = new FileReader()
          reader.onload = () => resolve(String(reader.result))
          reader.readAsDataURL(blob)
        })
      }
    }
  }
  const payload = { version: 1, canvas: doc, assets }
  const blob = new Blob([JSON.stringify(payload)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `canvas-${doc.name}-${doc.id}.json`
  a.click()
  window.setTimeout(() => URL.revokeObjectURL(url), 30000)
}

export async function importCanvas(file: File): Promise<boolean> {
  try {
    const text = await file.text()
    const payload = JSON.parse(text) as { version: number; canvas: CanvasDoc; assets: Record<string, string> }
    if (!payload?.canvas?.id) return false
    const doc: CanvasDoc = {
      ...payload.canvas,
      id: uid(),
      name: payload.canvas.name + ' ' + t('canvas.importedTag'),
      updatedAt: Date.now()
    }
    for (const [assetId, dataUrl] of Object.entries(payload.assets || {})) {
      const res = await fetch(dataUrl)
      await putAsset(assetId, await res.blob())
    }
    await tx('canvases', 'readwrite', s => s.put(doc))
    await loadCanvases()
    appStore().showSuccess(t('canvas.importDone', { name: doc.name }))
    return true
  } catch {
    appStore().showError(t('canvas.importFailed'))
    return false
  }
}

// ===== 提示词库 =====
const BUILTIN_PROMPTS: PromptItem[] = [
  { id: 'b1', name: t('canvas.p.applePosterName'), category: t('canvas.p.work'), builtin: true, updatedAt: 0, content: t('canvas.p.applePoster') },
  { id: 'b2', name: t('canvas.p.cityIslandName'), category: t('canvas.p.art'), builtin: true, updatedAt: 0, content: t('canvas.p.cityIsland') },
  { id: 'b3', name: t('canvas.p.stickerName'), category: t('canvas.p.fun'), builtin: true, updatedAt: 0, content: t('canvas.p.sticker') },
  { id: 'b4', name: t('canvas.p.doodleInfographicName'), category: t('canvas.p.study'), builtin: true, updatedAt: 0, content: t('canvas.p.doodleInfographic') },
  { id: 'b5', name: t('canvas.p.mindMapName'), category: t('canvas.p.study'), builtin: true, updatedAt: 0, content: t('canvas.p.mindMap') },
  { id: 'b6', name: t('canvas.p.nokiaName'), category: t('canvas.p.fun'), builtin: true, updatedAt: 0, content: t('canvas.p.nokia') },
  { id: 'b7', name: t('canvas.p.recipeName'), category: t('canvas.p.life'), builtin: true, updatedAt: 0, content: t('canvas.p.recipe') },
  { id: 'b8', name: t('canvas.p.roastName'), category: t('canvas.p.fun'), builtin: true, updatedAt: 0, content: t('canvas.p.roast') },
  { id: 'b9', name: t('canvas.p.glassPptName'), category: t('canvas.p.work'), builtin: true, updatedAt: 0, content: t('canvas.p.glassPpt') },
  { id: 'b10', name: t('canvas.p.videoCamName'), category: t('canvas.p.video'), builtin: true, updatedAt: 0, content: t('canvas.p.videoCam') }
]

export async function loadPrompts() {
  try {
    const items = await tx<PromptItem[]>('prompts', 'readonly', s => s.getAll() as IDBRequest<PromptItem[]>)
    prompts.value = [...BUILTIN_PROMPTS, ...items].sort((a, b) => b.updatedAt - a.updatedAt)
  } catch {
    prompts.value = [...BUILTIN_PROMPTS]
  }
}

export async function addPrompt(name: string, category: string, content: string) {
  const item: PromptItem = { id: uid(), name, category: category || t('canvas.p.uncategorized'), content, updatedAt: Date.now() }
  await tx('prompts', 'readwrite', s => s.put(item))
  await loadPrompts()
  appStore().showSuccess(t('canvas.promptSaved'))
}
export async function updatePrompt(item: PromptItem) {
  if (item.builtin) return
  item.updatedAt = Date.now()
  await tx('prompts', 'readwrite', s => s.put(item))
  await loadPrompts()
}
export async function deletePrompt(id: string) {
  await tx('prompts', 'readwrite', s => s.delete(id))
  await loadPrompts()
}
