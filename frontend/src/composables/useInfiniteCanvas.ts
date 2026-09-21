/**
 * 创作中心数据层(模块级单例)。
 *
 * IndexedDB `canvas-db`:
 *   - prompts: { id, name, category, content, builtin?, updatedAt }  提示词库
 */
import { ref } from 'vue'
import { i18n } from '@/i18n'
import { useAppStore } from '@/stores/app'

const t = i18n.global.t

// ===== 类型 =====
export interface PromptItem {
  id: string
  name: string
  category: string
  content: string
  builtin?: boolean
  updatedAt: number
}

// ===== 状态 =====
export const activeTab = ref<'image' | 'video' | 'prompts' | 'assets' | 'config'>('image')

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
