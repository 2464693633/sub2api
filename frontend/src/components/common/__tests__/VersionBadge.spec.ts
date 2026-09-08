import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

import VersionBadge from '../VersionBadge.vue'
import { useAuthStore } from '@/stores/auth'

const mocks = vi.hoisted(() => ({
  checkUpdates: vi.fn(),
  getRollbackVersions: vi.fn(),
  performUpdate: vi.fn(),
  restartService: vi.fn(),
  rollback: vi.fn()
}))

vi.mock('@/api/admin/system', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/admin/system')>()
  return {
    ...actual,
    checkUpdates: mocks.checkUpdates,
    getRollbackVersions: mocks.getRollbackVersions,
    performUpdate: mocks.performUpdate,
    restartService: mocks.restartService,
    rollback: mocks.rollback
  }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

describe('VersionBadge update repository', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    mocks.checkUpdates.mockReset()
    mocks.getRollbackVersions.mockReset()
    mocks.checkUpdates.mockResolvedValue({
      current_version: '1.2.3',
      latest_version: '1.2.3',
      has_update: false,
      repository: 'Acme/Sub2API',
      cached: false,
      build_type: 'release'
    })
    mocks.getRollbackVersions.mockResolvedValue({
      versions: [
        {
          version: '1.2.2',
          published_at: '2026-09-01T00:00:00Z',
          html_url: 'https://example.invalid/ignored'
        }
      ]
    })
  })

  it('uses the backend repository for release and manual rollback locations', async () => {
    const authStore = useAuthStore()
    authStore.user = { role: 'admin' } as typeof authStore.user
    const wrapper = mount(VersionBadge, {
      global: {
        stubs: {
          Icon: true
        }
      }
    })
    await flushPromises()

    await wrapper.find('button').trigger('click')
    expect(wrapper.find('a').attributes('href')).toBe(
      'https://github.com/Acme/Sub2API/releases/tag/v1.2.3'
    )

    const rollbackToggle = wrapper
      .findAll('button')
      .find((button) => button.text().includes('version.rollback'))
    expect(rollbackToggle).toBeDefined()
    await rollbackToggle!.trigger('click')
    await flushPromises()

    const versionButton = wrapper
      .findAll('button')
      .find((button) => button.text().includes('v1.2.2'))
    expect(versionButton).toBeDefined()
    await versionButton!.trigger('click')

    expect(wrapper.find('code').text()).toBe(
      'curl -sSL https://raw.githubusercontent.com/Acme/Sub2API/v1.2.2/deploy/install.sh | sudo bash -s -- rollback v1.2.2'
    )

    const dockerTab = wrapper
      .findAll('button')
      .find((button) => button.text() === 'version.deployDocker')
    expect(dockerTab).toBeDefined()
    await dockerTab!.trigger('click')
    expect(wrapper.find('code').text()).toContain('image: ghcr.io/acme/sub2api:1.2.2')

    wrapper.unmount()
  })

  it('keeps the exact release URL returned by GitHub when it is available', async () => {
    mocks.checkUpdates.mockResolvedValue({
      current_version: '1.2.3',
      latest_version: '1.2.3',
      has_update: false,
      repository: 'Acme/Sub2API',
      release_info: {
        name: 'Custom release',
        body: '',
        published_at: '2026-09-08T00:00:00Z',
        html_url: 'https://github.com/Acme/Sub2API/releases/tag/custom-release'
      },
      cached: false,
      build_type: 'release'
    })
    const authStore = useAuthStore()
    authStore.user = { role: 'admin' } as typeof authStore.user
    const wrapper = mount(VersionBadge, {
      global: { stubs: { Icon: true } }
    })
    await flushPromises()

    await wrapper.find('button').trigger('click')
    expect(wrapper.find('a').attributes('href')).toBe(
      'https://github.com/Acme/Sub2API/releases/tag/custom-release'
    )

    wrapper.unmount()
  })
})
