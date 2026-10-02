import { StudioSessionChangedError } from './studioSession'

export type StudioKind = 'image' | 'video'
const IMPORT_MARKER = 'legacy-history-imported'
const METADATA = 'metadata'

export function openStudioHistoryDb(kind: StudioKind, owner: string): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(`${kind}-studio-db-${owner}`, 2)
    let blocked = false
    req.onupgradeneeded = () => {
      if (!req.result.objectStoreNames.contains('history')) {
        req.result.createObjectStore('history', { keyPath: 'id' })
      }
      if (!req.result.objectStoreNames.contains(METADATA)) req.result.createObjectStore(METADATA)
    }
    req.onsuccess = () => {
      if (blocked) { req.result.close(); return }
      req.result.onversionchange = () => req.result.close()
      resolve(req.result)
    }
    req.onerror = () => reject(req.error)
    req.onblocked = () => {
      blocked = true
      reject(new Error('Studio history database upgrade blocked'))
    }
  })
}

// Abort creation when the old database does not exist. Checking for recovery
// must not leave an empty legacy database in every new user's browser.
function openLegacyDb(kind: StudioKind): Promise<IDBDatabase | null> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(`${kind}-studio-db`)
    let missing = false
    req.onupgradeneeded = () => {
      missing = true
      req.transaction?.abort()
    }
    req.onerror = () => missing ? resolve(null) : reject(req.error)
    req.onsuccess = () => {
      if (!req.result.objectStoreNames.contains('history')) {
        req.result.close()
        resolve(null)
        return
      }
      req.result.onversionchange = () => req.result.close()
      resolve(req.result)
    }
  })
}

function readRequest<T>(request: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error)
  })
}

export async function hasLegacyStudioHistory(kind: StudioKind, owner: string): Promise<boolean> {
  const legacy = await openLegacyDb(kind)
  if (!legacy) return false
  try {
    const count = await readRequest(legacy.transaction('history').objectStore('history').count())
    if (!count) return false
    const target = await openStudioHistoryDb(kind, owner)
    try {
      return !await readRequest(target.transaction(METADATA).objectStore(METADATA).get(IMPORT_MARKER))
    } finally {
      target.close()
    }
  } finally {
    legacy.close()
  }
}

function validRecord(kind: StudioKind, value: unknown): value is Record<string, unknown> {
  if (!value || typeof value !== 'object') return false
  const item = value as Record<string, unknown>
  if (typeof item.ts !== 'number' || typeof item.prompt !== 'string' || typeof item.model !== 'string') return false
  if (kind === 'video') return item.blob instanceof Blob
  return Array.isArray(item.images) && item.images.length > 0 && item.images.every(image =>
    image && (image.failed || image.blob instanceof Blob))
}

// Legacy records have no owner, so this must only run after explicit user
// confirmation. Keep the source as a backup. The target transaction atomically
// stores records and the one-time marker, including across concurrent tabs.
export async function importLegacyStudioHistory(
  kind: StudioKind,
  owner: string,
  isCurrentSession: () => boolean
): Promise<number> {
  const checkSession = () => {
    if (!isCurrentSession()) throw new StudioSessionChangedError()
  }
  checkSession()
  const legacy = await openLegacyDb(kind)
  if (!legacy) return 0
  let items: Record<string, unknown>[]
  try {
    const records: unknown[] = await readRequest(legacy.transaction('history').objectStore('history').getAll())
    items = records.filter(item => validRecord(kind, item))
  } finally {
    legacy.close()
  }
  checkSession()
  const target = await openStudioHistoryDb(kind, owner)
  try {
    checkSession()
    return await new Promise<number>((resolve, reject) => {
      const tx = target.transaction(['history', METADATA], 'readwrite')
      const history = tx.objectStore('history')
      const metadata = tx.objectStore(METADATA)
      let imported = 0
      let failure: unknown
      const abortOnSessionChange = () => {
        if (!isCurrentSession()) {
          failure = new StudioSessionChangedError()
          tx.abort()
        }
      }
      window.addEventListener('studio-session-ended', abortOnSessionChange)
      const cleanup = () => window.removeEventListener('studio-session-ended', abortOnSessionChange)
      tx.oncomplete = () => { cleanup(); resolve(imported) }
      tx.onabort = () => { cleanup(); reject(failure || tx.error || new Error('History import aborted')) }
      const marker = metadata.get(IMPORT_MARKER)
      marker.onsuccess = () => {
        if (marker.result) return
        const keys = history.getAllKeys()
        keys.onsuccess = () => {
          try {
            checkSession()
            // New generations use positive Date.now() IDs. Keep recovered IDs
            // negative so later saves cannot overwrite a restored record.
            let id = -1
            for (const key of keys.result) {
              if (typeof key === 'number' && key <= id) id = key - 1
            }
            for (const item of items) history.add({ ...item, id: id-- })
            metadata.put(true, IMPORT_MARKER)
            imported = items.length
          } catch (error) {
            failure = error
            tx.abort()
          }
        }
      }
    })
  } finally {
    target.close()
  }
}
