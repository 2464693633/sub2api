const REPOSITORY_PATTERN = /^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/

export function normalizeUpdateRepository(repository: string): string {
  const normalized = repository.trim()
  if (!REPOSITORY_PATTERN.test(normalized)) return ''

  const [owner, name] = normalized.split('/')
  if (owner === '.' || owner === '..' || name === '.' || name === '..') return ''

  return normalized
}

export function getGitHubRepositoryUrl(repository: string): string {
  const normalized = normalizeUpdateRepository(repository)
  return normalized ? `https://github.com/${normalized}` : ''
}

export function getGitHubReleaseUrl(repository: string, version: string): string {
  const repositoryUrl = getGitHubRepositoryUrl(repository)
  if (!repositoryUrl) return ''

  const normalizedVersion = version.trim()
  if (!normalizedVersion) return `${repositoryUrl}/releases`

  const tag = normalizedVersion.startsWith('v') ? normalizedVersion : `v${normalizedVersion}`
  return `${repositoryUrl}/releases/tag/${encodeURIComponent(tag)}`
}

export function getInstallScriptUrl(repository: string, version: string): string {
  const normalized = normalizeUpdateRepository(repository)
  const normalizedVersion = version.trim()
  if (!normalized || !normalizedVersion) return ''

  const tag = normalizedVersion.startsWith('v') ? normalizedVersion : `v${normalizedVersion}`
  return `https://raw.githubusercontent.com/${normalized}/${encodeURIComponent(tag)}/deploy/install.sh`
}

export function getGhcrImage(repository: string): string {
  const normalized = normalizeUpdateRepository(repository)
  return normalized ? `ghcr.io/${normalized.toLowerCase()}` : ''
}
