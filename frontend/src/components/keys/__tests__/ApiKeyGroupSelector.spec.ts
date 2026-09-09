import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import type { Group } from '@/types'
import ApiKeyGroupSelector from '../ApiKeyGroupSelector.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}:${JSON.stringify(params)}` : key
    })
  }
})

const group = (
  id: number,
  platform: Group['platform'],
  subscriptionType: Group['subscription_type'] = 'standard'
) => ({
  id,
  name: `Group ${id}`,
  description: null,
  platform,
  subscription_type: subscriptionType,
  rate_multiplier: 1,
  peak_rate_enabled: false,
  peak_start: '',
  peak_end: '',
  peak_rate_multiplier: 1
}) as Group

const SelectStub = {
  name: 'Select',
  props: ['modelValue', 'options', 'disabled'],
  emits: ['update:modelValue'],
  template: `
    <div data-test="select-stub" :data-disabled="String(disabled)">
      <button
        v-for="option in options"
        :key="option.value"
        type="button"
        :data-test="'add-group-' + option.value"
        @click="$emit('update:modelValue', option.value)"
      >
        {{ option.label }}
      </button>
    </div>
  `
}

const mountSelector = (modelValue: number[], groups: Group[]) =>
  mount(ApiKeyGroupSelector, {
    props: { modelValue, groups },
    global: {
      stubs: {
        Select: SelectStub,
        GroupBadge: { props: ['name'], template: '<span>{{ name }}</span>' },
        GroupOptionItem: true,
        Icon: true
      }
    }
  })

describe('ApiKeyGroupSelector', () => {
  it('offers groups from the same platform regardless of billing type', () => {
    const wrapper = mountSelector(
      [1],
      [group(1, 'anthropic'), group(2, 'anthropic', 'subscription'), group(3, 'openai')]
    )

    expect(wrapper.find('[data-test="add-group-2"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="add-group-3"]').exists()).toBe(false)
  })

  it('emits the ordered list when groups are added, moved, and removed', async () => {
    const groups = [group(1, 'anthropic'), group(2, 'anthropic')]
    const wrapper = mountSelector([1], groups)

    await wrapper.get('[data-test="add-group-2"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([[1, 2]])

    await wrapper.setProps({ modelValue: [1, 2] })
    await wrapper.get('[data-test="move-group-up-2"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([[2, 1]])

    await wrapper.setProps({ modelValue: [2, 1] })
    await wrapper.get('[data-test="remove-group-2"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([[1]])
  })

  it('removes incompatible externally supplied selections', () => {
    const wrapper = mountSelector(
      [1, 3, 2],
      [group(1, 'anthropic'), group(2, 'anthropic'), group(3, 'openai')]
    )

    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual([[1, 2]])
  })
})
