import type { ApiKey, Group } from '@/types'

export const API_KEY_GROUP_LIMIT = 10

export const getApiKeyGroupIds = (apiKey: Pick<ApiKey, 'group_id' | 'group_ids'>): number[] => {
  const ids = apiKey.group_ids?.length
    ? apiKey.group_ids
    : apiKey.group_id != null
      ? [apiKey.group_id]
      : []

  return [...new Set(ids)].slice(0, API_KEY_GROUP_LIMIT)
}

export const normalizeApiKeyGroupIds = (
  ids: number[],
  groups: Pick<Group, 'id' | 'platform'>[]
): number[] => {
  const uniqueIds = [...new Set(ids)].slice(0, API_KEY_GROUP_LIMIT)
  const groupsById = new Map(groups.map((group) => [group.id, group]))
  const firstKnownGroup = uniqueIds.map((id) => groupsById.get(id)).find(Boolean)

  if (!firstKnownGroup) return uniqueIds

  return uniqueIds.filter((id) => {
    const group = groupsById.get(id)
    return !group || group.platform === firstKnownGroup.platform
  })
}

export const moveApiKeyGroup = (ids: number[], index: number, direction: -1 | 1): number[] => {
  const targetIndex = index + direction
  if (index < 0 || index >= ids.length || targetIndex < 0 || targetIndex >= ids.length) {
    return ids
  }

  const reordered = [...ids]
  ;[reordered[index], reordered[targetIndex]] = [reordered[targetIndex], reordered[index]]
  return reordered
}
