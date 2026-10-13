import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import RechargeTiersEditor from '../RechargeTiersEditor.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('recharge offer editing', () => {
  it('adds the example and edits the credited amount without mutating the saved config', async () => {
    const wrapper = mount(RechargeTiersEditor, { props: { modelValue: [] } })
    await wrapper.get('button').trigger('click')
    const tiers = [{ amount: 100, credit_amount: 1100, currency: 'CNY' }]
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([tiers])
    await wrapper.setProps({ modelValue: tiers })
    await wrapper.get('#tier-credit-0').setValue('1200')
    expect(wrapper.emitted('update:modelValue')?.[1]).toEqual([[{ ...tiers[0], credit_amount: 1200 }]])
    expect(tiers[0].credit_amount).toBe(1100)
    await wrapper.get('[aria-label="admin.settings.payment.removeRechargeTier"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[2]).toEqual([[]])
  })
})
