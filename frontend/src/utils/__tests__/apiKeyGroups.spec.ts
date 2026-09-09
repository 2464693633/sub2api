import { describe, expect, it } from 'vitest'
import type { Group } from '@/types'
import {
  API_KEY_GROUP_LIMIT,
  getApiKeyGroupIds,
  moveApiKeyGroup,
  normalizeApiKeyGroupIds
} from '@/utils/apiKeyGroups'

const group = (id: number, platform: Group['platform']) => ({ id, platform }) as Group

describe('apiKeyGroups', () => {
  it('prefers ordered group_ids and falls back to legacy group_id', () => {
    expect(getApiKeyGroupIds({ group_id: 9 })).toEqual([9])
    expect(getApiKeyGroupIds({ group_id: 9, group_ids: [3, 2] })).toEqual([3, 2])
    expect(getApiKeyGroupIds({ group_id: null, group_ids: [] })).toEqual([])
  })

  it('keeps only unique groups on the first selected platform', () => {
    const groups = [group(1, 'anthropic'), group(2, 'anthropic'), group(3, 'openai')]

    expect(normalizeApiKeyGroupIds([1, 3, 2, 1], groups)).toEqual([1, 2])
  })

  it('limits the failover chain to ten groups', () => {
    const ids = Array.from({ length: API_KEY_GROUP_LIMIT + 3 }, (_, index) => index + 1)
    const groups = ids.map((id) => group(id, 'gemini'))

    expect(normalizeApiKeyGroupIds(ids, groups)).toEqual(ids.slice(0, API_KEY_GROUP_LIMIT))
  })

  it('moves a group without mutating the original order', () => {
    const ids = [1, 2, 3]

    expect(moveApiKeyGroup(ids, 2, -1)).toEqual([1, 3, 2])
    expect(ids).toEqual([1, 2, 3])
    expect(moveApiKeyGroup(ids, 0, -1)).toBe(ids)
  })
})
