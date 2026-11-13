import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

const { sync, update, showError, showSuccess } = vi.hoisted(() => ({
  sync: vi.fn(), update: vi.fn(), showError: vi.fn(), showSuccess: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess, showInfo: vi.fn(), showWarning: vi.fn() })
}))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isSimpleMode: true }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))
vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: { syncUpstreamModels: sync, update },
    settings: {
      getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false, providers: [] }),
      getSettings: vi.fn().mockResolvedValue({})
    },
    tlsFingerprintProfiles: { list: vi.fn().mockResolvedValue([]) }
  }
}))
vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn(),
  accountsAPI: { syncUpstreamModels: sync }
}))

import EditAccountModal from '../EditAccountModal.vue'
import ModelWhitelistSelector from '../ModelWhitelistSelector.vue'

const BaseDialog = defineComponent({
  props: ['show'],
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

function account(id = 7) {
  return {
    id, name: 'Kling', notes: '', platform: 'kling', type: 'apikey',
    credentials: { base_url: 'http://127.0.0.1:8020', model_mapping: { 'kling-v1': 'kling-v1', alias: 'kling-v2' } },
    credentials_status: { has_api_key: true }, extra: {}, proxy_id: null, concurrency: 1,
    priority: 1, rate_multiplier: 1, status: 'active', group_ids: [], expires_at: null,
    auto_pause_on_expired: false
  } as any
}

const wrappers: Array<{ unmount(): void }> = []
function mountModal() {
  const wrapper = mount(EditAccountModal, {
    props: { show: true, account: account(), proxies: [], groups: [] },
    global: { stubs: { BaseDialog, Select: true, Icon: true, ModelIcon: true, ProxySelector: true, GroupSelector: true } }
  })
  wrappers.push(wrapper)
  return wrapper
}

beforeEach(() => {
  vi.clearAllMocks()
  update.mockImplementation(async (_id, payload) => ({ ...account(), ...payload }))
})
afterEach(() => wrappers.splice(0).forEach(wrapper => wrapper.unmount()))

describe('Kling upstream models in account editing', () => {
  it('replaces old models and mappings, replaces static choices, and saves model_mapping', async () => {
    sync.mockResolvedValue({ models: ['kling-v4', ' kling-v4-flash ', 'kling-v4'] })
    const wrapper = mountModal()
    await wrapper.get('[data-testid="kling-sync-upstream-models"]').trigger('click')
    await flushPromises()
    expect(sync).toHaveBeenCalledWith(7)
    const selector = wrapper.getComponent(ModelWhitelistSelector)
    expect(selector.props('modelValue')).toEqual(['kling-v4', 'kling-v4-flash'])
    expect(selector.props('modelMappings')).toEqual([])
    expect(selector.props('availableModels')).toEqual(['kling-v4', 'kling-v4-flash'])
    await selector.get('.cursor-pointer').trigger('click')
    expect(selector.findAll('[data-testid="model-option"]').map(row => row.text())).toEqual(['kling-v4', 'kling-v4-flash'])
    const fillRelated = selector.findAll('button').find(button => button.text() === 'admin.accounts.fillRelatedModels')!
    await fillRelated.trigger('click')
    expect(selector.props('modelValue')).toEqual(['kling-v4', 'kling-v4-flash'])
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(update).toHaveBeenCalledWith(7, expect.objectContaining({
      credentials: expect.objectContaining({ model_mapping: { 'kling-v4': 'kling-v4', 'kling-v4-flash': 'kling-v4-flash' } })
    }))
    expect(showError).not.toHaveBeenCalled()
  })

  it.each(['failure', 'empty'])('preserves configured models on %s', async (result) => {
    if (result === 'failure') sync.mockRejectedValue(new Error('HTTP 404'))
    else sync.mockResolvedValue({ models: [] })
    const wrapper = mountModal()
    const previous = wrapper.getComponent(ModelWhitelistSelector).props()
    await wrapper.get('[data-testid="kling-sync-upstream-models"]').trigger('click')
    await flushPromises()
    const selector = wrapper.getComponent(ModelWhitelistSelector)
    expect(selector.props('modelValue')).toEqual(previous.modelValue)
    expect(selector.props('modelMappings')).toEqual(previous.modelMappings)
    expect(selector.props('availableModels')).toEqual(previous.availableModels)
    expect(showError).toHaveBeenCalledOnce()
    expect(update).not.toHaveBeenCalled()
  })

  it('disables duplicate sync and ignores results after switching accounts', async () => {
    let resolve!: (result: { models: string[] }) => void
    sync.mockReturnValue(new Promise(done => { resolve = done }))
    const wrapper = mountModal()
    await wrapper.get('[data-testid="kling-sync-upstream-models"]').trigger('click')
    expect(wrapper.get('[data-testid="kling-sync-upstream-models"]').attributes('disabled')).toBeDefined()
    await wrapper.setProps({ account: account(8) })
    const selector = wrapper.getComponent(ModelWhitelistSelector)
    const previous = selector.props('modelValue')
    resolve({ models: ['kling-v4'] })
    await flushPromises()
    expect(selector.props('modelValue')).toEqual(previous)
    expect(showSuccess).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="kling-sync-upstream-models"]').attributes('disabled')).toBeUndefined()
  })

  it('uses saved models when reopening the editor', async () => {
    const wrapper = mountModal()
    await wrapper.setProps({ show: false })
    const saved = account()
    saved.credentials.model_mapping = { 'kling-v4-flash': 'kling-v4-flash' }
    await wrapper.setProps({ show: true, account: saved })
    expect(wrapper.getComponent(ModelWhitelistSelector).props('availableModels')).toEqual(['kling-v4-flash'])
  })
})
