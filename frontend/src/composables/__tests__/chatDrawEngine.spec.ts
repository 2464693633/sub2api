import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { IDBFactory, IDBKeyRange } from 'fake-indexeddb'
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
    vi.stubGlobal('IDBKeyRange', IDBKeyRange)
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

  it('polishDraft: 旧账号的润色完成不得写入新账号输入框', async () => {
    const e = await import('../useChatDrawEngine')
    await e.initChatDraw()
    e.draftText.value = 'old'
    const oldPolish = deferred<string>()
    h.enhance.mockImplementationOnce(() => oldPolish.promise)
    const p1 = e.polishDraft()
    switchAccount(2)
    await e.initChatDraw()
    e.draftText.value = 'new'
    const newPolish = deferred<string>()
    h.enhance.mockImplementationOnce(() => newPolish.promise)
    const p2 = e.polishDraft()
    expect(e.polishing.value).toBe(true)
    oldPolish.resolve('old polished')
    await p1
    expect(e.draftText.value).toBe('new')
    newPolish.resolve('new polished')
    await p2
    expect(e.draftText.value).toBe('new polished')
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

/** 每次请求返回一张图;gate 不为空时按顺序等待放行 */
function okFetch(gates: Array<Promise<void>> = []) {
  let i = 0
  return vi.fn(async (_u: string, init: RequestInit) => {
    const gate = gates[i++]
    if (gate) {
      await new Promise<void>((res, rej) => {
        init.signal?.addEventListener('abort', () => rej(new DOMException('aborted', 'AbortError')))
        void gate.then(res)
      })
    }
    return new Response(JSON.stringify({ data: [{ b64_json: 'AAAA' }] }), { status: 200 })
  })
}

describe('chat draw engine sessions', () => {
  let urlSeq = 0
  beforeEach(() => {
    vi.resetModules()
    localStorage.clear()
    localStorage.setItem('auth_user', JSON.stringify({ id: 1 }))
    localStorage.setItem('auth_token', 'token-1')
    vi.stubGlobal('indexedDB', new IDBFactory())
    vi.stubGlobal('IDBKeyRange', IDBKeyRange)
    urlSeq = 0
    vi.stubGlobal('URL', class extends URL {
      static createObjectURL = vi.fn(() => `blob:test-${++urlSeq}`)
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

  async function send(e: typeof import('../useChatDrawEngine'), text: string) {
    e.draftText.value = text
    await e.sendDraft()
    await settle()
  }
  async function reload(e: typeof import('../useChatDrawEngine')) {
    window.dispatchEvent(new Event('studio-session-ended'))
    await e.initChatDraw()
    await settle()
  }

  it('starts with one unsaved session that is not persisted until the first turn', async () => {
    const e = await import('../useChatDrawEngine')
    await e.initChatDraw()
    expect(e.sessions.value).toHaveLength(1)
    expect(e.activeSession.value?.title).toBe('')
    // 空会话不落库:重载后仍只有一个新的内存会话
    const first = e.activeSessionId.value
    await reload(e)
    expect(e.sessions.value).toHaveLength(1)
    expect(e.activeSessionId.value).not.toBe(first)
  })

  it('titles a session from its first prompt and persists sessions with counts', async () => {
    const e = await import('../useChatDrawEngine')
    await e.initChatDraw()
    vi.stubGlobal('fetch', okFetch())
    const long = '一只在月光下奔跑的橘色小猫穿过竹林和溪流'
    await send(e, long)
    expect(e.activeSession.value?.title).toBe(e.buildSessionTitle(long))
    expect(e.activeSession.value?.title.endsWith('…')).toBe(true)
    await send(e, 'second prompt')
    // 标题只取首轮
    expect(e.activeSession.value?.title).toBe(e.buildSessionTitle(long))
    expect(e.activeSession.value?.turnCount).toBe(2)
    expect(e.activeSession.value?.imageCount).toBe(2)
    expect(e.totalImages.value).toBe(2)
    await reload(e)
    expect(e.sessions.value).toHaveLength(1)
    expect(e.turns.value).toHaveLength(2)
    expect(e.totalImages.value).toBe(2)
  })

  it('creates, switches and restores sessions with their own turns', async () => {
    const e = await import('../useChatDrawEngine')
    await e.initChatDraw()
    vi.stubGlobal('fetch', okFetch())
    await send(e, 'alpha')
    const a = e.activeSessionId.value
    const b = e.newSession()
    expect(b).not.toBe(a)
    expect(e.turns.value).toHaveLength(0)
    // 空的新会话上再点"新对话"复用它
    expect(e.newSession()).toBe(b)
    await send(e, 'beta')
    expect(e.sessions.value.map(s => s.id)).toEqual([b, a])
    await e.selectSession(a)
    expect(e.turns.value.map(x => x.prompt)).toEqual(['alpha'])
    await e.selectSession(b)
    expect(e.turns.value.map(x => x.prompt)).toEqual(['beta'])
    await reload(e)
    // 记住上次所在会话
    expect(e.activeSessionId.value).toBe(b)
    expect(e.turns.value.map(x => x.prompt)).toEqual(['beta'])
  })

  it('drops an empty unsaved session when switching away', async () => {
    const e = await import('../useChatDrawEngine')
    await e.initChatDraw()
    vi.stubGlobal('fetch', okFetch())
    await send(e, 'alpha')
    const a = e.activeSessionId.value
    e.newSession()
    expect(e.sessions.value).toHaveLength(2)
    await e.selectSession(a)
    expect(e.sessions.value.map(s => s.id)).toEqual([a])
  })

  it('keeps a background session running and lands results in it with a notice', async () => {
    const e = await import('../useChatDrawEngine')
    await e.initChatDraw()
    const gate = deferred<void>()
    vi.stubGlobal('fetch', okFetch([gate.promise]))
    e.draftText.value = 'slow'
    const sending = e.sendDraft()
    await settle()
    const a = e.activeSessionId.value
    expect(e.runningSessionIds.value.has(a)).toBe(true)
    const b = e.newSession()
    // 另一个会话不受 a 的生成阻塞
    expect(e.busy.value).toBe(false)
    gate.resolve()
    await sending
    await settle()
    expect(e.activeSessionId.value).toBe(b)
    expect(e.turns.value).toHaveLength(0)
    expect(e.sessionNotices.value[a]).toBe('done')
    await e.selectSession(a)
    expect(e.sessionNotices.value[a]).toBeUndefined()
    expect(e.turns.value[0].status).toBe('done')
    expect(e.turns.value[0].images[0].status).toBe('done')
  })

  it('a deleted session does not come back after its request aborts', async () => {
    const e = await import('../useChatDrawEngine')
    await e.initChatDraw()
    vi.stubGlobal('fetch', okFetch([new Promise(() => undefined)]))
    e.draftText.value = 'doomed'
    const sending = e.sendDraft()
    await settle()
    const a = e.activeSessionId.value
    await e.deleteSession(a)
    await sending
    await settle()
    expect(e.sessions.value.some(s => s.id === a)).toBe(false)
    expect(e.activeSessionId.value).not.toBe(a)
    expect(e.turns.value).toHaveLength(0)
    await reload(e)
    expect(e.sessions.value.some(s => s.id === a)).toBe(false)
    expect(e.turns.value).toHaveLength(0)
  })

  it('a polish result for a deleted session is discarded and the draft not carried over', async () => {
    const e = await import('../useChatDrawEngine')
    await e.initChatDraw()
    vi.stubGlobal('fetch', okFetch())
    e.polishEnabled.value = true
    const polish = deferred<string>()
    h.enhance.mockImplementationOnce(() => polish.promise)
    e.draftText.value = 'gone'
    const sending = e.sendDraft()
    const a = e.activeSessionId.value
    await e.deleteSession(a)
    polish.resolve('gone polished')
    await sending
    await settle()
    expect(e.sessions.value.some(s => s.id === a)).toBe(false)
    expect(e.turns.value).toHaveLength(0)
    // 删除会话后已切走:快照草稿不得归还,避免串入新会话(codex 审查修复)
    expect(e.draftText.value).toBe('')
  })

  it('polishDraft: 等待期间输入被修改则不覆盖,清空后不回填', async () => {
    const e = await import('../useChatDrawEngine')
    await e.initChatDraw()
    e.draftText.value = 'keep me'
    const polish = deferred<string>()
    h.enhance.mockImplementationOnce(() => polish.promise)
    const p = e.polishDraft()
    e.draftText.value = 'edited meanwhile'
    polish.resolve('polished text')
    await p
    expect(e.draftText.value).toBe('edited meanwhile')
    // 清空后再润色：正常写回
    const polish2 = deferred<string>()
    h.enhance.mockImplementationOnce(() => polish2.promise)
    e.draftText.value = 'again'
    const p2 = e.polishDraft()
    polish2.resolve('again polished')
    await p2
    expect(e.draftText.value).toBe('again polished')
  })

  it('deleting a turn updates session counts and survives reload', async () => {
    const e = await import('../useChatDrawEngine')
    await e.initChatDraw()
    vi.stubGlobal('fetch', okFetch())
    await send(e, 'one')
    await send(e, 'two')
    await e.deleteTurn(e.turns.value[0].id)
    expect(e.activeSession.value?.turnCount).toBe(1)
    expect(e.activeSession.value?.imageCount).toBe(1)
    await settle()
    await reload(e)
    expect(e.turns.value.map(x => x.prompt)).toEqual(['two'])
    expect(e.totalImages.value).toBe(1)
  })

  it('clearAllSessions wipes everything including in-flight turns', async () => {
    const e = await import('../useChatDrawEngine')
    await e.initChatDraw()
    vi.stubGlobal('fetch', okFetch())
    await send(e, 'one')
    e.newSession()
    vi.stubGlobal('fetch', okFetch([new Promise(() => undefined)]))
    e.draftText.value = 'pending'
    const sending = e.sendDraft()
    await settle()
    await e.clearAllSessions()
    await sending
    await settle()
    expect(e.sessions.value).toHaveLength(1)
    expect(e.totalImages.value).toBe(0)
    await reload(e)
    expect(e.sessions.value).toHaveLength(1)
    expect(e.turns.value).toHaveLength(0)
  })

  it('keeps ref urls shared with a redraw turn and revokes them once the session is evicted', async () => {
    const e = await import('../useChatDrawEngine')
    await e.initChatDraw()
    vi.stubGlobal('fetch', okFetch())
    await e.addDraftRefs([png('ref.png')])
    await send(e, 'one')
    const a = e.activeSessionId.value
    const refUrl = e.blobUrl(e.turns.value[0].refs[0])
    const imgUrl = e.blobUrl(e.turns.value[0].images[0].blob)
    await e.redrawTurn(e.turns.value[0])
    await settle()
    const revoke = (URL as unknown as { revokeObjectURL: ReturnType<typeof vi.fn> }).revokeObjectURL
    revoke.mockClear()
    // 删原轮:独占的结果图释放,与重绘轮共用的参考图保留
    await e.deleteTurn(e.turns.value[0].id)
    expect(revoke).toHaveBeenCalledWith(imgUrl)
    expect(revoke).not.toHaveBeenCalledWith(refUrl)
    expect(e.blobUrl(e.turns.value[0].refs[0])).toBe(refUrl)
    const redrawUrl = e.blobUrl(e.turns.value[0].images[0].blob)
    revoke.mockClear()
    // 新建会话后 a 空闲,被逐出内存,其地址全部释放
    e.newSession()
    expect(revoke).toHaveBeenCalledWith(refUrl)
    expect(revoke).toHaveBeenCalledWith(redrawUrl)
    // 切回 a:从库重新读出
    await e.selectSession(a)
    expect(e.turns.value.map(x => x.origin)).toEqual(['redraw'])
  })

  it('migrates v2 single-conversation turns into a legacy session', async () => {
    await new Promise<void>((resolve, reject) => {
      const req = indexedDB.open('chat-draw-db-user-1', 2)
      req.onupgradeneeded = () => {
        const st = req.result.createObjectStore('turns', { keyPath: 'id' })
        st.createIndex('ts', 'ts')
        const base = { refs: [], model: 'gpt-image-2', ratio: '1:1', size: '1024x1024', quality: 'auto', count: 1 }
        st.put({ ...base, id: 't1', ts: 1, prompt: 'old first', status: 'done', images: [{ id: 'i1', status: 'done', blob: new Blob(['x']) }] })
        st.put({ ...base, id: 't2', ts: 2, prompt: 'old second', status: 'running', images: [{ id: 'i2', status: 'pending' }] })
      }
      req.onsuccess = () => { req.result.close(); resolve() }
      req.onerror = () => reject(req.error)
    })
    const e = await import('../useChatDrawEngine')
    await e.initChatDraw()
    await settle()
    expect(e.sessions.value.map(s => s.id)).toEqual([e.LEGACY_SESSION_ID])
    expect(e.activeSession.value?.title).toBe('old first')
    expect(e.activeSession.value?.imageCount).toBe(1)
    expect(e.turns.value.map(x => x.prompt)).toEqual(['old first', 'old second'])
    expect(e.turns.value[1].status).toBe('cancelled')
    expect(e.turns.value[1].images[0].status).toBe('failed')
  })
})
