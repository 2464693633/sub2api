import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { readVideoDuration } from '../studioMedia'

describe('video source duration', () => {
  let video: HTMLVideoElement
  let controller: AbortController
  beforeEach(() => {
    vi.useFakeTimers()
    video = document.createElement('video')
    controller = new AbortController()
    vi.spyOn(document, 'createElement').mockReturnValue(video)
    vi.spyOn(video, 'load').mockImplementation(() => {})
  })
  afterEach(() => { vi.restoreAllMocks(); vi.useRealTimers() })

  it('reads fractional metadata duration and releases the media element', async () => {
    const pending = readVideoDuration('blob:video', controller.signal)
    expect(video.preload).toBe('metadata')
    Object.defineProperty(video, 'duration', { value: 2.75 })
    video.dispatchEvent(new Event('loadedmetadata'))
    expect(await pending).toBe(2.75)
    expect(video.hasAttribute('src')).toBe(false)
    expect(video.load).toHaveBeenCalledOnce()
    expect(vi.getTimerCount()).toBe(0)
  })

  it.each([NaN, Infinity, 0])('rejects unusable metadata duration %s', async duration => {
    const pending = readVideoDuration('blob:video', controller.signal)
    Object.defineProperty(video, 'duration', { value: duration })
    video.dispatchEvent(new Event('loadedmetadata'))
    await expect(pending).rejects.toThrow('Video duration is unavailable')
    expect(video.hasAttribute('src')).toBe(false)
  })

  it.each(['error', 'timeout', 'abort'])('cleans up after a metadata %s', async failure => {
    const pending = readVideoDuration('blob:video', controller.signal)
    const rejected = expect(pending).rejects.toThrow(failure === 'abort' ? 'Aborted' : 'Video duration is unavailable')
    if (failure === 'timeout') await vi.advanceTimersByTimeAsync(15000)
    else if (failure === 'abort') controller.abort()
    else video.dispatchEvent(new Event('error'))
    await rejected
    expect(video.hasAttribute('src')).toBe(false)
    expect(vi.getTimerCount()).toBe(0)
    // Events queued after cleanup must not settle or clean up the probe twice.
    video.dispatchEvent(new Event('loadedmetadata'))
    expect(video.load).toHaveBeenCalledOnce()
  })

  it('does not start a probe for an already closed session', async () => {
    controller.abort()
    await expect(readVideoDuration('blob:video', controller.signal)).rejects.toMatchObject({ name: 'AbortError' })
    expect(document.createElement).not.toHaveBeenCalled()
    expect(vi.getTimerCount()).toBe(0)
  })
})
