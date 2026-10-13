import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import CreativeTimelinePreview from '../CreativeTimelinePreview.vue'
import type { StudioAsset, StudioTimeline } from '@/api/creativeStudio'
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
let wrapper: VueWrapper | undefined
beforeEach(() => {
  vi.useFakeTimers()
  vi.spyOn(performance, 'now').mockImplementation(() => Date.now())
  vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
  vi.spyOn(HTMLMediaElement.prototype, 'play').mockImplementation(async () => {})
  HTMLDialogElement.prototype.showModal = function () { this.open = true }
  HTMLDialogElement.prototype.close = function () { this.open = false; this.dispatchEvent(new Event('close')) }
})
afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.useRealTimers(); vi.restoreAllMocks() })
const timeline: StudioTimeline = { size: '1280x720', audio_asset_id: 'music', clips: [{ id: 'one', asset_id: 'a', title: 'First', start: 0, duration: 1 }, { id: 'two', asset_id: 'b', title: 'Second', start: 0, duration: 1 }] }
const assets: StudioAsset[] = ['a', 'b', 'music'].map(id => ({ id, kind: id === 'music' ? 'audio' : 'image', name: id, mime: id === 'music' ? 'audio/wav' : 'image/png', size: 10, created_at: 1 }))

describe('timeline preview', () => {
  it('keeps the opened shot order and starts late-loading music after moving to the next shot', async () => {
    let finishMusic!: (url: string) => void
    const delayedMusic = new Promise<string>(resolve => { finishMusic = resolve })
    wrapper = mount(CreativeTimelinePreview, { props: { timeline, assets, resolvePreview: id => id === 'music' ? delayedMusic : Promise.resolve(`blob:${id}`) } })
    await (wrapper.vm as unknown as { open: () => Promise<void> }).open()
    await flushPromises()
    await wrapper.setProps({ timeline: { ...timeline, clips: [...timeline.clips].reverse() } })
    expect(wrapper.find('img').attributes('src')).toBe('blob:a')
    await wrapper.find('.player-controls button').trigger('click')
    await vi.advanceTimersByTimeAsync(1100)
    expect(wrapper.find('img').attributes('src')).toBe('blob:b')
    finishMusic('blob:music'); await flushPromises()
    const audio = wrapper.find('audio').element
    Object.defineProperty(audio, 'duration', { value: 5, configurable: true })
    await wrapper.find('audio').trigger('loadedmetadata')
    expect(audio.currentTime).toBeGreaterThanOrEqual(1)
    expect(audio.volume).toBe(.2)
    expect(vi.mocked(HTMLMediaElement.prototype.play).mock.instances).toContain(audio)
    ;(wrapper.vm as unknown as { close: () => void }).close()
    expect(wrapper.find('dialog').element.open).toBe(false)
    expect(vi.getTimerCount()).toBe(0)
  })

  it('rejects a video trim outside its real duration before preview playback', async () => {
    const videoTimeline: StudioTimeline = { size: '1280x720', clips: [{ id: 'one', asset_id: 'video', title: '', start: 4, duration: 4 }] }
    wrapper = mount(CreativeTimelinePreview, { props: { timeline: videoTimeline, assets: [{ ...assets[0]!, id: 'video', kind: 'video', mime: 'video/mp4' }], resolvePreview: async () => 'blob:video' } })
    await (wrapper.vm as unknown as { open: () => Promise<void> }).open()
    const media = wrapper.find('video')
    Object.defineProperty(media.element, 'duration', { value: 5, configurable: true })
    await media.trigger('loadedmetadata')
    expect(wrapper.find('[role="alert"]').text()).toBe('previewRangeError')
    expect(vi.mocked(HTMLMediaElement.prototype.play)).not.toHaveBeenCalled()
  })
})
