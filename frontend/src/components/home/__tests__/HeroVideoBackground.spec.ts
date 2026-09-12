import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import HeroVideoBackground from '../HeroVideoBackground.vue'
import { HOME_HERO_FALLBACK_IMAGE } from '@/config/homeThemes'

const themes = [
  { key: 'a', labelKey: 'a', src: '/landing/a.mp4', gradient: 'from-x to-y' },
  { key: 'b', labelKey: 'b', src: '/landing/b.mp4', gradient: 'from-x to-y' },
  { key: 'c', labelKey: 'c', src: '/landing/c.mp4', gradient: 'from-x to-y' }
]

/** jsdom 未实现媒体播放；在原型上替换属性与方法，所有 <video> 同时生效 */
function stubMediaElement(overrides: { currentTime?: number; duration?: number } = {}) {
  const proto = HTMLMediaElement.prototype
  vi.spyOn(proto, 'play').mockImplementation(function (this: HTMLVideoElement) {
    // 模拟真实浏览器：play() resolve 后异步触发 playing 事件
    // eslint-disable-next-line @typescript-eslint/no-this-alias -- 异步回调里需要 this 引用派发事件
    const el = this
    Promise.resolve().then(() => {
      el.dispatchEvent(new Event('playing'))
    })
    return Promise.resolve()
  })
  vi.spyOn(proto, 'pause').mockImplementation(() => {})
  vi.spyOn(proto, 'load').mockImplementation(() => {})
  vi.spyOn(proto, 'readyState', 'get').mockReturnValue(4)
  vi.spyOn(proto, 'duration', 'get').mockReturnValue(overrides.duration ?? 30)
  vi.spyOn(proto, 'currentTime', 'get').mockReturnValue(overrides.currentTime ?? 0)
}

function mountHero() {
  return mount(HeroVideoBackground, {
    props: { themes },
    attachTo: document.body
  })
}

describe('HeroVideoBackground', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.spyOn(window, 'matchMedia').mockReturnValue({ matches: false } as MediaQueryList)
    stubMediaElement()
  })

  it('renders one video per theme with only the active one visible', () => {
    const wrapper = mountHero()
    const videos = wrapper.findAll('video')
    expect(videos).toHaveLength(themes.length)
    expect(videos[0].classes()).toContain('opacity-100')
    expect(videos[1].classes()).toContain('opacity-0')
  })

  it('preloads only the active theme initially', async () => {
    const wrapper = mountHero()
    // onMounted 中提升 preload，需等待下一次渲染
    await flushPromises()
    const preloads = wrapper.findAll('video').map((v) => v.attributes('preload'))
    expect(preloads[0]).toBe('auto')
    expect(preloads[1]).toBe('none')
    expect(preloads[2]).toBe('none')
  })

  it('upgrades the next theme preload when the active video nears its end', async () => {
    stubMediaElement({ currentTime: 20, duration: 30 })
    const wrapper = mountHero()
    await wrapper.findAll('video')[0].trigger('timeupdate')
    // 剩余 10s < 默认 prewarm 12s，应预加载下一主题
    expect(wrapper.findAll('video')[1].attributes('preload')).toBe('auto')
  })

  it('keeps preload none while far from the end', async () => {
    stubMediaElement({ currentTime: 0, duration: 30 })
    const wrapper = mountHero()
    await wrapper.findAll('video')[0].trigger('timeupdate')
    expect(wrapper.findAll('video')[1].attributes('preload')).toBe('none')
  })

  it('advances to the next theme when the active video ends', async () => {
    const wrapper = mountHero()
    await wrapper.findAll('video')[0].trigger('ended')
    await flushPromises()
    expect(wrapper.emitted('update:activeIndex')?.at(-1)).toEqual([1])
    expect(wrapper.findAll('video')[1].classes()).toContain('opacity-100')
  })

  it('selects a theme through the exposed method', async () => {
    const wrapper = mountHero()
    await wrapper.vm.selectTheme(2)
    await flushPromises()
    expect(wrapper.emitted('update:activeIndex')?.at(-1)).toEqual([2])
    expect(wrapper.vm.isThemeFailed(0)).toBe(false)
  })

  it('keeps the current theme visible when the next theme fails to play', async () => {
    // 拦截主题 b 的 play:不触发 playing(模拟播放失败/超时)
    const proto = HTMLMediaElement.prototype
    const origPlay = proto.play
    vi.spyOn(proto, 'play').mockImplementation(function (this: HTMLVideoElement) {
      const src = this.querySelector('source')?.getAttribute('src')
      if (src === themes[1].src) {
        return Promise.reject(new DOMException('abort', 'AbortError'))
      }
      return origPlay.call(this)
    })
    const wrapper = mountHero()
    await wrapper.vm.selectTheme(1)
    await flushPromises()
    // 主题 b 播放失败:activeIndex 不变,主题 b 被标记失败
    expect(wrapper.emitted('update:activeIndex')).toBeUndefined()
    expect(wrapper.vm.isThemeFailed(1)).toBe(true)
    // 原主题 a 仍然可见
    expect(wrapper.findAll('video')[0].classes()).toContain('opacity-100')
  })

  it('falls back to the static image when every theme fails', async () => {
    const wrapper = mountHero()
    for (const video of wrapper.findAll('video')) {
      await video.trigger('error')
    }
    await flushPromises()
    expect(wrapper.find('img').attributes('src')).toBe(HOME_HERO_FALLBACK_IMAGE)
    expect(wrapper.findAll('video')).toHaveLength(0)
  })

  it('skips a failed theme when advancing', async () => {
    const wrapper = mountHero()
    await wrapper.findAll('video')[1].trigger('error')
    await wrapper.findAll('video')[0].trigger('ended')
    await flushPromises()
    expect(wrapper.emitted('update:activeIndex')?.at(-1)).toEqual([2])
  })
})
