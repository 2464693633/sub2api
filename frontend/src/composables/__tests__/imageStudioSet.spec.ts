import { IDBFactory } from 'fake-indexeddb'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/i18n', () => ({ i18n: { global: {
  t: (key: string, values?: Record<string, string | number>) => key === 'imageStudio.detailRequest'
    ? `Overview: ${values?.overview}\nDetail: ${values?.detail}`
    : key
} } }))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() })
}))

type ImageEngine = typeof import('../useImageStudioEngine')

function response(text = 'generated image', ok = true): Response {
  return {
    ok,
    json: async () => ok
      ? { data: [{ b64_json: btoa(text) }] }
      : { error: { message: text } }
  } as Response
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(done => { resolve = done })
  return { promise, resolve }
}

function fileText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result))
    reader.onerror = () => reject(reader.error)
    reader.readAsText(file)
  })
}

function configureSet(images: ImageEngine) {
  images.models.value = [{ id: 'gpt-image-2', group_id: 20, group_name: 'Images', platform: 'openai' }]
  images.model.value = 'gpt-image-2'
  images.prompt.value = 'A red leather handbag on a white background'
  images.imageSetEnabled.value = true
  images.detailPrompts.value = ['Front stitching', 'Metal clasp', 'Leather texture', 'Interior lining']
}

