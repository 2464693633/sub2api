import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { IDBFactory } from 'fake-indexeddb'
import { ref } from 'vue'

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
const tick = () => new Promise(r => setTimeout(r, 0))
async function settle(n = 20) { for (let i = 0; i < n; i++) await tick() }

const h = vi.hoisted(() => ({
  compress: vi.fn(async (f: File) => f),
  enhance: vi.fn(async (_k: string, p: string) => p),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/i18n', () => ({ i18n: { global: { t: (key: string) => key } } }))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: h.showSuccess, showError: h.showError })
}))
vi.mock('../useImageStudioEngine', () => ({
  MAX_REFS: 8,
  QUALITIES: ['auto', 'high', 'medium', 'low'],
  models: ref([{ id: 'gpt-image-2', group_id: 1, group_name: 'g' }]),
  model: ref('gpt-image-2'),
  initError: ref(''),
  loadModels: vi.fn(async () => undefined),
  blobFromB64: (b64: string) => new Blob([b64], { type: 'image/png' }),
  compressRefFile: h.compress
}))
vi.mock('../useAIPrompt', () => ({
  aiEnhancePrompt: h.enhance,
  aiKeyId: ref(1),
  aiModel: ref('chat')
}))

function switchAccount(id: number) {
  const oldValue = localStorage.getItem('auth_user')
  const newValue = JSON.stringify({ id })
  localStorage.setItem('auth_user', newValue)
  localStorage.setItem('auth_token', `token-${id}`)
  window.dispatchEvent(new StorageEvent('storage', { key: 'auth_user', oldValue, newValue }))
}
const png = (name = 'a.png') => new File(['x'], name, { type: 'image/png' })

describe('chat draw engine races', () => {
  beforeEach(() => {
    vi.resetModules()
    localStorage.clear()
    localStorage.setItem('auth_user', JSON.stringify({ id: 1 }))
    localStorage.setItem('auth_token', 'token-1')
    vi.stubGlobal('indexedDB', new IDBFactory())
    vi.stubGlobal('URL', class extends URL {
      static createObjectURL = vi.fn().mockReturnValue('blob:test')
      static revokeObjectURL = vi.fn()
    })
    vi.stubGlobal('createImageBitmap', vi.fn(async () => ({ width: 1, height: 1, close() {} })))
    h.compress.mockReset().mockImplementation(async (f: File) => f)
    h.enhance.mockReset().mockImplementation(async (_k: string, p: string) => p)
    h.showError.mockReset()
  })
  afterEach(() => {
    window.dispatchEvent(new Event('studio-session-ended'))
    vi.unstubAllGlobals()
  })

  it('P1-1 drops refs whose compression finishes after an account switch', async () => {
    const e = await import('../useChatDrawEngine')
    const gate = deferred<File>()
    h.compress.mockImplementationOnce(() => gate.promise)
    const pending = e.addDraftRefs([png()])
    switchAccount(2)
    gate.resolve(png())
    await pending
    expect(e.draftRefs.value).toHaveLength(0)
  })

  it('P2-6 concurrent adds never exceed MAX_REFS', async () => {
    const e = await import('../useChatDrawEngine')
    const gates = Array.from({ length: 12 }, () => deferred<File>())
    gates.forEach(g => h.compress.mockImplementationOnce(() => g.promise))
    const a = e.addDraftRefs(Array.from({ length: 6 }, () => png()))
    const b = e.addDraftRefs(Array.from({ length: 6 }, () => png()))
    for (let i = 0; i < 12; i++) { gates[i].resolve(png()); await tick() }
    await Promise.all([a, b])
    expect(e.draftRefs.value.length).toBeLessThanOrEqual(8)
    expect(h.showError).toHaveBeenCalledWith('imageStudio.maxRefs')
  })

  it('P2-3 blocks sending until the first load finishes and keeps new turns', async () => {
    const e = await import('../useChatDrawEngine')
    vi.stubGlobal('fetch', vi.fn(() => new Promise(() => undefined)))
    const first = e.initChatDraw()
    const second = e.initChatDraw()
    e.draftText.value = 'hello'
    expect(e.busy.value).toBe(true)
    await e.sendDraft()
    expect(e.turns.value).toHaveLength(0)
    await Promise.all([first, second])
    expect(e.loaded.value).toBe(true)
    void e.sendDraft()
    await settle()
    expect(e.turns.value).toHaveLength(1)
    // Returning to the tab must not re-read the DB and wipe the running turn.
    await e.initChatDraw()
    expect(e.turns.value).toHaveLength(1)
  })

  it('P2-2 cleared turns do not come back after their request aborts', async () => {
    const e = await import('../useChatDrawEngine')
    await e.initChatDraw()
    vi.stubGlobal('fetch', vi.fn((_u: string, init: RequestInit) => new Promise((_res, rej) => {
      init.signal?.addEventListener('abort', () => rej(new DOMException('aborted', 'AbortError')))
    })))
    e.draftText.value = 'cat'
    const sending = e.sendDraft()
    await settle()
    await e.clearConversation()
    await sending
    await settle()
    expect(e.turns.value).toHaveLength(0)
    window.dispatchEvent(new Event('studio-session-ended'))
    await e.initChatDraw()
    expect(e.turns.value).toHaveLength(0)
  })

  it('P2-4 an old account polish finishing does not unlock the new account', async () => {
    const e = await import('../useChatDrawEngine')
    await e.initChatDraw()
    e.polishEnabled.value = true
    const oldPolish = deferred<string>()
    h.enhance.mockImplementationOnce(() => oldPolish.promise)
    e.draftText.value = 'old'
    const oldSend = e.sendDraft()
    switchAccount(2)
    await e.initChatDraw()
    const newPolish = deferred<string>()
    h.enhance.mockImplementationOnce(() => newPolish.promise)
    e.draftText.value = 'new'
    const newSend = e.sendDraft()
    expect(e.polishing.value).toBe(true)
    oldPolish.resolve('old polished')
    await oldSend
    expect(e.polishing.value).toBe(true)
    expect(e.busy.value).toBe(true)
    vi.stubGlobal('fetch', vi.fn(() => new Promise(() => undefined)))
    newPolish.resolve('new polished')
    await settle()
    expect(e.turns.value.map(x => x.prompt)).toEqual(['new'])
    void newSend
  })

  it('P2-7 edits made while polishing are not mixed into the request or cleared', async () => {
    const e = await import('../useChatDrawEngine')
    await e.initChatDraw()
    vi.stubGlobal('fetch', vi.fn(() => new Promise(() => undefined)))
    await e.addDraftRefs([png('first.png')])
    e.polishEnabled.value = true
    const polish = deferred<string>()
    h.enhance.mockImplementationOnce(() => polish.promise)
    e.draftText.value = 'first'
    const sending = e.sendDraft()
    e.draftText.value = 'second draft'
    await e.addDraftRefs([png('second.png')])
    polish.resolve('first polished')
    await settle()
    expect(e.turns.value[0].prompt).toBe('first')
    expect(e.turns.value[0].refs).toHaveLength(1)
    expect((e.turns.value[0].refs[0] as File).name).toBe('first.png')
    expect(e.draftText.value).toBe('second draft')
    expect(e.draftRefs.value).toHaveLength(1)
    void sending
  })
})
