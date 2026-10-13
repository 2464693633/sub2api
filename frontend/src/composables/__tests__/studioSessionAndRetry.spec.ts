import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

function response(body: unknown, ok = true) {
  return { ok, json: async () => body }
}

function switchAccount(id: number) {
  const oldValue = localStorage.getItem('auth_user')
  const newValue = JSON.stringify({ id })
  localStorage.setItem('auth_user', newValue)
  localStorage.setItem('auth_token', `token-user-${id}`)
  window.dispatchEvent(new StorageEvent('storage', { key: 'auth_user', oldValue, newValue }))
}

vi.mock('@/i18n', () => ({ i18n: { global: { t: (key: string) => key } } }))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() })
}))

describe('studio session and task isolation', () => {
  beforeEach(() => {
    vi.resetModules()
    localStorage.clear()
    localStorage.setItem('auth_user', JSON.stringify({ id: 1 }))
    localStorage.setItem('auth_token', 'session-a')
    vi.stubGlobal('URL', class extends URL {
      static createObjectURL = vi.fn().mockReturnValue('blob:test')
      static revokeObjectURL = vi.fn()
    })
  })

  afterEach(() => {
    window.dispatchEvent(new Event('studio-session-ended'))
    vi.useRealTimers()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('reloads AI keys when the authenticated session changes', async () => {
    const fetcher = vi.fn(async (url: string) => ({
      ok: true,
      json: async () => url.includes('/keys?')
        ? { data: { items: [{ id: 1, name: 'key', key: 'test-key', status: 'active', group_id: 1 }] } }
        : { data: [{ id: 'chat-model' }] }
    }))
    vi.stubGlobal('fetch', fetcher)
    const ai = await import('../useAIPrompt')

    await ai.initAIOptions()
    switchAccount(2)
    fetcher.mockClear()
    await ai.initAIOptions()

    expect(fetcher).toHaveBeenCalled()
  })

  it('clears studio keys, drafts, tasks, and visible history when the session ends', async () => {
    const ai = await import('../useAIPrompt')
    const images = await import('../useImageStudioEngine')
    const video = await import('../useVideoStudioEngine')
    ai.aiKeys.value = [{ id: 1, name: 'user A', key: 'secret-a' }]
    images.prompt.value = 'private image draft'
    images.history.value = [{
      id: 1, ts: 1, model: 'image-model', quality: 'auto', format: 'png', size: 'auto',
      prompt: 'private history', mode: 't2i', images: [], starred: false
    }]
    video.prompt.value = 'private video draft'
    video.tasks.value = [{
      taskId: 1, remoteId: '', status: 'failed', prompt: 'private task',
      model: 'video-model', seconds: 6, resolution: '720p', submittedAt: 1
    }]

    window.dispatchEvent(new Event('studio-session-ended'))

    expect(ai.aiKeys.value).toEqual([])
    expect(images.prompt.value).toBe('')
    expect(images.history.value).toEqual([])
    expect(video.prompt.value).toBe('')
    expect(video.tasks.value).toEqual([])
  })

  it('sends the selected video size', async () => {
    const fetcher = vi.fn().mockRejectedValue(new Error('transport failure'))
    vi.stubGlobal('fetch', fetcher)
    const video = await import('../useVideoStudioEngine')
    video.prompt.value = 'test'
    video.videoSize.value = '720x1280'

    video.submitBatch()
    await vi.waitFor(() => expect(video.generating.value).toBe(false))

    const body = JSON.parse(String(fetcher.mock.calls[0][1]?.body))
    expect(body.aspect_ratio).toBe('9:16')
    expect(body.duration).toBe(6)
  })

  it('resumes an existing remote video task without posting a new generation', async () => {
    const fetcher = vi.fn().mockRejectedValue(new Error('transport failure'))
    vi.stubGlobal('fetch', fetcher)
    const video = await import('../useVideoStudioEngine')
    video.tasks.value = [{
      taskId: 1, remoteId: 'existing-job', status: 'failed', prompt: 'test',
      model: 'old-model', seconds: 6, resolution: '480p', submittedAt: 1, groupId: 10
    }]

    video.retryTask(1)
    await vi.waitFor(() => expect(fetcher).toHaveBeenCalled())

    expect(fetcher.mock.calls.some(call => call[1]?.method === 'POST')).toBe(false)
    expect(String(fetcher.mock.calls[0][0])).toContain('/tasks/existing-job')
  })

  it.each(['failed', 'error', 'expired', 'cancelled'])('resubmits a video after upstream status %s', async status => {
    vi.useFakeTimers()
    let submissions = 0
    const fetcher = vi.fn(async (_url: string, options?: RequestInit) =>
      options?.method === 'POST'
        ? response({ id: `job-${++submissions}` })
        : response({ status, error: 'upstream terminal failure' }))
    vi.stubGlobal('fetch', fetcher)
    const video = await import('../useVideoStudioEngine')
    video.models.value = [{ id: 'original-video', group_id: 20, group_name: 'video' }]
    video.model.value = 'original-video'
    video.prompt.value = 'original prompt'
    video.videoSize.value = '720x1280'
    video.mode.value = 'i2v'
    const reference = new File(['image'], 'ref.png', { type: 'image/png' })
    video.refImages.value = [{ file: reference, url: 'blob:reference' }]
    // Keep FileReader deterministic while exercising the actual submit/retry flow.
    vi.stubGlobal('FileReader', class {
      result = 'data:image/png;base64,aW1hZ2U='
      onload?: () => void
      readAsDataURL() { this.onload?.() }
    })

    video.submitBatch()
    await vi.advanceTimersByTimeAsync(5000)
    const task = video.tasks.value[0]
    expect(task.status).toBe('failed')
    expect(submissions).toBe(1)
    video.model.value = 'different-video'
    video.prompt.value = 'changed draft'
    video.refImages.value = []
    video.retryTask(task.taskId)
    await vi.advanceTimersByTimeAsync(5000)

    const posts = fetcher.mock.calls.filter(([, options]) => options?.method === 'POST')
    expect(posts).toHaveLength(2)
    expect(posts[1][1]?.body).toBe(posts[0][1]?.body)
    expect(JSON.parse(String(posts[1][1]?.body))).toMatchObject({
      model: 'original-video', prompt: 'original prompt', aspect_ratio: '9:16', duration: 6,
      image: { url: 'data:image/png;base64,aW1hZ2U=' }
    })
    expect(posts[1][1]?.headers).toMatchObject({ 'X-Video-Studio-Group': '20' })
    expect(fetcher.mock.calls.some(([url]) => url.endsWith('/tasks/job-2'))).toBe(true)
  })

  it('resumes polling after transport failures without creating another video', async () => {
    vi.useFakeTimers()
    const fetcher = vi.fn(async (_url: string, options?: RequestInit) => {
      if (options?.method === 'POST') return response({ id: 'original-job' })
      throw new Error('network unavailable')
    })
    vi.stubGlobal('fetch', fetcher)
    const video = await import('../useVideoStudioEngine')
    video.prompt.value = 'test'
    video.submitBatch()
    await vi.advanceTimersByTimeAsync(15000)
    const task = video.tasks.value[0]
    expect(task.status).toBe('failed')

    video.retryTask(task.taskId)
    await vi.advanceTimersByTimeAsync(10000)
    expect(task.status).toBe('failed')
    expect(fetcher.mock.calls.filter(([, options]) => options?.method === 'POST')).toHaveLength(1)
    expect(fetcher.mock.calls.filter(([url]) => url.endsWith('/tasks/original-job'))).toHaveLength(6)
  })

  it('retries only the download after a completed video cannot be fetched', async () => {
    vi.useFakeTimers()
    const fetcher = vi.fn(async (url: string, options?: RequestInit) => {
      if (options?.method === 'POST') return response({ id: 'completed-job' })
      if (url.endsWith('/content')) throw new Error('download failed')
      return response({ status: 'completed' })
    })
    vi.stubGlobal('fetch', fetcher)
    const video = await import('../useVideoStudioEngine')
    video.prompt.value = 'test'
    video.submitBatch()
    await vi.advanceTimersByTimeAsync(35000)
    const task = video.tasks.value[0]
    expect(task.status).toBe('failed')
    expect(task.upstreamDone).toBe(true)

    video.retryTask(task.taskId)
    await vi.advanceTimersByTimeAsync(30000)
    expect(fetcher.mock.calls.filter(([, options]) => options?.method === 'POST')).toHaveLength(1)
    expect(fetcher.mock.calls.filter(([url]) => url.endsWith('/tasks/completed-job'))).toHaveLength(1)
    expect(fetcher.mock.calls.filter(([url]) => url.endsWith('/content'))).toHaveLength(6)
  })

  it('uses the original video model and group when retrying submission', async () => {
    const fetcher = vi.fn().mockRejectedValue(new Error('transport failure'))
    vi.stubGlobal('fetch', fetcher)
    const video = await import('../useVideoStudioEngine')
    video.model.value = 'new-model'
    video.tasks.value = [{
      taskId: 1, remoteId: '', status: 'failed', prompt: 'test',
      model: 'old-model', seconds: 6, resolution: '480p', submittedAt: 1, groupId: 10
    }]

    video.retryTask(1)
    await vi.waitFor(() => expect(fetcher).toHaveBeenCalled())

    const body = JSON.parse(String(fetcher.mock.calls[0][1]?.body))
    expect(body.model).toBe('old-model')
    expect(fetcher.mock.calls[0][1]?.headers['X-Video-Studio-Group']).toBe('10')
  })

  it('uses refreshed tokens for the same owner during an image batch', async () => {
    let releaseFirst!: (value: unknown) => void
    const first = new Promise(resolve => { releaseFirst = resolve })
    const rejected = { ok: false, json: async () => ({ message: 'rejected' }) }
    const fetcher = vi.fn().mockImplementationOnce(() => first).mockResolvedValue(rejected)
    vi.stubGlobal('fetch', fetcher)
    const images = await import('../useImageStudioEngine')
    images.prompt.value = 'user A batch'
    images.quantity.value = 2
    localStorage.setItem('auth_token', 'session-a')

    const running = images.generateBatch()
    await vi.waitFor(() => expect(fetcher).toHaveBeenCalledTimes(1))
    localStorage.setItem('auth_token', 'session-b')
    releaseFirst(rejected)
    await running

    expect(fetcher).toHaveBeenCalledTimes(2)
    expect(fetcher.mock.calls[0][1]?.headers.Authorization).toBe('Bearer session-a')
    expect(fetcher.mock.calls[1][1]?.headers.Authorization).toBe('Bearer session-b')
  })

  it('stops an image batch after an account switch even before a storage event arrives', async () => {
    const first = deferred<ReturnType<typeof response>>()
    const fetcher = vi.fn().mockReturnValue(first.promise)
    vi.stubGlobal('fetch', fetcher)
    const images = await import('../useImageStudioEngine')
    images.prompt.value = 'private prompt'
    images.quantity.value = 2
    const running = images.generateBatch()

    localStorage.setItem('auth_user', JSON.stringify({ id: 2 }))
    localStorage.setItem('auth_token', 'session-b')
    first.resolve(response({ data: [{ b64_json: 'aGVsbG8=' }] }))
    await running

    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(URL.createObjectURL).not.toHaveBeenCalled()
    expect(images.history.value).toEqual([])
  })

  it('clears studio state on cross-tab account changes but preserves it on token refresh', async () => {
    const images = await import('../useImageStudioEngine')
    const video = await import('../useVideoStudioEngine')
    const ai = await import('../useAIPrompt')
    images.prompt.value = 'private image'
    video.prompt.value = 'private video'
    ai.aiKeys.value = [{ id: 1, name: 'private', key: 'secret' }]

    localStorage.setItem('auth_token', 'refreshed')
    window.dispatchEvent(new StorageEvent('storage', {
      key: 'auth_token', oldValue: 'session-a', newValue: 'refreshed'
    }))
    expect(images.prompt.value).toBe('private image')
    expect(video.prompt.value).toBe('private video')

    switchAccount(2)
    expect(images.prompt.value).toBe('')
    expect(video.prompt.value).toBe('')
    expect(ai.aiKeys.value).toEqual([])
  })

  it('ignores a late image retry response after the session resets', async () => {
    const retried = deferred<ReturnType<typeof response>>()
    const fetcher = vi.fn().mockResolvedValueOnce(response({ message: 'failed' }, false))
      .mockReturnValueOnce(retried.promise)
    vi.stubGlobal('fetch', fetcher)
    const images = await import('../useImageStudioEngine')
    images.prompt.value = 'test'
    await images.generateBatch()
    const running = images.retrySlot(images.batch.value[0].slotId)

    switchAccount(2)
    const newBatch = images.batch.value
    retried.resolve(response({ data: [{ b64_json: 'aGVsbG8=' }] }))
    await running

    expect(images.batch.value).toBe(newBatch)
    expect(URL.createObjectURL).not.toHaveBeenCalled()
  })

  it('ignores a late image decode result and does not continue the old batch', async () => {
    const decoded = deferred<{ width: number; height: number; close: () => void }>()
    const bitmap = vi.fn().mockReturnValue(decoded.promise)
    vi.stubGlobal('createImageBitmap', bitmap)
    const fetcher = vi.fn().mockResolvedValue(response({ data: [{ b64_json: 'aGVsbG8=' }] }))
    vi.stubGlobal('fetch', fetcher)
    const images = await import('../useImageStudioEngine')
    images.prompt.value = 'test'
    images.quantity.value = 2
    const running = images.generateBatch()
    await vi.waitFor(() => expect(bitmap).toHaveBeenCalled())

    switchAccount(2)
    decoded.resolve({ width: 1, height: 1, close: vi.fn() })
    await running

    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(images.batch.value).toEqual([])
    expect(images.history.value).toEqual([])
    expect(URL.createObjectURL).not.toHaveBeenCalled()
  })

  it('uses the refreshed token when image download falls back to the asset proxy', async () => {
    const cdn = deferred<ReturnType<typeof response>>()
    const fetcher = vi.fn()
      .mockResolvedValueOnce(response({ data: [{ url: 'https://cdn.example/image.png' }] }))
      .mockReturnValueOnce(cdn.promise)
      .mockResolvedValueOnce(response({}, false))
    vi.stubGlobal('fetch', fetcher)
    const images = await import('../useImageStudioEngine')
    images.prompt.value = 'test'
    const running = images.generateBatch()
    await vi.waitFor(() => expect(fetcher).toHaveBeenCalledTimes(2))
    localStorage.setItem('auth_token', 'refreshed')
    cdn.reject(new Error('CORS'))
    await running
    expect(fetcher.mock.calls[2][1]?.headers.Authorization).toBe('Bearer refreshed')
  })

  it('uses a refreshed token for the next video poll without resubmitting', async () => {
    vi.useFakeTimers()
    const polled = deferred<ReturnType<typeof response>>()
    const fetcher = vi.fn().mockReturnValueOnce(polled.promise)
      .mockResolvedValueOnce(response({ status: 'failed', error: 'upstream failure' }))
    vi.stubGlobal('fetch', fetcher)
    const video = await import('../useVideoStudioEngine')
    video.tasks.value = [{
      taskId: 1, remoteId: 'existing-job', status: 'failed', prompt: 'test',
      model: 'video-model', seconds: 6, resolution: '720p', submittedAt: 1
    }]
    video.retryTask(1)
    localStorage.setItem('auth_token', 'refreshed')
    polled.resolve(response({ status: 'processing' }))
    await vi.advanceTimersByTimeAsync(5000)

    expect(fetcher).toHaveBeenCalledTimes(2)
    expect(fetcher.mock.calls[0][1]?.headers.Authorization).toBe('Bearer session-a')
    expect(fetcher.mock.calls[1][1]?.headers.Authorization).toBe('Bearer refreshed')
    expect(fetcher.mock.calls.some(call => call[1]?.method === 'POST')).toBe(false)
  })

  it('does not download a video after a late successful poll from an ended session', async () => {
    vi.useFakeTimers()
    const polled = deferred<ReturnType<typeof response>>()
    const fetcher = vi.fn().mockReturnValue(polled.promise)
    vi.stubGlobal('fetch', fetcher)
    const video = await import('../useVideoStudioEngine')
    video.tasks.value = [{
      taskId: 1, remoteId: 'existing-job', status: 'failed', prompt: 'private',
      model: 'video-model', seconds: 6, resolution: '720p', submittedAt: 1
    }]
    video.retryTask(1)
    switchAccount(2)
    const newTasks = video.tasks.value
    polled.resolve(response({ status: 'done' }))
    await vi.advanceTimersByTimeAsync(0)

    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(video.tasks.value).toBe(newTasks)
    expect(video.history.value).toEqual([])
  })

  it('does not retry downloads or update tasks after a late download failure on logout', async () => {
    vi.useFakeTimers()
    const download = deferred<ReturnType<typeof response>>()
    const fetcher = vi.fn().mockReturnValue(download.promise)
    vi.stubGlobal('fetch', fetcher)
    const video = await import('../useVideoStudioEngine')
    video.tasks.value = [{
      taskId: 1, remoteId: 'existing-job', upstreamDone: true, status: 'failed', prompt: 'private',
      model: 'video-model', seconds: 6, resolution: '720p', submittedAt: 1
    }]
    video.retryTask(1)
    window.dispatchEvent(new Event('studio-session-ended'))
    const newTasks = video.tasks.value
    download.reject(new Error('connection closed'))
    await vi.advanceTimersByTimeAsync(30000)

    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(video.tasks.value).toBe(newTasks)
  })

  it('rejects late AI prompt results after logout even when the same user signs in again', async () => {
    const enhanced = deferred<ReturnType<typeof response>>()
    vi.stubGlobal('fetch', vi.fn().mockReturnValue(enhanced.promise))
    const ai = await import('../useAIPrompt')
    const running = ai.aiEnhancePrompt('image', 'private', { keyId: 1, model: 'chat-model' })
    const rejected = expect(running).rejects.toMatchObject({ name: 'StudioSessionChangedError' })
    window.dispatchEvent(new Event('studio-session-ended'))
    enhanced.resolve(response({ choices: [{ message: { content: 'private enhancement' } }] }))
    await rejected
  })

  it('discards late AI key responses after switching users', async () => {
    const keys = deferred<ReturnType<typeof response>>()
    const fetcher = vi.fn().mockReturnValue(keys.promise)
    vi.stubGlobal('fetch', fetcher)
    const ai = await import('../useAIPrompt')
    const running = ai.initAIOptions()
    switchAccount(2)
    keys.resolve(response({ data: { items: [{ id: 1, name: 'private', key: 'secret', status: 'active', group_id: 1 }] } }))
    await running
    expect(ai.aiKeys.value).toEqual([])
    expect(fetcher).toHaveBeenCalledTimes(1)
  })

  it('shows Grok image aliases in the image model picker', async () => {
    const fetcher = vi.fn().mockResolvedValue(response({ data: [
      { id: 'grok-imagine', group_id: 1, group_name: 'grok', platform: 'grok' },
      { id: 'xai/grok-imagine-1', group_id: 1, group_name: 'grok', platform: 'grok' },
      { id: 'grok-imagine-edit', group_id: 1, group_name: 'grok', platform: 'grok' },
      { id: 'imagine', group_id: 1, group_name: 'grok', platform: 'grok' },
      { id: 'grok-imagine-video-1.5', group_id: 1, group_name: 'grok', platform: 'grok' }
    ] }))
    vi.stubGlobal('fetch', fetcher)
    const images = await import('../useImageStudioEngine')
    await images.loadModels()
    expect(images.models.value.map(item => item.id)).toEqual([
      'grok-imagine', 'xai/grok-imagine-1', 'grok-imagine-edit', 'imagine'
    ])
  })

  it('removes Grok image aliases from the video model picker', async () => {
    const fetcher = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ data: [
        { id: 'grok-imagine-1', group_id: 1, group_name: 'grok' },
        { id: 'grok-imagine-edit', group_id: 1, group_name: 'grok' },
        { id: 'imagine', group_id: 1, group_name: 'grok' },
        { id: 'grok-imagine-video-1.5', group_id: 1, group_name: 'grok' }
      ] })
    })
    vi.stubGlobal('fetch', fetcher)
    const video = await import('../useVideoStudioEngine')

    await video.loadModels()

    expect(video.models.value.map(item => item.id)).toEqual(['grok-imagine-video-1.5'])
  })
})
