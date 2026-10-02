/** Shared identity helpers. Tokens may rotate while the studio owner stays the same. */
function storedUserId(value: string | null): number | null {
  try {
    const id = Number((JSON.parse(value || 'null') as { id?: unknown } | null)?.id)
    return Number.isSafeInteger(id) && id > 0 ? id : null
  } catch {
    return null
  }
}

export function sessionOwnerKey(): string {
  const id = storedUserId(localStorage.getItem('auth_user'))
  if (id !== null) return `user-${id}`
  // Legacy sessions without user metadata still need isolation, without putting
  // the actual token into IndexedDB names or localStorage keys.
  let hash = 2166136261
  for (const char of localStorage.getItem('auth_token') || '') {
    hash = Math.imul(hash ^ char.charCodeAt(0), 16777619)
  }
  return `session-${(hash >>> 0).toString(36)}`
}

export class StudioSessionChangedError extends Error {
  constructor() {
    super('Studio session ended')
    this.name = 'StudioSessionChangedError'
  }
}

// The auth store dispatches this event in the current tab. Storage events cover
// logout/account switches in another tab; a same-user token refresh must not stop jobs.
if (typeof window !== 'undefined') {
  window.addEventListener('storage', event => {
    if (event.storageArea && event.storageArea !== localStorage) return
    const ownerChanged = event.key === 'auth_user' &&
      storedUserId(event.oldValue) !== storedUserId(event.newValue)
    const tokenRemoved = event.key === 'auth_token' && !event.newValue
    const legacyTokenChanged = event.key === 'auth_token' &&
      storedUserId(localStorage.getItem('auth_user')) === null && event.oldValue !== event.newValue
    if (event.key === null || ownerChanged || tokenRemoved || legacyTokenChanged) {
      window.dispatchEvent(new Event('studio-session-ended'))
    }
  })
}
