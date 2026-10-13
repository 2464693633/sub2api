/** Read the source itself: requested generation seconds may differ from the returned video. */
export function readVideoDuration(url: string, signal: AbortSignal): Promise<number> {
  if (signal.aborted) return Promise.reject(new DOMException('Aborted', 'AbortError'))
  return new Promise((resolve, reject) => {
    const video = document.createElement('video')
    video.preload = 'metadata'
    const finish = (duration?: number, error?: Error) => {
      clearTimeout(timer)
      video.removeEventListener('loadedmetadata', ready)
      video.removeEventListener('error', failed)
      signal.removeEventListener('abort', aborted)
      video.removeAttribute('src')
      video.load()
      if (error) reject(error)
      else resolve(duration!)
    }
    const ready = () => {
      const duration = video.duration
      if (Number.isFinite(duration) && duration > 0) finish(duration)
      else failed()
    }
    const failed = () => finish(undefined, new Error('Video duration is unavailable'))
    const aborted = () => finish(undefined, new DOMException('Aborted', 'AbortError'))
    const timer = setTimeout(failed, 15000)
    video.addEventListener('loadedmetadata', ready)
    video.addEventListener('error', failed)
    signal.addEventListener('abort', aborted, { once: true })
    video.src = url
  })
}
