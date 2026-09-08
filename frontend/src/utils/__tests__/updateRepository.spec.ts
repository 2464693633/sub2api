import { describe, expect, it } from 'vitest'
import {
  getGhcrImage,
  getGitHubReleaseUrl,
  getGitHubRepositoryUrl,
  getInstallScriptUrl,
  normalizeUpdateRepository
} from '@/utils/updateRepository'

describe('updateRepository', () => {
  it('derives GitHub and GHCR locations from an owner/repository value', () => {
    const repository = 'Acme/Sub2API'

    expect(getGitHubRepositoryUrl(repository)).toBe('https://github.com/Acme/Sub2API')
    expect(getGitHubReleaseUrl(repository, '1.2.3')).toBe(
      'https://github.com/Acme/Sub2API/releases/tag/v1.2.3'
    )
    expect(getInstallScriptUrl(repository, 'v1.2.3')).toBe(
      'https://raw.githubusercontent.com/Acme/Sub2API/v1.2.3/deploy/install.sh'
    )
    expect(getGhcrImage(repository)).toBe('ghcr.io/acme/sub2api')
  })

  it('rejects values that are not a single GitHub owner/repository path', () => {
    expect(normalizeUpdateRepository('https://github.com/acme/sub2api')).toBe('')
    expect(getGitHubReleaseUrl('acme/sub2api/extra', '1.2.3')).toBe('')
    expect(getInstallScriptUrl('acme/sub2api; whoami', '1.2.3')).toBe('')
    expect(getGhcrImage('../sub2api')).toBe('')
  })
})