describe('image studio dependent image sets', () => {
  beforeEach(() => {
    vi.resetModules()
    localStorage.clear()
    localStorage.setItem('auth_user', JSON.stringify({ id: 1 }))
    localStorage.setItem('auth_token', 'session-a')
    vi.stubGlobal('indexedDB', new IDBFactory())
    vi.stubGlobal('createImageBitmap', vi.fn(async () => ({ width: 1024, height: 1024, close: vi.fn() })))
    let nextUrl = 0
    vi.stubGlobal('URL', class extends URL {
      static createObjectURL = vi.fn(() => `blob:image-${++nextUrl}`)
      static revokeObjectURL = vi.fn()
    })
  })

  afterEach(() => {
    window.dispatchEvent(new Event('studio-session-ended'))
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('starts with four editable details and preserves them when the set is enabled again', async () => {
    const images = await import('../useImageStudioEngine')
    expect(images.imageSetEnabled.value).toBe(false)

    images.initializeImageSet()
    expect(images.imageSetEnabled.value).toBe(true)
    expect(images.detailPrompts.value).toHaveLength(4)
    expect(images.detailPrompts.value.every(detail => detail.trim())).toBe(true)
    expect(images.plannedImageCount.value).toBe(5)

    images.detailPrompts.value[0] = 'Keep this custom close-up'
    images.imageSetEnabled.value = false
    images.initializeImageSet()
    expect(images.detailPrompts.value[0]).toBe('Keep this custom close-up')
    images.detailPrompts.value = [' ', 'One detail', '']
    expect(images.plannedImageCount.value).toBe(2)
    images.detailPrompts.value = Array.from({ length: 7 }, (_, i) => `Detail ${i + 1}`)
    expect(images.plannedImageCount.value).toBe(8)
  })

  it('creates the overall image first, then four distinct edits referencing that same image', async () => {
    const first = deferred<Response>()
    let submissions = 0
    const fetcher = vi.fn((_url: string, _options?: RequestInit) => {
      submissions++
      return submissions === 1 ? first.promise : Promise.resolve(response(`detail output ${submissions}`))
    })
    vi.stubGlobal('fetch', fetcher)
    const images = await import('../useImageStudioEngine')
    configureSet(images)
    images.quantity.value = 50
    const details = [...images.detailPrompts.value]
    const originalPrompt = images.prompt.value

    const running = images.generateBatch()
    await vi.waitFor(() => expect(fetcher).toHaveBeenCalledTimes(1))
    expect(images.batch.value).toHaveLength(5)
    expect(images.batch.value.map(slot => slot.status)).toEqual(['running', 'queued', 'queued', 'queued', 'queued'])
    first.resolve(response('original overall image'))
    await running

    expect(fetcher).toHaveBeenCalledTimes(5)
    expect(fetcher.mock.calls[0][0]).toContain('/image-studio/generations')
    expect(JSON.parse(String(fetcher.mock.calls[0][1]?.body))).toMatchObject({ prompt: originalPrompt, n: 1 })
    for (let i = 1; i < fetcher.mock.calls.length; i++) {
      const [url, options] = fetcher.mock.calls[i]
      expect(url).toContain('/image-studio/edits')
      expect(options?.headers).toMatchObject({ 'X-Image-Studio-Group': '20' })
      const form = options?.body as FormData
      expect(form).toBeInstanceOf(FormData)
      expect(String(form.get('prompt'))).toContain(originalPrompt)
      expect(String(form.get('prompt'))).toContain(details[i - 1])
      expect(form.getAll('image')).toHaveLength(1)
      expect(await fileText(form.get('image') as File)).toBe('original overall image')
    }
    expect(images.batch.value.every(slot => slot.status === 'done')).toBe(true)
    expect(images.history.value).toHaveLength(5)
    expect(new Set(images.history.value.map(item => item.id)).size).toBe(5)
    expect(images.history.value.every(item => item.images.length === 1)).toBe(true)
    for (const detail of details) {
      expect(images.history.value.some(item => item.mode === 'i2i' && item.prompt.includes(detail))).toBe(true)
    }
    await images.loadHistory()
    expect(images.history.value).toHaveLength(5)
  })

  it('blocks details after an overall failure and resumes them after retrying the original upload and parameters', async () => {
    let submissions = 0
    const fetcher = vi.fn(async (_url: string, _options?: RequestInit) => ++submissions === 1
      ? response('overall failed', false)
      : response('successful overall retry'))
    vi.stubGlobal('fetch', fetcher)
    const images = await import('../useImageStudioEngine')
    configureSet(images)
    const originalPrompt = images.prompt.value
    images.quality.value = 'high'
    images.outputFormat.value = 'webp'
    images.size.value = '1536x1024'
    images.addRefFiles([new File(['original source photo'], 'source.png', { type: 'image/png' })])

    await images.generateBatch()
    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(images.batch.value.map(slot => slot.status)).toEqual(['failed', 'blocked', 'blocked', 'blocked', 'blocked'])
    expect(images.history.value).toEqual([])

    images.prompt.value = 'Changed prompt'
    images.detailPrompts.value = ['Changed detail']
    images.model.value = 'grok-imagine'
    images.quality.value = 'low'
    images.outputFormat.value = 'jpeg'
    images.size.value = '512x512'
    images.mode.value = 't2i'
    images.removeRefItem(0)
    await images.retrySlot(images.batch.value[0].slotId)

    expect(fetcher).toHaveBeenCalledTimes(6)
    const [retryUrl, retryOptions] = fetcher.mock.calls[1]
    expect(retryUrl).toContain('/image-studio/edits')
    expect(retryOptions?.headers).toMatchObject({ 'X-Image-Studio-Group': '20' })
    const retryForm = retryOptions?.body as FormData
    expect(retryForm.get('model')).toBe('gpt-image-2')
    expect(retryForm.get('prompt')).toBe(originalPrompt)
    expect(retryForm.get('quality')).toBe('high')
    expect(retryForm.get('output_format')).toBe('webp')
    expect(retryForm.get('size')).toBe('1536x1024')
    expect(await fileText(retryForm.get('image') as File)).toBe('original source photo')
    for (const [, options] of fetcher.mock.calls.slice(2)) {
      const form = options?.body as FormData
      expect(form.get('model')).toBe('gpt-image-2')
      expect(form.get('quality')).toBe('high')
      expect(form.get('output_format')).toBe('webp')
      expect(form.get('size')).toBe('1536x1024')
      expect(String(form.get('prompt'))).toContain(originalPrompt)
      expect(String(form.get('prompt'))).not.toContain('Changed detail')
      expect(form.getAll('image')).toHaveLength(1)
      expect(await fileText(form.get('image') as File)).toBe('successful overall retry')
    }
    expect(images.batch.value.every(slot => slot.status === 'done')).toBe(true)
    await images.loadHistory()
    expect(images.history.value).toHaveLength(5)
  })

  it('retries only a failed detail with its original prompt, group, and overall image', async () => {
    let submissions = 0
    const fetcher = vi.fn(async (_url: string, _options?: RequestInit) => {
      submissions++
      if (submissions === 1) return response('overall reference')
      if (submissions === 3) return response('detail failed', false)
      return response(`successful detail ${submissions}`)
    })
    vi.stubGlobal('fetch', fetcher)
    const images = await import('../useImageStudioEngine')
    configureSet(images)
    await images.generateBatch()
    expect(fetcher).toHaveBeenCalledTimes(5)
    const failed = images.batch.value.find(slot => slot.status === 'failed')!
    expect(failed).toBeDefined()
    expect(images.history.value).toHaveLength(4)
    const failedPrompt = failed.prompt

    images.prompt.value = 'Unrelated new draft'
    images.detailPrompts.value = ['Unrelated detail']
    images.model.value = 'other-image-model'
    images.addRefFiles([new File(['unrelated source'], 'other.png', { type: 'image/png' })])
    await images.retrySlot(failed.slotId)

    expect(fetcher).toHaveBeenCalledTimes(6)
    const [url, options] = fetcher.mock.calls[5]
    expect(url).toContain('/image-studio/edits')
    expect(options?.headers).toMatchObject({ 'X-Image-Studio-Group': '20' })
    const form = options?.body as FormData
    expect(form.get('model')).toBe('gpt-image-2')
    expect(form.get('prompt')).toBe(failedPrompt)
    expect(form.getAll('image')).toHaveLength(1)
    expect(await fileText(form.get('image') as File)).toBe('overall reference')
    expect(images.batch.value.every(slot => slot.status === 'done')).toBe(true)
    await images.loadHistory()
    expect(images.history.value).toHaveLength(5)
    expect(images.history.value.filter(item => item.prompt === failedPrompt)).toHaveLength(1)
  })

  it('finishes the active image on cancellation and never starts the remaining details', async () => {
    const first = deferred<Response>()
    const fetcher = vi.fn((_url: string, _options?: RequestInit) => first.promise)
    vi.stubGlobal('fetch', fetcher)
    const images = await import('../useImageStudioEngine')
    configureSet(images)

    const running = images.generateBatch()
    await vi.waitFor(() => expect(fetcher).toHaveBeenCalledTimes(1))
    images.cancelBatch()
    first.resolve(response('finished overall image'))
    await running

    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(images.batch.value.map(slot => slot.status)).toEqual(['done', 'cancelled', 'cancelled', 'cancelled', 'cancelled'])
    expect(images.generating.value).toBe(false)
    await images.loadHistory()
    expect(images.history.value).toHaveLength(1)
  })

  it('discards the overall result and stops dependencies when the account changes before the response', async () => {
    const first = deferred<Response>()
    const fetcher = vi.fn((_url: string, _options?: RequestInit) => first.promise)
    vi.stubGlobal('fetch', fetcher)
    const images = await import('../useImageStudioEngine')
    configureSet(images)

    const running = images.generateBatch()
    await vi.waitFor(() => expect(fetcher).toHaveBeenCalledTimes(1))
    localStorage.setItem('auth_user', JSON.stringify({ id: 2 }))
    localStorage.setItem('auth_token', 'session-b')
    // Storage events may arrive later; the response must still be isolated.
    first.resolve(response('private image for user one'))
    await running

    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(URL.createObjectURL).not.toHaveBeenCalled()
    expect(images.history.value).toEqual([])
    await images.loadHistory()
    expect(images.history.value).toEqual([])
    window.dispatchEvent(new Event('studio-session-ended'))
    expect(images.imageSetEnabled.value).toBe(false)
    expect(images.detailPrompts.value).toEqual([])
  })

  it('retains repeated single-prompt generation when image sets are disabled', async () => {
    const fetcher = vi.fn(async (_url: string, _options?: RequestInit) => response('ordinary result'))
    vi.stubGlobal('fetch', fetcher)
    const images = await import('../useImageStudioEngine')
    images.prompt.value = 'A single landscape'
    images.quantity.value = 3
    expect(images.imageSetEnabled.value).toBe(false)
    expect(images.plannedImageCount.value).toBe(3)

    await images.generateBatch()

    expect(fetcher).toHaveBeenCalledTimes(3)
    for (const [url, options] of fetcher.mock.calls) {
      expect(url).toContain('/image-studio/generations')
      expect(JSON.parse(String(options?.body))).toMatchObject({ prompt: 'A single landscape', n: 1 })
    }
    expect(images.batch.value.every(slot => slot.status === 'done')).toBe(true)
    await images.loadHistory()
    expect(images.history.value).toHaveLength(3)
  })
})
