import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ImageSetPlan from '../ImageSetPlan.vue'
import {
  detailPrompts, generating, imageSetEnabled, plannedImageCount, quantity,
  resetImageStudioSession, validImagePlan
} from '@/composables/useImageStudioEngine'

vi.mock('@/i18n', () => ({ i18n: { global: { t: (key: string) => key } } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({
  t: (key: string, values?: { n?: number }) => values?.n === undefined ? key : `${key} ${values.n}`
}) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }) }))

describe('image generation plan controls', () => {
  beforeEach(resetImageStudioSession)
  afterEach(resetImageStudioSession)

  it('switches to a ready-to-edit overview plus four details and preserves drafts', async () => {
    const wrapper = mount(ImageSetPlan)
    await wrapper.get('#image-quantity').setValue(3)
    expect(plannedImageCount.value).toBe(3)
    await wrapper.get('button[aria-pressed="false"]').trigger('click')
    expect(imageSetEnabled.value).toBe(true)
    expect(wrapper.findAll('textarea')).toHaveLength(4)
    expect(wrapper.text()).toContain('imageStudio.totalImages 5')
    await wrapper.get('#image-detail-0').setValue('Focus on the clasp')
    await wrapper.get('button[aria-pressed="false"]').trigger('click')
    expect(quantity.value).toBe(3)
    await wrapper.get('button[aria-pressed="false"]').trigger('click')
    expect((wrapper.get('#image-detail-0').element as HTMLTextAreaElement).value).toBe('Focus on the clasp')
    wrapper.unmount()
  })

  it('skips blank details, validates the plan, and limits the detail count', async () => {
    const wrapper = mount(ImageSetPlan)
    await wrapper.get('button[aria-pressed="false"]').trigger('click')
    for (const textarea of wrapper.findAll('textarea')) await textarea.setValue(' ')
    expect(validImagePlan.value).toBe(false)
    expect(wrapper.get('[role="alert"]').text()).toBe('imageStudio.detailRequired')
    await wrapper.get('#image-detail-1').setValue('Texture close-up')
    expect(validImagePlan.value).toBe(true)
    expect(plannedImageCount.value).toBe(2)
    const addButton = wrapper.findAll('button').find(button => button.text().includes('imageStudio.addDetail'))!
    for (let i = 0; i < 3; i++) await addButton.trigger('click')
    expect(detailPrompts.value).toHaveLength(7)
    expect(addButton.attributes('disabled')).toBeDefined()
    await wrapper.get('button[aria-label="imageStudio.removeDetailN 7"]').trigger('click')
    expect(detailPrompts.value).toHaveLength(6)
    wrapper.unmount()
  })

  it('locks plan controls during generation', async () => {
    const wrapper = mount(ImageSetPlan)
    await wrapper.get('button[aria-pressed="false"]').trigger('click')
    generating.value = true
    await wrapper.vm.$nextTick()
    expect(wrapper.findAll('button, textarea').every(input => input.attributes('disabled') !== undefined)).toBe(true)
    wrapper.unmount()
  })
})
