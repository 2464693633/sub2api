import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import StudioLegacyHistory from '../StudioLegacyHistory.vue'

const storage = vi.hoisted(() => ({ probe: vi.fn(), recover: vi.fn() }))
vi.mock('@/composables/studioHistory', () => ({
  hasLegacyStudioHistory: storage.probe,
  importLegacyStudioHistory: storage.recover
}))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('legacy studio history recovery entry', () => {
  beforeEach(() => {
    localStorage.setItem('auth_user', JSON.stringify({ id: 1 }))
    storage.probe.mockResolvedValue(true)
    storage.recover.mockResolvedValue(2)
  })
  afterEach(() => { vi.restoreAllMocks(); vi.clearAllMocks() })

  it.each(['image', 'video'] as const)('only imports %s history after confirmation', async kind => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const wrapper = mount(StudioLegacyHistory, { props: { kind } })
    await flushPromises()
    expect(storage.recover).not.toHaveBeenCalled()
    await wrapper.get('button').trigger('click')
    expect(confirm).toHaveBeenCalledWith('canvas.legacyHistoryConfirm')
    expect(storage.recover).not.toHaveBeenCalled()

    confirm.mockReturnValue(true)
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(storage.recover).toHaveBeenCalledWith(kind, 'user-1', expect.any(Function))
    expect(wrapper.emitted('imported')).toHaveLength(1)
    expect(wrapper.find('button').exists()).toBe(false)
    wrapper.unmount()
  })

  it('keeps recovery available after an import failure', async () => {
    storage.recover.mockRejectedValueOnce(new Error('Storage full'))
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mount(StudioLegacyHistory, { props: { kind: 'image' } })
    await flushPromises()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('canvas.legacyHistoryFailed')
    expect(wrapper.emitted('imported')).toBeUndefined()
    expect(wrapper.get('button').attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('does not publish import completion after the session ends', async () => {
    let finish!: (count: number) => void
    storage.recover.mockReturnValueOnce(new Promise<number>(resolve => { finish = resolve }))
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mount(StudioLegacyHistory, { props: { kind: 'video' } })
    await flushPromises()
    await wrapper.get('button').trigger('click')
    const isCurrent = storage.recover.mock.calls[0][2] as () => boolean
    expect(isCurrent()).toBe(true)
    window.dispatchEvent(new Event('studio-session-ended'))
    finish(2)
    await flushPromises()
    expect(isCurrent()).toBe(false)
    expect(wrapper.emitted('imported')).toBeUndefined()
    wrapper.unmount()
  })
})
