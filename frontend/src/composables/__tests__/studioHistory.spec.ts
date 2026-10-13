import { Blob as NodeBlob } from 'node:buffer'
import { IDBFactory, IDBObjectStore } from 'fake-indexeddb'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { hasLegacyStudioHistory, importLegacyStudioHistory, openStudioHistoryDb, type StudioKind } from '../studioHistory'

function record(kind: StudioKind, id: number, prompt = 'old prompt') {
  const blob = new Blob(['saved media'], { type: kind === 'image' ? 'image/png' : 'video/mp4' })
  return {
    id, ts: 1234, model: `${kind}-model`, prompt, starred: true,
    ...(kind === 'image'
      ? { images: [{ blob, failed: false }], mode: 't2i', quality: 'auto', format: 'png', size: 'auto' }
      : { blob, seconds: 6, resolution: '720p', sizeMB: 0.1 })
  }
}

async function seed(name: string, items: object[]) {
  const db = await new Promise<IDBDatabase>((resolve, reject) => {
    const req = indexedDB.open(name, 1)
    req.onupgradeneeded = () => req.result.createObjectStore('history', { keyPath: 'id' })
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
  })
  try {
    await new Promise<void>((resolve, reject) => {
      const tx = db.transaction('history', 'readwrite')
      items.forEach(item => tx.objectStore('history').put(item))
      tx.oncomplete = () => resolve()
      tx.onabort = () => reject(tx.error)
    })
  } finally { db.close() }
}

async function records(name: string): Promise<Record<string, unknown>[]> {
  const db = await new Promise<IDBDatabase>((resolve, reject) => {
    const req = indexedDB.open(name)
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
  })
  try {
    return await new Promise((resolve, reject) => {
      const req = db.transaction('history').objectStore('history').getAll()
      req.onsuccess = () => resolve(req.result)
      req.onerror = () => reject(req.error)
    })
  } finally { db.close() }
}

describe.each<StudioKind>(['image', 'video'])('%s studio legacy history recovery', kind => {
  const legacyName = `${kind}-studio-db`
  const targetName = `${legacyName}-user-1`

  beforeEach(() => {
    vi.stubGlobal('indexedDB', new IDBFactory())
    // Native Blob supports the structured clone used by IndexedDB.
    vi.stubGlobal('Blob', NodeBlob)
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('does not create a legacy database while checking a fresh browser', async () => {
    expect(await hasLegacyStudioHistory(kind, 'user-1')).toBe(false)
    expect(await indexedDB.databases()).toEqual([])
  })

  it('preserves blobs and metadata, avoids ID collisions, and imports once into the chosen account', async () => {
    const id = Date.now()
    await seed(legacyName, [record(kind, id), record(kind, id + 1, 'second prompt')])
    await seed(targetName, [record(kind, id, 'new history')])
    expect(await hasLegacyStudioHistory(kind, 'user-1')).toBe(true)
    // Discovery upgrades v1 but never imports or exposes legacy records.
    expect((await records(targetName)).map(item => item.prompt)).toEqual(['new history'])

    expect(await importLegacyStudioHistory(kind, 'user-1', () => true)).toBe(2)
    const imported = await records(targetName)
    expect(imported).toHaveLength(3)
    expect(new Set(imported.map(item => item.id)).size).toBe(3)
    const old = imported.find(item => item.prompt === 'old prompt')!
    expect(old).toMatchObject({ ts: 1234, model: `${kind}-model`, starred: true })
    const blob = kind === 'image' ? (old.images as { blob: Blob }[])[0].blob : old.blob as Blob
    expect(await blob.text()).toBe('saved media')
    expect((await records(legacyName))).toHaveLength(2)
    expect(await hasLegacyStudioHistory(kind, 'user-1')).toBe(false)
    expect(await importLegacyStudioHistory(kind, 'user-1', () => true)).toBe(0)
    expect(await records(targetName)).toHaveLength(3)

    const target = await openStudioHistoryDb(kind, 'user-1')
    await new Promise<void>((resolve, reject) => {
      const tx = target.transaction('history', 'readwrite')
      tx.objectStore('history').put(record(kind, Date.now() + 1, 'generated after import'))
      tx.oncomplete = () => resolve()
      tx.onabort = () => reject(tx.error)
    })
    target.close()
    expect(await records(targetName)).toHaveLength(4)

    const other = await openStudioHistoryDb(kind, 'user-2')
    other.close()
    expect(await records(`${legacyName}-user-2`)).toEqual([])
  })

  it('serializes concurrent imports and commits the one-time marker with the records', async () => {
    await seed(legacyName, [record(kind, 1)])
    const counts = await Promise.all([
      importLegacyStudioHistory(kind, 'user-1', () => true),
      importLegacyStudioHistory(kind, 'user-1', () => true)
    ])
    expect(counts.sort()).toEqual([0, 1])
    expect(await records(targetName)).toHaveLength(1)
  })

  it('rolls back a failed import without losing the source or marking it imported', async () => {
    await seed(legacyName, [record(kind, 1), record(kind, 2)])
    const add = IDBObjectStore.prototype.add
    let writes = 0
    const spy = vi.spyOn(IDBObjectStore.prototype, 'add').mockImplementation(function (value, key) {
      if (++writes === 2) throw new DOMException('Storage full', 'QuotaExceededError')
      return add.call(this, value, key)
    })
    await expect(importLegacyStudioHistory(kind, 'user-1', () => true)).rejects.toThrow('Storage full')
    spy.mockRestore()

    expect(await records(targetName)).toEqual([])
    expect(await records(legacyName)).toHaveLength(2)
    expect(await hasLegacyStudioHistory(kind, 'user-1')).toBe(true)
    expect(await importLegacyStudioHistory(kind, 'user-1', () => true)).toBe(2)
  })

  it('aborts a pending import when the session ends', async () => {
    await seed(legacyName, [record(kind, 1)])
    let current = true
    const add = IDBObjectStore.prototype.add
    const spy = vi.spyOn(IDBObjectStore.prototype, 'add').mockImplementation(function (value, key) {
      const req = add.call(this, value, key)
      req.addEventListener('success', () => {
        current = false
        window.dispatchEvent(new Event('studio-session-ended'))
      })
      return req
    })
    await expect(importLegacyStudioHistory(kind, 'user-1', () => current))
      .rejects.toMatchObject({ name: 'StudioSessionChangedError' })
    spy.mockRestore()
    expect(await records(targetName)).toEqual([])
    expect(await records(legacyName)).toHaveLength(1)
    expect(await hasLegacyStudioHistory(kind, 'user-1')).toBe(true)
  })
})
